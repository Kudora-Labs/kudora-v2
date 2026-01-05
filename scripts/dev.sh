#!/bin/bash
# ============================================================================
# Quick Development Node Script
# ============================================================================
# Starts a fresh development node with minimal configuration.
# Perfect for rapid testing during development.
#
# Usage:
#   ./scripts/dev.sh          # Start fresh node
#   ./scripts/dev.sh --keep   # Keep existing data
# ============================================================================

set -e

HOME_DIR="$HOME/.kudora"
CHAIN_ID="kudora_12000-1"
BINARY="kudorad"

# Check for --keep flag
KEEP_DATA=false
if [ "$1" == "--keep" ]; then
    KEEP_DATA=true
fi

# Setup fresh chain if needed
if [ "$KEEP_DATA" == "false" ] || [ ! -d "$HOME_DIR/config" ]; then
    echo "Setting up fresh development node..."
    rm -rf "$HOME_DIR"
    
    # Initialize
    $BINARY init dev-node --chain-id $CHAIN_ID --default-denom kud --home $HOME_DIR
    
    # Create dev account
    $BINARY keys add dev --keyring-backend test --home $HOME_DIR
    DEV_ADDR=$($BINARY keys show dev --keyring-backend test --home $HOME_DIR -a)
    
    # Fund account
    $BINARY genesis add-genesis-account $DEV_ADDR 1000000000000000000000000000kud --home $HOME_DIR
    
    # Create validator
    $BINARY genesis gentx dev 100000000000000000000000kud \
        --chain-id $CHAIN_ID \
        --keyring-backend test \
        --home $HOME_DIR
    
    $BINARY genesis collect-gentxs --home $HOME_DIR
    
    # Fast blocks
    sed -i'' -e 's/timeout_commit = ".*"/timeout_commit = "1s"/' "$HOME_DIR/config/config.toml"
    
    echo ""
    echo "Dev account address: $DEV_ADDR"
    echo ""
fi

echo "Starting Kudora development node..."
echo ""
echo "Endpoints:"
echo "  Cosmos RPC:    http://localhost:26657"
echo "  REST API:      http://localhost:1317"
echo "  EVM JSON-RPC:  http://localhost:8545"
echo ""
echo "Press Ctrl+C to stop"
echo ""

$BINARY start \
    --home "$HOME_DIR" \
    --json-rpc.enable \
    --json-rpc.address="0.0.0.0:8545" \
    --json-rpc.ws-address="0.0.0.0:8546" \
    --json-rpc.api="eth,web3,net,txpool,debug,personal"