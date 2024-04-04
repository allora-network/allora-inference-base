package main

import (
	"context"
	"github.com/allora-network/b7s/host"
	"github.com/allora-network/b7s/models/blockless"
	"github.com/allora-network/b7s/models/execute"
	"github.com/allora-network/b7s/node/aggregate"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/suite"
	"os"
	"testing"
	"time"
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
				Stdout:   "{\"value\": 66}",
				Stderr:   "",
				ExitCode: 0,
				Log:      "",
			},
			Peers:     []peer.ID{"12D3KooWAPLuwMuvH9pmGfAZkCrfhKdhJNYn8eaDSJyX9Kd3HjVS"},
			Frequency: 25,
		},
		{
			Result: execute.RuntimeOutput{
				Stdout:   "{\"value\": 82}",
				Stderr:   "",
				ExitCode: 0,
				Log:      "",
			},
			Peers:     []peer.ID{"12D3KooWQrN5U3BApv4JYjE5HyKXFKkRF2U8c5FgK3zMPjzkZTpQ"},
			Frequency: 25,
		},
		{
			Result: execute.RuntimeOutput{
				Stdout:   "{\"value\": 19}",
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
