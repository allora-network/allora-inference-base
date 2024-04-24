# Allora Inference Base

![Docker!](https://img.shields.io/badge/Docker-2CA5E0?style=for-the-badge&logo=docker&logoColor=white)
![Go!](https://img.shields.io/badge/Go-00ADD8?style=for-the-badge&logo=go&logoColor=white)
![Apache License](https://img.shields.io/badge/Apache%20License-D22128?style=for-the-badge&logo=Apache&logoColor=white)

This node allows model providers to participate in the Allora network.
This repo is the base for worker nodes (for providing inferences) and head nodes (which coordinates requests and aggregates results).

# Build locally

In order to build locally:

```
GOOS=linux GOARCH=amd64 make
```

# Run locally

## Head

```
./allora-node \
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
  --allora-chain-initial-stake=1000
```
## Worker

Worker (for inference or forecast requests) node: 

```
./allora-node \
  --role=worker  \
  --peer-db=/data/peerdb \
  --function-db=/data/function-db \
  --runtime-path=/app/runtime \
  --runtime-cli=bls-runtime \
  --workspace=/data/workspace \
  --private-key=/var/keys/priv.bin \
  --port=9011 \
  --rest-api=:6000 \
  --boot-nodes=/ip4/<head-ip-addr>/tcp/9010/p2p/<advertised-head-peerid-key>
  --topic=1 \
  --allora-chain-key-name=local-worker \
  --allora-chain-restore-mnemonic='your mnemonic words...' --allora-node-rpc-address=https://some-allora-rpc-address/ \
  --allora-chain-topic-id=1 \
  --allora-chain-initial-stake=1000 \
  --allora-chain-worker-mode=worker
```

Reputer (for reputation requests) node: 

```
./allora-node \
  --role=worker  \
  --peer-db=/data/peerdb \
  --function-db=/data/function-db \
  --runtime-path=/app/runtime \
  --runtime-cli=bls-runtime \
  --workspace=/data/workspace \
  --private-key=/var/keys/priv.bin \
  --port=9011 \
  --rest-api=:6000 \
  --boot-nodes=/ip4/<head-ip-addr>/tcp/9010/p2p/<advertised-head-peerid-key>
  --topic=1 \
  --allora-chain-key-name=local-worker \
  --allora-chain-restore-mnemonic='your mnemonic words...' --allora-node-rpc-address=https://some-allora-rpc-address/ \
  --allora-chain-topic-id=1 \
  --allora-chain-initial-stake=1000 \
  --allora-chain-worker-mode=reputer
```

## Notes 

If you plan to deploy temporarily without attempting to connect to the Allora blockchain, e.g. just for testing your setup and your inferences and forecasts, do not set any `--allora-...` flag.

### Topic registration

`--topic` defines the topic internally as a Blockless channel, so the heads are able to identify which workers can respond to requests on that topic.

`--allora-chain-topic-id` is the topic in which your worker registers on the appchain. This will be used for evaluating performance and allocating rewards.

`allora-chain-initial-stake` is the stake that you want your node to register as initial stake. The stake is cross-topic, so this is applied only upon registration of a node on the chain. It will not have an effect on subsequent runs when the node is already registered. To modify node stake, please refer to [Allora Network](github.com/allora-network/allora-appchain) client.

### Keys
The `--private-key` sets the Blockless peer key for your particular node. Obviously, please use different keys for different nodes.
The `--allora-chain-key-name` and `--allora-chain-restore-mnemonic` are used to set the local Allora client keyring that your node will use when communicating with the Allora chain.

### Blockless directories

Blockless nodes need to define a number of directories: 
`--peer-db`: database for peers in the network.
`--function-db`: database for WASM functions to be executed in the workers.
`--runtime-path`: runtime path to execute the functions
`--runtime-cli`: runtime command to run functions
`--workspace`: work directory where temporary files are stored.


# Docker images

To build the image for the head:

```
docker build --pull -f docker/Dockerfile_head -t allora-inference-base:dev-head --build-arg "GH_TOKEN=${YOUR_GH_TOKEN}" --build-arg "BLS_EXTENSION_VER=${BLS_EXTENSION_VERSION}" . 
```

Then to build the image for the worker:

```
docker build --pull -f docker/Dockerfile_worker -t allora-inference-base:dev-worker --build-arg "GH_TOKEN=${YOUR_GH_TOKEN}" --build-arg "BLS_EXTENSION_VER=${BLS_EXTENSION_VERSION}" . 
```

where `YOUR_GH_TOKEN` is your Github token, and optionally `BLS_EXTENSION_VER` is the release version of the [Allora inference extension](https://github.com/allora-network/allora-inference-extension) to use (it will use latest if none is set).

You can use the worker image as a base for building your own worker.


# Debugging Locally Using VSCode.

This project comes with some static identities, as well as debug settings for `VSCode`. Use the `VSCode` debugger to start a head node instance, and a worker node instance.

* Ensure you've installed the Runtime `make setup`
* Start the Head Node First
* Start the Worker Node
