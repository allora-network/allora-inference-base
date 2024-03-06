package main

import (
	"github.com/spf13/pflag"

	"github.com/blocklessnetwork/b7s/node"
)

// Default values.
const (
	defaultPort         = 0
	defaultAddress      = "0.0.0.0"
	defaultPeerDB       = "peer-db"
	defaultFunctionDB   = "function-db"
	defaultConcurrency  = uint(node.DefaultConcurrency)
	defaultUseWebsocket = false
	defaultRole         = "worker"
)

func parseFlags() *alloraCfg {

	var cfg alloraCfg

	pflag.StringVarP(&cfg.Log.Level, "log-level", "l", "info", "log level to use")

	// Node configuration.
	pflag.StringVarP(&cfg.Role, "role", "r", defaultRole, "role this note will have in the Blockless protocol (head or worker)")
	pflag.StringVar(&cfg.PeerDatabasePath, "peer-db", defaultPeerDB, "path to the database used for persisting peer data")
	pflag.StringVar(&cfg.FunctionDatabasePath, "function-db", defaultFunctionDB, "path to the database used for persisting function data")
	pflag.UintVarP(&cfg.Concurrency, "concurrency", "c", defaultConcurrency, "maximum number of requests node will process in parallel")
	pflag.StringVar(&cfg.API, "rest-api", "", "address where the head node REST API will listen on")
	pflag.StringVar(&cfg.Workspace, "workspace", "./workspace", "directory that the node can use for file storage")
	pflag.StringVar(&cfg.RuntimePath, "runtime-path", "", "runtime path (used by the worker node)")
	pflag.StringVar(&cfg.RuntimeCLI, "runtime-cli", "", "runtime path (used by the worker node)")
	pflag.BoolVar(&cfg.LoadAttributes, "attributes", false, "node should try to load its attribute data from IPFS")
	pflag.StringSliceVar(&cfg.Topics, "topic", nil, "topics node should subscribe to")

	// Host configuration.
	pflag.StringVar(&cfg.Host.PrivateKey, "private-key", "", "private key that the b7s host will use")
	pflag.StringVarP(&cfg.Host.Address, "address", "a", defaultAddress, "address that the b7s host will use")
	pflag.UintVarP(&cfg.Host.Port, "port", "p", defaultPort, "port that the b7s host will use")
	pflag.StringSliceVar(&cfg.BootNodes, "boot-nodes", nil, "list of addresses that this node will connect to on startup, in multiaddr format")

	// For external IPs.
	pflag.StringVarP(&cfg.Host.DialBackAddress, "dialback-address", "", defaultAddress, "external address that the b7s host will advertise")
	pflag.UintVarP(&cfg.Host.DialBackPort, "dialback-port", "", defaultPort, "external port that the b7s host will advertise")
	pflag.UintVarP(&cfg.Host.DialBackWebsocketPort, "websocket-dialback-port", "", defaultPort, "external port that the b7s host will advertise for websocket connections")

	// Websocket connection.
	pflag.BoolVarP(&cfg.Host.Websocket, "websocket", "w", defaultUseWebsocket, "should the node use websocket protocol for communication")
	pflag.UintVar(&cfg.Host.WebsocketPort, "websocket-port", defaultPort, "port to use for websocket connections")

	// Limit configuration.
	pflag.Float64Var(&cfg.CPUPercentage, "cpu-percentage-limit", 1.0, "amount of CPU time allowed for Blockless Functions in the 0-1 range, 1 being unlimited")
	pflag.Int64Var(&cfg.MemoryMaxKB, "memory-limit", 0, "memory limit (kB) for Blockless Functions")

	// Allora L1 configuration
	pflag.StringVarP(&cfg.AppChainConfig.AlloraHomeDir, "allora-chain-home-dir", "", "", "The Home folder of the client, use the user home if not set")
	pflag.StringVarP(&cfg.AppChainConfig.AddressKeyName, "allora-chain-key-name", "", "", "The name of a key stored in the Allora Blockchain Wallet")
	pflag.StringVarP(&cfg.AppChainConfig.AddressRestoreMnemonic, "allora-chain-restore-mnemonic", "", "", "The restore mnemonic for an Allora Blockchain Wallet")
	pflag.StringVarP(&cfg.AppChainConfig.AddressAccountPassphrase, "allora-chain-account-password", "", "", "The password for an Allora Blockchain Wallet Key")
	pflag.StringVarP(&cfg.AppChainConfig.NodeRPCAddress, "allora-node-rpc-address", "", "http://localhost:26657", "The address for the client to connect to a node.")
	pflag.Uint64Var(&cfg.AppChainConfig.TopicId, "allora-chain-topic-id", 0, "The topic id for the topic that the node will subscribe to.")
	pflag.Uint64Var(&cfg.AppChainConfig.ReconnectSeconds, "allora-chain-reconnect-seconds", 60, "If connection to Allora Appchain breaks, it will attempt to reconnect with this interval. O means no reconnection.")
	pflag.Uint64Var(&cfg.AppChainConfig.InitialStake, "allora-chain-initial-stake", 1000, "Upon registering on a new topic, amount of stake to use.")

	pflag.CommandLine.SortFlags = false

	pflag.Parse()

	return &cfg
}
