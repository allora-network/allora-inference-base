FROM --platform=linux/amd64 golang:1.21-bookworm AS builder

WORKDIR /src

ADD . /src

RUN go mod download && \
    make all

###########################
FROM --platform=linux/amd64 debian:bookworm-slim

ARG ghcr_token

WORKDIR /app

## curl, unzip other utilities
RUN apt update && \
    apt -y dist-upgrade && \
    apt install -y --no-install-recommends \
      tzdata \
      curl \
      unzip \
      pv \
      ca-certificates \
      gnupg2 \
      python3 \
      python3-pip && \
    rm -rf /var/cache/apt/*

# gomplete for updating config with env vars
RUN curl -o ./gomplate -sSL https://github.com/hairyhenderson/gomplate/releases/download/v3.10.0/gomplate_linux-amd64 && \
    chmod 755 gomplate

# get the runtime
RUN curl -o ./runtime.tar.gz -sSL https://github.com/blocklessnetwork/runtime/releases/download/v0.3.1/blockless-runtime.ubuntu-20.04.x86_64.tar.gz && \
    mkdir /app/runtime && \
    tar -xvkf ./runtime.tar.gz -C /app/runtime && \
    rm ./runtime.tar.gz


# get the upshot-extension
RUN curl -L -s -H "Authorization: token ${ghcr_token}" -H 'Accept:application/octet-stream' \
      "https://api.github.com/repos/upshot-tech/upshot-blockless-extension/releases/assets/142012669" \
      -o upshot-extension.tar.gz && \
    mkdir /app/runtime/extensions && \
    tar -xvkf ./upshot-extension.tar.gz -C /app/runtime/extensions && \
    rm ./upshot-extension.tar.gz

# libssl 1.1
RUN curl -o ./libssl.deb -sSL http://archive.ubuntu.com/ubuntu/pool/main/o/openssl/libssl1.1_1.1.0g-2ubuntu4_amd64.deb && \
    dpkg -i ./libssl.deb && \
    rm ./libssl.deb

# Install AWS CLI so we can use to backup to IPFS s3 compatible storage providers
RUN curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip" && \
    unzip awscliv2.zip -d /usr/src && \
    rm -f awscliv2.zip && \
    /usr/src/aws/install --bin-dir /usr/bin

## setup
RUN mkdir /app/keys
COPY --from=builder /src/dist/upshot-node upshot-node
COPY --from=builder /src/dist/upshot-keys upshot-keys

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
