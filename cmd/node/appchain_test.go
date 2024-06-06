package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	alloraMath "github.com/allora-network/allora-chain/math"
	"github.com/allora-network/allora-chain/x/emissions/types"
	"github.com/allora-network/b7s/host"
	"github.com/allora-network/b7s/models/blockless"
	"github.com/allora-network/b7s/models/execute"
	"github.com/allora-network/b7s/node/aggregate"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/suite"
)

type AppChainTestSuit struct {
	suite.Suite
	app *AppChain
}

func TestAppChainTestSuite(t *testing.T) {
	suite.Run(t, new(AppChainTestSuit))
}

func (ap *AppChainTestSuit) SetupTest() {

	cfg := parseFlags()

	// Initialize logging.
	log := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).With().Timestamp().Logger().Level(zerolog.DebugLevel)

	// Get the list of boot nodes addresses.
	bootNodeAddrs, err := getBootNodeAddresses(cfg.BootNodes)
	if err != nil {
		return
	}

	host, err := host.New(log, cfg.Host.Address, cfg.Host.Port,
		host.WithPrivateKey(cfg.Host.PrivateKey),
		host.WithBootNodes(bootNodeAddrs),
		host.WithDialBackPeers(nil),
		host.WithDialBackAddress(cfg.Host.DialBackAddress),
		host.WithDialBackPort(cfg.Host.DialBackPort),
		host.WithDialBackWebsocketPort(cfg.Host.DialBackWebsocketPort),
		host.WithWebsocket(cfg.Host.Websocket),
		host.WithWebsocketPort(cfg.Host.WebsocketPort),
	)

	if err != nil {
		return
	}

	cfg.AppChainConfig.NodeRole = blockless.WorkerNode
	cfg.AppChainConfig.AddressPrefix = "allo"
	cfg.AppChainConfig.StringSeperator = "|"
	cfg.AppChainConfig.LibP2PKey = host.ID().String()
	cfg.AppChainConfig.MultiAddress = host.Addresses()[0]

	ap.app, err = connectToAlloraBlockchain(cfg.AppChainConfig, log)
}
func (ap *AppChainTestSuit) TestSendWorkerModeData() {
	ctx, _ := context.WithCancel(context.Background())
	var aggre = []aggregate.Result{
		{
			Result: execute.RuntimeOutput{
				Stdout:   "{\"infererValue\": \"3234.12\",\"forecasterValues\":[{\"node\":\"allo1inf1\",\"value\",\"0.256\"},{\"node\":\"allo1inf2\",\"value\":\"1.48\"},{\"node\":\"allo1inf1111\",\"value\":\"0.569885\"}]}",
				Stderr:   "",
				ExitCode: 0,
				Log:      "",
			},
			Peers:     []peer.ID{"12D3KooWAPLuwMuvH9pmGfAZkCrfhKdhJNYn8eaDSJyX9Kd3HjVS"},
			Frequency: 25,
		},
		{
			Result: execute.RuntimeOutput{
				Stdout:   "{\"infererValue\": \"1234.56\",\"forecasterValues\":[{\"node\":\"allo1inf1\",\"value\",\"0.256\"},{\"node\":\"allo1inf2\",\"value\":\"1.48\"},{\"node\":\"allo1inf1111\",\"value\":\"0.569885\"}]}",
				Stderr:   "",
				ExitCode: 0,
				Log:      "",
			},
			Peers:     []peer.ID{"12D3KooWQrN5U3BApv4JYjE5HyKXFKkRF2U8c5FgK3zMPjzkZTpQ"},
			Frequency: 25,
		},
		{
			Result: execute.RuntimeOutput{
				Stdout:   "{\"infererValue\": \"9876.34\",\"forecasterValues\":[{\"node\":\"allo1inf1\",\"value\",\"0.256\"},{\"node\":\"allo1inf2\",\"value\":\"1.48\"},{\"node\":\"allo1inf1111\",\"value\":\"0.569885\"}]}",
				Stderr:   "",
				ExitCode: 0,
				Log:      "",
			},
			Peers:     []peer.ID{"12D3KooWRfswWHRe4718tbMWHU2B2sEbZxJXQsdWCJmURReMozqX"},
			Frequency: 25,
		},
	}
	ap.app.SendWorkerModeData(ctx, 1, aggre)
}
func (ap *AppChainTestSuit) TestSendReputerModeData() {
	ctx, _ := context.WithCancel(context.Background())
	var aggre = []aggregate.Result{
		{
			Result: execute.RuntimeOutput{
				Stdout:   "{\"networkInference\":0.014399999999973807,\n                        \"naiveNetworkInference\":0.01960000000002801,\n                        \"inferrerInferences\":[\n                            {\"node\":\"allo1inf1\",\"value\":0.18490000000005475},\n                            {\"node\":\"allo1inf2\",\"value\":0.3248999999999274},\n                            {\"node\":\"allo1inf0000\",\"value\":0.02889999999994743}\n                        ],\n                        \"forecasterInferences\":[\n                            {\"node\":\"allo1inf1\",\"value\":0.05760000000000436},\n                            {\"node\":\"allo1inf2\",\"value\":1.587599999999977},\n                            {\"node\":\"allo1inf1111\",\"value\":0.006399999999988359}\n                        ],\n                        \"oneOutNetworkInferences\":[\n                            {\"node\":\"allo1inf1\",\"value\":0.05760000000000436},\n                            {\"node\":\"allo1inf2\",\"value\":0.06759999999999528},\n                            {\"node\":\"allo1inf0000\",\"value\":1.0200999999999816}\n                        ],\n                        \"oneInNetworkInferences\":[\n                            {\"node\":\"allo1inf1\",\"value\":0.05760000000000436},\n                            {\"node\":\"allo1inf2\",\"value\":0.06759999999999528},\n                            {\"node\":\"allo1inf1111\",\"value\":1.0200999999999816}\n                        ]}",
				Stderr:   "",
				ExitCode: 0,
				Log:      "",
			},
			Peers:     []peer.ID{"allo1lfhccfylj30t2zz9mzudx54h25x8mu0jrzjfz2"},
			Frequency: 25,
		},
		{
			Result: execute.RuntimeOutput{
				Stdout:   "{\"networkInference\":0.014399999999973807,\n                        \"naiveNetworkInference\":0.01960000000002801,\n                        \"inferrerInferences\":[\n                            {\"node\":\"allo1inf1\",\"value\":0.18490000000005475},\n                            {\"node\":\"allo1inf2\",\"value\":0.3248999999999274},\n                            {\"node\":\"allo1inf0000\",\"value\":0.02889999999994743}\n                        ],\n                        \"forecasterInferences\":[\n                            {\"node\":\"allo1inf1\",\"value\":0.05760000000000436},\n                            {\"node\":\"allo1inf2\",\"value\":1.587599999999977},\n                            {\"node\":\"allo1inf1111\",\"value\":0.006399999999988359}\n                        ],\n                        \"oneOutNetworkInferences\":[\n                            {\"node\":\"allo1inf1\",\"value\":0.05760000000000436},\n                            {\"node\":\"allo1inf2\",\"value\":0.06759999999999528},\n                            {\"node\":\"allo1inf0000\",\"value\":1.0200999999999816}\n                        ],\n                        \"oneInNetworkInferences\":[\n                            {\"node\":\"allo1inf1\",\"value\":0.05760000000000436},\n                            {\"node\":\"allo1inf2\",\"value\":0.06759999999999528},\n                            {\"node\":\"allo1inf1111\",\"value\":1.0200999999999816}\n                        ]}",
				Stderr:   "",
				ExitCode: 0,
				Log:      "",
			},
			Peers:     []peer.ID{"12D3KooWQrN5U3BApv4JYjE5HyKXFKkRF2U8c5FgK3zMPjzkZTpQ"},
			Frequency: 25,
		},
		{
			Result: execute.RuntimeOutput{
				Stdout:   "{\"networkInference\":0.014399999999973807,\n                        \"naiveNetworkInference\":0.01960000000002801,\n                        \"inferrerInferences\":[\n                            {\"node\":\"allo1inf1\",\"value\":0.18490000000005475},\n                            {\"node\":\"allo1inf2\",\"value\":0.3248999999999274},\n                            {\"node\":\"allo1inf0000\",\"value\":0.02889999999994743}\n                        ],\n                        \"forecasterInferences\":[\n                            {\"node\":\"allo1inf1\",\"value\":0.05760000000000436},\n                            {\"node\":\"allo1inf2\",\"value\":1.587599999999977},\n                            {\"node\":\"allo1inf1111\",\"value\":0.006399999999988359}\n                        ],\n                        \"oneOutNetworkInferences\":[\n                            {\"node\":\"allo1inf1\",\"value\":0.05760000000000436},\n                            {\"node\":\"allo1inf2\",\"value\":0.06759999999999528},\n                            {\"node\":\"allo1inf0000\",\"value\":1.0200999999999816}\n                        ],\n                        \"oneInNetworkInferences\":[\n                            {\"node\":\"allo1inf1\",\"value\":0.05760000000000436},\n                            {\"node\":\"allo1inf2\",\"value\":0.06759999999999528},\n                            {\"node\":\"allo1inf1111\",\"value\":1.0200999999999816}\n                        ]}",
				Stderr:   "",
				ExitCode: 0,
				Log:      "",
			},
			Peers:     []peer.ID{"12D3KooWRfswWHRe4718tbMWHU2B2sEbZxJXQsdWCJmURReMozqX"},
			Frequency: 25,
		},
	}
	ap.app.SendReputerModeData(ctx, 1, aggre)
}
func (ap *AppChainTestSuit) TestSendDataWithRetry() {
	ctx, _ := context.WithCancel(context.Background())

	req := &types.MsgInsertBulkWorkerPayload{
		Sender:  "allo1mnfm9c7cdgqnkk66sganp78m0ydmcr4pce8kju",
		Nonce:   &types.Nonce{BlockHeight: 1},
		TopicId: 0,
		WorkerDataBundles: []*types.WorkerDataBundle{
			{
				Worker: "allo1353a4uac03etdylz86tyq9ssm3x2704j66t99e",
				InferenceForecastsBundle: &types.InferenceForecastBundle{
					Inference: &types.Inference{
						TopicId:     0,
						BlockHeight: 1,
						Inferer:     "allo163xn94xytks2375ulpxdv7kqvvvxfvazpxyqzh",
						Value:       alloraMath.NewDecFromInt64(100),
					},
					Forecast: &types.Forecast{
						TopicId:     0,
						BlockHeight: 10,
						Forecaster:  "allo1d8vj2se0f63x90u4msfuy5mva3arengrr7t5f6",
						ForecastElements: []*types.ForecastElement{
							{
								Inferer: "allo148awdjfw7jf0847mkqvqn8mu4tqa92r52tzw5j",
								Value:   alloraMath.NewDecFromInt64(100),
							},
							{
								Inferer: "allo148awdjfw7jf0847mkqvqn8mu4tqa92r52tzw5j",
								Value:   alloraMath.NewDecFromInt64(100),
							},
						},
					},
				},
				InferencesForecastsBundleSignature: []byte("Signature"),
			},
		},
	}
	src := make([]byte, 0)
	src, _ = req.WorkerDataBundles[0].InferenceForecastsBundle.XXX_Marshal(src, true)
	sig, pk, err := ap.app.Client.Context().Keyring.Sign(ap.app.Account.Name, src, signing.SignMode_SIGN_MODE_DIRECT)
	pkStr := hex.EncodeToString(pk.Bytes())
	if err != nil {
		fmt.Println("Error signing the nonce: ", err)
		return
	}
	req.WorkerDataBundles[0].Pubkey = pkStr
	req.WorkerDataBundles[0].InferencesForecastsBundleSignature = sig
	ap.app.SendDataWithRetry(ctx, req, 5, 0, 2, "test send with retry")
}
