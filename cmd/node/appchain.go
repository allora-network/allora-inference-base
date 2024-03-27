package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	cosmossdk_io_math "cosmossdk.io/math"
	types "github.com/allora-network/allora-chain/x/emissions"
	"github.com/allora-network/b7s/models/blockless"
	"github.com/allora-network/b7s/node/aggregate"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosaccount"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosclient"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func getAlloraClient(config AppChainConfig) (*cosmosclient.Client, error) {
	// create a allora client instance
	ctx := context.Background()
	userHomeDir, _ := os.UserHomeDir()
	alloraClientHome := filepath.Join(userHomeDir, ".allorad")
	if config.AlloraHomeDir != "" {
		alloraClientHome = config.AlloraHomeDir
	}

	// Check that the given home folder exist
	if _, err := os.Stat(alloraClientHome); errors.Is(err, os.ErrNotExist) {
		log.Warn().Err(err).Msg("could not get home directory for allora client, creating...")
		err = os.MkdirAll(alloraClientHome, 0755)
		if err != nil {
			log.Warn().Err(err).Str("directory", alloraClientHome).Msg("Cannot create allora client home directory")
			config.SubmitTx = false
			return nil, err
		}
		log.Info().Err(err).Str("directory", alloraClientHome).Msg("allora client home directory created")
	}

	client, err := cosmosclient.New(ctx, cosmosclient.WithNodeAddress(config.NodeRPCAddress), cosmosclient.WithAddressPrefix(config.AddressPrefix), cosmosclient.WithHome(alloraClientHome))
	if err != nil {
		log.Warn().Err(err).Msg("unable to create an allora blockchain client")
		config.SubmitTx = false
		return nil, err
	}
	return &client, nil
}

// create a new appchain client that we can use
func NewAppChain(config AppChainConfig, log zerolog.Logger) (*AppChain, error) {
	config.SubmitTx = true
	client, err := getAlloraClient(config)
	if err != nil {
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
		log.Warn().Msg("no allora account was loaded")
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
		Client:         client,
		QueryClient:    queryClient,
		Config:         config,
	}

	registerWithBlockchain(appchain)

	return appchain, nil
}

