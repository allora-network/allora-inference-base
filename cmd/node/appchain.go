package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	cosmossdk_io_math "cosmossdk.io/math"
	alloraMath "github.com/allora-network/allora-chain/math"
	"github.com/allora-network/allora-chain/x/emissions/types"
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
		// restore from mnemonic
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
	} else {
		log.Info().Str("address", address).Msg("allora blockchain address loaded")
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

	if config.NodeRole == blockless.WorkerNode {
		registerWithBlockchain(appchain)
	} else {
		appchain.Logger.Info().Msg("Node is not a worker, not registering with blockchain")
	}
	return appchain, nil
}

// Function that receives an array of topicId as string, and parses them to uint64 extracting
// the topicId from the string prior to the "/" character.
func parseTopicIds(appchain *AppChain, topicIds []string) []uint64 {
	var b7sTopicIds []uint64
	for _, topicId := range topicIds {
		topicUint64, err := strconv.ParseUint(topicId, 10, 64)
		if err != nil {
			appchain.Logger.Warn().Err(err).Str("topic", topicId).Msg("Could not register for topic, not numerical")
			continue
		}
		b7sTopicIds = append(b7sTopicIds, topicUint64)
	}
	return b7sTopicIds
}

// / Registration
func registerWithBlockchain(appchain *AppChain) {
	ctx := context.Background()

	var isReputer bool
	if appchain.Config.WorkerMode == WorkerModeReputer {
		isReputer = true
	} else if appchain.Config.WorkerMode == WorkerModeWorker {
		isReputer = false
	} else {
		appchain.Logger.Fatal().Str("WorkerMode", appchain.Config.WorkerMode).Msg("Invalid Worker Mode")
	}
	appchain.Logger.Info().Bool("isReputer", isReputer).Msg("Node mode")

	// Parse topics into b7sTopicIds as numerical ids. Reputers and worker use different schema.
	b7sTopicIds := parseTopicIds(appchain, appchain.Config.TopicIds)
	// Print the array entries as a comma-separated value list
	topicsList := strings.Join(strings.Fields(fmt.Sprint(b7sTopicIds)), ", ")
	appchain.Logger.Info().Str("topicsList", topicsList).Msg("Topics list")

	appchain.Logger.Info().Str("Address", appchain.ReputerAddress).Msg("Node address")
	// Check if address is already registered in a topic, getting all topics already reg'd
	res, err := appchain.QueryClient.GetRegisteredTopicIds(ctx, &types.QueryRegisteredTopicIdsRequest{
		Address:   appchain.ReputerAddress,
		IsReputer: isReputer,
	})
	if err != nil {
		appchain.Logger.Error().Err(err).Msg("could not check if the node is already registered. Topic not created?")
		return
	}
	var msg sdktypes.Msg
	appchain.Logger.Info().Str("Worker", appchain.ReputerAddress).Msg("Current Address")
	if len(res.TopicIds) > 0 {
		appchain.Logger.Debug().Msg("Worker already registered for some topics, checking...")
		var topicsToRegister []uint64
		var topicsToDeRegister []uint64
		// Calculate topics to deregister
		for _, topicUint64 := range res.TopicIds {
			if !slices.Contains(b7sTopicIds, topicUint64) {
				appchain.Logger.Info().Uint64("topic", topicUint64).Msg("marking deregistration for topic")
				topicsToDeRegister = append(topicsToDeRegister, topicUint64)
			} else {
				appchain.Logger.Info().Uint64("topic", topicUint64).Msg("Not deregistering topic")
			}
		}
		// Calculate topics to register
		for _, topicUint64 := range b7sTopicIds {
			if !slices.Contains(res.TopicIds, topicUint64) {
				appchain.Logger.Info().Uint64("topic", topicUint64).Msg("marking registration for topic")
				topicsToRegister = append(topicsToRegister, topicUint64)
			} else {
				appchain.Logger.Info().Uint64("topic", topicUint64).Msg("Topic is already registered, no registration for topic")
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
						appchain.Logger.Info().Str("balance", coin.Amount.BigInt().Text(10)).Msg("Found uallo balance in account, calculating...")
						ualloBalance = coin.Amount.Uint64()
						break
					} else if coin.Denom == "allo" {
						appchain.Logger.Info().Msg("Found allo balance in account, calculating...")
					}
				}
				if ualloBalance >= appchain.Config.InitialStake {
					var topicsToRegister []uint64
					for _, topicToRegisterUint64 := range b7sTopicIds {
						if err != nil {
							appchain.Logger.Info().Err(err).Uint64("topicId", topicToRegisterUint64).Msg("Could not register for topic, not numerical, skipping")
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
					appchain.Logger.Fatal().Int("balance", int(ualloBalance)).Int("InitialStake", int(appchain.Config.InitialStake)).Msg("account balance is lower than the initialStake requested")
				}
			} else {
				appchain.Logger.Info().Str("account", appchain.ReputerAddress).Msg("account is not funded in uallo")
				return
			}
		}
	}
}

