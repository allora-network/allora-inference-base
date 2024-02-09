package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	cosmossdk_io_math "cosmossdk.io/math"
	types "github.com/allora-network/allora-chain/x/emissions"
	"github.com/blocklessnetwork/b7s/models/blockless"
	"github.com/blocklessnetwork/b7s/node/aggregate"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosaccount"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosclient"
	"github.com/rs/zerolog"
)

// create a new appchain client that we can use
func NewAppChain(config AppChainConfig, log zerolog.Logger) (*AppChain, error) {
	ctx := context.Background()
	config.SubmitTx = true
	userHomeDir, _ := os.UserHomeDir()
	cosmosClientHome := filepath.Join(userHomeDir, ".allorad")
	if config.CosmosHomeDir != "" {
		cosmosClientHome = config.CosmosHomeDir
	}

	// Check that the given home folder exist
	if _, err := os.Stat(cosmosClientHome); errors.Is(err, os.ErrNotExist) {
		log.Warn().Err(err).Msg("could not get home directory for cosmos client, creating...")
		err = os.MkdirAll(cosmosClientHome, 0755)
		if err != nil {
			log.Warn().Err(err).Str("directory", cosmosClientHome).Msg("Cannot create cosmos client home directory")
			config.SubmitTx = false
			return nil, err
		}
		log.Info().Err(err).Str("directory", cosmosClientHome).Msg("cosmos client home directory created")
	}

	// create a cosmos client instance
	client, err := cosmosclient.New(ctx, cosmosclient.WithNodeAddress(config.NodeRPCAddress), cosmosclient.WithAddressPrefix(config.AddressPrefix), cosmosclient.WithHome(cosmosClientHome))
	if err != nil {
		log.Warn().Err(err).Msg("unable to create an allora blockchain client")
		config.SubmitTx = false
		return nil, err
	}

	var account cosmosaccount.Account

	// if we're giving a keyring ring name, with no mnemonic restore
	if config.AddressRestoreMnemonic == "" && config.AddressKeyName != "" {
		// get account from the keyring
		account, err = client.Account(config.AddressKeyName)
		if err != nil {
			config.SubmitTx = false
			log.Warn().Err(err).Msg("could not retrieve account from keyring")
		}
	} else if config.AddressRestoreMnemonic != "" && config.AddressKeyName != "" {
		// restore from mneumonic
		account, err = client.AccountRegistry.Import(config.AddressKeyName, config.AddressRestoreMnemonic, config.AddressAccountPassphrase)
		if err != nil {
			if err.Error() == "account already exists" {
				account, err = client.Account(config.AddressKeyName)
			}

			if err != nil {
				config.SubmitTx = false
				log.Error().Err(err).Msg("error getting account")
			}
		}
	} else {
		log.Warn().Msg("no cosmos account was loaded")
		return nil, nil
	}

	address, err := account.Address(config.AddressPrefix)
	if err != nil {
		config.SubmitTx = false
		log.Warn().Err(err).Msg("could not retrieve allora blockchain address, transactions will not be submitted to chain")
	}

	// Create query client
	queryClient := types.NewQueryClient(client.Context())

	// this is terrible, no isConnected as part of this code path
	if client.Context().ChainID == "" {
		return nil, nil
	}

	appchain := &AppChain{
		ReputerAddress: address,
		ReputerAccount: account,
		Logger:         log,
		Client:         &client,
		QueryClient:    queryClient,
		Config:         config,
	}
	if !queryIsNodeRegistered(*appchain) {
		registerWithBlockchain(appchain)
	}

	return appchain, nil
}

// / Registration
func registerWithBlockchain(appchain *AppChain) {
	ctx := context.Background()

	var msg sdktypes.Msg
	if appchain.Config.NodeRole == blockless.HeadNode {
		msg = &types.MsgRegisterReputer{
			Creator:      appchain.ReputerAddress,
			LibP2PKey:    appchain.Config.LibP2PKey,
			MultiAddress: appchain.Config.MultiAddress,
			InitialStake: cosmossdk_io_math.NewUint(1),
			TopicId:      appchain.Config.TopicId,
		}
	} else {
		msg = &types.MsgRegisterWorker{
			Creator:      appchain.ReputerAddress,
			Owner:        appchain.ReputerAddress, // we need to allow a pass in of a claim address
			LibP2PKey:    appchain.Config.LibP2PKey,
			MultiAddress: appchain.Config.MultiAddress,
			InitialStake: cosmossdk_io_math.NewUint(1),
			TopicId:      appchain.Config.TopicId,
		}
	}

	txResp, err := appchain.Client.BroadcastTx(ctx, appchain.ReputerAccount, msg)
	if err != nil {
		if strings.Contains(fmt.Sprint(err), types.Err_ErrWorkerAlreadyRegistered.String()) || strings.Contains(fmt.Sprint(err), types.Err_ErrReputerAlreadyRegistered.String()) {
			appchain.Logger.Info().Err(err).Msg("node is already registered")
		} else {
			appchain.Logger.Fatal().Err(err).Msg("could not register the node with the allora blockchain")
		}
	} else {
		appchain.Logger.Info().Str("txhash", txResp.TxHash).Msg("successfully registered node with Allora blockchain")
	}
}

