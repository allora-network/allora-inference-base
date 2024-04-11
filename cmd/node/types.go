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

type NodeValue struct {
	Worker string `json:"worker,omitempty"`
	Value  string `json:"value,omitempty"`
}

type InferenceForecastResponse struct {
	InfererValue     string      `json:"infererValue,omitempty"`
	ForecasterValues []NodeValue `json:"forecasterValues,omitempty"`
	Nonce            string      `json:"nonce,omitempty"`
	Signature        string      `json:"signature,omitempty"`
}

type LossResponse struct {
	CombinedValue          string      `json:"combinedValue,omitempty"`
	NaiveValue             string      `json:"naiveValue,omitempty"`
	InferrerValues         []NodeValue `json:"inferrerValues,omitempty"`
	ForecasterValues       []NodeValue `json:"forecasterValues,omitempty"`
	OneOutInfererValues    []NodeValue `json:"oneOutInfererValues,omitempty"`
	OneOutForecasterValues []NodeValue `json:"oneOutForecasterValues,omitempty"`
	OneInForecasterValues  []NodeValue `json:"oneInForecasterValues,omitempty"`
	Nonce                  types.Nonce `json:"nonce,omitempty"`
	Signature              string      `json:"signature,omitempty"`
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
