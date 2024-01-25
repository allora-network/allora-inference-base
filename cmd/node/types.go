package main

import (
	"context"

	cosmossdk_io_math "cosmossdk.io/math"
	"github.com/blocklessnetwork/b7s/config"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosaccount"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosclient"
	"github.com/rs/zerolog"
	types "github.com/upshot-tech/protocol-state-machine-module"
)

type alloraCfg struct {
	config.Config
	AppChainConfig AppChainConfig
}

type AppChain struct {
	Ctx            context.Context
	ReputerAddress string
	ReputerAccount cosmosaccount.Account
	Client         cosmosclient.Client
	QueryClient    types.QueryClient
	WorkersAddress map[string]string
	Config AppChainConfig
}

type AppChainConfig struct {
	AddressPrefix   string // prefix for the cosmos addresses
	AddressKeyName  string // load a address by key from the keystore
	AddressRestoreMnemonic  string
	AddressAccountPassphrase string
	HomeDirectory   string // home directory for the cosmos keystore
	StringSeperator string // string seperator used for key identifiers in cosmos
	LibP2PKey 		string // the libp2p key used to sign offchain communications 
	Logger 			zerolog.Logger
}

type WorkerInference struct {
	Worker    string `json:"worker"`
	Inference cosmossdk_io_math.Uint `json:"inference"`
}

type WeightsResponse struct {
	Value string `json:"value"`
}

type WorkerWeights struct {
	Weights map[string]float64 `json:"-"` // Use a map to dynamically handle worker identifiers
}

type ResponseInfo struct {
	Mode string `json:"mode"`
	TopicId uint64 `json:"topicId"`
}

type Response struct {
	Value string `json:"value"`
}

var (
	inferenceType = "inferences"
	weightsType   = "weights"
)
