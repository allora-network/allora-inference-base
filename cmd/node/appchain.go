package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"

	cosmossdk_io_math "cosmossdk.io/math"

	"github.com/blocklessnetwork/b7s/node/aggregate"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosaccount"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosclient"
	types "github.com/upshot-tech/protocol-state-machine-module"
)

func (ap *AppChain) start(ctx context.Context) {
	go ap.startClient(ctx, ap.Config)
}

func (ap *AppChain)  startClient(ctx context.Context, config AppChainConfig) {
    client := ap.Client

	account, err := client.Account(config.AddressKeyName)
    if err != nil {
       config.Logger.Fatal().Err(err).Msg("could not retrieve allora blockchain account")
    }

	address, err := account.Address(config.AddressPrefix)
    if err != nil {
        log.Fatal(err)
    }

	if (!queryIsNodeRegistered(ctx, client, address, config)) {
		// not registered, register the node
		registerWithBlockchain(ctx, client, account, config)
	}

	config.Logger.Info().Msg("allora blockchain registration verification complete")
}

func (ap *AppChain) New() (*AppChain, error) {
	ctx := context.Background()

	nodeAddress := ap.Config.LibP2PKey
	if nodeAddress == "" {
		return nil, fmt.Errorf("NODE_ADDRESS environment variable is not set")
	}
	uptAccountMnemonic := ap.Config.AddressRestoreMnemonic
	if uptAccountMnemonic == "" {
		return nil, fmt.Errorf("UPT_ACCOUNT_MNEMONIC environment variable is not set")
	}
	uptAccountName := ap.Config.AddressKeyName
	if uptAccountName == "" {
		return nil, fmt.Errorf("UPT_ACCOUNT_NAME environment variable is not set")
	}
	// Passpharase is optional
	uptAccountPassphrase := ap.Config.AddressAccountPassphrase

	client, err := cosmosclient.New(ctx, cosmosclient.WithAddressPrefix(ap.Config.AddressPrefix), cosmosclient.WithNodeAddress(nodeAddress))
	if err != nil {
		log.Fatal(err)
	}
	queryClient := types.NewQueryClient(client.Context())

	// Create a Cosmos account instance
	account, err := client.AccountRegistry.Import(uptAccountName, uptAccountMnemonic, uptAccountPassphrase)
	if err != nil {
		if err.Error() == "account already exists" {
			//TODO: Check how to use an existing account
			account, err = client.Account(uptAccountName)
		} 
		
		if err != nil {
			ap.Config.Logger.Error().Err(err).Msg("error getting account")
			log.Fatal(err)
		}
	}

	address, err := account.Address(ap.Config.AddressPrefix)
	if err != nil {
		return nil, err
	}

	return &AppChain{
		Ctx:            ctx,
		ReputerAddress: address,
		ReputerAccount: account,
		Client:         client,
		QueryClient:    queryClient,
		WorkersAddress: generateWorkersMap(),
	}, nil
}

func registerWithBlockchain(ctx context.Context, client cosmosclient.Client, account cosmosaccount.Account, config AppChainConfig) {
	
	address, err := account.Address(config.AddressPrefix)
    if err != nil {
		config.Logger.Fatal().Err(err).Msg("could not retrieve address for the allora blockchain")
    }

	msg := &types.MsgRegisterWorker{
		Creator: address,
	}

	txResp, err := client.BroadcastTx(ctx, account, msg)
    if err != nil {
        config.Logger.Fatal().Err(err).Msg("could not register the node with the allora blockchain")
    }

	config.Logger.Info().Str("txhash", txResp.TxHash).Msg("successfully registered node with Allora blockchain")
}

// query NodeId in the InferenceNode type of the Cosmos chain
func queryIsNodeRegistered(ctx context.Context, client cosmosclient.Client, address string, config AppChainConfig) bool {
	// queryClient := types.NewQueryClient(client.Context())
    // queryResp, err := queryClient.GetInferenceNodeRegistration(ctx, &types.QueryRegisteredInferenceNodesRequest{
	// 	NodeId: address + config.StringSeperator + config.LibP2PKey,
	// })

    // if err != nil {
    //     config.Logger.Fatal().Err(err).Msg("node could not be registered with blockchain")
    // }
	// (len(queryResp.Nodes) >= 1) 
	return false
}

