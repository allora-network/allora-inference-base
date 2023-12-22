# Upshot Compute Node

This node allows model providers to participate providing inferences to the Upshot Network.

# Build locally

```
GOOS=linux GOARCH=amd64 make
docker build -f docker/Dockerfile -t upshot:dev --build-arg "ghcr_token=${YOU_GH_TOKEN}" . 
```