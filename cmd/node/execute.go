package main

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/blocklessnetwork/b7s/api"
	"github.com/blocklessnetwork/b7s/models/blockless"
	"github.com/blocklessnetwork/b7s/models/codes"
	"github.com/blocklessnetwork/b7s/models/execute"
	"github.com/blocklessnetwork/b7s/node/aggregate"
	"github.com/labstack/echo/v4"
)

// ExecuteRequest describes the payload for the REST API request for function execution.
type ExecuteRequest execute.Request

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

func createExecutor(a api.API) func (ctx echo.Context) error  {
    return func (ctx echo.Context) error {

		// Unpack the API request.
		var req ExecuteRequest
		err := ctx.Bind(&req)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Errorf("could not unpack request: %w", err))
		}
	
		// Get the execution result.
		code, id, results, cluster, err := a.Node.ExecuteFunction(ctx.Request().Context(), execute.Request(req))
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
	
		// Send the response.
		return ctx.JSON(http.StatusOK, res)
	}
}