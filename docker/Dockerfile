FROM --platform=linux/amd64 golang:1.21-bookworm AS builder

WORKDIR /src

ADD . /src

RUN go mod download && \
    make all

###########################
FROM --platform=linux/amd64 python:3.12-slim-bookworm

WORKDIR /app

## curl, unzip other utilities
#! libssl-dev - BLS_RUNTIME dependency
#! gh - to doenaload release from priv repo
RUN apt update && \
    apt -y dist-upgrade && \
    apt install -y --no-install-recommends \
      tzdata \
      curl \
      unzip \
      ca-certificates \
      gh && \
    rm -rf /var/cache/apt/*
      # libssl-dev \  # - temporary use libssl 1.1 TODO: Should use fresher libssl

# libssl 1.1
RUN curl -o ./libssl.deb -sSL http://archive.ubuntu.com/ubuntu/pool/main/o/openssl/libssl1.1_1.1.0g-2ubuntu4_amd64.deb && \
    dpkg -i ./libssl.deb && \
    rm ./libssl.deb

# gomplete for updating config with env vars
RUN GOMPLATE_VER="v3.11.6" && \
    curl -o /usr/local/bin/gomplate -sSL https://github.com/hairyhenderson/gomplate/releases/download/${GOMPLATE_VER}/gomplate_linux-amd64 && \
    chmod 755 /usr/local/bin/gomplate

# get the runtime
RUN curl -o ./runtime.tar.gz -sSL https://github.com/blocklessnetwork/runtime/releases/download/v0.3.1/blockless-runtime.ubuntu-20.04.x86_64.tar.gz
RUN mkdir /app/runtime && tar -xvkf ./runtime.tar.gz -C /app/runtime
RUN python3 -m pip install --upgrade pip

ARG GH_TOKEN
ARG BLS_EXTENSION_VER
# If BLS_EXTENSION_VER install the version else install latest
RUN if [ -n $BLS_EXTENSION_VER]; then \
      gh release download $BLS_EXTENSION_VER \
        --repo "upshot-tech/upshot-blockless-extension" \
        --pattern '*linux-amd64.tar.gz' \
        --output upshot-extension.tar.gz; \
    else \
      gh release download $(gh release list --repo "upshot-tech/upshot-blockless-extension" | grep Latest | awk '{print($1)}') \
        --repo "upshot-tech/upshot-blockless-extension" \
        --pattern '*linux-amd64.tar.gz' \
        --output upshot-extension.tar.gz; \
    fi && \
    mkdir -p /app/runtime/extensions && \
    tar -xvkf ./upshot-extension.tar.gz -C /app/runtime/extensions && \
    rm ./upshot-extension.tar.gz

## setup
RUN mkdir /app/keys
COPY --from=builder /src/dist/upshot-node upshot-node
COPY --from=builder /src/dist/upshot-keys upshot-keys

# Smoke test
RUN /app/runtime/bls-runtime --help && \
    /app/runtime/extensions/upshot-blockless-extension --help

# TODO: to decrease number of installed tools, => size of the image and Increase security posture
#       should move aws cli into another image
# Install AWS CLI so we can use to backup to IPFS s3 compatible storage providers
RUN curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip" \
  && unzip awscliv2.zip -d /usr/src && rm -f awscliv2.zip \
  && /usr/src/aws/install --bin-dir /usr/bin

## run script
COPY ./docker/run.sh ./run.sh
RUN chmod +x ./run.sh

ENV AWS_ACCESS_KEY_ID=
ENV AWS_SECRET_ACCESS_KEY=
ENV S3_HOST="${S3_HOST:-https://s3.filebase.com}"

ENV KEY_PATH=""
ENV KEY_PASSWORD=""
ENV CHAIN_RPC_NODE="http://0.0.0.0:26657"
ENV NODE_KEY_PATH=/app/keys/priv.bin
ENV WORKSPACE_ROOT=/tmp/node
ENV RUNETIME_PATH=/app/runtime
ENV REST_API=8080
ENV P2P_PORT=9527

EXPOSE 8080 9527
# VOLUME ${APP_PATH}

## run script
ENTRYPOINT ["/app/run.sh"]
