package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/allora-network/b7s/api"
	"github.com/allora-network/b7s/models/blockless"
	"github.com/allora-network/b7s/models/codes"
	"github.com/allora-network/b7s/models/execute"
	"github.com/allora-network/b7s/node"
	"github.com/allora-network/b7s/node/aggregate"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog"
	"net/http"
	"strconv"
)

const (
	consensusPBFT = "pbft"
	consensusRAFT = "raft"
)

// ExecuteRequest describes the payload for the REST API request for function execution.
type ExecuteRequest struct {
	execute.Request
	Topic string `json:"topic,omitempty"`
}

// ExecuteResponse describes the REST API response for function execution.
type ExecuteResponse struct {
	Code      codes.Code        `json:"code,omitempty"`
	RequestID string            `json:"request_id,omitempty"`
	Message   string            `json:"message,omitempty"`
	Results   aggregate.Results `json:"results,omitempty"`
	Cluster   execute.Cluster   `json:"cluster,omitempty"`
}

func sendResultsToChain(log zerolog.Logger, appChainClient *AppChain, res node.ChanData) {

	if appChainClient == nil || res.Res != codes.OK {
		reason := "unknown"
		if appChainClient == nil {
			reason = "AppChainClient is disabled"
		} else if res.Res != codes.OK {
			reason = fmt.Sprintf("Response code is not OK: %s", res.Res)
		}
		log.Warn().Msgf("Worker results not submitted to chain, not attempted. Reason: %s", reason)
		return
	}
	stdout := aggregate.Aggregate(res.Data)[0].Result.Stdout
	log.Info().Str("", stdout).Msg("WASM function stdout result")
	// Only in weight functions that we will have a "type" in the response
	//functionType := "inferences"
	//
	//functionTypeFromFn, err := getResponseInfo(stdout)
	//if err != nil {
	//	log.Warn().Str("function", res.FunctionId).Err(err).Msg("node failed to extract response info from stdout")
	//} else {
	//	if functionTypeFromFn != "" {
	//		functionType = functionTypeFromFn
	//	}
	//}

	// var topicId uint64 = req.Topic
	topicId, err := strconv.ParseUint(res.Topic, 10, 64)
	if err != nil {
		log.Error().Str("Topic", res.Topic).Str("worker mode", appChainClient.Config.WorkerMode).Err(err).Msg("Cannot parse topic ID")
		return
	}
	log.Debug().Str("Topic", res.Topic).Str("worker mode", appChainClient.Config.WorkerMode).Msg("Found topic ID")

	// TODO: We can move this context to the AppChain struct (previous context was breaking the tx broadcast response)
	reqCtx := context.Background()
	if appChainClient.Config.WorkerMode == WorkerModeWorker { // for inference or forecast
		appChainClient.SendWorkerModeData(reqCtx, topicId, aggregate.Aggregate(res.Data))
	} else { // for losses
		appChainClient.SendReputerModeData(reqCtx, topicId, aggregate.Aggregate(res.Data))
	}
}

func getResponseInfo(stdout string) (string, error) {
	var responseInfo ResponseInfo
	err := json.Unmarshal([]byte(stdout), &responseInfo)
	if err != nil {
		return "", err
	}

	return responseInfo.FunctionType, nil
}

func createExecutor(a api.API) func(ctx echo.Context) error {

	return func(ctx echo.Context) error {

		// Unpack the API request.
		var req ExecuteRequest
		err := ctx.Bind(&req)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Errorf("could not unpack request: %w", err))
		}

		a.Log.Debug().Msgf("Request: %+v", req)
		// Add the topic to the req.Config.Environment vars as TOPIC_ID
		// This is used by the Allora Extension to know which topic it is being executed on
		req.Config.Environment = append(req.Config.Environment, execute.EnvVar{
			Name:  "TOPIC_ID",
			Value: req.Topic,
		})

		// Get the execution result.
		code, id, results, cluster, err := a.Node.ExecuteFunction(ctx.Request().Context(), execute.Request(req.Request), req.Topic)
		if err != nil {
			a.Log.Warn().Str("function", req.FunctionID).Err(err).Msg("node failed to execute function")
		}

		// Transform the node response format to the one returned by the API.
		res := ExecuteResponse{
			Code:      code,
			RequestID: id,
			Results:   aggregate.Aggregate(results),
			Cluster:   cluster,
		}
		a.Log.Debug().Msgf("Response: %+v", res)
		// Communicate the reason for failure in these cases.
		if errors.Is(err, blockless.ErrRollCallTimeout) || errors.Is(err, blockless.ErrExecutionNotEnoughNodes) {
			res.Message = err.Error()
		}

		// Send the response.
		return ctx.JSON(http.StatusOK, res)
	}
}
