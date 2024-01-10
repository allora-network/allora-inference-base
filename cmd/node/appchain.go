package main

import (
	"context"
	"log"

	"github.com/ignite/cli/v28/ignite/pkg/cosmosclient"
)

func registerNode(ctx context.Context) {
 
}

func startClient() {
	ctx := context.Background()
    addressPrefix := "cosmos"

    // Create a Cosmos client instance
    _, err := cosmosclient.New(ctx, cosmosclient.WithAddressPrefix(addressPrefix))
    if err != nil {
        log.Fatal(err)
    }
}

func Start(ctx context.Context) {
	registerNode(ctx)
	go startClient()
}