package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog"
	"github.com/ziflex/lecho/v3"

	alloraMath "github.com/allora-network/allora-chain/math"
	"github.com/allora-network/allora-chain/x/emissions/types"
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
	success       = 0
	failure       = 1
	notFoundValue = -1
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
	// print incoming result:
	fmt.Println("Result from WASM: ", result.Result.Stdout)

	// Get the topicId from the env var
	var topicId uint64
	var topicFound bool = false
	var alloraBlockHeightCurrent int64 = notFoundValue
	var alloraBlockHeightEval int64 = notFoundValue
	for _, envVar := range req.Config.Environment {
		if envVar.Name == "TOPIC_ID" {
			topicFound = true
			// Get the topicId from the environment variable from str  as uint64
			topicId, err = strconv.ParseUint(envVar.Value, 10, 64)
			if err != nil {
				// check if it ends with "/reputer" and extract the previous numerical value
				if len(envVar.Value) > 8 && envVar.Value[len(envVar.Value)-8:] == "/reputer" {
					topicId, err = strconv.ParseUint(envVar.Value[:len(envVar.Value)-8], 10, 64)
					if err != nil {
						fmt.Println("Error parsing topic ID: ", err)
						return result, err
					}
				} else {
					fmt.Println("Error parsing topic ID: no int, no '/reputer' suffix ", err)
					return result, err
				}
			}
			fmt.Println("TOPIC_ID: ", topicId)
		} else if envVar.Name == "ALLORA_BLOCK_HEIGHT_CURRENT" {
			alloraBlockHeightCurrent, err = strconv.ParseInt(envVar.Value, 10, 64)
			if err != nil {
				fmt.Println("Error parsing ALLORA_BLOCK_HEIGHT_CURRENT: ", err)
				return result, err
			}
			fmt.Println("ALLORA_BLOCK_HEIGHT_CURRENT: ", alloraBlockHeightCurrent)
		} else if envVar.Name == "ALLORA_BLOCK_HEIGHT_EVAL" {
			// Get the topicId from the environment variable from str  as uint64
			alloraBlockHeightEval, err = strconv.ParseInt(envVar.Value, 10, 64)
			if err != nil {
				fmt.Println("Error parsing ALLORA_BLOCK_HEIGHT_EVAL: ", err)
				return result, err
			}
			fmt.Println("ALLORA_BLOCK_HEIGHT_EVAL: ", alloraBlockHeightEval)
		}
	}
	if !topicFound {
		fmt.Println("No topic ID found in the environment variables.")
		return result, nil
	}
	if alloraBlockHeightCurrent == notFoundValue {
		fmt.Println("No ALLORA_BLOCK_HEIGHT_CURRENT found in the environment variables.")
		return result, nil
	}

	if e.appChain == nil {
		fmt.Println("Appchain is nil, cannot sign the payload, returning as is.")
		return result, nil
	}
	// Iterate env vars to get the ALLORA_NONCE, if found, sign it and add the signature to the result
	// Check if this worker node is reputer or worker mode
	if e.appChain.Config.WorkerMode == WorkerModeWorker {
		// Get the nonce from the environment variable, convert to bytes
		// If appchain is null or SubmitTx is false, do not sign the nonce
		if e.appChain != nil && e.appChain.Client != nil {
			// Get the account from the appchain
			accountName := e.appChain.ReputerAccount.Name
			var responseValue InferenceForecastResponse
			err = json.Unmarshal([]byte(result.Result.Stdout), &responseValue)
			if err != nil {
				fmt.Println("Error serializing InferenceForecastResponse proto message: ", err)
			} else {
				// Build inference
				infererValue := alloraMath.MustNewDecFromString(responseValue.InfererValue)
				inference := &types.Inference{
					TopicId:     topicId,
					Inferer:     e.appChain.ReputerAddress,
					Value:       infererValue,
					BlockHeight: alloraBlockHeightCurrent,
				}
				// Build Forecast
				var forecasterElements []*types.ForecastElement
				for _, val := range responseValue.ForecasterValues {
					forecasterElements = append(forecasterElements, &types.ForecastElement{
						Inferer: val.Worker,
						Value:   alloraMath.MustNewDecFromString(val.Value),
					})
				}

				forecasterValues := &types.Forecast{
					TopicId:          topicId,
					BlockHeight:      alloraBlockHeightCurrent,
					Forecaster:       e.appChain.ReputerAddress,
					ForecastElements: forecasterElements,
				}

				inferenceForecastsBundle := &types.InferenceForecastBundle{
					Inference: inference,
					Forecast:  forecasterValues,
				}
				// Marshall and sign the bundle
				protoBytesIn := make([]byte, 0) // Create a byte slice with initial length 0 and capacity greater than 0
				protoBytesIn, err := inferenceForecastsBundle.XXX_Marshal(protoBytesIn, true)
				if err != nil {
					fmt.Println("Error Marshalling InferenceForecastsBundle: ", err)
					return result, err
				}
				sig, pk, err := e.appChain.Client.Context().Keyring.Sign(accountName, protoBytesIn, signing.SignMode_SIGN_MODE_DIRECT)
				pkStr := hex.EncodeToString(pk.Bytes())
				if err != nil {
					fmt.Println("Error signing the InferenceForecastsBundle message: ", err)
					return result, err
				}
				// Create workerDataBundle with signature
				workerDataBundle := &types.WorkerDataBundle{
					Worker:                             e.appChain.ReputerAddress,
					InferenceForecastsBundle:           inferenceForecastsBundle,
					InferencesForecastsBundleSignature: sig,
					Pubkey:                             pkStr,
				}

				// Bundle it with topic and blockheight info
				workerDataResponse := &WorkerDataResponse{
					WorkerDataBundle: workerDataBundle,
					BlockHeight:      alloraBlockHeightCurrent,
					TopicId:          int64(topicId),
				}
				// Serialize the workerDataBundle into json
				workerDataBundleBytes, err := json.Marshal(workerDataResponse)
				if err != nil {
					fmt.Println("Error serializing WorkerDataBundle: ", err)
					return result, err
				}
				outputJson := string(workerDataBundleBytes)
				fmt.Println("Signed OutputJson sent to consensus: ", outputJson)
				result.Result.Stdout = outputJson
			}
		} else {
			fmt.Println("Appchain is nil, cannot sign the payload.")
		}
	} else if e.appChain.Config.WorkerMode == WorkerModeReputer {
		// Get the nonce from the environment variable, convert to bytes
		// If appchain is null or SubmitTx is false, do not sign the nonce
		if e.appChain != nil && e.appChain.Client != nil {
			fmt.Println("Worker mode is Reputer, packaging output for consensus.")
			// Check also the EVAL nonce
			if alloraBlockHeightEval == notFoundValue {
				fmt.Println("No ALLORA_BLOCK_HEIGHT_EVAL found in the environment variables.")
				return result, nil
			}
			// Create ReputerRequestNonce
			reputerRequestNonce := &types.ReputerRequestNonce{
				ReputerNonce: &types.Nonce{
					BlockHeight: alloraBlockHeightCurrent,
				},
				WorkerNonce: &types.Nonce{
					BlockHeight: alloraBlockHeightEval,
				},
			}

			// Now get the string of the value, unescape it and unmarshall into ValueBundle
			// Unmarshal the "value" field from the LossResponse struct
			var wasmValueBundle ReputerWASMResponse
			err = json.Unmarshal([]byte(result.Result.Stdout), &wasmValueBundle)
			if err != nil {
				e.appChain.Logger.Error().Err(err).Msg("Error unmarshalling JSON Value.")
				return result, err
			}
			var nestedValueBundle ValueBundle
			err = json.Unmarshal([]byte(wasmValueBundle.Value), &nestedValueBundle)
			if err != nil {
				e.appChain.Logger.Error().Err(err).Msg("Error unmarshalling nested JSON ValueBundle:")
				return result, err
			}

			// Get the values from the nestedValueBundle
			var (
				inferVal       []*types.WorkerAttributedValue
				forecastsVal   []*types.WorkerAttributedValue
				outInferVal    []*types.WithheldWorkerAttributedValue
				outForecastVal []*types.WithheldWorkerAttributedValue
				inInferVal     []*types.WorkerAttributedValue
			)

			//
			// Test Data
			inferer1address := "allo1tvh6nv02vq6m4mevsa9wkscw53yxvfn7xt8rud"
			inferer2address := "allo12vncm038gpyr2u2v524pgqmmdg39uqn3qgnjjc"
			inferVal = make([]*types.WorkerAttributedValue, 0)
			forecastsVal = make([]*types.WorkerAttributedValue, 0)
			outInferVal = make([]*types.WithheldWorkerAttributedValue, 0)
			outForecastVal = make([]*types.WithheldWorkerAttributedValue, 0)
			inInferVal = make([]*types.WorkerAttributedValue, 0)
			inferVal = append(inferVal, &types.WorkerAttributedValue{
				Worker: inferer1address,
				Value:  alloraMath.MustNewDecFromString(fmt.Sprintf("%f", rand.Float64()*100)),
			})
			inferVal = append(inferVal, &types.WorkerAttributedValue{
				Worker: inferer2address,
				Value:  alloraMath.MustNewDecFromString(fmt.Sprintf("%f", rand.Float64()*100)),
			})

			forecastsVal = append(forecastsVal, &types.WorkerAttributedValue{
				Worker: inferer1address,
				Value:  alloraMath.MustNewDecFromString(fmt.Sprintf("%f", rand.Float64()*100)),
			})
			forecastsVal = append(forecastsVal, &types.WorkerAttributedValue{
				Worker: inferer2address,
				Value:  alloraMath.MustNewDecFromString(fmt.Sprintf("%f", rand.Float64()*100)),
			})

			outInferVal = append(outInferVal, &types.WithheldWorkerAttributedValue{
				Worker: inferer1address,
				Value:  alloraMath.MustNewDecFromString(fmt.Sprintf("%f", rand.Float64()*100)),
			})
			outInferVal = append(outInferVal, &types.WithheldWorkerAttributedValue{
				Worker: inferer2address,
				Value:  alloraMath.MustNewDecFromString(fmt.Sprintf("%f", rand.Float64()*100)),
			})

			outForecastVal = append(outForecastVal, &types.WithheldWorkerAttributedValue{
				Worker: inferer1address,
				Value:  alloraMath.MustNewDecFromString(fmt.Sprintf("%f", rand.Float64()*100)),
			})
			outForecastVal = append(outForecastVal, &types.WithheldWorkerAttributedValue{
				Worker: inferer2address,
				Value:  alloraMath.MustNewDecFromString(fmt.Sprintf("%f", rand.Float64()*100)),
			})

			inInferVal = append(inInferVal, &types.WorkerAttributedValue{
				Worker: inferer1address,
				Value:  alloraMath.MustNewDecFromString(fmt.Sprintf("%f", rand.Float64()*100)),
			})
			inInferVal = append(inInferVal, &types.WorkerAttributedValue{
				Worker: inferer2address,
				Value:  alloraMath.MustNewDecFromString(fmt.Sprintf("%f", rand.Float64()*100)),
			})
			incomingCombinedValue := alloraMath.MustNewDecFromString(fmt.Sprintf("%f", rand.Float64()*100))
			incomingNaiveValue := alloraMath.MustNewDecFromString(fmt.Sprintf("%f", rand.Float64()*100))

			// for _, inf := range nestedValueBundle.InferrerValues {
			// 	inferVal = append(inferVal, &types.WorkerAttributedValue{
			// 		Worker: inf.Worker,
			// 		Value:  alloraMath.MustNewDecFromString(inf.Value),
			// 	})
			// }
			// for _, inf := range nestedValueBundle.ForecasterValues {
			// 	forecastsVal = append(forecastsVal, &types.WorkerAttributedValue{
			// 		Worker: inf.Worker,
			// 		Value:  alloraMath.MustNewDecFromString(inf.Value),
			// 	})
			// }
			// for _, inf := range nestedValueBundle.OneOutInfererValues {
			// 	outInferVal = append(outInferVal, &types.WithheldWorkerAttributedValue{
			// 		Worker: inf.Worker,
			// 		Value:  alloraMath.MustNewDecFromString(inf.Value),
			// 	})
			// }
			// for _, inf := range nestedValueBundle.OneOutForecasterValues {
			// 	outForecastVal = append(outForecastVal, &types.WithheldWorkerAttributedValue{
			// 		Worker: inf.Worker,
			// 		Value:  alloraMath.MustNewDecFromString(inf.Value),
			// 	})
			// }
			// for _, inf := range nestedValueBundle.OneInForecasterValues {
			// 	inInferVal = append(inInferVal, &types.WorkerAttributedValue{
			// 		Worker: inf.Worker,
			// 		Value:  alloraMath.MustNewDecFromString(inf.Value),
			// 	})
			// }
			// incomingCombinedValue := alloraMath.MustNewDecFromString(nestedValueBundle.CombinedValue)
			// incomingNaiveValue := alloraMath.MustNewDecFromString(nestedValueBundle.NaiveValue)

			// Create ValueBundle
			newValueBundle := &types.ValueBundle{
				TopicId:                topicId,
				ReputerRequestNonce:    reputerRequestNonce,
				Reputer:                e.appChain.ReputerAddress,
				CombinedValue:          incomingCombinedValue,
				NaiveValue:             incomingNaiveValue,
				InfererValues:          inferVal,
				ForecasterValues:       forecastsVal,
				OneOutInfererValues:    outInferVal,
				OneOutForecasterValues: outForecastVal,
				OneInForecasterValues:  inInferVal,
			}

			// Marshall and sign the bundle
			// Get the account from the appchain
			accountName := e.appChain.ReputerAccount.Name
			protoBytesIn := make([]byte, 0)
			protoBytesIn, err := newValueBundle.XXX_Marshal(protoBytesIn, true)
			if err != nil {
				fmt.Println("Error Marshalling newValueBundle: ", err)
				return result, err
			}
			sig, pk, err := e.appChain.Client.Context().Keyring.Sign(accountName, protoBytesIn, signing.SignMode_SIGN_MODE_DIRECT)
			pkStr := hex.EncodeToString(pk.Bytes())
			if err != nil {
				fmt.Println("Error signing the InferenceForecastsBundle message: ", err)
				return result, err
			}

			// Create workerDataBundle with signature and pubkey
			valueBundle := &types.ReputerValueBundle{
				ValueBundle: newValueBundle,
				Signature:   sig,
				Pubkey:      pkStr,
			}

			reputerDataResponse := &ReputerDataResponse{
				ReputerValueBundle: valueBundle,
				BlockHeight:        alloraBlockHeightCurrent,
				BlockHeightEval:    alloraBlockHeightEval,
				TopicId:            int64(topicId),
			}

			// Serialize the workerDataBundle into json
			reputerDataResponseBytes, err := json.Marshal(reputerDataResponse)
			if err != nil {
				fmt.Println("Error serializing WorkerDataBundle: ", err)
				return result, err
			}
			outputJson := string(reputerDataResponseBytes)
			fmt.Println("Signed OutputJson sent to consensus: ", outputJson)
			result.Result.Stdout = outputJson
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

	var appchain *AppChain = nil
	if role == blockless.WorkerNode {
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
							log.Debug().Msg("Resetting up chain connection.")
							if alloraExecutor != nil {
								executor.appChain = appchain
							} else {
								log.Warn().Msg("No valid alloraExecutor with which to associate chain client.")
							}
						}
					}
				}
			}(alloraExecutor) // Pass alloraExecutor as an argument to the goroutine
		}
	}

	var resLoc sync.RWMutex
	response := func(msg []byte) {
		resLoc.Lock()
		var data node.ChanData
		msgerr := json.Unmarshal(msg, &data)
		if msgerr == nil {
			sendResultsToChain(log, appchain, data)
		} else {
			log.Error().Err(msgerr).Msg("Unable to unmarshall")
		}
		resLoc.Unlock()
	}
	// Instantiate node.
	node, err := node.New(log, host, peerstore, fstore, response, opts...)
	if err != nil {
		log.Error().Err(err).Msg("could not create node")
		return failure
	}
	// Create the main context.
	ctx, rcancel := context.WithCancel(context.Background())
	defer rcancel()

	done := make(chan struct{})
	failed := make(chan struct{})

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
	//go func() {
	//	select {
	//	case msg := <-node.CommunicatorAppLayer():
	//		msgerr := json.Unmarshal(msg, &response)
	//		if msgerr == nil {
	//			sendResultsToChain(log, appchain, response)
	//		} else {
	//			log.Error().Err(msgerr).Msg("Unable to unmarshall")
	//		}
	//	}
	//}()

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
		server.POST("/api/v1/functions/execute", createExecutor(*api))
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
