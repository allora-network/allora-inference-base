package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/blocklessnetwork/b7s/api"
	// "github.com/blocklessnetwork/b7s/models/blockless"
	"github.com/blocklessnetwork/b7s/models/codes"
	"github.com/blocklessnetwork/b7s/models/execute"
	"github.com/blocklessnetwork/b7s/testing/mocks"
	"github.com/libp2p/go-libp2p/core/peer"
)

type dummyNode struct {
	Node api.Node
}

func newDummyNode(node api.Node) *dummyNode {

	return &dummyNode{
		Node: node,
	}
}

func (d *dummyNode) ExecuteFunction(ctx context.Context, req execute.Request, subgroup string) (codes.Code, string, execute.ResultMap, execute.Cluster, error) {
	fmt.Println("Called dummy ExecuteFunction")

	fmt.Println("req.FunctionID", req.FunctionID)
	fmt.Println("req.Method", req.Method)
	fmt.Println("req.Config", req.Config)

	// Inference function
	if req.FunctionID == "bafybeif5cu26lo7wh7pdn2tuv6un3c3kdxberxznlgnvntkftkpkiesqdi" {
		fmt.Println("\n DUMMY FUNCTION EXECUTED \n")
		return dummyFunction()
	}
	fmt.Println("\n REAL FUNCTION EXECUTED \n")
	return d.Node.ExecuteFunction(ctx, req, subgroup)
}
 
func dummyFunction() (codes.Code, string, execute.ResultMap, execute.Cluster, error) {
	// Seed the random number generator
	rand.Seed(time.Now().UnixNano())

	// Generate a random float between 2500 and 2600
	
	peers := mocks.GenericPeerIDs
	peers = peers[:3]
	log.Printf("number of peers %v", len(peers))
	
	results := make(map[peer.ID]execute.Result)
	for _, peer := range peers {
		peer := peer
		fmt.Println("peer", peer)
		randomFloat := 2500.0 + rand.Float64()*(2600.0-2500.0)
		dummyOutput := fmt.Sprintf("{\"value\":\"%.12f\",\"type\":\"inferences\"}\n\n", randomFloat)
		// log.Printf("called dummy ExecuteFunction that will return %v for nodes: %s", dummyOutput, peer)

		results[peer] = execute.Result{
			Code: codes.OK,
			Result: execute.RuntimeOutput{
				Stdout: dummyOutput,
			},
		}
	}

	cluster := execute.Cluster{
		Peers: peers,
	}

	return codes.OK, "dummy-request-id", results, cluster, nil
}

func (d *dummyNode) ExecutionResult(id string) (execute.Result, bool) {
	return execute.Result{}, false
}

func (d *dummyNode) PublishFunctionInstall(ctx context.Context, uri string, cid string, subgroup string) error {
	return errors.New("TBD: not implemented")
}
