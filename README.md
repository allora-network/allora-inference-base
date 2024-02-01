# Upshot Compute Node

This node allows model providers to participate providing inferences to the Upshot Network.

# Build locally

First you need to set the flags to connect with the Upshot Appchain. This is done setting the flags:

```
"--allora-chain-key-name",
"alice",
"--allora-chain-restore-mnemonic",
"palm key track...",
"--allora-chain-account-password",
"",
"--allora-node-rpc-address",
"https://localhost:26657",
"--allora-chain-topic-id",
"1"
```

***WARNING***

This repo is currently relying on a private module, current development requires

```bash
export GOPRIVATE=github.com/upshot-tech/upshot-appchain   
```

Then to build

```
GOOS=linux GOARCH=amd64 make
docker build -f docker/Dockerfile -t upshot:dev --build-arg "GH_TOKEN=${YOUR_GH_TOKEN}" --build-arg "BLS_EXTENSION_VER=${BLS_UPSHOT_EXTENSION_VERSION}" . 
```

where `YOUR_GH_TOKEN` is you Github token, and optionally `BLS_EXTENSION_VER` is the release version of the [Upshot blockless extension](https://github.com/upshot-tech/upshot-blockless-extension) to use (will use latest if not set).

# Debugging Locally Using VSCode.

This project comes with some static identities, as well as debug settings for `VSCode`. Use the `VSCode` debugger to start a head node instance, and a worker node instance.

* Ensure you've installed the Runtime `make setup`
* Start the Head Node First
* Start the Worker Node
