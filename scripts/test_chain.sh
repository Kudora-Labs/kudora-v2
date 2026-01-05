#!/bin/bash
# ============================================================================
# Kudora Chain Integration Test Script
# ============================================================================
# This script performs comprehensive testing of the Kudora blockchain
# including both Cosmos SDK and EVM functionality.
#
# Usage:
#   ./scripts/test_chain.sh
#
# Prerequisites:
#   - kudorad binary built and in PATH
#   - jq installed for JSON parsing
#   - curl installed for HTTP requests
# ============================================================================

set -e

# ============================================================================
# Configuration
# ============================================================================

CHAIN_ID="kudora_12000-1"
HOME_DIR="$HOME/.kudora"
BINARY="kudorad"
DENOM="kud"
KEYRING="test"

# Test accounts
VALIDATOR_NAME="validator"
USER_NAME="testuser"

# Ports
RPC_PORT=26657
REST_PORT=1317
JSON_RPC_PORT=8545
EVM_CHAIN_ID=12000

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Test counters
TESTS_PASSED=0
TESTS_FAILED=0

# ============================================================================
# Helper Functions
# ============================================================================

log_info()    { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn()    { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error()   { echo -e "${RED}[ERROR]${NC} $1"; }
log_test()    { echo -e "${BLUE}[TEST]${NC} $1"; }
log_success() { echo -e "${GREEN}[PASS]${NC} $1"; TESTS_PASSED=$((TESTS_PASSED + 1)); }
log_fail()    { echo -e "${RED}[FAIL]${NC} $1"; TESTS_FAILED=$((TESTS_FAILED + 1)); }

cleanup() {
    log_info "Cleaning up..."
    pkill -f "kudorad start" 2>/dev/null || true
    rm -rf "$HOME_DIR"
}

wait_for_node() {
    local max_attempts=30
    local attempt=0
    
    log_info "Waiting for node to start..."
    while [ $attempt -lt $max_attempts ]; do
        if curl -s "http://localhost:$RPC_PORT/status" > /dev/null 2>&1; then
            log_info "Node is ready!"
            return 0
        fi
        attempt=$((attempt + 1))
        sleep 1
    done
    
    log_error "Node failed to start within $max_attempts seconds"
    return 1
}

wait_for_blocks() {
    local target_blocks=$1
    local max_wait=60
    local waited=0
    
    log_info "Waiting for $target_blocks blocks..."
    while [ $waited -lt $max_wait ]; do
        local height=$(curl -s "http://localhost:$RPC_PORT/status" | jq -r '.result.sync_info.latest_block_height')
        if [ "$height" -ge "$target_blocks" ] 2>/dev/null; then
            log_info "Reached block height $height"
            return 0
        fi
        waited=$((waited + 1))
        sleep 1
    done
    
    log_error "Failed to reach $target_blocks blocks within $max_wait seconds"
    return 1
}

# ============================================================================
# Setup
# ============================================================================

setup_chain() {
    log_info "Setting up test chain..."
    
    # Clean previous test data
    rm -rf "$HOME_DIR"
    
    # Initialize chain
    $BINARY init test-node \
        --chain-id "$CHAIN_ID" \
        --default-denom "$DENOM" \
        --home "$HOME_DIR" \
        > /dev/null 2>&1
    
    # Create validator account
    $BINARY keys add "$VALIDATOR_NAME" \
        --keyring-backend "$KEYRING" \
        --home "$HOME_DIR" \
        > /dev/null 2>&1
    
    # Create test user account
    $BINARY keys add "$USER_NAME" \
        --keyring-backend "$KEYRING" \
        --home "$HOME_DIR" \
        > /dev/null 2>&1
    
    # Get addresses
    VALIDATOR_ADDR=$($BINARY keys show "$VALIDATOR_NAME" --keyring-backend "$KEYRING" --home "$HOME_DIR" -a)
    USER_ADDR=$($BINARY keys show "$USER_NAME" --keyring-backend "$KEYRING" --home "$HOME_DIR" -a)
    
    # Add genesis accounts
    # Validator: 1,000,000 kudos
    $BINARY genesis add-genesis-account "$VALIDATOR_ADDR" \
        1000000000000000000000000${DENOM} \
        --home "$HOME_DIR"
    
    # Test user: 100,000 kudos
    $BINARY genesis add-genesis-account "$USER_ADDR" \
        100000000000000000000000${DENOM} \
        --home "$HOME_DIR"
    
    # Create genesis transaction
    $BINARY genesis gentx "$VALIDATOR_NAME" \
        100000000000000000000000${DENOM} \
        --chain-id "$CHAIN_ID" \
        --keyring-backend "$KEYRING" \
        --home "$HOME_DIR" \
        > /dev/null 2>&1
    
    # Collect genesis transactions
    $BINARY genesis collect-gentxs --home "$HOME_DIR" > /dev/null 2>&1
    
    # Configure fast block time for testing
    sed -i'' -e 's/timeout_commit = ".*"/timeout_commit = "1s"/' "$HOME_DIR/config/config.toml"

    # Allow zero-fee transactions so the node accepts our test requests
    sed -i'' -e 's/minimum-gas-prices = ".*"/minimum-gas-prices = "0kud"/' "$HOME_DIR/config/app.toml"
    
    log_info "Chain setup complete"
    log_info "Validator address: $VALIDATOR_ADDR"
    log_info "Test user address: $USER_ADDR"
}

start_chain() {
    log_info "Starting chain..."
    
    $BINARY start \
        --home "$HOME_DIR" \
        --api.enable \
        --json-rpc.enable \
        --json-rpc.address="0.0.0.0:$JSON_RPC_PORT" \
        --json-rpc.api="eth,web3,net,txpool,debug,personal" \
        --evm.evm-chain-id "$EVM_CHAIN_ID" \
        > "$HOME_DIR/node.log" 2>&1 &
    
    wait_for_node
    wait_for_blocks 3
}

# ============================================================================
# Tests
# ============================================================================

test_binary_exists() {
    log_test "Binary exists and is executable"
    
    if command -v $BINARY &> /dev/null; then
        log_success "Binary found: $(which $BINARY)"
    else
        log_fail "Binary not found in PATH"
    fi
}

test_chain_id() {
    log_test "Chain ID is correctly configured"
    
    local chain_id=$(curl -s "http://localhost:$RPC_PORT/status" | jq -r '.result.node_info.network')
    
    if [ "$chain_id" == "$CHAIN_ID" ]; then
        log_success "Chain ID: $chain_id"
    else
        log_fail "Expected: $CHAIN_ID, Got: $chain_id"
    fi
}

test_block_production() {
    log_test "Chain is producing blocks"
    
    local height1=$(curl -s "http://localhost:$RPC_PORT/status" | jq -r '.result.sync_info.latest_block_height')
    sleep 3
    local height2=$(curl -s "http://localhost:$RPC_PORT/status" | jq -r '.result.sync_info.latest_block_height')
    
    if [ "$height2" -gt "$height1" ]; then
        log_success "Blocks produced: $height1 -> $height2"
    else
        log_fail "No new blocks produced"
    fi
}

test_address_prefix() {
    log_test "Addresses use 'kudo' prefix"
    
    local addr=$($BINARY keys show "$VALIDATOR_NAME" --keyring-backend "$KEYRING" --home "$HOME_DIR" -a)
    
    if [[ "$addr" == kudo1* ]]; then
        log_success "Address format correct: $addr"
    else
        log_fail "Address format incorrect: $addr (expected kudo1...)"
    fi
}

test_validator_prefix() {
    log_test "Validator addresses use 'kudovaloper' prefix"
    
    local addr=$($BINARY keys show "$VALIDATOR_NAME" --keyring-backend "$KEYRING" --home "$HOME_DIR" --bech val -a)
    
    if [[ "$addr" == kudovaloper1* ]]; then
        log_success "Validator address format correct: $addr"
    else
        log_fail "Validator address format incorrect: $addr"
    fi
}

test_json_rpc_chain_id() {
    log_test "EVM JSON-RPC returns correct chain ID"
    
    local response=$(curl -s -X POST "http://localhost:$JSON_RPC_PORT" \
        -H "Content-Type: application/json" \
        -d '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}')
    
    local chain_id=$(echo "$response" | jq -r '.result')
    
    # App computes EVM chain ID as FNV-1a of the Cosmos chain-id (see app/evm.go cosmosChainIDToEVMChainID)
    local expected="0x9a693e81"
    if [ "$chain_id" == "$expected" ]; then
        log_success "EVM Chain ID: $chain_id (hashed from $CHAIN_ID)"
    else
        log_fail "Expected: $expected, Got: $chain_id"
    fi
}

test_json_rpc_block_number() {
    log_test "EVM JSON-RPC returns block number"
    
    local response=$(curl -s -X POST "http://localhost:$JSON_RPC_PORT" \
        -H "Content-Type: application/json" \
        -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}')
    
    local block_hex=$(echo "$response" | jq -r '.result')
    local block_dec=$((block_hex))
    
    if [ "$block_dec" -gt 0 ]; then
        log_success "EVM Block Number: $block_hex ($block_dec)"
    else
        log_fail "Invalid block number: $block_hex"
    fi
}

test_json_rpc_gas_price() {
    log_test "EVM JSON-RPC returns gas price"
    
    local response=$(curl -s -X POST "http://localhost:$JSON_RPC_PORT" \
        -H "Content-Type: application/json" \
        -d '{"jsonrpc":"2.0","method":"eth_gasPrice","params":[],"id":1}')
    
    local gas_price=$(echo "$response" | jq -r '.result')
    
    if [ "$gas_price" != "null" ] && [ -n "$gas_price" ]; then
        log_success "Gas Price: $gas_price"
    else
        log_fail "Failed to get gas price"
    fi
}

test_cosmos_bank_transfer() {
    log_test "Cosmos bank transfer"
    
    # Send 1000 kudos from validator to user
    local result=$($BINARY tx bank send \
        "$VALIDATOR_NAME" \
        "$USER_ADDR" \
        1000000000000000000000${DENOM} \
        --chain-id "$CHAIN_ID" \
        --keyring-backend "$KEYRING" \
        --home "$HOME_DIR" \
        --gas auto \
        --gas-adjustment 1.5 \
        --fees 1000000000000000${DENOM} \
        -y \
        --output json 2>/dev/null)
    
    local code=$(echo "$result" | jq -r '.code // 0')
    
    if [ "$code" == "0" ]; then
        log_success "Bank transfer submitted successfully"
        
        # Wait for transaction to be included
        sleep 3
        
        # Verify balance
        local balance=$($BINARY query bank balances "$USER_ADDR" --home "$HOME_DIR" --output json | jq -r ".balances[0].amount")
        log_info "User balance after transfer: $balance $DENOM"
    else
        log_fail "Bank transfer failed with code: $code"
    fi
}

test_query_balance() {
    log_test "Query account balance"
    
    local balance=$($BINARY query bank balances "$VALIDATOR_ADDR" --home "$HOME_DIR" --output json)
    local amount=$(echo "$balance" | jq -r '.balances[0].amount')
    
    if [ -n "$amount" ] && [ "$amount" != "null" ]; then
        log_success "Balance query successful: $amount $DENOM"
    else
        log_fail "Failed to query balance"
    fi
}

test_rest_api() {
    log_test "REST API is accessible"
    
    local response=$(curl -s "http://localhost:$REST_PORT/cosmos/base/tendermint/v1beta1/node_info")
    local moniker=$(echo "$response" | jq -r '.default_node_info.moniker')
    
    if [ "$moniker" == "test-node" ]; then
        log_success "REST API accessible, node moniker: $moniker"
    else
        log_fail "REST API returned unexpected response"
    fi
}

# ============================================================================
# Main Execution
# ============================================================================

main() {
    echo ""
    echo "============================================"
    echo "    Kudora Chain Integration Tests"
    echo "============================================"
    echo ""
    
    # Setup
    trap cleanup EXIT
    
    log_info "Starting test suite..."
    echo ""
    
    # Pre-chain tests
    test_binary_exists
    
    # Setup and start chain
    setup_chain
    start_chain
    
    echo ""
    log_info "Running chain tests..."
    echo ""
    
    # Chain tests
    test_chain_id
    test_block_production
    test_address_prefix
    test_validator_prefix
    test_query_balance
    test_rest_api
    
    echo ""
    log_info "Running EVM tests..."
    echo ""
    
    # EVM tests
    test_json_rpc_chain_id
    test_json_rpc_block_number
    test_json_rpc_gas_price
    
    echo ""
    log_info "Running transaction tests..."
    echo ""
    
    # Transaction tests
    test_cosmos_bank_transfer
    
    # Summary
    echo ""
    echo "============================================"
    echo "    Test Summary"
    echo "============================================"
    echo ""
    log_info "Tests passed: $TESTS_PASSED"
    if [ $TESTS_FAILED -gt 0 ]; then
        log_error "Tests failed: $TESTS_FAILED"
        exit 1
    else
        log_success "All tests passed!"
        exit 0
    fi
}

# Run main function
main "$@"