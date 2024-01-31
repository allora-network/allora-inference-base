package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"

	cosmossdk_io_math "cosmossdk.io/math"
	"github.com/blocklessnetwork/b7s/node/aggregate"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosaccount"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosclient"
	"github.com/rs/zerolog"
	types "github.com/upshot-tech/protocol-state-machine-module"
)

// create a new appchain client that we can use
func NewAppChain(config AppChainConfig, log zerolog.Logger) (*AppChain, error){
	ctx := context.Background()

	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		log.Warn().Err(err).Msg("could not get home directory for app chain")
		return nil, err
	}

	DefaultNodeHome := filepath.Join(userHomeDir, ".uptd")

	// create a cosmos client instance
	client, err := cosmosclient.New(ctx, cosmosclient.WithAddressPrefix(config.AddressPrefix), cosmosclient.WithHome(DefaultNodeHome))
	if err != nil {
		log.Warn().Err(err).Msg("werr")
		config.SubmitTx = true
	}

	var account cosmosaccount.Account
	var address string

	// if we're giving a keyring ring name, with no mnemonic restore
	if(config.AddressRestoreMnemonic == "" && config.AddressKeyName != "") {
		// get account from the keyring
		account, err = client.Account(config.AddressKeyName)
		if err != nil {
			config.SubmitTx = false
			log.Warn().Err(err).Msg("could not retrieve account from keyring")
		} else {

			address, err = account.Address(config.AddressPrefix)
			if err != nil {
				config.SubmitTx = false
				log.Warn().Err(err).Msg("could not retrieve allora blockchain address, transactions will not be submitted to chain")
			}
		}
	}	

	// Create query client
	queryClient := types.NewQueryClient(client.Context())

	// this is terrible, no isConnected as part of this code path
	if(len(client.Context().ChainID) < 1) {
		return nil, nil
	}

	appchain := &AppChain{
		ReputerAddress: address,
		ReputerAccount: account,
		Logger: log,
		Client: client,
		QueryClient: queryClient,
		Config: config,
	}

	queryIsNodeRegistered(*appchain)

	return appchain, nil
}


/// Registration 
func registerWithBlockchain(appchain AppChain) {
	ctx := context.Background()

	msg := &types.MsgRegisterWorker{
		Creator: appchain.ReputerAddress,
	}

	txResp, err := appchain.Client.BroadcastTx(ctx, appchain.ReputerAccount, msg)
    if err != nil {
        appchain.Logger.Fatal().Err(err).Msg("could not register the node with the allora blockchain")
    }

	appchain.Logger.Info().Str("txhash", txResp.TxHash).Msg("successfully registered node with Allora blockchain")
}


func queryIsNodeRegistered(appchain AppChain) bool {
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


/// Sending Inferences to the AppChain


func (ap *AppChain) SendInferences(ctx context.Context, topicId uint64, results aggregate.Results) []WorkerInference {
	// Aggregate the inferences from all peers/workers
	var inferences []*types.Inference
	var workersInferences []WorkerInference
	for _, result := range results {
		for _, peer := range result.Peers {
			ap.Logger.Info().Any("peer", peer)
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

	txResp, err := ap.Client.BroadcastTx(ctx, ap.ReputerAccount, req)
	if err != nil {
		ap.Logger.Fatal().Err(err).Msg("failed to send inferences to allora blockchain")
	}

	ap.Logger.Info().Any("txResp:", txResp).Msg("sent inferences to allora blockchain")

	return workersInferences
}

func (ap *AppChain) SendUpdatedWeights(ctx context.Context, topicId uint64, results aggregate.Results) {

	weights := make([]*types.Weight, 0)
	for _, result := range results {
		extractedWeights, err := extractWeights(result.Result.Stdout)
		if err != nil {
			ap.Logger.Error().Err(err).Msg("Error extracting weight")
			continue
		}

		for peer, value := range extractedWeights {
			ap.Logger.Info().Str("peer", peer);
			parsed, err := parseFloatToUint64Weights(strconv.FormatFloat(value, 'f', -1, 64))
			if err != nil {
				ap.Logger.Error().Err(err).Msg("Error parsing uint")
				continue
			}
			weight := &types.Weight{
				TopicId: topicId,
				Reputer: ap.ReputerAddress,
				Worker:  "upt16ar7k93c6razqcuvxdauzdlaz352sfjp2rpj3i",
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
		ap.Logger.Fatal().Err(err).Msg("could not send weights to the allora blockchain")
	}

	ap.Logger.Info().Str("txResp:", txResp.TxHash).Msg("weights sent to allora blockchain")
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