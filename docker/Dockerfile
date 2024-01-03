FROM --platform=linux/amd64 ubuntu:latest 

ARG ghcr_token

WORKDIR /app

## curl, unzip other utilities
RUN apt-get update && \
  apt-get install --no-install-recommends --assume-yes curl unzip pv ca-certificates gnupg2 python3 python3-pip

# gomplete for updating config with env vars
RUN curl -o ./gomplate -sSL https://github.com/hairyhenderson/gomplate/releases/download/v3.10.0/gomplate_linux-amd64
RUN chmod 755 gomplate

# get the runtime
RUN curl -o ./runtime.tar.gz -sSL https://github.com/blocklessnetwork/runtime/releases/download/v0.3.1/blockless-runtime.ubuntu-20.04.x86_64.tar.gz
RUN mkdir /app/runtime && tar -xvkf ./runtime.tar.gz -C /app/runtime

# get attribution cli
RUN curl -o ./attribute-cli.tar.gz -sSL https://github.com/blocklessnetwork/b7s-attributes/releases/download/v0.0.0/b7s-attributes-v0.0.0-linux-arm64.tar.gz
RUN tar -xvkf ./attribute-cli.tar.gz -C /app/
derek/ora-158-have-nodes-advertise-that-they-have-an-extension-by-default

RUN python3 -m pip install --upgrade pip

# get the upshot-extension
RUN curl -L -s -H "Authorization: token ${ghcr_token}" -H 'Accept:application/octet-stream' "https://api.github.com/repos/upshot-tech/upshot-blockless-extension/releases/assets/142012669" -o upshot-extension.tar.gz
RUN mkdir /app/runtime/extensions && tar -xvkf ./upshot-extension.tar.gz -C /app/runtime/extensions

# libssl 1.1
RUN curl -o ./libssl.deb -sSL http://archive.ubuntu.com/ubuntu/pool/main/o/openssl/libssl1.1_1.1.0g-2ubuntu4_amd64.deb
RUN dpkg -i ./libssl.deb

# Install AWS CLI so we can use to backup to IPFS s3 compatible storage providers
RUN curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip" \
  && unzip awscliv2.zip -d /usr/src && rm -f awscliv2.zip \
  && /usr/src/aws/install --bin-dir /usr/bin

## setup 
RUN mkdir /app/keys
COPY ./dist/upshot-node upshot-node
COPY ./dist/upshot-keys upshot-keys

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
ENV B7S_NODE_TYPE=upshot

## run script
ENTRYPOINT ["/app/run.sh"]