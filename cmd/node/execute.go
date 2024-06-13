package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/allora-network/b7s/api"
	"github.com/allora-network/b7s/models/blockless"
	"github.com/allora-network/b7s/models/codes"
	"github.com/allora-network/b7s/models/execute"
	"github.com/allora-network/b7s/node"
	"github.com/allora-network/b7s/node/aggregate"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog"
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
	log.Info().Msg("Sending Results to chain")
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
	log.Info().Str("stdout", stdout).Msg("Aggregated stdout result")

	log.Debug().Str("Topic", res.Topic).Str("worker mode", appChainClient.Config.WorkerMode).Msg("Found topic ID")

	reqCtx := context.Background()
	if appChainClient.Config.WorkerMode == WorkerModeWorker { // for inference or forecast
		topicId, err := strconv.ParseUint(res.Topic, 10, 64)
		if err != nil {
			log.Error().Str("Topic", res.Topic).Str("worker mode", appChainClient.Config.WorkerMode).Err(err).Msg("Cannot parse worker topic ID")
			return
		}
		appChainClient.SendWorkerModeData(reqCtx, topicId, aggregate.Aggregate(res.Data))
	} else { // for losses
		// if topicId does not end in "/reputer

		if !strings.HasSuffix(res.Topic, REPUTER_TOPIC_SUFFIX) {
			log.Error().Str("Topic", res.Topic).Str("worker mode", appChainClient.Config.WorkerMode).Msg("Invalid reputer topic format")
			return
		}
		// Get the topicId from the reputer topic string
		index := strings.Index(res.Topic, "/")
		if index == -1 {
			// Handle the error: "/" not found in res.Topic
			log.Error().Str("Topic", res.Topic).Msg("Invalid topic format")
			return
		}
		topicId, err := strconv.ParseUint(res.Topic[:index], 10, 64)
		if err != nil {
			log.Error().Str("Topic", res.Topic).Str("worker mode", appChainClient.Config.WorkerMode).Err(err).Msg("Cannot parse reputer topic ID")
			return
		}
		appChainClient.SendReputerModeData(reqCtx, topicId, aggregate.Aggregate(res.Data))
	}
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
