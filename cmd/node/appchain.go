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
		Address:     address,
		Account:     account,
		Logger:      log,
		Client:      client,
		QueryClient: queryClient,
		Config:      config,
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
		Address: appchain.Address,
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
		Address: appchain.Address,
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
				Sender:       appchain.Address,
				LibP2PKey:    appchain.Config.LibP2PKey,
				MultiAddress: appchain.Config.MultiAddress,
				TopicId:      topicId,
				Owner:        appchain.Address,
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
							Sender:  appchain.Address,
							Amount:  cosmossdk_io_math.NewInt(initstake),
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
		txResponse, err := ap.Client.BroadcastTx(ctx, ap.Account, req)
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

	if nonce == nil {
		ap.Logger.Warn().Msg("No valid WorkerDataBundles with nonces found, not sending data to the chain")
		return
	}

	// Make 1 request per worker
	req := &types.MsgInsertBulkWorkerPayload{
		Sender:            ap.Address,
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

// Can only look up the topic stakes of this many reputers at a time
const DEFAULT_MAX_REPUTERS_FOR_STAKE_QUERY = uint64(100)

// Only this number times MaxLimit (whose default is given above) of reputer stakes can be gathered at once
const MAX_NUMBER_STAKE_QUERIES_PER_REQUEST = uint64(3)

// Get the stake of each reputer in the given topic
func (ap *AppChain) getStakePerReputer(ctx context.Context, topicId uint64, reputerAddrs []*string) (map[string]cosmossdk_io_math.Int, error) {
	maxReputers := DEFAULT_MAX_REPUTERS_FOR_STAKE_QUERY
	params, err := ap.QueryClient.Params(ctx, &types.QueryParamsRequest{})
	if err != nil {
		ap.Logger.Error().Err(err).Uint64("topic", topicId).Msg("could not get chain params")
	}
	if err == nil {
		maxReputers = params.Params.MaxLimit
	}

	numberRequestsForStake := MAX_NUMBER_STAKE_QUERIES_PER_REQUEST
	var stakesPerReputer = make(map[string]cosmossdk_io_math.Int) // This will be populated with each request/loop below
	for i := uint64(0); i < numberRequestsForStake; i++ {
		// Dereference only the needed reputer addresses to get the actual strings
		addresses := make([]string, 0)
		start := i * maxReputers
		end := (i + 1) * maxReputers
		if end > uint64(len(reputerAddrs)) {
			end = uint64(len(reputerAddrs))
		}
		if start >= end {
			break
		}
		for _, addr := range reputerAddrs[start:end] {
			if addr == nil {
				return nil, fmt.Errorf("nil address in reputerAddrs")
			}
			addresses = append(addresses, *addr)
		}
		res, err := ap.QueryClient.GetMultiReputerStakeInTopic(ctx, &types.QueryMultiReputerStakeInTopicRequest{
			TopicId:   topicId,
			Addresses: addresses,
		})
		if err != nil {
			ap.Logger.Error().Err(err).Uint64("topic", topicId).Msg("could not get reputer stakes from the chain")
			return nil, err
		}

		// Create a map of reputer addresses to their stakes
		for _, stake := range res.Amounts {
			stakesPerReputer[stake.Reputer] = stake.Amount
		}
	}

	return stakesPerReputer, err
}

func (ap *AppChain) argmaxBlockByStake(
	blockToReputer *map[int64][]string,
	stakesPerReputer map[string]cosmossdk_io_math.Int,
) int64 {
	// Find the current block height with the highest voting power
	firstIter := true
	highestVotingPower := cosmossdk_io_math.ZeroInt()
	blockOfMaxPower := int64(-1)
	for block, reputersWhoVotedForBlock := range *blockToReputer {
		// Calc voting power of this candidate block by total voting reputer stake
		blockVotingPower := cosmossdk_io_math.ZeroInt()
		for _, reputerAddr := range reputersWhoVotedForBlock {
			blockVotingPower = blockVotingPower.Add(stakesPerReputer[reputerAddr])
		}

		// Decide if voting power exceeds that of current front-runner
		if firstIter || blockVotingPower.GT(highestVotingPower) {
			blockOfMaxPower = block
		}

		firstIter = false
	}

	return blockOfMaxPower
}

func (ap *AppChain) argmaxBlockByCount(
	blockToReputer *map[int64][]string,
) int64 {
	// Find the current block height with the highest voting power
	firstIter := true
	highestVotingPower := cosmossdk_io_math.ZeroInt()
	blockOfMaxPower := int64(-1)
	for block, reputersWhoVotedForBlock := range *blockToReputer {
		// Calc voting power of this candidate block by total reputer count
		blockVotingPower := cosmossdk_io_math.NewInt(int64(len(reputersWhoVotedForBlock)))

		// Decide if voting power exceeds that of current front-runner
		if firstIter || blockVotingPower.GT(highestVotingPower) {
			blockOfMaxPower = block
		}

		firstIter = false
	}

	return blockOfMaxPower
}

// Take stake-weighted vote of what the reputer leader thinks the current and eval block heights should be
func (ap *AppChain) getStakeWeightedBlockHeights(
	ctx context.Context,
	topicId uint64,
	blockCurrentToReputer, blockEvalToReputer *map[int64][]string,
	reputerAddrs []*string,
) (int64, int64, error) {
	useWeightedVote := true
	stakesPerReputer, err := ap.getStakePerReputer(ctx, topicId, reputerAddrs)
	if err != nil {
		ap.Logger.Error().Err(err).Uint64("topic", topicId).Msg("error getting reputer stakes from the chain => using unweighted vote")
		// This removes a strict requirement for the reputer leader to have the correct stake
		// at the cost of potentially allowing sybil attacks, though Blockless itself somewhat mitigates this
		useWeightedVote = false
	}

	// Find the current and ev block height with the highest voting power
	if useWeightedVote {
		return ap.argmaxBlockByStake(blockCurrentToReputer, stakesPerReputer), ap.argmaxBlockByStake(blockEvalToReputer, stakesPerReputer), nil
	} else {
		return ap.argmaxBlockByCount(blockCurrentToReputer), ap.argmaxBlockByCount(blockEvalToReputer), nil
	}
}

// Sending Losses to the AppChain
func (ap *AppChain) SendReputerModeData(ctx context.Context, topicId uint64, results aggregate.Results) {
	// Aggregate the forecast from reputer leader
	var valueBundles []*types.ReputerValueBundle
	var reputerAddrs []*string
	var reputerAddrSet = make(map[string]bool) // Prevents duplicate reputer addresses from being counted in vote tally
	var nonceCurrent *types.Nonce
	var nonceEval *types.Nonce
	var blockCurrentToReputer = make(map[int64][]string) // map blockHeight to addresses of reputers who sent data for current block height
	var blockEvalToReputer = make(map[int64][]string)    // map blockHeight to addresses of reputers who sent data for eval block height

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

			if _, ok := reputerAddrSet[res.Address]; !ok {
				reputerAddrSet[res.Address] = true

				// Parse the result from the reputer to get the losses
				// Parse the result from the worker to get the inferences and forecasts
				var value ReputerDataResponse
				err = json.Unmarshal([]byte(result.Result.Stdout), &value)
				if err != nil {
					ap.Logger.Warn().Err(err).Str("peer", peer.String()).Str("Value", result.Result.Stdout).Msg("error extracting ReputerDataResponse from stdout, ignoring bundle.")
					continue
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
				reputerAddrs = append(reputerAddrs, &res.Address)
				blockCurrentToReputer[value.BlockHeight] = append(blockCurrentToReputer[value.BlockHeight], res.Address)
				blockEvalToReputer[value.BlockHeightEval] = append(blockEvalToReputer[value.BlockHeightEval], res.Address)
			}
		} else {
			ap.Logger.Warn().Msg("No peers in the result, ignoring")
		}
	}

	if len(reputerAddrs) == 0 {
		ap.Logger.Warn().Msg("No reputer addresses found, not sending data to the chain")
		return
	}

	blockCurrentHeight, blockEvalHeight, err := ap.getStakeWeightedBlockHeights(ctx, topicId, &blockCurrentToReputer, &blockEvalToReputer, reputerAddrs)
	if err != nil {
		ap.Logger.Error().Err(err).Msg("could not get stake-weighted block heights, not sending data to the chain")
		return
	}
	if blockCurrentHeight == -1 || blockEvalHeight == -1 {
		ap.Logger.Error().Msg("could not get stake-weighted block heights, not sending data to the chain")
		return
	}
	if blockCurrentHeight < blockEvalHeight {
		ap.Logger.Error().Int64("blockCurrentHeight", blockCurrentHeight).Int64("blockEvalHeight", blockEvalHeight).Msg("blockCurrentHeight < blockEvalHeight, not sending data to the chain")
		return
	}
	nonceCurrent = &types.Nonce{BlockHeight: blockCurrentHeight}
	nonceEval = &types.Nonce{BlockHeight: blockEvalHeight}

	// Remove those bundles that do not come from the current block height
	var valueBundlesFiltered []*types.ReputerValueBundle

	for _, valueBundle := range valueBundles {
		if valueBundle.ValueBundle.ReputerRequestNonce.ReputerNonce.BlockHeight == blockCurrentHeight && valueBundle.ValueBundle.ReputerRequestNonce.WorkerNonce.BlockHeight == blockEvalHeight {
			ap.Logger.Debug().
				Str("reputer", valueBundle.ValueBundle.Reputer).
				Str("nonce reputer", strconv.FormatInt(valueBundle.ValueBundle.ReputerRequestNonce.ReputerNonce.BlockHeight, 10)).
				Str("nonce worker", strconv.FormatInt(valueBundle.ValueBundle.ReputerRequestNonce.WorkerNonce.BlockHeight, 10)).
				Msg("Valid nonce, adding to valueBundlesFiltered")
			valueBundlesFiltered = append(valueBundlesFiltered, valueBundle)
		} else {
			ap.Logger.Warn().
				Str("reputer", valueBundle.ValueBundle.Reputer).
				Str("nonce reputer", strconv.FormatInt(valueBundle.ValueBundle.ReputerRequestNonce.ReputerNonce.BlockHeight, 10)).
				Str("nonce worker", strconv.FormatInt(valueBundle.ValueBundle.ReputerRequestNonce.WorkerNonce.BlockHeight, 10)).
				Msg("Rejected Bundle, non-matching nonces.")
		}
	}

	// Make 1 request per worker
	req := &types.MsgInsertBulkReputerPayload{
		Sender: ap.Address,
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
