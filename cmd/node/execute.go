package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/blocklessnetwork/b7s/api"
	"github.com/blocklessnetwork/b7s/models/blockless"
	"github.com/blocklessnetwork/b7s/models/codes"
	"github.com/blocklessnetwork/b7s/models/execute"
	"github.com/blocklessnetwork/b7s/node/aggregate"
	"github.com/labstack/echo/v4"
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

// ExecuteResult represents the API representation of a single execution response.
// It is similar to the model in `execute.Result`, except it omits the usage information for now.
type ExecuteResult struct {
	Code      codes.Code            `json:"code,omitempty"`
	Result    execute.RuntimeOutput `json:"result,omitempty"`
	RequestID string                `json:"request_id,omitempty"`
}

func sendResultsToChain(ctx echo.Context, a api.API, appChainClient AppChain, req ExecuteRequest, res ExecuteResponse) {

	// Only in weight functions that we will have a "type" in the response
	functionType := "inferences"
	functionTypeFromFn, err := getResponseInfo(res.Results[0].Result.Stdout)
	if err != nil {
		a.Log.Warn().Str("function", req.FunctionID).Err(err).Msg("node failed to extract response info from stdout")
	} else {
		if functionTypeFromFn != "" {
			functionType = functionTypeFromFn
		}
	}

	// var topicId uint64 = req.Topic
	topicId, err := strconv.ParseUint(req.Topic, 10, 64)
	if err != nil {
		a.Log.Error().Str("Topic", req.Topic).Str("function", functionType).Err(err).Msg("Cannot parse topic ID")
		return
	}
	a.Log.Debug().Str("Topic", req.Topic).Str("function", functionType).Msg("Found topic ID")

	// TODO: We can move this context to the AppChain struct (previous context was breaking the tx broadcast response)
	reqCtx := context.Background()
	if functionType == inferenceType {
		appChainClient.SendInferences(reqCtx, topicId, res.Results)
	} else if functionType == weightsType {
		appChainClient.SendUpdatedWeights(reqCtx, topicId, res.Results)
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

func createExecutor(a api.API, appChainClient *AppChain) func(ctx echo.Context) error {
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

		// Might be disabled if so we should log out
		if appChainClient != nil && appChainClient.Config.SubmitTx && res.Code == codes.OK {
			// don't block the return to the consumer to send these to chain
			go sendResultsToChain(ctx, a, *appChainClient, req, res)
		} else {
			a.Log.Debug().Msg("Inference results would have been submitted to chain.")
			reason := "unknown"
			if appChainClient == nil {
				reason = "AppChainClient is disabled"
			} else if !appChainClient.Config.SubmitTx {
				reason = "Submitting transactions is disabled in AppChainClient"
			} else if res.Code != codes.OK {
				reason = fmt.Sprintf("Response code is not OK: %s, message: %s", res.Code, res.Message)
			}
			a.Log.Warn().Msgf("Inference results not submitted to chain, not attempted. Reason: %s", reason)
		}

		// Send the response.
		return ctx.JSON(http.StatusOK, res)
	}
}
