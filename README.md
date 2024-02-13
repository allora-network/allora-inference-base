# Allora Inference Base

This node allows model providers to participate in the Allora network.
This repo is the base for worker nodes (for providing inferences) and head nodes (which coordinates requests and aggregates results).

# Build locally

In order to build locally:

```
GOOS=linux GOARCH=amd64 make
```

# Run locally

```
./upshot-node \
  --role=head  \
  --peer-db=/data/peerdb \
  --function-db=/data/function-db \
  --runtime-path=/app/runtime \
  --runtime-cli=bls-runtime \
  --workspace=/data/workspace \
  --private-key=/var/keys/priv.bin \
  --port=9010 \
  --rest-api=:6000 \
  --allora-chain-key-name=local-head \
  --allora-chain-restore-mnemonic='your mnemonic words...' --allora-node-rpc-address=https://some-allora-rpc-address/ \
  --allora-chain-topic-id=1

```


***WARNING***

This repo is currently relying on a private module, current development requires

```bash
export GOPRIVATE=github.com/allora-network/allora-appchain
```

# Docker images

To build the image for the head:

```
docker build -f docker/Dockerfile_head -t allora-inference-base:dev-head --build-arg "GH_TOKEN=${YOUR_GH_TOKEN}" --build-arg "BLS_EXTENSION_VER=${BLS_EXTENSION_VERSION}" . 
```

Then to build the image for the head:

```
docker build -f docker/Dockerfile_worker -t allora-inference-base:dev-worker --build-arg "GH_TOKEN=${YOUR_GH_TOKEN}" --build-arg "BLS_EXTENSION_VER=${BLS_EXTENSION_VERSION}" . 
```

where `YOUR_GH_TOKEN` is your Github token, and optionally `BLS_EXTENSION_VER` is the release version of the [Allora inference extension](https://github.com/allora-network/allora-inference-extension) to use (it will use latest if none is set).

You can use the worker image as a base for building your own worker.


# Debugging Locally Using VSCode.

This project comes with some static identities, as well as debug settings for `VSCode`. Use the `VSCode` debugger to start a head node instance, and a worker node instance.

* Ensure you've installed the Runtime `make setup`
* Start the Head Node First
* Start the Worker Node
