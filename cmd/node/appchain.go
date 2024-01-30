package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	// "path/filepath"
	"strconv"

	cosmossdk_io_math "cosmossdk.io/math"

	"github.com/blocklessnetwork/b7s/node/aggregate"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosaccount"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosclient"
	"github.com/rs/zerolog"
	types "github.com/upshot-tech/protocol-state-machine-module"
)

func (ap *AppChain) init(ctx context.Context) {
	err := ap.startClient(ctx, ap.Config)
	if err !=nil {
		ap.Logger.Error().Err(err).Msg("error starting the allora blockchain client")
	}
}


func restoreAccount(ctx context.Context, ap AppChain) (cosmosaccount.Account, error) {

	var account cosmosaccount.Account
	uptAccountMnemonic := ap.Config.AddressRestoreMnemonic
	uptAccountName := ap.Config.AddressKeyName
	if uptAccountName == "" {
		return account, fmt.Errorf("allora-chain-key-name flag is not set")
	}

	// Passpharase is optional
	uptAccountPassphrase := ap.Config.AddressAccountPassphrase

	// Create a Cosmos account instance
	account, err := ap.Client.AccountRegistry.Import(uptAccountName, uptAccountMnemonic, uptAccountPassphrase)
	if err != nil {
		return account, err
	}

	return account, nil
}

func createClient(ctx context.Context, ap AppChain) (cosmosclient.Client, error) {
	var client cosmosclient.Client
	// userHomeDir, err := os.UserHomeDir()

	// if err != nil {
	// 	ap.Logger.Warn().Err(err).Msg("could not get home directory for app chain")
	// 	return client, err
	// }

	// DefaultNodeHome := filepath.Join(userHomeDir, ".uptd")
	client, err := cosmosclient.New(ctx, cosmosclient.WithAddressPrefix(ap.Config.AddressPrefix), cosmosclient.WithNodeAddress("http://localhost:26657"))
	if err != nil {
		fmt.Println(">>>>>>>>>>>>> client ERR : ", err)
		ap.Logger.Warn().Err(err).Msg("could not create allora blockchain client")
		return client, err
	}
	
	// this is terrible, no isConnected as part of this code path
	if(len(client.Context().ChainID) < 1) {
		return client, fmt.Errorf("client can not connect to allora blockchain")
	}
	ap.Client = client

	// fmt.Println(">>>>>>>>>>>>> client: ", client)

	// _, err = getAccount(&ap)
    // if err != nil {
	//  	ap.Config.SubmitTx = false
    //    	ap.Logger.Warn().Err(err).Msg("could not retrieve allora blockchain account, transactions will not be submitted to chain")
	// 	return client, nil
	// }

	return ap.Client, nil
}

func getAccount(ap *AppChain) (cosmosaccount.Account, error) {
	var account cosmosaccount.Account

	if(len(ap.Config.AddressRestoreMnemonic) > 0) {
		account, err := restoreAccount(context.Background(), *ap)
		if err != nil {
			ap.Config.SubmitTx = false
			  ap.Logger.Warn().Err(err).Msg("could not *restore* allora blockchain account, transactions will not be submitted to chain")
		   return account, nil
	   }
	} else {
		account, err := ap.Client.Account(ap.Config.AddressKeyName)
		if err != nil {
			ap.Config.SubmitTx = false
			  ap.Logger.Warn().Err(err).Msg("could not retrieve allora blockchain account, transactions will not be submitted to chain")
		   return account, err
	   }
	}

	return account, nil
}

func (ap *AppChain)  startClient(ctx context.Context, config AppChainConfig) error {

	client, err := createClient(ctx, *ap);
	if err != nil {
		return err
	}

	account, err := getAccount(ap)
    if err != nil {
	 	ap.Config.SubmitTx = false
       	ap.Logger.Warn().Err(err).Msg("could not retrieve allora blockchain account, transactions will not be submitted to chain")
		return err
	}

	address, err := account.Address(config.AddressPrefix)
    if err != nil {
		ap.Config.SubmitTx = false
        ap.Logger.Warn().Err(err).Msg("could not retrieve allora blockchain address, transactions will not be submitted to chain")
		return err
	}

	if(config.SubmitTx) {
		if (!queryIsNodeRegistered(ctx, client, address, config)) {
			// not registered, register the node
			registerWithBlockchain(ctx, client, account, *ap)
		}
		ap.Logger.Info().Msg("allora blockchain registration verification complete")
	} else {
		ap.Logger.Warn().Err(err).Msg("could not retrieve allora blockchain address, transactions will not be submitted to chain")
	}

	return nil
}

