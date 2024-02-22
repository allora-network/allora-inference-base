package main

import (
	types "github.com/allora-network/allora-chain/x/emissions"
	"github.com/blocklessnetwork/b7s/config"
	"github.com/blocklessnetwork/b7s/models/blockless"
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
	TopicId                  uint64
	NodeRole                 blockless.NodeRole
	ReconnectSeconds         uint64 // seconds to wait for reconnection
}

type WeightsResponse struct {
	Value string `json:"value"`
}

type WorkerWeights struct {
	Type    string            `json:"type"`
	Weights map[string]string `json:"weights"`
}

type WeightsCalcDependencies struct {
	LatestWeights map[string]float64
	ActualPrice   float64
}

type ResponseInfo struct {
	FunctionType string `json:"type"`
}

type Response struct {
	Value string `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}

var (
	inferenceType = "inferences"
	weightsType   = "weights"
)
