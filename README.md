# Upshot Compute Node

This node allows model providers to participate providing inferences to the Upshot Network.

# Build locally


***WARNING***

This repo is currently relying on a replaced lib-p2p module.

Clone the following repo as a sibling to this

```bash
git clone https://github.com/dmikey/go-libp2p-raft
```

Then to build

```
GOOS=linux GOARCH=amd64 make
docker build -f docker/Dockerfile -t upshot:dev --build-arg "ghcr_token=${YOUR_GH_TOKEN}" --build-arg "BLS_EXTENSION_VER=${BLS_UPSHOT_EXTENSION_VERSION}" . 
```

where `YOUR_GH_TOKEN` is you Github token, and `BLS_EXTENSION_VER` is the release version of the [Upshot blockless extension](https://github.com/upshot-tech/upshot-blockless-extension) to use.

# Debugging Locally Using VSCode.

This project comes with some static identities, as well as debug settings for `VSCode`. Use the `VSCode` debugger to start a head node instance, and a worker node instance.

* Ensure you've installed the Runtime `make setup`
* Start the Head Node First
* Start the Worker Node