func NewAppChain(ctx context.Context, config AppChainConfig, logger zerolog.Logger) (*AppChain, error) {

	ap := &AppChain{
		Logger: logger,
		Config: config,
	}

	client, err := createClient(ctx, *ap);
	if err != nil {
		return ap, err
	}

	queryClient := types.NewQueryClient(client.Context())

	var account cosmosaccount.Account
	uptAccountMnemonic := ap.Config.AddressRestoreMnemonic
	uptAccountName := ap.Config.AddressKeyName
	if uptAccountName == "" {
		return ap, fmt.Errorf("allora-chain-key-name flag is not set")
	}

	// Passpharase is optional
	uptAccountPassphrase := ap.Config.AddressAccountPassphrase

	// Create a Cosmos account instance
	account, err = client.AccountRegistry.Import(uptAccountName, uptAccountMnemonic, uptAccountPassphrase)
	if err != nil {
		fmt.Println(">>>>>>", err, account)
		account, err = client.Account(uptAccountName)
		if err != nil {
			return ap, err
		}
	}
	
	address, err := account.Address(ap.Config.AddressPrefix)
	if err != nil {
		return nil, err
	}

	return &AppChain{
		ReputerAddress: address,
		ReputerAccount: account,
		Client:         client,
		QueryClient:    queryClient,
		WorkersAddress: generateWorkersMap(),
	}, nil
}

func registerWithBlockchain(ctx context.Context, client cosmosclient.Client, account cosmosaccount.Account, ap AppChain) {
	
	address, err := account.Address(ap.Config.AddressPrefix)
    if err != nil {
		ap.Logger.Fatal().Err(err).Msg("could not retrieve address for the allora blockchain")
    }

	msg := &types.MsgRegisterWorker{
		Creator: address,
	}

	txResp, err := client.BroadcastTx(ctx, account, msg)
    if err != nil {
        ap.Logger.Fatal().Err(err).Msg("could not register the node with the allora blockchain")
    }

	ap.Logger.Info().Str("txhash", txResp.TxHash).Msg("successfully registered node with Allora blockchain")
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

func (ap *AppChain) SendInferences(ctx context.Context, topicId uint64, results aggregate.Results) []WorkerInference {
	// Aggregate the inferences from all peers/workers
	var inferences []*types.Inference
	var workersInferences []WorkerInference
	count := 0
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
				Worker:  fmt.Sprintf("upt16ar7k93c6razqcuvxdauzdlaz352sfjp2rpj3%v", count),
				Value:   cosmossdk_io_math.NewUint(parsed),
				ExtraData: []byte("extra data"),
			}
			fmt.Println("Inference: ", inference)
			inferences = append(inferences, inference)
			workersInferences = append(workersInferences, WorkerInference{Worker: inference.Worker, Inference: inference.Value})
			count++
		}
	}
	
	req := &types.MsgProcessInferences{
		Sender:     ap.ReputerAddress,
		Inferences: inferences,
	}

	_, err := ap.Client.BroadcastTx(ctx, ap.ReputerAccount, req)
	if err != nil {
		// fmt.Println("Error sending inferences to chain: ", err)
		// ap.Logger.Fatal().Err(err).Msg("failed to send inferences to allora blockchain")
	}
	// fmt.Println("txResp: ", txResp)
	// ap.Logger.Info().Any("txResp:", txResp).Msg("sent inferences to allora blockchain")

	return workersInferences
}

func (ap *AppChain) SendUpdatedWeights(ctx context.Context, topicId uint64, results aggregate.Results) {
	fmt.Println("SendUpdatedWeights: ")
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

			fmt.Println("Weight: ", weight)
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