# Upshot Compute Node

This node allows model providers to participate providing inferences to the Upshot Network.

# Build locally

First you need to set the env vars to connect with the Upshot Appchain
```
export NODE_ADDRESS="http://localhost:26657"
export UPT_ACCOUNT_MNEMONIC="palm key track hammer early love act cat area betray ..."
export UPT_ACCOUNT_NAME="alice"
export UPT_ACCOUNT_PASSPHRASE="1234567890"
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