// / Registration
func registerWithBlockchain(appchain *AppChain) {
	ctx := context.Background()

	isReputer := false
	if appchain.Config.NodeRole == blockless.HeadNode {
		isReputer = true
	}
	appchain.Logger.Info().Bool("isReputer", isReputer).Msg("Node mode")

	// Check if address is already registered in a topic
	res, err := appchain.QueryClient.GetRegisteredTopicIds(ctx, &types.QueryRegisteredTopicIdsRequest{
		Address:   appchain.ReputerAddress,
		IsReputer: isReputer,
	})
	if err != nil {
		appchain.Logger.Fatal().Err(err).Msg("could not check if the node is already registered. Topic not created?")
	}
	var msg sdktypes.Msg
	appchain.Logger.Info().Str("ReputerAddress", appchain.ReputerAddress).Msg("Current Address")
	if len(res.TopicIds) > 0 {
		appchain.Logger.Debug().Msg("Worker already registered for some topics, checking...")
		var topicsToRegister []uint64
		var topicsToDeRegister []uint64
		// Calculate topics to deregister
		for _, topicUint64 := range res.TopicIds {
			topicIdStr := strconv.FormatUint(uint64(topicUint64), 10)
			if !slices.Contains(appchain.Config.TopicIds, topicIdStr) {
				appchain.Logger.Info().Str("topic", topicIdStr).Msg("marking deregistration for topic")
				topicsToDeRegister = append(topicsToDeRegister, topicUint64)
			} else {
				appchain.Logger.Info().Str("topic", topicIdStr).Msg("Not deregistering topic")
			}
		}
		// Calculate topics to register
		for _, topicIdStr := range appchain.Config.TopicIds {
			topicUint64, err := strconv.ParseUint(topicIdStr, 10, 64)
			if err != nil {
				appchain.Logger.Info().Err(err).Uint64("topic", topicUint64).Msg("Could not register for topic, not numerical")
				continue
			}
			if !slices.Contains(res.TopicIds, topicUint64) {
				appchain.Logger.Info().Uint64("topic", topicUint64).Msg("marking registration for topic")
				topicsToRegister = append(topicsToRegister, topicUint64)
			} else {
				appchain.Logger.Info().Str("topic", topicIdStr).Msg("Topic is already registered, no registration for topic")
			}
		}
		// Registration on new topics
		for _, topicId := range topicsToRegister {
			if err != nil {
				appchain.Logger.Info().Err(err).Uint64("topic", topicId).Msg("Could not register for topic")
				break
			}
			msg = &types.MsgAddNewRegistration{
				Creator:      appchain.ReputerAddress,
				LibP2PKey:    appchain.Config.LibP2PKey,
				MultiAddress: appchain.Config.MultiAddress,
				TopicId:      topicId,
				Owner:        appchain.ReputerAddress,
				IsReputer:    isReputer,
			}

			txResp, err := appchain.Client.BroadcastTx(ctx, appchain.ReputerAccount, msg)
			if err != nil {
				appchain.Logger.Fatal().Err(err).Uint64("topic", topicId).Msg("could not register the node with the Allora blockchain in topic")
			} else {
				appchain.Logger.Info().Str("txhash", txResp.TxHash).Uint64("topic", topicId).Msg("successfully registered node with Allora blockchain in topic")
			}
		}
		// Deregistration on old topics
		for _, topicId := range topicsToDeRegister {
			if err != nil {
				appchain.Logger.Info().Err(err).Uint64("topic", topicId).Msg("Could not register for topic")
				break
			}
			msg = &types.MsgRemoveRegistration{
				Creator:   appchain.ReputerAddress,
				TopicId:   topicId,
				IsReputer: isReputer,
			}

			txResp, err := appchain.Client.BroadcastTx(ctx, appchain.ReputerAccount, msg)
			if err != nil {
				appchain.Logger.Fatal().Err(err).Uint64("topic", topicId).Msg("could not deregister the node with the Allora blockchain in topic")
			} else {
				appchain.Logger.Info().Str("txhash", txResp.TxHash).Uint64("topic", topicId).Msg("successfully deregistered node with Allora blockchain in topic")
			}
		}
	} else {
		appchain.Logger.Debug().Msg("Attempting first registration for this node")
		// First registration: Check current balance of the account
		pageRequest := &query.PageRequest{
			Limit:  100,
			Offset: 0,
		}
		// Check balance is over initial stake configured
		balanceRes, err := appchain.Client.BankBalances(ctx, appchain.ReputerAddress, pageRequest)
		if err != nil {
			appchain.Logger.Error().Err(err).Msg("could not get account balance - is account funded?")
			return
		} else {
			if len(balanceRes) > 0 {
				// Get uallo balance
				var ualloBalance uint64
				for _, coin := range balanceRes {
					if coin.Denom == "uallo" {
						// Found the balance in "uallo"
						ualloBalance = coin.Amount.Uint64()
						break
					}
				}
				if ualloBalance >= appchain.Config.InitialStake {
					var topicsToRegister []uint64
					for _, topicToRegister := range appchain.Config.TopicIds {
						topicToRegisterUint64, err := strconv.ParseUint(topicToRegister, 10, 64)
						if err != nil {
							appchain.Logger.Info().Err(err).Str("topic", topicToRegister).Msg("Could not register for topic, not numerical, skipping")
						} else {
							topicsToRegister = append(topicsToRegister, topicToRegisterUint64)
						}
					}
					// If not registered in any topic, need an initial stake
					msg = &types.MsgRegister{
						Creator:      appchain.ReputerAddress,
						LibP2PKey:    appchain.Config.LibP2PKey,
						MultiAddress: appchain.Config.MultiAddress,
						InitialStake: cosmossdk_io_math.NewUint(appchain.Config.InitialStake),
						TopicIds:     topicsToRegister,
						Owner:        appchain.ReputerAddress,
						IsReputer:    isReputer,
					}
					txResp, err := appchain.Client.BroadcastTx(ctx, appchain.ReputerAccount, msg)
					if err != nil {
						appchain.Logger.Fatal().Err(err).Msg("could not register the node with the Allora blockchain in specified topics")
					} else {
						appchain.Logger.Info().Str("txhash", txResp.TxHash).Msg("successfully registered node with Allora blockchain")
					}
					appchain.Logger.Info().Str("balance", balanceRes.String()).Msg("Registered Node")
				} else {
					appchain.Logger.Fatal().Msg("account balance is lower than the initialStake requested")
				}
			} else {
				appchain.Logger.Info().Str("account", appchain.ReputerAddress).Msg("account is not funded in uallo")
				return
			}
		}
	}
}

