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

ensure_wasm_artifact() {
    local artifact="$HOME_DIR/cw20_base.wasm"
    local url="https://github.com/CosmWasm/cw-plus/releases/download/v1.1.0/cw20_base.wasm"

    if [ -f "$artifact" ]; then
        return 0
    fi

    log_info "Fetching cw20_base.wasm for WASM tests..."
    if ! curl -fL "$url" -o "$artifact"; then
        log_fail "Failed to download cw20_base.wasm from $url"
        return 1
    fi
}

ensure_tokenfactory_wasm_artifact() {
    local artifact="$HOME_DIR/tokenfactory.wasm"
    local url="https://raw.githubusercontent.com/cosmos/tokenfactory/main/x/tokenfactory/bindings/testdata/tokenfactory.wasm"

    if [ -f "$artifact" ]; then
        return 0
    fi

    log_info "Fetching tokenfactory.wasm for binding tests..."
    if ! curl -fL "$url" -o "$artifact"; then
        log_fail "Failed to download tokenfactory.wasm from $url"
        return 1
    fi
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
    VALIDATOR_VAL_ADDR=$($BINARY keys show "$VALIDATOR_NAME" --keyring-backend "$KEYRING" --home "$HOME_DIR" --bech val -a)
    
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
    log_info "Validator operator address: $VALIDATOR_VAL_ADDR"
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
        --output json 2>&1 || true)
    
    local json=$(echo "$result" | sed -n '/^{/,$p')
    if [ -z "$json" ]; then
        log_fail "Bank transfer failed (non-JSON response): $result"
        return
    fi
    
    local code
    if ! code=$(echo "$json" | jq -r '.code // 0' 2>/dev/null); then
        log_fail "Bank transfer failed (invalid JSON): $result"
        return
    fi
    
    if [ "$code" == "0" ]; then
        log_success "Bank transfer submitted successfully"
        
        # Wait for transaction to be included
        sleep 3
        
        # Verify balance
        local balance=$($BINARY query bank balances "$USER_ADDR" --home "$HOME_DIR" --output json | jq -r ".balances[0].amount")
        log_info "User balance after transfer: $balance $DENOM"
    else
        local raw_log=$(echo "$json" | jq -r '.raw_log // empty')
        log_fail "Bank transfer failed with code: $code ${raw_log:+- $raw_log}"
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

test_tokenfactory_mint() {
    log_test "Tokenfactory create denom and mint"

    local subdenom="testdenom"

    local create_out=$($BINARY tx tokenfactory create-denom "$subdenom" \
        --from "$VALIDATOR_NAME" \
        --chain-id "$CHAIN_ID" \
        --keyring-backend "$KEYRING" \
        --home "$HOME_DIR" \
        --gas auto \
        --gas-adjustment 1.5 \
        --fees 1000000000000000${DENOM} \
        -y \
        --output json 2>&1)

    local create_json=$(echo "$create_out" | sed -n '/^{/,$p')
    if [ -z "$create_json" ]; then
        log_fail "Tokenfactory create-denom failed (non-JSON response): $create_out"
        return
    fi

    local create_code
    if ! create_code=$(echo "$create_json" | jq -r '.code // 0' 2>/dev/null); then
        log_fail "Tokenfactory create-denom failed (invalid JSON): $create_out"
        return
    fi

    if [ "$create_code" != "0" ]; then
        local raw_log=$(echo "$create_json" | jq -r '.raw_log // empty')
        log_fail "Tokenfactory create-denom failed with code: $create_code ${raw_log:+- $raw_log}"
        return
    fi

    sleep 2

    local denoms_json=$($BINARY query tokenfactory denoms-from-creator "$VALIDATOR_ADDR" --home "$HOME_DIR" --output json 2>/dev/null)
    local denom=$(echo "$denoms_json" | jq -r '.denoms[0] // empty')

    if [ -z "$denom" ]; then
        log_fail "Tokenfactory denoms-from-creator returned no denoms: $denoms_json"
        return
    fi

    TOKENFACTORY_DENOM="$denom"

    local mint_amount="5000000000000000000" # 5 tokens with 18 decimals
    local mint_out=$($BINARY tx tokenfactory mint "${mint_amount}${denom}" \
        --from "$VALIDATOR_NAME" \
        --chain-id "$CHAIN_ID" \
        --keyring-backend "$KEYRING" \
        --home "$HOME_DIR" \
        --gas auto \
        --gas-adjustment 1.5 \
        --fees 1000000000000000${DENOM} \
        -y \
        --output json 2>&1)

    local mint_json=$(echo "$mint_out" | sed -n '/^{/,$p')
    if [ -z "$mint_json" ]; then
        log_fail "Tokenfactory mint failed (non-JSON response): $mint_out"
        return
    fi

    local mint_code
    if ! mint_code=$(echo "$mint_json" | jq -r '.code // 0' 2>/dev/null); then
        log_fail "Tokenfactory mint failed (invalid JSON): $mint_out"
        return
    fi

    if [ "$mint_code" != "0" ]; then
        local raw_log=$(echo "$mint_json" | jq -r '.raw_log // empty')
        log_fail "Tokenfactory mint failed with code: $mint_code ${raw_log:+- $raw_log}"
        return
    fi

    sleep 2

    local balances_json=$($BINARY query bank balances "$VALIDATOR_ADDR" --home "$HOME_DIR" --output json 2>/dev/null)
    local minted_balance=$(echo "$balances_json" | jq -r --arg denom "$denom" '.balances[] | select(.denom==$denom) | .amount' 2>/dev/null)

    if [ "$minted_balance" = "$mint_amount" ]; then
        log_success "Tokenfactory mint successful: $minted_balance $denom to $VALIDATOR_ADDR"
    else
        log_fail "Tokenfactory balance mismatch: expected $mint_amount got ${minted_balance:-none}; balances: $balances_json"
    fi
}

