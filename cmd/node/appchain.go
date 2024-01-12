package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"

	"github.com/blocklessnetwork/b7s/node/aggregate"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosaccount"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosclient"
	types "github.com/upshot-tech/protocol-state-machine-module"
)

func registerNode(ctx context.Context) {

}

func startClient() {
	ctx := context.Background()
	addressPrefix := "cosmos"

	// Create a Cosmos client instance
	_, err := cosmosclient.New(ctx, cosmosclient.WithAddressPrefix(addressPrefix))
	if err != nil {
		log.Fatal(err)
	}
}

func Start(ctx context.Context) {
	registerNode(ctx)
	go startClient()
}

type AppChain struct {
	Ctx            context.Context
	ReputerAddress string
	ReputerAccount cosmosaccount.Account
	Client         cosmosclient.Client
	QueryClient    types.QueryClient
	WorkersAddress map[string]string
}

type WorkerPrediction struct {
	Worker     string
	Prediction uint64
}

func NewAppChainClient() *AppChain {
	ctx := context.Background()
	addressPrefix := "upt"

	// Path to appchain home directory (e.g. ~/.upt)
	homePath := os.Getenv("HOME_PATH")
    if homePath == "" {
        log.Fatal("HOME_PATH environment variable is not set")
    }

	client, err := cosmosclient.New(ctx, cosmosclient.WithAddressPrefix(addressPrefix), cosmosclient.WithHome(homePath))
	if err != nil {
		log.Fatal(err)
	}
	queryClient := types.NewQueryClient(client.Context())

	// Using a fixed account for now (which is one of the wallet in genesis file)
	account, err := client.Account("alice")
	if err != nil {
		log.Fatal(err)
	}

	address, err := account.Address(addressPrefix)
	if err != nil {
		log.Fatal(err)
	}

	return &AppChain{
		Ctx:            ctx,
		ReputerAddress: address,
		ReputerAccount: account,
		Client:         client,
		QueryClient:    queryClient,
		WorkersAddress: generateWorkersMap(),
	}
}

func (ap *AppChain) SendPredictionsToAppChain(topicId uint64, results aggregate.Results) {
	// Aggregate the predictions from all peers/workers
	var predictions []*types.Prediction
	var workersPredictions []WorkerPrediction
	for _, result := range results {
		for _, peer := range result.Peers {
			value := extractNumber(result.Result.Stdout)
			prediction := &types.Prediction{
				TopicId: topicId,
				Worker:  ap.WorkersAddress[peer.String()],
				Value:   uint64(value),
			}
			predictions = append(predictions, prediction)
			workersPredictions = append(workersPredictions, WorkerPrediction{Worker: prediction.Worker, Prediction: prediction.Value})
		}
	}

	req := &types.MsgSetPredictions{
		Sender:      ap.ReputerAddress,
		Predictions: predictions,
	}

	txResp, err := ap.Client.BroadcastTx(ap.Ctx, ap.ReputerAccount, req)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("txResp:", txResp)

	ap.ProcessPredictions(workersPredictions)
}

// Process the predictions and start the weight calculation
func (ap *AppChain) ProcessPredictions(workersPredictions []WorkerPrediction) {
	// Get lastest weight of each peer/worker
	var workerLatestWeights map[string]float64 = make(map[string]float64)
	for _, p := range workersPredictions {
		req := &types.QueryWeightRequest{
			TopicId: 1,
			Reputer: ap.ReputerAddress,
			Worker:  p.Worker,
		}

		weight, err := ap.QueryClient.GetWeight(ap.Ctx, req)
		if err != nil {
			log.Fatal(err)
		}

		workerLatestWeights[p.Worker] = float64(weight.Amount)
	}

	// Get actual ETH price
	ethPrice, err := getEthereumPrice()
	if err != nil {
		log.Fatal(err)
	}

	// Calculate the loss for each worker
	losses := make(map[string]float64)
	var scores []float64
	for _, prediction := range workersPredictions {
		predictedPrice, _ := strconv.ParseFloat(fmt.Sprint(prediction.Prediction), 64)
		loss := math.Abs(ethPrice - predictedPrice)
		losses[prediction.Worker] = loss
		scores = append(scores, loss)
	}

	// Scale scores
	minScore, maxScore := scores[0], scores[0]
	for _, score := range scores {
		if score < minScore {
			minScore = score
		}
		if score > maxScore {
			maxScore = score
		}
	}

	for id, score := range losses {
		losses[id] = 1 - ((score - minScore) / (maxScore - minScore))
	}

	alpha := 0.5 // TODO: Check if this is the correct value
	updatedWeights := updateWeightsWithEma(losses, workerLatestWeights, alpha)

	// Send updated weights to AppChain
	weights := make([]*types.Weight, 0)
	for worker, value := range updatedWeights {
		weight := &types.Weight{
			TopicId: 1,
			Reputer: ap.ReputerAddress,
			Worker:  worker,
			Weight:  uint64(value),
		}
		weights = append(weights, weight)
	}

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

func basicEMA(currentValue, previousEMA, alpha float64) float64 {
	return alpha*currentValue + (1-alpha)*previousEMA
}

func updateWeightsWithEma(normalizedScores map[string]float64, existingWeights map[string]float64, alpha float64) map[string]float64 {
	updatedWeights := make(map[string]float64)
	for id, normalizedScore := range normalizedScores {
		if weight, ok := existingWeights[id]; ok {
			updatedWeights[id] = basicEMA(normalizedScore, weight, alpha)
		} else {
			updatedWeights[id] = normalizedScore
		}
	}
	return updatedWeights
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

// extractNumber extracts the number from the given stdout string.
func extractNumber(stdout string) int {
	var value int
	// Assuming the random number is present after "number: " and ends before "\n"
	_, err := fmt.Sscanf(stdout, "number: %d", &value)
	if err != nil {
		fmt.Println("Error extracting number:", err)
	}
	return value
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
