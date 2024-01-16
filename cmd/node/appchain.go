package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/blocklessnetwork/b7s/node/aggregate"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosaccount"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosclient"
	types "github.com/upshot-tech/protocol-state-machine-module"
)

const ADDRESS_PREFIX = "upt"
const ADDRESS_NAME = "alice"
const HOME_DIRECTORY = ".uptd"
const STRING_SEPERATOR = "|"

func getAccount(accountName string, client cosmosclient.Client) cosmosaccount.Account  {
    account, err := client.Account(accountName)
    if err != nil {
        log.Fatal(err)
    }
	return account
}

func getAddress(addressPrefix string, account cosmosaccount.Account) string {
	addr, err := account.Address(addressPrefix)
    if err != nil {
        log.Fatal(err)
    }
	return addr
}

func getClient(ctx context.Context, addressPrefix string) cosmosclient.Client {
    // Create a Cosmos client instance
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	DefaultNodeHome := filepath.Join(userHomeDir, HOME_DIRECTORY)
    client, err := cosmosclient.New(ctx, cosmosclient.WithAddressPrefix(addressPrefix), cosmosclient.WithHome(DefaultNodeHome))
    if err != nil {
        log.Fatal(err)
    }
	return client
}

func registerNodeWithL1(ctx context.Context, client cosmosclient.Client, account cosmosaccount.Account, libp2pkey string) {
	msg := &types.MsgRegisterInferenceNode{
		Sender: getAddress(ADDRESS_PREFIX, account),
		LibP2PKey: libp2pkey,
	}

	txResp, err := client.BroadcastTx(ctx, account, msg)
    if err != nil {
        log.Fatal(err)
    }

	log.Printf(txResp.TxHash)
}

func queryIsNodeRegistered(ctx context.Context, client cosmosclient.Client, address string, libp2pkey string) bool {
	queryClient := types.NewQueryClient(client.Context())
    queryResp, err := queryClient.GetInferenceNodeRegistration(ctx, &types.QueryRegisteredInferenceNodesRequest{
		NodeId: address + STRING_SEPERATOR + libp2pkey,
	})

    if err != nil {
        log.Fatal(err)
    }

	return (len(queryResp.Nodes) >= 1)
}

func startClient(ctx context.Context, libp2pkey string) {
    client := getClient(ctx, ADDRESS_PREFIX)
	account := getAccount(ADDRESS_NAME, client)
	address := getAddress(ADDRESS_PREFIX, account)
	if (!queryIsNodeRegistered(ctx, client, address, libp2pkey)) {
		// not registered, register the node
		registerNodeWithL1(ctx, client, account, libp2pkey)
	}
}

type AppChain struct {
	Ctx            context.Context
	ReputerAddress string
	ReputerAccount cosmosaccount.Account
	Client         cosmosclient.Client
	QueryClient    types.QueryClient
	WorkersAddress map[string]string
}

type WorkerInference struct {
	Worker    string `json:"worker"`
	Inference uint64 `json:"inference"`
}

func NewAppChainClient() (*AppChain, error) {
	ctx := context.Background()
	addressPrefix := "upt"

	nodeAddress := os.Getenv("NODE_ADDRESS")
	if nodeAddress == "" {
		return nil, fmt.Errorf("NODE_ADDRESS environment variable is not set")
	}
	uptAccountMnemonic := os.Getenv("UPT_ACCOUNT_MNEMONIC")
	if uptAccountMnemonic == "" {
		return nil, fmt.Errorf("UPT_ACCOUNT_MNEMONIC environment variable is not set")
	}
	uptAccountName := os.Getenv("UPT_ACCOUNT_NAME")
	if uptAccountName == "" {
		return nil, fmt.Errorf("UPT_ACCOUNT_NAME environment variable is not set")
	}
	// Passpharase is optional
	uptAccountPassphrase := os.Getenv("UPT_ACCOUNT_PASSPHRASE")

	client, err := cosmosclient.New(ctx, cosmosclient.WithAddressPrefix(addressPrefix), cosmosclient.WithNodeAddress(nodeAddress))
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
			fmt.Println("Error getting account: ", err)
			log.Fatal(err)
		}
	}

	address, err := account.Address(addressPrefix)
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

