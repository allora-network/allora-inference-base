package main

import (
	"encoding/json"
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

func createExecutor(a api.API, appChainClient AppChain) func(ctx echo.Context) error {
	return func(ctx echo.Context) error {

		// Unpack the API request.
		var req ExecuteRequest
		err := ctx.Bind(&req)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Errorf("could not unpack request: %w", err))
		}

		a.Log.Debug().Str("Executing inference function: ", req.FunctionID)

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

		a.Log.Info().Msg("Sending inferences to appchain")
		inferences := appChainClient.SendInferencesToAppChain(1, res.Results)
		a.Log.Debug().Any("inferences", inferences).Msg("Inferences sent to appchain")
		// Get the dependencies for the weights calculation
		ethPrice, latestWeights := appChainClient.GetWeightsCalcDependencies(inferences)

		a.Log.Debug().Float64("ETH price: ", ethPrice)
		a.Log.Debug().Float64("eth price", ethPrice)
		a.Log.Debug().Any("latest weights", latestWeights)
		a.Log.Debug().Any("inferences", inferences)

		// Format the payload for the weights calculation
		var weightsReq map[string]interface{} = make(map[string]interface{})
		weightsReq["eth_price"] = ethPrice
		weightsReq["inferences"] = inferences
		weightsReq["latest_weights"] = latestWeights
		payload, err := json.Marshal(weightsReq)
		if err != nil {
			a.Log.Error().Err(err).Msg("error marshalling weights request")
		}
		payloadCopy := string(payload)
		a.Log.Debug().Any("payload: ", payloadCopy)

		// Calculate the weights
		calcWeightsReq := execute.Request{
			FunctionID: "bafybeibuzoxt3jsf6mswlw5sq2o7cltxfpeodseduwhzrv4d33k32baaau",
			Method:     "eth-price-processing.wasm",
			Config: execute.Config{
				Stdin: &payloadCopy,
			},
		}
		a.Log.Debug().Any("Executing weight adjusment function: ", calcWeightsReq.FunctionID)

		// Get the execution result.
		_, _, weightsResults, _, err := a.Node.ExecuteFunction(ctx.Request().Context(), execute.Request(calcWeightsReq), "")
		if err != nil {
			a.Log.Warn().Str("function", req.FunctionID).Err(err).Msg("node failed to execute function")
		}
		a.Log.Debug().Any("weights results", weightsResults)

		// Transform the node response format to the one returned by the API.
		appChainClient.SendUpdatedWeights(aggregate.Aggregate(weightsResults))

		// Send the response.
		return ctx.JSON(http.StatusOK, res)
	}
}