test_tokenfactory_burn() {
    log_test "Tokenfactory burn reduces balance"

    if [ -z "$TOKENFACTORY_DENOM" ]; then
        log_fail "Tokenfactory burn missing denom (mint must run first)"
        return
    fi

    local burn_amount="2000000000000000000" # burn 2 tokens

    local before_json=$($BINARY query bank balances "$VALIDATOR_ADDR" --home "$HOME_DIR" --output json 2>/dev/null)
    local before_balance=$(echo "$before_json" | jq -r --arg denom "$TOKENFACTORY_DENOM" '.balances[] | select(.denom==$denom) | .amount' 2>/dev/null)

    if [ -z "$before_balance" ]; then
        log_fail "Tokenfactory burn missing existing balance for $TOKENFACTORY_DENOM: $before_json"
        return
    fi

    local burn_out=$($BINARY tx tokenfactory burn "${burn_amount}${TOKENFACTORY_DENOM}" \
        --from "$VALIDATOR_NAME" \
        --chain-id "$CHAIN_ID" \
        --keyring-backend "$KEYRING" \
        --home "$HOME_DIR" \
        --gas auto \
        --gas-adjustment 1.5 \
        --fees 1000000000000000${DENOM} \
        -y \
        --output json 2>&1)

    local burn_json=$(echo "$burn_out" | sed -n '/^{/,$p')
    if [ -z "$burn_json" ]; then
        log_fail "Tokenfactory burn failed (non-JSON response): $burn_out"
        return
    fi

    local burn_code
    if ! burn_code=$(echo "$burn_json" | jq -r '.code // 0' 2>/dev/null); then
        log_fail "Tokenfactory burn failed (invalid JSON): $burn_out"
        return
    fi

    if [ "$burn_code" != "0" ]; then
        local raw_log=$(echo "$burn_json" | jq -r '.raw_log // empty')
        log_fail "Tokenfactory burn failed with code: $burn_code ${raw_log:+- $raw_log}"
        return
    fi

    sleep 2

    local expected=$(BEFORE_BAL="$before_balance" BURN_AMT="$burn_amount" python3 - <<'PY'
import os
before = int(os.environ['BEFORE_BAL'])
burn = int(os.environ['BURN_AMT'])
print(before - burn)
PY
    )

    local after_json=$($BINARY query bank balances "$VALIDATOR_ADDR" --home "$HOME_DIR" --output json 2>/dev/null)
    local after_balance=$(echo "$after_json" | jq -r --arg denom "$TOKENFACTORY_DENOM" '.balances[] | select(.denom==$denom) | .amount' 2>/dev/null)

    if [ "$after_balance" = "$expected" ]; then
        log_success "Tokenfactory burn successful: balance $after_balance $TOKENFACTORY_DENOM (burned $burn_amount)"
    else
        log_fail "Tokenfactory burn mismatch: expected $expected got ${after_balance:-none}; balances: $after_json"
    fi
}

test_tokenfactory_nonadmin_forbidden() {
    log_test "Tokenfactory mint forbidden for non-admin"

    if [ -z "$TOKENFACTORY_DENOM" ]; then
        log_fail "Tokenfactory non-admin test missing denom (mint must run first)"
        return
    fi

    local mint_amount="1000000000000000000" # 1 token

    local out=$($BINARY tx tokenfactory mint "${mint_amount}${TOKENFACTORY_DENOM}" \
        --from "$USER_NAME" \
        --chain-id "$CHAIN_ID" \
        --keyring-backend "$KEYRING" \
        --home "$HOME_DIR" \
        --gas auto \
        --gas-adjustment 1.5 \
        --fees 1000000000000000${DENOM} \
        -y \
        --output json 2>&1 || true)

    # Check if error message contains "unauthorized" or similar
    if echo "$out" | grep -qi "unauthorized"; then
        log_success "Tokenfactory non-admin mint correctly rejected (unauthorized)"
        return
    fi

    local json=$(echo "$out" | sed -n '/^{/,$p')
    if [ -z "$json" ]; then
        log_fail "Tokenfactory non-admin mint produced unexpected output: $out"
        return
    fi

    local code
    if ! code=$(echo "$json" | jq -r '.code // 0' 2>/dev/null); then
        log_fail "Tokenfactory non-admin mint invalid JSON: $out"
        return
    fi

    if [ "$code" = "0" ]; then
        log_fail "Tokenfactory non-admin mint unexpectedly succeeded"
    else
        local raw_log=$(echo "$json" | jq -r '.raw_log // empty')
        log_success "Tokenfactory non-admin mint correctly rejected (code $code${raw_log:+, $raw_log})"
    fi
}