func queryIsNodeRegistered(appchain AppChain) bool {
	ctx := context.Background()
	queryResp, err := appchain.QueryClient.GetWorkerNodeRegistration(ctx, &types.QueryRegisteredWorkerNodesRequest{
		NodeId: appchain.ReputerAddress + appchain.Config.StringSeperator + appchain.Config.LibP2PKey,
	})

	if err != nil {
		appchain.Logger.Fatal().Err(err).Msg("node could not be registered with blockchain")
	}

	return (len(queryResp.Nodes) >= 1)
}

// Sending Inferences to the AppChain
func (ap *AppChain) SendInferences(ctx context.Context, topicId uint64, results aggregate.Results) []WorkerInference {
	// Aggregate the inferences from all peers/workers
	var inferences []*types.Inference
	var workersInferences []WorkerInference
	for _, result := range results {
		for _, peer := range result.Peers {
			ap.Logger.Info().Any("peer", peer)

			// Get Peer $allo address
			res, err := ap.QueryClient.GetWorkerAddressByP2PKey(ctx, &types.QueryWorkerAddressByP2PKeyRequest{
				Libp2PKey: peer.String(),
			})
			if err != nil {
				ap.Logger.Fatal().Err(err).Msg("error getting peer address")
			}

			value, err := extractNumber(result.Result.Stdout)
			if err != nil || value == "" {
				ap.Logger.Fatal().Err(err).Msg("error extracting number from stdout")
				value = "0"
			}
			parsed, err := parseFloatToUint64(value)
			if err != nil {
				ap.Logger.Fatal().Err(err).Msg("error parsing uint")
			}
			inference := &types.Inference{
				TopicId: topicId,
				Worker:  res.Address,
				Value:   cosmossdk_io_math.NewUint(parsed),
			}
			inferences = append(inferences, inference)
			workersInferences = append(workersInferences, WorkerInference{Worker: inference.Worker, Inference: inference.Value})
		}
	}

	req := &types.MsgProcessInferences{
		Sender:     ap.ReputerAddress,
		Inferences: inferences,
	}

	txResp, err := ap.Client.BroadcastTx(ctx, ap.ReputerAccount, req)
	if err != nil {
		ap.Logger.Info().Err(err).Msg("failed to send inferences to allora blockchain")
	}

	ap.Logger.Info().Any("Tx Resp:", txResp).Msg("successfully sent inferences to allora blockchain")

	return workersInferences
}

func (ap *AppChain) SendUpdatedWeights(ctx context.Context, topicId uint64, results aggregate.Results) {

	weights := make([]*types.Weight, 0)
	for _, result := range results {
		extractedWeights, err := extractWeights(result.Result.Stdout)
		if err != nil {
			ap.Logger.Info().Err(err).Msg("Error extracting weight")
			continue
		}

		for peer, value := range extractedWeights {
			ap.Logger.Info().Str("peer", peer)
			parsed, err := parseFloatToUint64Weights(value)
			if err != nil {
				ap.Logger.Error().Err(err).Msg("Error parsing uint")
				continue
			}

			fmt.Printf("\n Worker Node: %s Weight: %v \n", peer, cosmossdk_io_math.NewUint(parsed))

			weight := &types.Weight{
				TopicId: topicId,
				Reputer: ap.ReputerAddress,
				Worker:  peer,
				Weight:  cosmossdk_io_math.NewUint(parsed),
			}
			weights = append(weights, weight)
		}
	}

	// Send updated weights to AppChain
	req := &types.MsgSetWeights{
		Sender:  ap.ReputerAddress,
		Weights: weights,
	}

	txResp, err := ap.Client.BroadcastTx(ctx, ap.ReputerAccount, req)
	if err != nil {
		ap.Logger.Info().Err(err).Msg("failed to send weights to allora blockchain")
	}

	ap.Logger.Info().Any("Tx Resp:", txResp).Msg("successfully sent weights to allora blockchain")
}

func parseFloatToUint64Weights(input string) (uint64, error) {
	// Parse the string to a floating-point number
	floatValue, err := strconv.ParseFloat(input, 64)
	if err != nil {
		return 0, err
	}

	// Truncate or round the floating-point number to an integer
	roundedValue := uint64(floatValue * 100000) // TODO: Change

	return roundedValue, nil
}

func parseFloatToUint64(input string) (uint64, error) {
	// Parse the string to a floating-point number
	floatValue, err := strconv.ParseFloat(input, 64)
	if err != nil {
		return 0, err
	}

	// Truncate or round the floating-point number to an integer
	roundedValue := uint64(math.Round(floatValue))

	return roundedValue, nil
}

func extractNumber(stdout string) (string, error) {
	// Parse the unquoted JSON string
	var response Response
	err := json.Unmarshal([]byte(stdout), &response)
	if err != nil {
		return "", err
	}

	return response.Value, nil
}

func extractWeights(stdout string) (map[string]string, error) {
	fmt.Println("Extracting weights from stdout: ", stdout)

	var weights WorkerWeights
	err := json.Unmarshal([]byte(stdout), &weights)
	if err != nil {
		return nil, err
	}

	return weights.Weights, nil
}
