#!/bin/bash
# ============================================================================
# Kudora EVM Development Node Startup Script
# ============================================================================
# This script starts a local Kudora node with EVM JSON-RPC enabled
# for development and testing purposes.
#
# Usage:
#   ./scripts/start_evm.sh           # Start with existing data
#   ./scripts/start_evm.sh --clean   # Reset and start fresh
#
# Endpoints:
#   - Cosmos RPC:    http://localhost:26657
#   - Cosmos REST:   http://localhost:1317
#   - EVM JSON-RPC:  http://localhost:8545
#   - EVM WebSocket: ws://localhost:8546
# ============================================================================

set -e

# ============================================================================
# Configuration
# ============================================================================

CHAIN_ID="${CHAIN_ID:-kudora_12000-1}"
MONIKER="${MONIKER:-kudora-dev}"
KEYRING="${KEYRING:-test}"
HOME_DIR="${HOME_DIR:-$HOME/.kudora}"
BINARY="${BINARY:-kudorad}"
DENOM="${DENOM:-kud}"
BLOCK_TIME="${BLOCK_TIME:-2s}"

# EVM JSON-RPC configuration
JSON_RPC_ADDRESS="${JSON_RPC_ADDRESS:-0.0.0.0:8545}"
JSON_RPC_WS_ADDRESS="${JSON_RPC_WS_ADDRESS:-0.0.0.0:8546}"
JSON_RPC_API="${JSON_RPC_API:-eth,web3,net,txpool,debug,personal}"

# Logging colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# ============================================================================
# Helper Functions
# ============================================================================

log_info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_step()  { echo -e "${BLUE}[STEP]${NC} $1"; }

# ============================================================================
# Pre-flight Checks
# ============================================================================

log_step "Checking prerequisites..."

# Check if binary exists
if ! command -v $BINARY &> /dev/null; then
    log_error "$BINARY not found. Run 'ignite chain build' first."
    exit 1
fi

log_info "Using binary: $(which $BINARY)"
log_info "Binary version: $($BINARY version)"

# ============================================================================
# Clean Start (if requested)
# ============================================================================

if [ "$1" == "--clean" ] || [ "$CLEAN" == "true" ]; then
    log_step "Cleaning previous data..."
    rm -rf "$HOME_DIR"
    log_info "Removed $HOME_DIR"
fi

# ============================================================================
# Initialize Chain (if needed)
# ============================================================================

if [ ! -d "$HOME_DIR/config" ]; then
    log_step "Initializing new chain..."
    
    # Initialize the node
    $BINARY init $MONIKER \
        --chain-id $CHAIN_ID \
        --home $HOME_DIR \
        --default-denom $DENOM
    
    log_info "Node initialized with chain ID: $CHAIN_ID"
    
    # Create validator account
    log_step "Creating validator account..."
    $BINARY keys add validator \
        --keyring-backend $KEYRING \
        --home $HOME_DIR
    
    VALIDATOR_ADDR=$($BINARY keys show validator \
        --keyring-backend $KEYRING \
        --home $HOME_DIR -a)
    log_info "Validator address: $VALIDATOR_ADDR"
    
    # Add genesis account with initial balance
    # 1,000,000 kudos = 10^24 kud (with 18 decimals)
    log_step "Adding genesis account..."
    $BINARY genesis add-genesis-account $VALIDATOR_ADDR \
        1000000000000000000000000${DENOM} \
        --home $HOME_DIR
    
    # Create genesis transaction
    log_step "Creating genesis transaction..."
    $BINARY genesis gentx validator \
        100000000000000000000000${DENOM} \
        --chain-id $CHAIN_ID \
        --keyring-backend $KEYRING \
        --home $HOME_DIR
    
    # Collect genesis transactions
    $BINARY genesis collect-gentxs --home $HOME_DIR
    
    # Validate genesis
    $BINARY genesis validate-genesis --home $HOME_DIR
    log_info "Genesis validated successfully"
fi

# ============================================================================
# Configure Node
# ============================================================================

log_step "Configuring node settings..."

CONFIG_TOML="$HOME_DIR/config/config.toml"
APP_TOML="$HOME_DIR/config/app.toml"

# Set block time
if [ -f "$CONFIG_TOML" ]; then
    sed -i'' -e "s/timeout_commit = \".*\"/timeout_commit = \"$BLOCK_TIME\"/" "$CONFIG_TOML"
    log_info "Block time set to $BLOCK_TIME"
fi

# Enable API endpoints
if [ -f "$APP_TOML" ]; then
    # Enable REST API
    sed -i'' -e 's/enable = false/enable = true/' "$APP_TOML"
    # Enable Swagger
    sed -i'' -e 's/swagger = false/swagger = true/' "$APP_TOML"
fi

# ============================================================================
# Start Node
# ============================================================================

log_step "Starting Kudora node with EVM JSON-RPC..."
echo ""
log_info "============================================"
log_info "Kudora Development Node"
log_info "============================================"
log_info "Chain ID:      $CHAIN_ID"
log_info "EVM Chain ID:  12000 (0x2ee0)"
log_info ""
log_info "Endpoints:"
log_info "  Cosmos RPC:    http://localhost:26657"
log_info "  Cosmos REST:   http://localhost:1317"
log_info "  EVM JSON-RPC:  http://localhost:8545"
log_info "  EVM WebSocket: ws://localhost:8546"
log_info "============================================"
echo ""

# Start the node with EVM JSON-RPC enabled
$BINARY start \
    --home "$HOME_DIR" \
    --json-rpc.enable \
    --json-rpc.address="$JSON_RPC_ADDRESS" \
    --json-rpc.ws-address="$JSON_RPC_WS_ADDRESS" \
    --json-rpc.api="$JSON_RPC_API"