test_staking_delegation() {
    log_test "Cosmos staking delegation from user to validator"

    local amount="100000000000000000000${DENOM}" # 100 kud (18 decimals)

    local result=$($BINARY tx staking delegate \
        "$VALIDATOR_VAL_ADDR" \
        "$amount" \
        --from "$USER_NAME" \
        --chain-id "$CHAIN_ID" \
        --keyring-backend "$KEYRING" \
        --home "$HOME_DIR" \
        --gas auto \
        --gas-adjustment 1.5 \
        --fees 1000000000000000${DENOM} \
        -y \
        --output json 2>&1)

    local json=$(echo "$result" | sed -n '/^{/,$p')
    local code
    if [ -z "$json" ]; then
        log_fail "Staking delegation failed (non-JSON response): $result"
        return
    fi

    if ! code=$(echo "$json" | jq -r '.code // 0' 2>/dev/null); then
        log_fail "Staking delegation failed (invalid JSON): $result"
        return
    fi

    if [ "$code" != "0" ]; then
        local raw_log=$(echo "$json" | jq -r '.raw_log // empty')
        log_fail "Staking delegation failed with code: $code ${raw_log:+- $raw_log}"
        return
    fi

    sleep 3

    local delegation_output
    if ! delegation_output=$($BINARY query staking delegation "$USER_ADDR" "$VALIDATOR_VAL_ADDR" --home "$HOME_DIR" --output json 2>/dev/null); then
        log_fail "Delegation query failed"
        return
    fi

    local delegated_amount=$(echo "$delegation_output" | jq -r '.delegation_response.balance.amount // empty' 2>/dev/null)

    if [ -n "$delegated_amount" ] && [ "$delegated_amount" != "null" ]; then
        log_success "Delegation successful: $delegated_amount $DENOM delegated to $VALIDATOR_NAME"
    else
        log_fail "Delegation query returned unexpected result: $delegation_output"
    fi
}