func (ap *AppChain) SendInferences(topicId uint64, results aggregate.Results) []WorkerInference {
	// Aggregate the inferences from all peers/workers
	var inferences []*types.Inference
	var workersInferences []WorkerInference
	for _, result := range results {
		for _, peer := range result.Peers {
			ap.Config.Logger.Info().Any("peer", peer)
			value, err := extractNumber(result.Result.Stdout)
			if err != nil || value == "" {
				ap.Config.Logger.Fatal().Err(err).Msg("error extracting number from stdout")
				value = "0"
			}
			parsed, err := parseFloatToUint64(value)
			if err != nil {
				ap.Config.Logger.Fatal().Err(err).Msg("error parsing uint")
			}
			inference := &types.Inference{
				TopicId: topicId,
				Worker:  "upt16ar7k93c6razqcuvxdauzdlaz352sfjp2rpj3i",
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

	txResp, err := ap.Client.BroadcastTx(ap.Ctx, ap.ReputerAccount, req)
	if err != nil {
		ap.Config.Logger.Fatal().Err(err).Msg("failed to send inferences to allora blockchain")
	}

	ap.Config.Logger.Info().Any("txResp:", txResp).Msg("sent inferences to allora blockchain")

	return workersInferences
}

func (ap *AppChain) SendUpdatedWeights(topicId uint64, results aggregate.Results) {

	weights := make([]*types.Weight, 0)
	for _, result := range results {
		extractedWeights, err := extractWeights(result.Result.Stdout)
		if err != nil {
			ap.Config.Logger.Error().Err(err).Msg("Error extracting weight")
			continue
		}

		for peer, value := range extractedWeights {
			ap.Config.Logger.Info().Str("peer", peer);
			parsed, err := parseFloatToUint64Weights(strconv.FormatFloat(value, 'f', -1, 64))
			if err != nil {
				ap.Config.Logger.Error().Err(err).Msg("Error parsing uint")
				continue
			}
			weight := &types.Weight{
				TopicId: topicId,
				Reputer: ap.ReputerAddress,
				Worker:  "upt16ar7k93c6razqcuvxdauzdlaz352sfjp2rpj3i", // Assuming the peer string matches the worker identifiers
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

	txResp, err := ap.Client.BroadcastTx(ap.Ctx, ap.ReputerAccount, req)
	if err != nil {
		ap.Config.Logger.Fatal().Err(err).Msg("could not send weights to the allora blockchain")
	}

	ap.Config.Logger.Info().Str("txResp:", txResp.TxHash).Msg("weights sent to allora blockchain")
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

func extractWeights(stdout string) (map[string]float64, error) {
	fmt.Println("Extracting weights from stdout: ", stdout)

	var weights WorkerWeights
	err := json.Unmarshal([]byte(stdout), &weights.Weights)
	if err != nil {
		return nil, err
	}

	return weights.Weights, nil
}

func generateWorkersMap() map[string]string {
	// TODO: Add query to get all workers from AppChain
	workerMap := make(map[string]string)

	peer1Address := os.Getenv("PEER_ADDRESS_1")
	if peer1Address == "" {
		peer1Address = "2xgSimWsrD59sW3fPxLo3ej2Q6dFNc6DRsWH5stnHB3bkaVTsHZjKDULEL"
	}
	peer2Address := os.Getenv("PEER_ADDRESS_2")
	if peer2Address == "" {
		peer2Address = "2xgSimWsrD59sW3fPxLo3ej2Q6dFNc6DRsWH5stnHB3bkaVTsHZjKDULEA"
	}
	worker1Address := os.Getenv("WORKER_ADDRESS_1")
	if worker1Address == "" {
		worker1Address = "upt16ar7k93c6razqcuvxdauzdlaz352sfjp2rpj3i"
	}
	worker2Address := os.Getenv("WORKER_ADDRESS_2")
	if worker2Address == "" {
		worker2Address = "upt16ar7k93c6razqcuvxdauzdlaz352sfjp2rpj3a"
	}

	workerMap[peer1Address] = worker1Address
	workerMap[peer2Address] = worker2Address

	return workerMap
}