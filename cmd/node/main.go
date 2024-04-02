package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog"
	"github.com/ziflex/lecho/v3"

	"github.com/allora-network/b7s/api"
	"github.com/allora-network/b7s/executor"
	"github.com/allora-network/b7s/executor/limits"
	"github.com/allora-network/b7s/fstore"
	"github.com/allora-network/b7s/host"
	"github.com/allora-network/b7s/models/blockless"
	"github.com/allora-network/b7s/models/execute"
	"github.com/allora-network/b7s/node"
	"github.com/allora-network/b7s/peerstore"
	"github.com/allora-network/b7s/store"
)

const (
	success = 0
	failure = 1
)

func main() {
	os.Exit(run())
}

func connectToAlloraBlockchain(cfg AppChainConfig, log zerolog.Logger) (*AppChain, error) {
	appchain, err := NewAppChain(cfg, log)
	if err != nil || appchain == nil {
		log.Warn().Err(err).Msg("error connecting to allora blockchain")
		return nil, err
	} else {
		log.Info().Msg("connected to allora blockchain")
	}
	appchain.Config.SubmitTx = true
	return appchain, nil
}

func NewAlloraExecutor(e blockless.Executor) *AlloraExecutor {
	return &AlloraExecutor{
		Executor: e,
	}
}

func (e *AlloraExecutor) ExecuteFunction(requestID string, req execute.Request) (execute.Result, error) {
	// First call the blockless.Executor's method to get the result
	result, err := e.Executor.ExecuteFunction(requestID, req)
	// Iterate env vars to get the ALLORA_NONCE, if found, sign it and add the signature to the result
	for _, envVar := range req.Config.Environment {
		if envVar.Name == "ALLORA_NONCE" {
			// Get the nonce from the environment variable, convert to bytes
			nonce := envVar.Value
			nonceBytes := []byte(nonce)
			// Get the account from the appchain
			accountName := e.appChain.ReputerAccount.Name

			// Sign using the e.appChain.ReputerAccount.
			sig, _, err := e.appChain.Client.Context().Keyring.Sign(accountName, nonceBytes, signing.SignMode_SIGN_MODE_DIRECT)
			if err != nil {
				fmt.Println("Error signing the nonce: ", err)
				break
			}
			// Marshalling/unmarshalling the result to add the signature to the result
			stdout := make(map[string]interface{})
			err = json.Unmarshal([]byte(result.Result.Stdout), &stdout)
			if err != nil {
				fmt.Println("Error unmarshalling the stdout: ", err)
			} else {
				// Add the signature to the stdout object
				stdout["signature"] = sig
				stdout["nonce"] = nonceBytes
				// Marshal the stdout map back into a JSON string
				stdoutBytes, err := json.Marshal(stdout)
				if err != nil {
					fmt.Println("Error marshalling the stdout: ", err)
				} else {
					// Set the JSON string back to result.Result.Stdout
					result.Result.Stdout = string(stdoutBytes)
				}
			}
			// Process ALLORA_NONCE only once if found.
			break
		}
	}

	return result, err
}