test_wasm_store_and_instantiate() {
    log_test "WASM store, instantiate, and query cw20 contract"
    
        # Disable errexit locally to surface detailed failures without aborting the suite
        set +e

    if ! ensure_wasm_artifact; then
           set -e
           return
    fi

    local store_out
    store_out=$($BINARY tx wasm store "$HOME_DIR/cw20_base.wasm" \
        --from "$VALIDATOR_NAME" \
        --chain-id "$CHAIN_ID" \
        --keyring-backend "$KEYRING" \
        --home "$HOME_DIR" \
        --gas auto \
        --gas-adjustment 1.5 \
        --fees 2000000000000000${DENOM} \
        -y \
        --output json 2>&1 || true)

    local store_json=$(echo "$store_out" | sed -n '/^{/,$p')
    if [ -z "$store_json" ]; then
        log_fail "WASM store failed (non-JSON response): $store_out"
        return
    fi

    local store_code
    if ! store_code=$(echo "$store_json" | jq -r '.code // 0' 2>/dev/null); then
        log_fail "WASM store failed (invalid JSON): $store_out"
        return
    fi

    if [ "$store_code" != "0" ]; then
        local raw_log=$(echo "$store_json" | jq -r '.raw_log // empty')
        log_fail "WASM store failed with code: $store_code ${raw_log:+- $raw_log}"
        set -e
        return
    fi

    local txhash=$(echo "$store_json" | jq -r '.txhash // empty')
    if [ -z "$txhash" ] || [ "$txhash" = "null" ]; then
        log_fail "WASM store did not return a txhash: $store_json"
        set -e
        return
    fi

    sleep 3

    local tx_query
    tx_query=$($BINARY query tx "$txhash" --home "$HOME_DIR" --output json 2>/dev/null || true)
    
    WASM_CODE_ID=$(echo "$tx_query" | jq -r '.events[] | select(.type=="store_code") | .attributes[] | select(.key=="code_id") | .value' 2>/dev/null)
    if [ -z "$WASM_CODE_ID" ] || [ "$WASM_CODE_ID" = "null" ]; then
        log_fail "WASM store did not return a code_id in tx query: $tx_query"
        set -e
        return
    fi

    local init_msg
    init_msg=$(cat <<EOF
{"name":"Kudora Test Token","symbol":"KTK","decimals":6,"initial_balances":[{"address":"$USER_ADDR","amount":"1000"}]}
EOF
    )

    local instantiate_out
    instantiate_out=$($BINARY tx wasm instantiate "$WASM_CODE_ID" "$init_msg" \
        --label "cw20-test" \
        --no-admin \
        --from "$VALIDATOR_NAME" \
        --chain-id "$CHAIN_ID" \
        --keyring-backend "$KEYRING" \
        --home "$HOME_DIR" \
        --gas auto \
        --gas-adjustment 1.5 \
        --fees 2000000000000000${DENOM} \
        -y \
        --output json 2>&1 || true)

    local instantiate_json=$(echo "$instantiate_out" | sed -n '/^{/,$p')
    if [ -z "$instantiate_json" ]; then
        log_fail "WASM instantiate failed (non-JSON response): $instantiate_out"
        return
    fi

    local instantiate_code
    if ! instantiate_code=$(echo "$instantiate_json" | jq -r '.code // 0' 2>/dev/null); then
        log_fail "WASM instantiate failed (invalid JSON): $instantiate_out"
        return
    fi

    if [ "$instantiate_code" != "0" ]; then
        local raw_log=$(echo "$instantiate_json" | jq -r '.raw_log // empty')
        log_fail "WASM instantiate failed with code: $instantiate_code ${raw_log:+- $raw_log}"
        set -e
        return
    fi

    local inst_txhash=$(echo "$instantiate_json" | jq -r '.txhash // empty')
    if [ -z "$inst_txhash" ] || [ "$inst_txhash" = "null" ]; then
        log_fail "WASM instantiate did not return a txhash: $instantiate_json"
        set -e
        return
    fi

    sleep 3

    local inst_tx_query
    inst_tx_query=$($BINARY query tx "$inst_txhash" --home "$HOME_DIR" --output json 2>/dev/null || true)
    
    WASM_CONTRACT_ADDR=$(echo "$inst_tx_query" | jq -r '.events[] | select(.type=="instantiate") | .attributes[] | select(.key=="_contract_address" or .key=="contract_address") | .value' 2>/dev/null)
    if [ -z "$WASM_CONTRACT_ADDR" ] || [ "$WASM_CONTRACT_ADDR" = "null" ]; then
        log_fail "WASM instantiate did not return a contract address in tx query: $inst_tx_query"
        set -e
        return
    fi

    sleep 2

    local balance_json
    balance_json=$($BINARY query wasm contract-state smart "$WASM_CONTRACT_ADDR" "{\"balance\":{\"address\":\"$USER_ADDR\"}}" \
        --home "$HOME_DIR" \
        --output json 2>/dev/null || true)

    local cw20_balance=$(echo "$balance_json" | jq -r '.data.balance // empty' 2>/dev/null)

    if [ "$cw20_balance" = "1000" ]; then
        log_success "WASM cw20 instantiated successfully: balance $cw20_balance for $USER_ADDR"
    else
        log_fail "WASM cw20 balance query mismatch: expected 1000 got ${cw20_balance:-none}; response: $balance_json"
    fi
    
        # Re-enable errexit for subsequent steps
        set -e
}

test_wasm_cw20_token_info() {
    log_test "WASM query CW20 token metadata"
    
    set +e
    
    if [ -z "$WASM_CONTRACT_ADDR" ]; then
        log_fail "WASM token info test missing contract address (instantiate must run first)"
        set -e
        return
    fi
    
    local token_info_json
    token_info_json=$($BINARY query wasm contract-state smart "$WASM_CONTRACT_ADDR" '{"token_info":{}}' \
        --home "$HOME_DIR" \
        --output json 2>/dev/null || true)
    
    local name=$(echo "$token_info_json" | jq -r '.data.name // empty' 2>/dev/null)
    local symbol=$(echo "$token_info_json" | jq -r '.data.symbol // empty' 2>/dev/null)
    local decimals=$(echo "$token_info_json" | jq -r '.data.decimals // empty' 2>/dev/null)
    local total_supply=$(echo "$token_info_json" | jq -r '.data.total_supply // empty' 2>/dev/null)
    
    if [ "$name" = "Kudora Test Token" ] && [ "$symbol" = "KTK" ] && [ "$decimals" = "6" ] && [ "$total_supply" = "1000" ]; then
        log_success "WASM token info correct: $name ($symbol), $decimals decimals, supply $total_supply"
    else
        log_fail "WASM token info mismatch: name=$name symbol=$symbol decimals=$decimals supply=$total_supply; response: $token_info_json"
    fi
    
    set -e
}