// Retry function with a constant number of retries.
func (ap *AppChain) SendInferencesWithRetry(ctx context.Context, req *types.MsgProcessInferences, MaxRetries, MinDelay, MaxDelay int) (*cosmosclient.Response, error) {
	var txResp *cosmosclient.Response
	var err error

	for retryCount := 0; retryCount <= MaxRetries; retryCount++ {
		txResp, err := ap.Client.BroadcastTx(ctx, ap.ReputerAccount, req)
		if err == nil {
			ap.Logger.Info().Str("Tx Hash:", txResp.TxHash).Msg("successfully sent inferences to allora blockchain")
			break
		}
		// Log the error for each retry.
		ap.Logger.Info().Err(err).Msgf("Failed to send inferences to allora blockchain, retrying... (Retry %d/%d)", retryCount, MaxRetries)
		// Generate a random number between MinDelay and MaxDelay
		randomDelay := rand.Intn(MaxDelay-MinDelay+1) + MinDelay
		// Apply exponential backoff to the random delay
		backoffDelay := randomDelay << retryCount
		// Wait for the calculated delay before retrying
		time.Sleep(time.Duration(backoffDelay) * time.Second)
	}
	return txResp, err
}

// Retry function with a constant number of retries.
func (ap *AppChain) SendWeightsWithRetry(ctx context.Context, req *types.MsgSetWeights, MaxRetries, MinDelay, MaxDelay int) (*cosmosclient.Response, error) {
	var txResp *cosmosclient.Response
	var err error

	for retryCount := 0; retryCount <= MaxRetries; retryCount++ {
		txResp, err := ap.Client.BroadcastTx(ctx, ap.ReputerAccount, req)
		if err == nil {
			ap.Logger.Info().Any("Tx Hash:", txResp.TxHash).Msg("successfully sent weights to allora blockchain")
			break
		}
		// Log the error for each retry.
		ap.Logger.Info().Err(err).Msgf("Failed to send weights to allora blockchain, retrying... (Retry %d/%d)", retryCount, MaxRetries)
		// Generate a random number between MinDelay and MaxDelay
		randomDelay := rand.Intn(MaxDelay-MinDelay+1) + MinDelay
		// Apply exponential backoff to the random delay
		backoffDelay := randomDelay << retryCount
		// Wait for the calculated delay before retrying
		time.Sleep(time.Duration(backoffDelay) * time.Second)
	}
	return txResp, err
}

// Sending Inferences to the AppChain
func (ap *AppChain) SendInferences(ctx context.Context, topicId uint64, results aggregate.Results) {
	// Aggregate the inferences from all peers/workers
	var inferences []*types.Inference

	for _, result := range results {
		for _, peer := range result.Peers {
			ap.Logger.Debug().Any("peer", peer)

			// Get Peer $allo address
			res, err := ap.QueryClient.GetWorkerAddressByP2PKey(ctx, &types.QueryWorkerAddressByP2PKeyRequest{
				Libp2PKey: peer.String(),
			})
			if err != nil {
				ap.Logger.Warn().Err(err).Str("peer", peer.String()).Msg("error getting peer address from chain, worker not registered? Ignoring peer.")
				continue
			}

			value, err := checkJSONValueError(result.Result.Stdout)
			if err != nil || value == "" {
				ap.Logger.Warn().Err(err).Str("peer", peer.String()).Msg("error extracting value as number from stdout, ignoring inference.")
				continue
			}
			parsed, err := parseFloatToUint64(value)
			if err != nil {
				ap.Logger.Warn().Err(err).Str("peer", peer.String()).Str("value", value).Msg("error parsing inference as uint")
				continue
			}
			inference := &types.Inference{
				TopicId: topicId,
				Worker:  res.Address,
				Value:   cosmossdk_io_math.NewUint(parsed),
			}
			inferences = append(inferences, inference)
		}
	}

	req := &types.MsgProcessInferences{
		Sender:     ap.ReputerAddress,
		Inferences: inferences,
	}

	ap.SendInferencesWithRetry(ctx, req, 5, 0, 2)
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

	ap.SendWeightsWithRetry(ctx, req, 5, 0, 2)
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

func checkJSONValueError(stdout string) (string, error) {
	var response Response
	err := json.Unmarshal([]byte(stdout), &response)
	if err != nil {
		return "Unable to unmarshall", err
	}

	if response.Value != "" {
		return response.Value, nil
	} else if response.Error != "" {
		return "", errors.New("error found: " + response.Error)
	} else {
		return "", errors.New("no Error or Value field found in response")
	}
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