// Retry function with a constant number of retries.
func (ap *AppChain) SendDataWithRetry(ctx context.Context, req sdktypes.Msg, MaxRetries, MinDelay, MaxDelay int) (*cosmosclient.Response, error) {
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

// Sending Inferences/Forecasts to the AppChain
func (ap *AppChain) SendWorkerModeData(ctx context.Context, topicId uint64, results aggregate.Results) {
	// Aggregate the inferences from all peers/workers
	var inferences []*types.Inference
	var forecasterValues []*types.Forecast
	var nonce *types.Nonce
	for _, result := range results {
		for _, peer := range result.Peers {

			ap.Logger.Debug().Str("worker peer", peer.String())

			// Get Peer's $allo address
			res, err := ap.QueryClient.GetWorkerAddressByP2PKey(ctx, &types.QueryWorkerAddressByP2PKeyRequest{
				Libp2PKey: peer.String(),
			})
			if err != nil {
				ap.Logger.Warn().Err(err).Str("peer", peer.String()).Msg("error getting peer address from chain, worker not registered? Ignoring peer.")
				continue
			}
			ap.Logger.Debug().Str("worker address", res.Address).Msgf("%+v", result.Result)
			// Parse the result from the worker to get the inference and forecasts
			var value InferenceForecastResponse
			err = json.Unmarshal([]byte(result.Result.Stdout), &value)
			if err != nil {
				ap.Logger.Warn().Err(err).Str("peer", peer.String()).Msg("error extracting value as number from stdout, ignoring inference.")
				continue
			}

			// Get first Nonce
			if nonce == nil {
				// Parse the value.nonce as str from the result as int64
				nonceInt64, err := strconv.ParseInt(value.Nonce, 10, 64)
				if err != nil {
					ap.Logger.Warn().Err(err).Str("peer", peer.String()).Msg("error extracting nonce as number from stdout, ignoring inference.")
					continue
				}
				nonce = &types.Nonce{Nonce: nonceInt64}
			}

			infererValue := alloraMath.MustNewDecFromString(value.InfererValue)
			inference := &types.Inference{
				TopicId: topicId,
				Worker:  res.Address,
				Value:   infererValue,
				Proof:   value.Signature,
			}
			// Create array with one inference only to be infererValue
			inferences = append(inferences, inference)

			// Aggregate forecasts
			var forecasterValues []*types.Forecast
			var forecasterVal []*types.ForecastElement
			for _, val := range value.ForecasterValues {
				forecasterVal = append(forecasterVal, &types.ForecastElement{
					Inferer: val.Worker,
					Value:   alloraMath.MustNewDecFromString(val.Value),
					Proof:   value.Signature,
				})
			}
			forecasterValues = append(forecasterValues, &types.Forecast{
				TopicId:          topicId,
				Forecaster:       res.Address,
				ForecastElements: forecasterVal,
			})
			// Make 1 request per worker
			req := &types.MsgInsertBulkWorkerPayload{
				Sender:     ap.ReputerAddress,
				Nonce:      nonce,
				TopicId:    topicId,
				Inferences: inferences,
				Forecasts:  forecasterValues,
			}
			go func() {
				_, _ = ap.SendDataWithRetry(ctx, req, 5, 0, 2)
			}()
		}
	}

	// Make 1 request per worker
	req := &types.MsgInsertBulkWorkerPayload{
		Sender:     ap.ReputerAddress,
		Nonce:      nonce,
		TopicId:    topicId,
		Inferences: inferences,
		Forecasts:  forecasterValues,
	}
	go func() {
		_, _ = ap.SendDataWithRetry(ctx, req, 5, 0, 2)
	}()
}

// Sending Losses to the AppChain
func (ap *AppChain) SendReputerModeData(ctx context.Context, topicId uint64, results aggregate.Results) {
	// Aggregate the forecast from reputer leader
	var valueBundles []*types.ReputerValueBundle
	var nonce *types.Nonce

	// Log the whole set of results
	ap.Logger.Debug().Interface("Results received", results)

	for _, result := range results {
		ap.Logger.Debug().Interface("Result", result)
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

			var value LossResponse
			err = json.Unmarshal([]byte(result.Result.Stdout), &value)
			if err != nil {
				ap.Logger.Warn().Err(err).Str("peer", peer.String()).Msg("error extracting loss object from stdout, ignoring loss.")
				continue
			}

			// Get first Nonce
			if nonce == nil {
				nonce = &value.Nonce
			}

			var (
				inferVal       []*types.WorkerAttributedValue
				forecastsVal   []*types.WorkerAttributedValue
				outInferVal    []*types.WithheldWorkerAttributedValue
				outForecastVal []*types.WithheldWorkerAttributedValue
				inInferVal     []*types.WorkerAttributedValue
			)

			for _, inf := range value.InferrerValues {
				inferVal = append(inferVal, &types.WorkerAttributedValue{
					Worker: inf.Worker,
					Value:  alloraMath.MustNewDecFromString(inf.Value),
				})
			}
			for _, inf := range value.ForecasterValues {
				forecastsVal = append(forecastsVal, &types.WorkerAttributedValue{
					Worker: inf.Worker,
					Value:  alloraMath.MustNewDecFromString(inf.Value),
				})
			}
			for _, inf := range value.OneOutInfererValues {
				outInferVal = append(outInferVal, &types.WithheldWorkerAttributedValue{
					Worker: inf.Worker,
					Value:  alloraMath.MustNewDecFromString(inf.Value),
				})
			}
			for _, inf := range value.OneOutForecasterValues {
				outForecastVal = append(outForecastVal, &types.WithheldWorkerAttributedValue{
					Worker: inf.Worker,
					Value:  alloraMath.MustNewDecFromString(inf.Value),
				})
			}
			for _, inf := range value.OneInForecasterValues {
				inInferVal = append(inInferVal, &types.WorkerAttributedValue{
					Worker: inf.Worker,
					Value:  alloraMath.MustNewDecFromString(inf.Value),
				})
			}

			valueBundle := &types.ReputerValueBundle{
				Reputer: res.Address,
				ValueBundle: &types.ValueBundle{
					TopicId:                topicId,
					CombinedValue:          alloraMath.MustNewDecFromString(value.CombinedValue),
					NaiveValue:             alloraMath.MustNewDecFromString(value.NaiveValue),
					InfererValues:          inferVal,
					ForecasterValues:       forecastsVal,
					OneOutInfererValues:    outInferVal,
					OneOutForecasterValues: outForecastVal,
					OneInForecasterValues:  inInferVal,
				},
			}
			valueBundles = append(valueBundles, valueBundle)
		}
	}

	// Make 1 request per worker
	req := &types.MsgInsertBulkReputerPayload{
		Sender:              ap.ReputerAddress,
		Nonce:               nonce,
		TopicId:             topicId,
		ReputerValueBundles: valueBundles,
	}
	// Print req to the log
	ap.Logger.Info().Interface("req", req).Msg("Sending Reputer Mode Data")

	_, _ = ap.SendDataWithRetry(ctx, req, 5, 0, 2)
}
