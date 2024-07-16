#!/bin/bash
set -eu

NETWORK="${NETWORK:-allora-testnet-1}"                 #! Replace with your network name
HEADS_URL="https://raw.githubusercontent.com/allora-network/networks/main/${NETWORK}/heads.txt"

export APP_HOME="${APP_HOME:-./data}"

INIT_FLAG="${APP_HOME}/.initialized"
if [ ! -f $INIT_FLAG ]; then
    echo "Generate p2p keys"
    rm -rf ${APP_HOME}/keys
    mkdir -p ${APP_HOME}/keys
    allora-keys

    touch $INIT_FLAG
fi

HEADS=$(curl -Ls ${HEADS_URL})

echo "Starting validator node"
allora-node \
    --role=head \
    --peer-db=$APP_HOME/peer-database \
    --function-db=$APP_HOME/function-database \
    --workspace=/tmp/node \
    --private-key=$APP_HOME/private.key \
    --port=9010 \
    --rest-api=:6000 \
    --dialback-address=$DIALBACK_ADDRESS \
    --dialback-port=$DIALBACK_PORT \
    --log-level=debug \
    --boot-nodes=$HEADS
