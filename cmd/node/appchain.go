package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	cosmossdk_io_math "cosmossdk.io/math"
	"github.com/allora-network/allora-chain/x/emissions/types"
	"github.com/allora-network/b7s/models/blockless"
	"github.com/allora-network/b7s/node/aggregate"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosaccount"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosclient"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Exponential backoff retry settings
const NUM_WORKER_RETRIES = 5
const NUM_REPUTER_RETRIES = 5
const NUM_REGISTRATION_RETRIES = 3
const NUM_STAKING_RETRIES = 3
const NUM_WORKER_RETRY_MIN_DELAY = 0
const NUM_WORKER_RETRY_MAX_DELAY = 2
const NUM_REPUTER_RETRY_MIN_DELAY = 0
const NUM_REPUTER_RETRY_MAX_DELAY = 2
const NUM_REGISTRATION_RETRY_MIN_DELAY = 1
const NUM_REGISTRATION_RETRY_MAX_DELAY = 2
const NUM_STAKING_RETRY_MIN_DELAY = 1
const NUM_STAKING_RETRY_MAX_DELAY = 2
const REPUTER_TOPIC_SUFFIX = "/reputer"

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

	client, err := cosmosclient.New(ctx,
		cosmosclient.WithNodeAddress(config.NodeRPCAddress),
		cosmosclient.WithAddressPrefix(config.AddressPrefix),
		cosmosclient.WithHome(alloraClientHome),
		cosmosclient.WithGas(config.Gas),
		cosmosclient.WithGasAdjustment(config.GasAdjustment),
	)
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

func isReputerRegistered(appchain *AppChain, topicId uint64) (bool, error) {
	ctx := context.Background()

	res, err := appchain.QueryClient.IsReputerRegisteredInTopicId(ctx, &types.QueryIsReputerRegisteredInTopicIdRequest{
		TopicId: topicId,
		Address: appchain.ReputerAddress,
	})

	if err != nil {
		return false, err
	}

	return res.IsRegistered, nil
}

func isWorkerRegistered(appchain *AppChain, topicId uint64) (bool, error) {
	ctx := context.Background()

	res, err := appchain.QueryClient.IsWorkerRegisteredInTopicId(ctx, &types.QueryIsWorkerRegisteredInTopicIdRequest{
		TopicId: topicId,
		Address: appchain.ReputerAddress,
	})

	if err != nil {
		return false, err
	}

	return res.IsRegistered, nil
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

	// Iterate each topic
	for _, topicId := range b7sTopicIds {
		var is_registered bool
		var err error
		if isReputer {
			is_registered, err = isReputerRegistered(appchain, topicId)
		} else {
			is_registered, err = isWorkerRegistered(appchain, topicId)
		}
		if err != nil {
			appchain.Logger.Error().Err(err).Uint64("topicId", topicId).Msg("could not check if the node is already registered for topic, skipping.")
			continue
		}
		if !is_registered {
			// register the wroker in the topic
			msg := &types.MsgRegister{
				Sender:       appchain.ReputerAddress,
				LibP2PKey:    appchain.Config.LibP2PKey,
				MultiAddress: appchain.Config.MultiAddress,
				TopicId:      topicId,
				Owner:        appchain.ReputerAddress,
				IsReputer:    isReputer,
			}
			res, err := appchain.SendDataWithRetry(ctx, msg, NUM_REGISTRATION_RETRIES,
				NUM_REGISTRATION_RETRY_MIN_DELAY, NUM_REGISTRATION_RETRY_MAX_DELAY, "register node")
			if err != nil {
				appchain.Logger.Fatal().Err(err).Uint64("topic", topicId).Str("txHash", res.TxHash).
					Msg("could not register the node with the Allora blockchain in topic")
			} else {
				if isReputer {
					var initstake = appchain.Config.InitialStake
					if initstake > 0 {
						msg := &types.MsgAddStake{
							Sender:  appchain.ReputerAddress,
							Amount:  cosmossdk_io_math.NewUint(initstake),
							TopicId: topicId,
						}
						res, err := appchain.SendDataWithRetry(ctx, msg, NUM_STAKING_RETRIES,
							NUM_STAKING_RETRY_MIN_DELAY, NUM_STAKING_RETRY_MAX_DELAY, "add stake")
						if err != nil {
							appchain.Logger.Error().Err(err).Uint64("topic", topicId).Str("txHash", res.TxHash).
								Msg("could not stake the node with the Allora blockchain in specified topic")
						}
					} else {
						appchain.Logger.Info().Msg("No initial stake configured")
					}
				}
			}
		} else {
			appchain.Logger.Info().Uint64("topic", topicId).Msg("node already registered for topic")
		}
	}
}

// Retry function with a constant number of retries.
func (ap *AppChain) SendDataWithRetry(ctx context.Context, req sdktypes.Msg, MaxRetries, MinDelay, MaxDelay int, SuccessMsg string) (*cosmosclient.Response, error) {
	var txResp *cosmosclient.Response
	var err error
	for retryCount := 0; retryCount <= MaxRetries; retryCount++ {
		txResponse, err := ap.Client.BroadcastTx(ctx, ap.ReputerAccount, req)
		txResp = &txResponse
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
		_, _ = ap.SendDataWithRetry(ctx, req, NUM_WORKER_RETRIES, NUM_WORKER_RETRY_MIN_DELAY, NUM_WORKER_RETRY_MAX_DELAY, "Sent Worker Leader Data")
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

	go func() {
		_, _ = ap.SendDataWithRetry(ctx, req, NUM_REPUTER_RETRIES, NUM_REPUTER_RETRY_MIN_DELAY, NUM_REPUTER_RETRY_MAX_DELAY, "Send Reputer Leader Data")
	}()
}
