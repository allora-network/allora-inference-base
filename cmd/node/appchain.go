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
	"strings"
	"time"

	cosmossdk_io_math "cosmossdk.io/math"
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
		// Check if libp2p key is already registered - if not, register it
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
			msg = &types.MsgRegister{
				Sender:       appchain.ReputerAddress,
				LibP2PKey:    appchain.Config.LibP2PKey,
				MultiAddress: appchain.Config.MultiAddress,
				TopicId:      topicId,
				Owner:        appchain.ReputerAddress,
				IsReputer:    isReputer,
			}
			res, err := appchain.SendDataWithRetry(ctx, msg, 3, 0, 2, "registered node")
			if err != nil {
				appchain.Logger.Fatal().Err(err).Uint64("topic", topicId).Str("txHash", res.TxHash).Msg("could not register the node with the Allora blockchain in topic")
			} else {
				if isReputer {
					var initstake = appchain.Config.InitialStake
					if initstake > 0 {
						msg = &types.MsgAddStake{
							Sender:  appchain.ReputerAddress,
							Amount:  cosmossdk_io_math.NewUint(initstake),
							TopicId: topicId,
						}
						res, err := appchain.SendDataWithRetry(ctx, msg, 3, 0, 2, "add stake")
						if err != nil {
							appchain.Logger.Error().Err(err).Uint64("topic", topicId).Str("txHash", res.TxHash).Msg("could not register the node with the Allora blockchain in specified topic")
						}
					}
				} else {
					appchain.Logger.Info().Msg("No initial stake configured")
				}
			}
		}
		// Deregistration on old topics
		for _, topicId := range topicsToDeRegister {
			if err != nil {
				appchain.Logger.Info().Err(err).Uint64("topic", topicId).Msg("Could not register for topic")
				break
			}
			msg = &types.MsgRemoveRegistration{
				Sender:    appchain.ReputerAddress,
				TopicId:   topicId,
				IsReputer: isReputer,
			}

			res, err := appchain.SendDataWithRetry(ctx, msg, 3, 0, 2, "deregister node")
			if err != nil {
				appchain.Logger.Fatal().Err(err).Uint64("topic", topicId).Msg("could not deregister the node with the Allora blockchain in topic")
			} else {
				appchain.Logger.Info().Str("txhash", res.TxHash).Uint64("topic", topicId).Msg("successfully deregistered node with Allora blockchain in topic")
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
				var ualloBalance sdktypes.Coin
				var initstake = appchain.Config.InitialStake
				for _, coin := range balanceRes {
					if coin.Denom == "uallo" {
						// Found the balance in "uallo"
						appchain.Logger.Info().Str("balance", coin.Amount.BigInt().String()).Msg("Found allo balance in account, calculating...")
						ualloBalance = coin
						break
					} else if coin.Denom == "allo" {
						appchain.Logger.Info().Msg("Found allo balance in account, calculating...")
					}
				}
				if initstake > math.MaxInt64 {
					initstake = math.MaxInt64
				}
				if ualloBalance.Amount.GTE(cosmossdk_io_math.NewInt(int64(initstake))) {
					for _, topicToRegisterUint64 := range b7sTopicIds {
						// If not registered in any topic
						msg = &types.MsgRegister{
							Sender:       appchain.ReputerAddress,
							LibP2PKey:    appchain.Config.LibP2PKey,
							MultiAddress: appchain.Config.MultiAddress,
							TopicId:      topicToRegisterUint64,
							Owner:        appchain.ReputerAddress,
							IsReputer:    isReputer,
						}
						res, err := appchain.SendDataWithRetry(ctx, msg, 3, 0, 2, "register node")
						if err != nil {
							appchain.Logger.Fatal().Err(err).Msg("could not register the node with the Allora blockchain in specified topics")
						} else {
							appchain.Logger.Info().Str("txhash", res.TxHash).Msg("successfully registered node with Allora blockchain")
							if isReputer {
								if initstake > 0 {
									msg = &types.MsgAddStake{
										Sender:  appchain.ReputerAddress,
										Amount:  cosmossdk_io_math.NewUint(initstake),
										TopicId: topicToRegisterUint64,
									}
									res, err := appchain.SendDataWithRetry(ctx, msg, 3, 0, 2, "add stake")
									if err != nil {
										appchain.Logger.Fatal().Err(err).Msg("could not register the node with the Allora blockchain in specified topics")
									} else {
										appchain.Logger.Info().Str("txhash", res.TxHash).Uint64("stake", initstake).Msg("successfully staked with Allora blockchain")
									}
								} else {
									appchain.Logger.Info().Msg("No initial stake configured")
								}
							}
						}
						appchain.Logger.Info().Str("balance", balanceRes.String()).Msg("Registered Node")
					}
				} else {
					appchain.Logger.Fatal().Str("balance", ualloBalance.Amount.BigInt().Text(10)).Int("InitialStake", int(appchain.Config.InitialStake)).Msg("account balance is lower than the initialStake requested")
				}
			} else {
				appchain.Logger.Info().Str("account", appchain.ReputerAddress).Msg("account is not funded in uallo")
				return
			}
		}
	}
}

