# Upshot Compute Node

This node allows model providers to participate providing inferences to the Upshot Network.

# Build locally

First you need to set the env vars to connect with the Uptshot Appchain
```
export NODE_ADDRESS="http://localhost:26657"
export UPT_ACCOUNT_MNEMONIC="palm key track hammer early love act cat area betray ..."
export UPT_ACCOUNT_NAME="alice"
export UPT_ACCOUNT_PASSPHRASE="1234567890"
```

***WARNING***

This repo is currently relying on a replaced lib-p2p module.

Clone the following repo as a sibling to this

```bash
git clone https://github.com/dmikey/go-libp2p-raft
```

Then to build

```
GOOS=linux GOARCH=amd64 make
docker build -f docker/Dockerfile -t upshot:dev --build-arg "ghcr_token=${YOU_GH_TOKEN}" . 
```

# Debugging Locally Using VSCode.

This project comes with some static identities, as well as debug settings for `VSCode`. Use the `VSCode` debugger to start a head node instance, and a worker node instance.

* Ensure you've installed the Runtime `make setup`
* Start the Head Node First
* Start the Worker Node