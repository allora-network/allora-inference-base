package main

import (
	"context"

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
	Inference uint64 `json:"inference"`
}

type WeightsResponse struct {
	Value string `json:"value"`
}

type WorkerWeights struct {
	Weights map[string]float64 `json:"-"` // Use a map to dynamically handle worker identifiers
}

type WeightsCalcDependencies struct {
	LatestWeights map[string]float64
	ActualPrice   float64
}

// EthereumPriceResponse represents the JSON structure returned by CoinGecko API
type EthereumPriceResponse struct {
	Ethereum map[string]float64 `json:"ethereum"`
}

// Define a struct that matches the JSON structure of your stdout
type StdoutData struct {
	Value string `json:"value"`
}

type Response struct {
	Value string `json:"value"`
}