test_wasm_cw20_transfer() {
    log_test "WASM CW20 token transfer between accounts"
    
    set +e
    
    if [ -z "$WASM_CONTRACT_ADDR" ]; then
        log_fail "WASM transfer test missing contract address (instantiate must run first)"
        set -e
        return
    fi
    
    # Check initial balances
    local user_balance_before
    user_balance_before=$($BINARY query wasm contract-state smart "$WASM_CONTRACT_ADDR" "{\"balance\":{\"address\":\"$USER_ADDR\"}}" \
        --home "$HOME_DIR" \
        --output json 2>/dev/null || true)
    local user_bal_before=$(echo "$user_balance_before" | jq -r '.data.balance // empty' 2>/dev/null)
    
    local validator_balance_before
    validator_balance_before=$($BINARY query wasm contract-state smart "$WASM_CONTRACT_ADDR" "{\"balance\":{\"address\":\"$VALIDATOR_ADDR\"}}" \
        --home "$HOME_DIR" \
        --output json 2>/dev/null || true)
    local validator_bal_before=$(echo "$validator_balance_before" | jq -r '.data.balance // "0"' 2>/dev/null)
    
    # Transfer 300 tokens from USER to VALIDATOR
    local transfer_amount="300"
    local transfer_msg="{\"transfer\":{\"recipient\":\"$VALIDATOR_ADDR\",\"amount\":\"$transfer_amount\"}}"
    
    local transfer_out
    transfer_out=$($BINARY tx wasm execute "$WASM_CONTRACT_ADDR" "$transfer_msg" \
        --from "$USER_NAME" \
        --chain-id "$CHAIN_ID" \
        --keyring-backend "$KEYRING" \
        --home "$HOME_DIR" \
        --gas auto \
        --gas-adjustment 1.5 \
        --fees 1000000000000000${DENOM} \
        -y \
        --output json 2>&1 || true)
    
    local transfer_json=$(echo "$transfer_out" | sed -n '/^{/,$p')
    if [ -z "$transfer_json" ]; then
        log_fail "WASM transfer failed (non-JSON response): $transfer_out"
        set -e
        return
    fi
    
    local transfer_code
    if ! transfer_code=$(echo "$transfer_json" | jq -r '.code // 0' 2>/dev/null); then
        log_fail "WASM transfer failed (invalid JSON): $transfer_out"
        set -e
        return
    fi
    
    if [ "$transfer_code" != "0" ]; then
        local raw_log=$(echo "$transfer_json" | jq -r '.raw_log // empty')
        log_fail "WASM transfer failed with code: $transfer_code ${raw_log:+- $raw_log}"
        set -e
        return
    fi
    
    sleep 3
    
    # Check final balances
    local user_balance_after
    user_balance_after=$($BINARY query wasm contract-state smart "$WASM_CONTRACT_ADDR" "{\"balance\":{\"address\":\"$USER_ADDR\"}}" \
        --home "$HOME_DIR" \
        --output json 2>/dev/null || true)
    local user_bal_after=$(echo "$user_balance_after" | jq -r '.data.balance // empty' 2>/dev/null)
    
    local validator_balance_after
    validator_balance_after=$($BINARY query wasm contract-state smart "$WASM_CONTRACT_ADDR" "{\"balance\":{\"address\":\"$VALIDATOR_ADDR\"}}" \
        --home "$HOME_DIR" \
        --output json 2>/dev/null || true)
    local validator_bal_after=$(echo "$validator_balance_after" | jq -r '.data.balance // empty' 2>/dev/null)
    
    # Verify balances changed correctly
    local expected_user=$((user_bal_before - transfer_amount))
    local expected_validator=$((validator_bal_before + transfer_amount))
    
    if [ "$user_bal_after" = "$expected_user" ] && [ "$validator_bal_after" = "$expected_validator" ]; then
        log_success "WASM transfer successful: $USER_NAME $user_bal_after (was $user_bal_before), $VALIDATOR_NAME $validator_bal_after (was $validator_bal_before)"
    else
        log_fail "WASM transfer balance mismatch: user expected $expected_user got $user_bal_after, validator expected $expected_validator got $validator_bal_after"
    fi
    
    set -e
}

