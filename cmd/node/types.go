package main

import (
	emissionstypes "github.com/allora-network/allora-chain/x/emissions/types"
	"github.com/allora-network/b7s/config"
	"github.com/allora-network/b7s/models/blockless"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosaccount"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosclient"
	"github.com/rs/zerolog"
)

type alloraCfg struct {
	config.Config
	AppChainConfig AppChainConfig
}

type AppChain struct {
	Address              string
	Account              cosmosaccount.Account
	Client               *cosmosclient.Client
	EmissionsQueryClient emissionstypes.QueryClient
	BankQueryClient      banktypes.QueryClient
	Config               AppChainConfig
	Logger               zerolog.Logger
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
	ReconnectSeconds         uint64  // seconds to wait for reconnection
	InitialStake             int64   // uallo to initially stake upon registration on a new topi
	WorkerMode               string  // Allora Network worker mode to use
	Gas                      string  // gas to use for the allora client
	GasAdjustment            float64 // gas adjustment to use for the allora client
}

type NodeValue struct {
	Worker string `json:"worker,omitempty"`
	Value  string `json:"value,omitempty"`
}

// WORKER
type InferenceForecastResponse struct {
	InfererValue     string      `json:"infererValue,omitempty"`
	ForecasterValues []NodeValue `json:"forecasterValue,omitempty"`
}

type WorkerDataResponse struct {
	*emissionstypes.WorkerDataBundle
	BlockHeight int64 `json:"blockHeight,omitempty"`
	TopicId     int64 `json:"topicId,omitempty"`
}

// REPUTER
// Local struct to hold the value bundle from the wasm function response
type ValueBundle struct {
	CombinedValue          string      `json:"combinedValue,omitempty"`
	NaiveValue             string      `json:"naiveValue,omitempty"`
	InfererValues          []NodeValue `json:"infererValues,omitempty"`
	ForecasterValues       []NodeValue `json:"forecasterValues,omitempty"`
	OneOutInfererValues    []NodeValue `json:"oneOutInfererValues,omitempty"`
	OneOutForecasterValues []NodeValue `json:"oneOutForecasterValues,omitempty"`
	OneInForecasterValues  []NodeValue `json:"oneInForecasterValues,omitempty"`
}

// Wrapper around the ReputerValueBundle to include the block height and topic id for the leader
type ReputerDataResponse struct {
	*emissionstypes.ReputerValueBundle
	BlockHeight     int64 `json:"blockHeight,omitempty"`
	BlockHeightEval int64 `json:"blockHeightEval,omitempty"`
	TopicId         int64 `json:"topicId,omitempty"`
}

type ReputerWASMResponse struct {
	Value string `json:"value,omitempty"`
}

const (
	WorkerModeWorker  = "worker"
	WorkerModeReputer = "reputer"
)

type AlloraExecutor struct {
	blockless.Executor
	appChain *AppChain
}

const AlloraExponential = 18
