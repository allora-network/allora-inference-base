package main

import (
	"github.com/allora-network/allora-chain/x/emissions/types"
	"github.com/allora-network/b7s/config"
	"github.com/allora-network/b7s/models/blockless"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosaccount"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosclient"
	"github.com/rs/zerolog"
)

type alloraCfg struct {
	config.Config
	AppChainConfig AppChainConfig
}

type AppChain struct {
	ReputerAddress string
	ReputerAccount cosmosaccount.Account
	Client         *cosmosclient.Client
	QueryClient    types.QueryClient
	WorkersAddress map[string]string
	Config         AppChainConfig
	Logger         zerolog.Logger
}

type AppChainConfig struct {
	NodeRPCAddress           string // rpc node to attach to
	AddressPrefix            string // prefix for the allora addresses
	AddressKeyName           string // load a address by key from the keystore
	AddressRestoreMnemonic   string
	AddressAccountPassphrase string
	AlloraHomeDir            string // home directory for the allora keystore
	StringSeperator          string // string seperator used for key identifiers in allora
	LibP2PKey                string // the libp2p key used to sign offchain communications
	SubmitTx                 bool   // do we need to commit these to the chain, might be a reason not to
	MultiAddress             string
	TopicIds                 []string
	NodeRole                 blockless.NodeRole
	ReconnectSeconds         uint64 // seconds to wait for reconnection
	InitialStake             uint64 // uallo to initially stake upon registration on a new topi
	WorkerMode               string // Allora Network worker mode to use
}

type ResponseInfo struct {
	FunctionType string `json:"type"`
}

type Response struct {
	Value string `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}

type Inference struct {
	Node  string  `json:"node,omitempty"`
	Value float64 `json:"value,omitempty"`
}
type LossResponse struct {
	NetworkInference        float64     `json:"networkInference,omitempty"`
	NaiveNetworkInference   float64     `json:"naiveNetworkInference,omitempty"`
	InferrerInferences      []Inference `json:"inferrerInferences,omitempty"`
	ForecasterInferences    []Inference `json:"forecasterInferences,omitempty"`
	OneOutNetworkInferences []Inference `json:"oneOutNetworkInferences,omitempty"`
	OneInNetworkInferences  []Inference `json:"oneInNetworkInferences,omitempty"`
}

const (
	WorkerModeWorker  = "worker"
	WorkerModeReputer = "reputer"
)

type AlloraExecutor struct {
	blockless.Executor
	appChain *AppChain
}

const (
	AlloraExponent = 18
)