test_wasm_cw20_insufficient_funds() {
    log_test "WASM CW20 transfer fails with insufficient balance"
    
    set +e
    
    if [ -z "$WASM_CONTRACT_ADDR" ]; then
        log_fail "WASM insufficient funds test missing contract address (instantiate must run first)"
        set -e
        return
    fi
    
    # Check current balance of validator (should be 300 from previous transfer)
    local current_balance
    current_balance=$($BINARY query wasm contract-state smart "$WASM_CONTRACT_ADDR" "{\"balance\":{\"address\":\"$VALIDATOR_ADDR\"}}" \
        --home "$HOME_DIR" \
        --output json 2>/dev/null || true)
    local balance=$(echo "$current_balance" | jq -r '.data.balance // "0"' 2>/dev/null)
    
    # Try to transfer more than available balance
    local excessive_amount="1000"
    local transfer_msg="{\"transfer\":{\"recipient\":\"$USER_ADDR\",\"amount\":\"$excessive_amount\"}}"
    
    local transfer_out
    transfer_out=$($BINARY tx wasm execute "$WASM_CONTRACT_ADDR" "$transfer_msg" \
        --from "$VALIDATOR_NAME" \
        --chain-id "$CHAIN_ID" \
        --keyring-backend "$KEYRING" \
        --home "$HOME_DIR" \
        --gas auto \
        --gas-adjustment 1.5 \
        --fees 1000000000000000${DENOM} \
        -y \
        --output json 2>&1 || true)
    
    # Check if error contains "insufficient" or similar
    if echo "$transfer_out" | grep -qi "insufficient\|cannot subtract\|overflow"; then
        log_success "WASM transfer correctly rejected (insufficient funds: balance=$balance, attempted=$excessive_amount)"
        set -e
        return
    fi
    
    local transfer_json=$(echo "$transfer_out" | sed -n '/^{/,$p')
    if [ -z "$transfer_json" ]; then
        log_fail "WASM insufficient funds test produced unexpected output: $transfer_out"
        set -e
        return
    fi
    
    local transfer_code
    if ! transfer_code=$(echo "$transfer_json" | jq -r '.code // 0' 2>/dev/null); then
        log_fail "WASM insufficient funds test invalid JSON: $transfer_out"
        set -e
        return
    fi
    
    if [ "$transfer_code" = "0" ]; then
        log_fail "WASM transfer with insufficient funds unexpectedly succeeded"
    else
        local raw_log=$(echo "$transfer_json" | jq -r '.raw_log // empty')
        log_success "WASM transfer correctly rejected (code $transfer_code, balance=$balance, attempted=$excessive_amount)"
    fi
    
    set -e
}

test_wasm_cw20_all_accounts() {
    log_test "WASM CW20 verify all account balances"
    
    set +e
    
    if [ -z "$WASM_CONTRACT_ADDR" ]; then
        log_fail "WASM all accounts test missing contract address (instantiate must run first)"
        set -e
        return
    fi
    
    # Query all accounts and verify final state
    local user_balance
    user_balance=$($BINARY query wasm contract-state smart "$WASM_CONTRACT_ADDR" "{\"balance\":{\"address\":\"$USER_ADDR\"}}" \
        --home "$HOME_DIR" \
        --output json 2>/dev/null || true)
    local user_bal=$(echo "$user_balance" | jq -r '.data.balance // empty' 2>/dev/null)
    
    local validator_balance
    validator_balance=$($BINARY query wasm contract-state smart "$WASM_CONTRACT_ADDR" "{\"balance\":{\"address\":\"$VALIDATOR_ADDR\"}}" \
        --home "$HOME_DIR" \
        --output json 2>/dev/null || true)
    local validator_bal=$(echo "$validator_balance" | jq -r '.data.balance // empty' 2>/dev/null)
    
    # Get token info for total supply verification
    local token_info
    token_info=$($BINARY query wasm contract-state smart "$WASM_CONTRACT_ADDR" '{"token_info":{}}' \
        --home "$HOME_DIR" \
        --output json 2>/dev/null || true)
    local total_supply=$(echo "$token_info" | jq -r '.data.total_supply // empty' 2>/dev/null)
    
    # Expected state after all transfers:
    # - Initial: USER=1000, VALIDATOR=0
    # - After transfer: USER=700, VALIDATOR=300
    # - Failed transfer (insufficient funds) should not change balances
    local expected_user="700"
    local expected_validator="300"
    local expected_total="1000"
    
    # Calculate actual total from all balances
    local actual_total=$((user_bal + validator_bal))
    
    if [ "$user_bal" = "$expected_user" ] && [ "$validator_bal" = "$expected_validator" ] && [ "$total_supply" = "$expected_total" ] && [ "$actual_total" = "$expected_total" ]; then
        log_success "WASM all accounts verified: user=$user_bal, validator=$validator_bal, total_supply=$total_supply (sum=$actual_total)"
    else
        log_fail "WASM account verification failed: user expected $expected_user got $user_bal, validator expected $expected_validator got $validator_bal, supply expected $expected_total got $total_supply (sum=$actual_total)"
    fi
    
    set -e
}