// Retry function with a constant number of retries.
func (ap *AppChain) SendDataWithRetry(ctx context.Context, req sdktypes.Msg, MaxRetries, MinDelay, MaxDelay int, SuccessMsg string) (*cosmosclient.Response, error) {
	var txResp *cosmosclient.Response
	var err error

	for retryCount := 0; retryCount <= MaxRetries; retryCount++ {
		txResp, err := ap.Client.BroadcastTx(ctx, ap.ReputerAccount, req)
		if err == nil {
			ap.Logger.Info().Str("Tx Hash:", txResp.TxHash).Msg("Success: " + SuccessMsg)
			break
		}
		// Log the error for each retry.
		ap.Logger.Info().Err(err).Msgf("Failed: "+SuccessMsg+", retrying... (Retry %d/%d)", retryCount, MaxRetries)
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
	WorkerDataBundles := make([]*types.WorkerDataBundle, 0)
	var nonce *types.Nonce
	for _, result := range results {
		for _, peer := range result.Peers {
			ap.Logger.Debug().Str("worker peer", peer.String())

			// Get Peer's $allo address
			res, err := ap.QueryClient.GetWorkerAddressByP2PKey(ctx, &types.QueryWorkerAddressByP2PKeyRequest{
				Libp2PKey: peer.String(),
			})
			if err != nil {
				ap.Logger.Warn().Err(err).Str("peer", peer.String()).Msg("error getting worker peer address from chain, worker not registered? Ignoring peer.")
				continue
			}
			ap.Logger.Debug().Str("worker address", res.Address).Msgf("%+v", result.Result)

			// Parse the result from the worker to get the inference and forecasts
			var value WorkerDataResponse
			err = json.Unmarshal([]byte(result.Result.Stdout), &value)
			if err != nil {
				ap.Logger.Warn().Err(err).Str("peer", peer.String()).Msg("error extracting WorkerDataBundle from stdout, ignoring bundle.")
				continue
			}
			if nonce == nil {
				nonce = &types.Nonce{BlockHeight: value.BlockHeight}
			}
			// Here reputer leader can choose to validate data further to ensure set is correct and act accordingly
			if value.WorkerDataBundle == nil {
				ap.Logger.Warn().Str("peer", peer.String()).Msg("WorkerDataBundle is nil from stdout, ignoring bundle.")
				continue
			}
			if value.WorkerDataBundle.InferenceForecastsBundle == nil {
				ap.Logger.Warn().Str("peer", peer.String()).Msg("InferenceForecastsBundle is nil from stdout, ignoring bundle.")
				continue
			}
			if value.WorkerDataBundle.InferenceForecastsBundle.Inference != nil &&
				value.WorkerDataBundle.InferenceForecastsBundle.Inference.TopicId != topicId {
				ap.Logger.Warn().Str("peer", peer.String()).Msg("InferenceForecastsBundle topicId does not match with request topic, ignoring bundle.")
				continue
			}

			// Append the WorkerDataBundle (only) to the WorkerDataBundles slice
			WorkerDataBundles = append(WorkerDataBundles, value.WorkerDataBundle)
		}
	}

	// Make 1 request per worker
	req := &types.MsgInsertBulkWorkerPayload{
		Sender:            ap.ReputerAddress,
		Nonce:             nonce,
		TopicId:           topicId,
		WorkerDataBundles: WorkerDataBundles,
	}
	// Print req as JSON to the log
	reqJSON, err := json.Marshal(req)
	if err != nil {
		ap.Logger.Error().Err(err).Msg("Error marshaling MsgInsertBulkWorkerPayload to print Msg as JSON")
	} else {
		ap.Logger.Info().Str("req_json", string(reqJSON)).Msg("Sending Worker Mode Data")
	}

	go func() {
		_, _ = ap.SendDataWithRetry(ctx, req, 5, 0, 2, "Sent Worker Leader Data")
	}()
}

// Sending Losses to the AppChain
func (ap *AppChain) SendReputerModeData(ctx context.Context, topicId uint64, results aggregate.Results) {
	// Aggregate the forecast from reputer leader
	var valueBundles []*types.ReputerValueBundle
	var nonceCurrent *types.Nonce
	var nonceEval *types.Nonce

	for _, result := range results {
		if len(result.Peers) > 0 {
			peer := result.Peers[0]
			ap.Logger.Debug().Str("worker peer", peer.String())

			// Get Peer $allo address
			res, err := ap.QueryClient.GetReputerAddressByP2PKey(ctx, &types.QueryReputerAddressByP2PKeyRequest{
				Libp2PKey: peer.String(),
			})
			if err != nil {
				ap.Logger.Warn().Err(err).Str("peer", peer.String()).Msg("error getting reputer peer address from chain, worker not registered? Ignoring peer.")
				continue
			} else {
				// Print the address of the reputer
				ap.Logger.Info().Str("Reputer Address", res.Address).Msg("Reputer Address")
			}

			// Parse the result from the reputer to get the inference and forecasts
			// Parse the result from the worker to get the inference and forecasts
			var value ReputerDataResponse
			err = json.Unmarshal([]byte(result.Result.Stdout), &value)
			if err != nil {
				ap.Logger.Warn().Err(err).Str("peer", peer.String()).Msg("error extracting WorkerDataBundle from stdout, ignoring bundle.")
				continue
			}
			if nonceCurrent == nil {
				nonceCurrent = &types.Nonce{BlockHeight: value.BlockHeight}
			}
			if nonceEval == nil {
				nonceEval = &types.Nonce{BlockHeight: value.BlockHeightEval}
			}

			// Here reputer leader can choose to validate data further to ensure set is correct and act accordingly
			if value.ReputerValueBundle == nil {
				ap.Logger.Warn().Str("peer", peer.String()).Msg("ReputerValueBundle is nil from stdout, ignoring bundle.")
				continue
			}
			if value.ReputerValueBundle.ValueBundle == nil {
				ap.Logger.Warn().Str("peer", peer.String()).Msg("ValueBundle is nil from stdout, ignoring bundle.")
				continue
			}
			if value.ReputerValueBundle.ValueBundle.TopicId != topicId {
				ap.Logger.Warn().Str("peer", peer.String()).Msg("ReputerValueBundle topicId does not match with request topicId, ignoring bundle.")
				continue
			}
			// Append the WorkerDataBundle (only) to the WorkerDataBundles slice
			valueBundles = append(valueBundles, value.ReputerValueBundle)

		} else {
			ap.Logger.Warn().Msg("No peers in the result, ignoring")
		}
	}

	// Make 1 request per worker
	req := &types.MsgInsertBulkReputerPayload{
		Sender: ap.ReputerAddress,
		ReputerRequestNonce: &types.ReputerRequestNonce{
			ReputerNonce: nonceCurrent,
			WorkerNonce:  nonceEval,
		},
		TopicId:             topicId,
		ReputerValueBundles: valueBundles,
	}
	// Print req as JSON to the log
	reqJSON, err := json.Marshal(req)
	if err != nil {
		ap.Logger.Error().Err(err).Msg("Error marshaling MsgInsertBulkReputerPayload to print Msg as JSON")
	} else {
		ap.Logger.Info().Str("req_json", string(reqJSON)).Msg("Sending Reputer Mode Data")
	}

	_, _ = ap.SendDataWithRetry(ctx, req, 5, 0, 2, "Send Reputer Leader Data")
}
