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
	Subgroup string `json:"subgroup,omitempty"`
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
	functionType, err := getResponseInfo(res.Results[0].Result.Stdout)
	if err != nil {
		a.Log.Warn().Str("function", req.FunctionID).Err(err).Msg("node failed to extract response info from stdout")
	}

	var topicId uint64 = 0
	for _, envVar := range req.Config.Environment {
        if envVar.Name == "TOPIC_ID" {
            fmt.Println("Found TOPIC_ID:", envVar.Value)
			topicId, err = strconv.ParseUint(envVar.Value, 10, 64)
			if err != nil {
				a.Log.Warn().Str("function", req.FunctionID).Err(err).Msg("node failed to parse topic id")
				return
			}
            break
        }
    }

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

		a.Log.Debug().Str("executing inference function: ", req.FunctionID)

		// Get the execution result.
		code, id, results, cluster, err := a.Node.ExecuteFunction(ctx.Request().Context(), req.Request, req.Subgroup)
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

		// Communicate the reason for failure in these cases.
		if errors.Is(err, blockless.ErrRollCallTimeout) || errors.Is(err, blockless.ErrExecutionNotEnoughNodes) {
			res.Message = err.Error()
		}

		// Might be disabled if so we should log out
		if appChainClient != nil && appChainClient.Config.SubmitTx && res.Code == codes.OK {
			// don't block the return to the consumer to send these to chain
			go sendResultsToChain(ctx, a, *appChainClient, req, res)
		} else {
			a.Log.Debug().Msg("inference results would have been submitted to chain")
		}

		// Send the response.
		return ctx.JSON(http.StatusOK, res)
	}
}