test_wasm_tokenfactory_bindings() {
    log_test "WASM tokenfactory bindings: create denom and mint"

    set +e

    if ! ensure_tokenfactory_wasm_artifact; then
        set -e
        return
    fi

    local artifact="$HOME_DIR/tokenfactory.wasm"

    local store_out
    store_out=$($BINARY tx wasm store "$artifact" \
        --from "$VALIDATOR_NAME" \
        --chain-id "$CHAIN_ID" \
        --keyring-backend "$KEYRING" \
        --home "$HOME_DIR" \
        --gas auto \
        --gas-adjustment 1.5 \
        --fees 2000000000000000${DENOM} \
        -y \
        --output json 2>&1 || true)

    local store_json=$(echo "$store_out" | sed -n '/^{/,$p')
    if [ -z "$store_json" ]; then
        log_fail "Tokenfactory wasm store failed (non-JSON response): $store_out"
        set -e
        return
    fi

    local store_code
    if ! store_code=$(echo "$store_json" | jq -r '.code // 0' 2>/dev/null); then
        log_fail "Tokenfactory wasm store failed (invalid JSON): $store_out"
        set -e
        return
    fi

    if [ "$store_code" != "0" ]; then
        local raw_log=$(echo "$store_json" | jq -r '.raw_log // empty')
        log_fail "Tokenfactory wasm store failed with code: $store_code ${raw_log:+- $raw_log}"
        set -e
        return
    fi

    local store_txhash=$(echo "$store_json" | jq -r '.txhash // empty')
    if [ -z "$store_txhash" ] || [ "$store_txhash" = "null" ]; then
        log_fail "Tokenfactory wasm store missing txhash: $store_json"
        set -e
        return
    fi

    sleep 3

    local store_tx_query
    store_tx_query=$($BINARY query tx "$store_txhash" --home "$HOME_DIR" --output json 2>/dev/null || true)

    local tf_code_id
    tf_code_id=$(echo "$store_tx_query" | jq -r '.events[] | select(.type=="store_code") | .attributes[] | select(.key=="code_id") | .value' 2>/dev/null)
    if [ -z "$tf_code_id" ] || [ "$tf_code_id" = "null" ]; then
        log_fail "Tokenfactory wasm store did not return a code_id: $store_tx_query"
        set -e
        return
    fi

    local instantiate_out
    instantiate_out=$($BINARY tx wasm instantiate "$tf_code_id" "{}" \
        --label "tf-bindings" \
        --no-admin \
        --from "$VALIDATOR_NAME" \
        --chain-id "$CHAIN_ID" \
        --keyring-backend "$KEYRING" \
        --home "$HOME_DIR" \
        --gas auto \
        --gas-adjustment 1.5 \
        --fees 2000000000000000${DENOM} \
        -y \
        --output json 2>&1 || true)

    local inst_json=$(echo "$instantiate_out" | sed -n '/^{/,$p')
    if [ -z "$inst_json" ]; then
        log_fail "Tokenfactory wasm instantiate failed (non-JSON response): $instantiate_out"
        set -e
        return
    fi

    local inst_code
    if ! inst_code=$(echo "$inst_json" | jq -r '.code // 0' 2>/dev/null); then
        log_fail "Tokenfactory wasm instantiate failed (invalid JSON): $instantiate_out"
        set -e
        return
    fi

    if [ "$inst_code" != "0" ]; then
        local raw_log=$(echo "$inst_json" | jq -r '.raw_log // empty')
        log_fail "Tokenfactory wasm instantiate failed with code: $inst_code ${raw_log:+- $raw_log}"
        set -e
        return
    fi

    local inst_txhash=$(echo "$inst_json" | jq -r '.txhash // empty')
    if [ -z "$inst_txhash" ] || [ "$inst_txhash" = "null" ]; then
        log_fail "Tokenfactory wasm instantiate missing txhash: $inst_json"
        set -e
        return
    fi

    sleep 3

    local inst_tx_query
    inst_tx_query=$($BINARY query tx "$inst_txhash" --home "$HOME_DIR" --output json 2>/dev/null || true)

    local tf_contract_addr
    tf_contract_addr=$(echo "$inst_tx_query" | jq -r '.events[] | select(.type=="instantiate") | .attributes[] | select(.key=="_contract_address" or .key=="contract_address") | .value' 2>/dev/null)
    if [ -z "$tf_contract_addr" ] || [ "$tf_contract_addr" = "null" ]; then
        log_fail "Tokenfactory wasm instantiate missing contract address: $inst_tx_query"
        set -e
        return
    fi

    # Fund the contract with denom creation fee so bindings can create denom
    local denom_creation_fee
    denom_creation_fee=$($BINARY query tokenfactory params --home "$HOME_DIR" --output json 2>/dev/null | jq -r '.params.denom_creation_fee[0].amount // "0"')
    if [ -z "$denom_creation_fee" ] || [ "$denom_creation_fee" = "null" ]; then
        denom_creation_fee="0"
    fi

    if [ "$denom_creation_fee" != "0" ]; then
        local fund_out
        fund_out=$($BINARY tx bank send "$VALIDATOR_NAME" "$tf_contract_addr" "${denom_creation_fee}${DENOM}" \
            --chain-id "$CHAIN_ID" \
            --keyring-backend "$KEYRING" \
            --home "$HOME_DIR" \
            --gas auto \
            --gas-adjustment 1.5 \
            --fees 500000000000000${DENOM} \
            -y \
            --output json 2>&1 || true)

        local fund_json=$(echo "$fund_out" | sed -n '/^{/,$p')
        local fund_code="0"
        if [ -n "$fund_json" ]; then
            fund_code=$(echo "$fund_json" | jq -r '.code // 0' 2>/dev/null)
        fi
        if [ "$fund_code" != "0" ]; then
            local raw_log=$(echo "$fund_json" | jq -r '.raw_log // empty')
            log_fail "Funding contract for denom fee failed with code: $fund_code ${raw_log:+- $raw_log}"
            set -e
            return
        fi

        sleep 4
    fi

    local subdenom="tfwasm"
    local create_msg
    create_msg=$(cat <<EOF
{"create_denom":{"subdenom":"$subdenom"}}
EOF
    )

    local create_out
    create_out=$($BINARY tx wasm execute "$tf_contract_addr" "$create_msg" \
        --from "$VALIDATOR_NAME" \
        --chain-id "$CHAIN_ID" \
        --keyring-backend "$KEYRING" \
        --home "$HOME_DIR" \
        --gas auto \
        --gas-adjustment 1.5 \
        --fees 2000000000000000${DENOM} \
        -y \
        --output json 2>&1 || true)

    local create_json=$(echo "$create_out" | sed -n '/^{/,$p')
    if [ -z "$create_json" ]; then
        log_fail "Tokenfactory wasm create_denom failed (non-JSON response): $create_out"
        set -e
        return
    fi

    local create_code
    if ! create_code=$(echo "$create_json" | jq -r '.code // 0' 2>/dev/null); then
        log_fail "Tokenfactory wasm create_denom failed (invalid JSON): $create_out"
        set -e
        return
    fi

    if [ "$create_code" != "0" ]; then
        local raw_log=$(echo "$create_json" | jq -r '.raw_log // empty')
        log_fail "Tokenfactory wasm create_denom failed with code: $create_code ${raw_log:+- $raw_log}"
        set -e
        return
    fi

    sleep 4

    local denom="factory/${tf_contract_addr}/$subdenom"
    local mint_amount="1000"
    local mint_msg
    mint_msg=$(cat <<EOF
{"mint_tokens":{"denom":"$denom","amount":"$mint_amount","mint_to_address":"$VALIDATOR_ADDR"}}
EOF
    )

    local mint_out
    mint_out=$($BINARY tx wasm execute "$tf_contract_addr" "$mint_msg" \
        --from "$VALIDATOR_NAME" \
        --chain-id "$CHAIN_ID" \
        --keyring-backend "$KEYRING" \
        --home "$HOME_DIR" \
        --gas auto \
        --gas-adjustment 1.5 \
        --fees 2000000000000000${DENOM} \
        -y \
        --output json 2>&1 || true)

    local mint_json=$(echo "$mint_out" | sed -n '/^{/,$p')
    if [ -z "$mint_json" ]; then
        log_fail "Tokenfactory wasm mint failed (non-JSON response): $mint_out"
        set -e
        return
    fi

    local mint_code
    if ! mint_code=$(echo "$mint_json" | jq -r '.code // 0' 2>/dev/null); then
        log_fail "Tokenfactory wasm mint failed (invalid JSON): $mint_out"
        set -e
        return
    fi

    if [ "$mint_code" != "0" ]; then
        local raw_log=$(echo "$mint_json" | jq -r '.raw_log // empty')
        log_fail "Tokenfactory wasm mint failed with code: $mint_code ${raw_log:+- $raw_log}"
        set -e
        return
    fi

    sleep 4

    local balances_json
    balances_json=$($BINARY query bank balances "$VALIDATOR_ADDR" --home "$HOME_DIR" --output json 2>/dev/null)
    local minted_balance=$(echo "$balances_json" | jq -r --arg denom "$denom" '.balances[] | select(.denom==$denom) | .amount' 2>/dev/null)

    if [ "$minted_balance" = "$mint_amount" ]; then
        log_success "Tokenfactory bindings OK via WASM: minted $minted_balance $denom to $VALIDATOR_ADDR"
    else
        log_fail "Tokenfactory bindings mismatch: expected $mint_amount got ${minted_balance:-none}; balances: $balances_json"
    fi

    set -e
}

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
    log_info "Running WASM tests..."
    echo ""

    # WASM tests
    test_wasm_store_and_instantiate
    test_wasm_cw20_token_info
    test_wasm_cw20_transfer
    test_wasm_cw20_insufficient_funds
    test_wasm_cw20_all_accounts
    test_wasm_tokenfactory_bindings

    echo ""
    log_info "Running transaction tests..."
    echo ""
    
    # Transaction tests
    test_cosmos_bank_transfer
    test_staking_delegation
    test_tokenfactory_mint
    test_tokenfactory_burn
    test_tokenfactory_nonadmin_forbidden
    
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