func (ap *AppChain) SendInferencesToAppChain(topicId uint64, results aggregate.Results) []WorkerInference {
	// Aggregate the inferences from all peers/workers
	var inferences []*types.Inference
	var workersInferences []WorkerInference
	for _, result := range results {
		for _, peer := range result.Peers {
			fmt.Println("Peer: ", peer)
			value, err := extractNumber(result.Result.Stdout)
			if err != nil || value == "" {
				fmt.Println("Error extracting number from stdout: ", err)
				value = "0" // TODO: Check what to do in this situation
			}
			parsed, err := parseFloatToUint64(value)
			if err != nil {
				fmt.Println("Error parsing uint: ", err)
			}
			inference := &types.Inference{
				TopicId: topicId,
				Worker:  "upt16ar7k93c6razqcuvxdauzdlaz352sfjp2rpj3i",
				Value:   parsed, // TODO: Check later - change the format to string
			}
			inferences = append(inferences, inference)
			workersInferences = append(workersInferences, WorkerInference{Worker: inference.Worker, Inference: inference.Value})
		}
	}

	req := &types.MsgSetInferences{
		Sender:     ap.ReputerAddress,
		Inferences: inferences,
	}

	txResp, err := ap.Client.BroadcastTx(ap.Ctx, ap.ReputerAccount, req)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("txResp:", txResp)

	return workersInferences
}

type WeightsCalcDependencies struct {
	LatestWeights map[string]float64
	ActualPrice   float64
}

// Process the inferences and start the weight calculation
func (ap *AppChain) GetWeightsCalcDependencies(workersInferences []WorkerInference) (float64, map[string]float64) {
	// Get lastest weight of each peer/worker
	var workerLatestWeights map[string]float64 = make(map[string]float64)
	for _, p := range workersInferences {
		req := &types.QueryWeightRequest{
			TopicId: 1,
			Reputer: ap.ReputerAddress,
			Worker:  p.Worker,
		}

		weight, err := ap.QueryClient.GetWeight(ap.Ctx, req)
		if err != nil {
			weight = &types.QueryWeightResponse{
				Amount: 0, // TODO: Check what to do in this situation
			}
		}

		workerLatestWeights[p.Worker] = float64(weight.Amount) / 100000.0 // TODO: Change
	}

	// Get actual ETH price
	ethPrice, err := getEthereumPrice()
	if err != nil {
		log.Fatal(err)
	}

	return ethPrice, workerLatestWeights
}

func (ap *AppChain) SendUpdatedWeights(results aggregate.Results) {

	weights := make([]*types.Weight, 0)
	for _, result := range results {
		extractedWeights, err := extractWeights(result.Result.Stdout)
		if err != nil {
			fmt.Println("Error extracting weights: ", err)
			continue
		}

		for peer, value := range extractedWeights {
			fmt.Println("Peer: ", peer)
			parsed, err := parseFloatToUint64Weights(strconv.FormatFloat(value, 'f', -1, 64))
			if err != nil {
				fmt.Println("Error parsing uint: ", err)
				continue
			}
			weight := &types.Weight{
				TopicId: 1,
				Reputer: ap.ReputerAddress,
				Worker:  "upt16ar7k93c6razqcuvxdauzdlaz352sfjp2rpj3i", // Assuming the peer string matches the worker identifiers
				Weight:  parsed,
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
		log.Fatal(err)
	}
	fmt.Println("txResp:", txResp)
}

// EthereumPriceResponse represents the JSON structure returned by CoinGecko API
type EthereumPriceResponse struct {
	Ethereum map[string]float64 `json:"ethereum"`
}

func getEthereumPrice() (float64, error) {
	url := "https://api.coingecko.com/api/v3/simple/price?ids=ethereum&vs_currencies=usd"
	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var result EthereumPriceResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	return result.Ethereum["usd"], nil
}

// Define a struct that matches the JSON structure of your stdout
type StdoutData struct {
	Value string `json:"value"`
}

type Response struct {
	Value string `json:"value"`
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

type WeightsResponse struct {
	Value string `json:"value"`
}

type WorkerWeights struct {
	Weights map[string]float64 `json:"-"` // Use a map to dynamically handle worker identifiers
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
func startAppChainClient(ctx context.Context, libp2pkey string) {
	go startClient(ctx, libp2pkey)
}