func run() int {

	// Signal catching for clean shutdown.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)

	// Initialize logging.
	log := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).With().Timestamp().Logger().Level(zerolog.DebugLevel)

	// Parse CLI flags and validate that the configuration is valid.
	cfg := parseFlags()

	// Set log level.
	level, err := zerolog.ParseLevel(cfg.Log.Level)
	if err != nil {
		log.Error().Err(err).Str("level", cfg.Log.Level).Msg("could not parse log level")
		return failure
	}
	log = log.Level(level)

	// Determine node role.
	role, err := parseNodeRole(cfg.Role)
	if err != nil {
		log.Error().Err(err).Str("role", cfg.Role).Msg("invalid node role specified")
		return failure
	}

	// Convert workspace path to an absolute one.
	workspace, err := filepath.Abs(cfg.Workspace)
	if err != nil {
		log.Error().Err(err).Str("path", cfg.Workspace).Msg("could not determine absolute path for workspace")
		return failure
	}
	cfg.Workspace = workspace

	// Open the pebble peer database.
	pdb, err := pebble.Open(cfg.PeerDatabasePath, &pebble.Options{Logger: &pebbleNoopLogger{}})
	if err != nil {
		log.Error().Err(err).Str("db", cfg.PeerDatabasePath).Msg("could not open pebble peer database")
		return failure
	}
	defer pdb.Close()

	// Create a new store.
	pstore := store.New(pdb)
	peerstore := peerstore.New(pstore)

	// Get the list of dial back peers.
	peers, err := peerstore.Peers()
	if err != nil {
		log.Error().Err(err).Msg("could not get list of dial-back peers")
		return failure
	}

	// Get the list of boot nodes addresses.
	bootNodeAddrs, err := getBootNodeAddresses(cfg.BootNodes)
	if err != nil {
		log.Error().Err(err).Msg("could not get boot node addresses")
		return failure
	}

	// Create libp2p host.
	host, err := host.New(log, cfg.Host.Address, cfg.Host.Port,
		host.WithPrivateKey(cfg.Host.PrivateKey),
		host.WithBootNodes(bootNodeAddrs),
		host.WithDialBackPeers(peers),
		host.WithDialBackAddress(cfg.Host.DialBackAddress),
		host.WithDialBackPort(cfg.Host.DialBackPort),
		host.WithDialBackWebsocketPort(cfg.Host.DialBackWebsocketPort),
		host.WithWebsocket(cfg.Host.Websocket),
		host.WithWebsocketPort(cfg.Host.WebsocketPort),
	)
	if err != nil {
		log.Error().Err(err).Str("key", cfg.Host.PrivateKey).Msg("could not create host")
		return failure
	}
	defer host.Close()

	log.Info().
		Str("id", host.ID().String()).
		Strs("addresses", host.Addresses()).
		Int("boot_nodes", len(bootNodeAddrs)).
		Int("dial_back_peers", len(peers)).
		Msg("created host")

	// Set node options.
	opts := []node.Option{
		node.WithRole(role),
		node.WithConcurrency(cfg.Concurrency),
		node.WithAttributeLoading(cfg.LoadAttributes),
	}

	// If this is a worker node, initialize an executor.
	var alloraExecutor *AlloraExecutor = nil
	if role == blockless.WorkerNode {

		// Executor options.
		execOptions := []executor.Option{
			executor.WithWorkDir(cfg.Workspace),
			executor.WithRuntimeDir(cfg.RuntimePath),
			executor.WithExecutableName(cfg.RuntimeCLI),
		}

		if needLimiter(cfg) {
			limiter, err := limits.New(limits.WithCPUPercentage(cfg.CPUPercentage), limits.WithMemoryKB(cfg.MemoryMaxKB))
			if err != nil {
				log.Error().Err(err).Msg("could not create resource limiter")
				return failure
			}

			defer func() {
				err = limiter.Shutdown()
				if err != nil {
					log.Error().Err(err).Msg("could not shutdown resource limiter")
				}
			}()

			execOptions = append(execOptions, executor.WithLimiter(limiter))
		}

		// Create an executor.
		executor, err := executor.New(log, execOptions...)
		if err != nil {
			log.Error().
				Err(err).
				Str("workspace", cfg.Workspace).
				Str("runtime_path", cfg.RuntimePath).
				Msg("could not create an executor")
			return failure
		}

		alloraExecutor = NewAlloraExecutor(executor)

		opts = append(opts, node.WithExecutor(alloraExecutor))
		opts = append(opts, node.WithWorkspace(cfg.Workspace))
	}

	// Open the pebble function database.
	fdb, err := pebble.Open(cfg.FunctionDatabasePath, &pebble.Options{Logger: &pebbleNoopLogger{}})
	if err != nil {
		log.Error().Err(err).Str("db", cfg.FunctionDatabasePath).Msg("could not open pebble function database")
		return failure
	}
	defer fdb.Close()

	functionStore := store.New(fdb)

	// Create function store.
	fstore := fstore.New(log, functionStore, cfg.Workspace)

	// If we have topics specified, use those.
	if len(cfg.Topics) > 0 {
		opts = append(opts, node.WithTopics(cfg.Topics))
	}

	response := node.ChanData{}
	// Instantiate node.
	node, err := node.New(log, host, peerstore, fstore, opts...)
	if err != nil {
		log.Error().Err(err).Msg("could not create node")
		return failure
	}
	// Create the main context.
	ctx, rcancel := context.WithCancel(context.Background())
	defer rcancel()

	done := make(chan struct{})
	failed := make(chan struct{})

	var appchain *AppChain = nil
	cfg.AppChainConfig.NodeRole = role
	cfg.AppChainConfig.AddressPrefix = "allo"
	cfg.AppChainConfig.StringSeperator = "|"
	cfg.AppChainConfig.LibP2PKey = host.ID().String()
	cfg.AppChainConfig.MultiAddress = host.Addresses()[0]
	appchain, err = connectToAlloraBlockchain(cfg.AppChainConfig, log)
	if alloraExecutor != nil {
		alloraExecutor.appChain = appchain
	}

	if cfg.AppChainConfig.ReconnectSeconds > 0 {
		go func(executor *AlloraExecutor) {
			ticker := time.NewTicker(time.Second * time.Duration(math.Max(1, math.Min(float64(cfg.AppChainConfig.ReconnectSeconds), 3600))))
			defer ticker.Stop()

			for range ticker.C {
				if appchain == nil || !appchain.Config.SubmitTx {
					log.Debug().Uint64("reconnectSeconds", cfg.AppChainConfig.ReconnectSeconds).Msg("Attempt reconnection to allora blockchain")
					appchain, err = connectToAlloraBlockchain(cfg.AppChainConfig, log)
					if err != nil {
						log.Debug().Msg("Failed to connect to allora blockchain")
					} else {
						log.Debug().Msg("Resetting up chain connection")
						if alloraExecutor != nil {
							executor.appChain = appchain
						} else {
							log.Warn().Msg("No valid alloraExecutor with which to associate chain client")
						}
					}
				}
			}

		}(alloraExecutor) // Pass alloraExecutor as an argument to the goroutine
	}

	// Start node main loop in a separate goroutine.
	go func() {

		log.Info().
			Str("role", role.String()).
			Msg("Allora Node starting")

		err := node.Run(ctx)
		if err != nil {
			log.Error().Err(err).Msg("Allora Node failed")
			close(failed)
		} else {
			close(done)
		}

		log.Info().Msg("Allora Node stopped")
	}()
	go func() {
		select {
		case msg := <-node.CommunicatorAppLayer():
			msgerr := json.Unmarshal(msg, &response)
			if msgerr == nil {
				sendResultsToChain(log, appchain, response)
			} else {
				log.Error().Err(msgerr).Msg("Unable to unmarshall")
			}
		}
	}()

	// If we're a head node - start the REST API.
	if role == blockless.HeadNode {

		if cfg.API == "" {
			log.Error().Err(err).Msg("REST API address is required")
			return failure
		}

		// Create echo server and initialize logging.
		server := echo.New()
		server.HideBanner = true
		server.HidePort = true

		elog := lecho.From(log)
		server.Logger = elog
		server.Use(lecho.Middleware(lecho.Config{Logger: elog}))

		// Create an API handler.
		api := api.New(log, node)

		// Set endpoint handlers.
		server.GET("/api/v1/health", api.Health)
		server.POST("/api/v1/functions/execute", createExecutor(*api, appchain))
		server.POST("/api/v1/functions/install", api.Install)
		server.POST("/api/v1/functions/requests/result", api.ExecutionResult)

		// Start API in a separate goroutine.
		go func() {

			log.Info().Str("port", cfg.API).Msg("Node API starting")
			err := server.Start(cfg.API)
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Warn().Err(err).Msg("Node API failed")
				close(failed)
			} else {
				close(done)
			}

			log.Info().Msg("Node API stopped")
		}()
	}

	select {
	case <-sig:
		log.Info().Msg("Allora Node stopping")
	case <-done:
		log.Info().Msg("Allora Node done")
	case <-failed:
		log.Info().Msg("Allora Node aborted")
		return failure
	}

	// If we receive a second interrupt signal, exit immediately.
	go func() {
		<-sig
		log.Warn().Msg("forcing exit")
		os.Exit(1)
	}()

	return success
}

func needLimiter(cfg *alloraCfg) bool {
	return cfg.CPUPercentage != 1.0 || cfg.MemoryMaxKB > 0
}
