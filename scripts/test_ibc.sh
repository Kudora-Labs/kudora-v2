#!/bin/bash
# ============================================================================
# Kudora IBC Two-Chain Test Script
# ============================================================================
# Boots two local Kudora chains and verifies they can communicate over IBC by
# creating a transfer channel (ICS-20) and sending a test transfer.
#
# Prerequisites:
#   - kudorad in PATH
#   - jq, curl
#   - Go toolchain (used to run the relayer if `rly` is not installed)
#
# Usage:
#   ./scripts/test_ibc.sh
#
# Optional env vars:
#   WORKDIR=/tmp/kudora-ibc-test   (default: a temp dir)
#   KEEP=1                        (do not cleanup on exit)
#   LOG_LEVEL=info|debug          (default: info)
# ============================================================================

set -euo pipefail

BINARY="${BINARY:-kudorad}"
DENOM="${DENOM:-kud}"
KEYRING="${KEYRING:-test}"
LOG_LEVEL="${LOG_LEVEL:-info}"

# These chain-ids MUST be known by the app (see app/config.go ChainsCoinInfo)
CHAIN_A_ID="${CHAIN_A_ID:-kudora_12000-1}"
CHAIN_B_ID="${CHAIN_B_ID:-kudora_9000-1}"

WORKDIR="${WORKDIR:-$(mktemp -d -t kudora-ibc-test.XXXXXX)}"
KEEP="${KEEP:-0}"

HOME_A="$WORKDIR/chain-a"
HOME_B="$WORKDIR/chain-b"
RLY_HOME="$WORKDIR/relayer"

# Ports (chain A)
RPC_A=26657
P2P_A=26656
APP_A=26658
API_A=1317
GRPC_A=9090
GRPC_WEB_A=9091
PROM_A=26660
PPROF_A=6060

# Ports (chain B)
RPC_B=26667
P2P_B=26666
APP_B=26668
API_B=1327
GRPC_B=9092
GRPC_WEB_B=9093
PROM_B=26670
PPROF_B=6061

CHAIN_A_EVM_ID=12000
CHAIN_B_EVM_ID=9000

VAL_NAME="validator"
USER_NAME="testuser"

# Colors and Formatting
BOLD="\033[1m"
RESET="\033[0m"
GREEN="\033[0;32m"
BLUE="\033[0;34m"
YELLOW="\033[0;33m"
RED="\033[0;31m"
CYAN="\033[0;36m"

require_cmd() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo -e "${RED}[error] missing required command: $1${RESET}" >&2
		exit 1
	fi
}

log() {
	echo -e "${BLUE}[INFO]${RESET} $*"
}

log_step() {
	echo -e "\n${BOLD}${CYAN}➜ $*${RESET}"
}

log_success() {
	echo -e "${GREEN}✔ $*${RESET}"
}

log_error() {
	echo -e "${RED}✘ $*${RESET}"
}

log_debug() {
  if [ "${LOG_LEVEL}" = "debug" ]; then
	  echo -e "${YELLOW}[DEBUG] $*${RESET}"
  fi
}

cleanup() {
	if [ "$KEEP" = "1" ]; then
		log "KEEP=1 set; preserving workdir: $WORKDIR"
		return 0
	fi

	log "Cleaning up (processes + workdir)"
	pkill -f "$BINARY start.*$HOME_A" 2>/dev/null || true
	pkill -f "$BINARY start.*$HOME_B" 2>/dev/null || true
	pkill -f "rly start" 2>/dev/null || true
	rm -rf "$WORKDIR"
}
trap cleanup EXIT

wait_for_rpc() {
	local rpc_port="$1"
	local max_attempts=60
	local attempt=0

	log "Waiting for RPC on port ${rpc_port}..."
	while [ $attempt -lt $max_attempts ]; do
		if curl -s "http://127.0.0.1:${rpc_port}/status" >/dev/null 2>&1; then
			return 0
		fi
		attempt=$((attempt + 1))
		sleep 1
	done

	log_error "node rpc not ready on port ${rpc_port} after ${max_attempts}s" >&2
	return 1
}

wait_for_account() {
	local node_rpc="$1"
	local addr="$2"
	local max_attempts=60
	local attempt=0

	log "Waiting for account ${addr}..."
	while [ $attempt -lt $max_attempts ]; do
		if "$BINARY" q auth account "$addr" --node "$node_rpc" >/dev/null 2>&1; then
			return 0
		fi
		attempt=$((attempt + 1))
		sleep 1
	done

	log_error "account ${addr} not found on ${node_rpc} after ${max_attempts}s" >&2
	return 1
}

set_chain_ports() {
	local home_dir="$1"
	local rpc_port="$2"
	local p2p_port="$3"
	local app_port="$4"
	local api_port="$5"
	local grpc_port="$6"
	local grpc_web_port="$7"
	local prom_port="$8"
	local pprof_port="$9"

	# cometbft config
	sed -i'' -e "s#tcp://127.0.0.1:26657#tcp://127.0.0.1:${rpc_port}#g" "$home_dir/config/config.toml"
	sed -i'' -e "s#tcp://0.0.0.0:26656#tcp://0.0.0.0:${p2p_port}#g" "$home_dir/config/config.toml"
	sed -i'' -e "s#tcp://127.0.0.1:26658#tcp://127.0.0.1:${app_port}#g" "$home_dir/config/config.toml"
	sed -i'' -e "s#localhost:6060#localhost:${pprof_port}#g" "$home_dir/config/config.toml"
	sed -i'' -e "s#:\\?26660#:${prom_port}#g" "$home_dir/config/config.toml" || true

	# faster blocks for local testing
	sed -i'' -e 's/timeout_commit = ".*"/timeout_commit = "1s"/' "$home_dir/config/config.toml"

	# app config
  # API and gRPC default to localhost on many Cosmos SDK configs; rewrite both forms.
  sed -i'' -E "s#address = \"tcp://(localhost|127\\.0\\.0\\.1|0\\.0\\.0\\.0):1317\"#address = \"tcp://0.0.0.0:${api_port}\"#g" "$home_dir/config/app.toml" || true
  sed -i'' -E "s#address = \"(localhost|127\\.0\\.0\\.1|0\\.0\\.0\\.0):9090\"#address = \"0.0.0.0:${grpc_port}\"#g" "$home_dir/config/app.toml" || true
  # For older SDKs / custom configs that embed raw endpoints.
  sed -i'' -e "s#tcp://0.0.0.0:1317#tcp://0.0.0.0:${api_port}#g" "$home_dir/config/app.toml" || true
  sed -i'' -e "s#tcp://localhost:1317#tcp://0.0.0.0:${api_port}#g" "$home_dir/config/app.toml" || true
  sed -i'' -e "s#0.0.0.0:9090#0.0.0.0:${grpc_port}#g" "$home_dir/config/app.toml" || true
  sed -i'' -e "s#localhost:9090#0.0.0.0:${grpc_port}#g" "$home_dir/config/app.toml" || true
  sed -i'' -e "s#0.0.0.0:9091#0.0.0.0:${grpc_web_port}#g" "$home_dir/config/app.toml" || true
  sed -i'' -e "s#localhost:9091#0.0.0.0:${grpc_web_port}#g" "$home_dir/config/app.toml" || true

  # Set minimum gas prices - use a very low value compatible with EVM
  sed -i'' -e 's/minimum-gas-prices = "[^"]*"/minimum-gas-prices = "1kud"/' "$home_dir/config/app.toml"
  sed -i'' -e 's/^minimum-gas-prices.*$/minimum-gas-prices = "1kud"/' "$home_dir/config/app.toml"
}

init_chain() {
  local home_dir="$1"
  local chain_id="$2"

  rm -rf "$home_dir"

  "$BINARY" init test-node \
    --chain-id "$chain_id" \
    --default-denom "$DENOM" \
    --home "$home_dir" \
    >/dev/null 2>&1

  "$BINARY" keys add "$VAL_NAME" --keyring-backend "$KEYRING" --home "$home_dir" >/dev/null 2>&1
  "$BINARY" keys add "$USER_NAME" --keyring-backend "$KEYRING" --home "$home_dir" >/dev/null 2>&1

  local val_addr
  local user_addr
  val_addr=$("$BINARY" keys show "$VAL_NAME" --keyring-backend "$KEYRING" --home "$home_dir" -a)
  user_addr=$("$BINARY" keys show "$USER_NAME" --keyring-backend "$KEYRING" --home "$home_dir" -a)

  "$BINARY" genesis add-genesis-account "$val_addr" 1000000000000000000000000${DENOM} --home "$home_dir" >/dev/null
  "$BINARY" genesis add-genesis-account "$user_addr" 1000000000000000000000000${DENOM} --home "$home_dir" >/dev/null

  "$BINARY" genesis gentx "$VAL_NAME" 100000000000000000000000${DENOM} \
    --chain-id "$chain_id" \
    --keyring-backend "$KEYRING" \
    --home "$home_dir" \
    >/dev/null 2>&1

  "$BINARY" genesis collect-gentxs --home "$home_dir" >/dev/null 2>&1
}

start_chain() {
	local home_dir="$1"
	local evm_id="$2"
	local log_file="$3"

	"$BINARY" start \
		--home "$home_dir" \
		--api.enable \
		--log_level "$LOG_LEVEL" \
		--evm.evm-chain-id "$evm_id" \
		>"$log_file" 2>&1 &
}

rly_setup() {

  local relayer_pkg="github.com/cosmos/relayer/v2@v2.6.0"
  local rly_cmd
  if command -v rly >/dev/null 2>&1; then
    rly_cmd=(rly)
  else
    require_cmd go
    rly_cmd=(go run "$relayer_pkg")
  fi

  rly() { "${rly_cmd[@]}" --home "$RLY_HOME" "$@"; }

  mkdir -p "$RLY_HOME"
  rly config init >/dev/null

	cat >"$WORKDIR/chain-a.json" <<EOF
{
	"type": "cosmos",
	"value": {
		"key": "relayer",
		"chain-id": "${CHAIN_A_ID}",
		"rpc-addr": "http://127.0.0.1:${RPC_A}",
    "grpc-addr": "127.0.0.1:${GRPC_A}",
		"account-prefix": "kudo",
		"keyring-backend": "test",
		"gas-adjustment": 1.5,
    "gas-prices": "1000000000${DENOM}",
		"debug": false,
		"timeout": "10s",
		"output-format": "json",
		"sign-mode": "direct"
	}
}
EOF

	cat >"$WORKDIR/chain-b.json" <<EOF
{
	"type": "cosmos",
	"value": {
		"key": "relayer",
		"chain-id": "${CHAIN_B_ID}",
		"rpc-addr": "http://127.0.0.1:${RPC_B}",
    "grpc-addr": "127.0.0.1:${GRPC_B}",
		"account-prefix": "kudo",
		"keyring-backend": "test",
		"gas-adjustment": 1.5,
    "gas-prices": "1000000000${DENOM}",
		"debug": false,
		"timeout": "10s",
		"output-format": "json",
		"sign-mode": "direct"
	}
}
EOF

	rly chains add --file "$WORKDIR/chain-a.json" "${CHAIN_A_ID}" >/dev/null
	rly chains add --file "$WORKDIR/chain-b.json" "${CHAIN_B_ID}" >/dev/null

	# Generate new keys for the relayer on both chains
	rly keys add "${CHAIN_A_ID}" relayer >/dev/null
	rly keys add "${CHAIN_B_ID}" relayer >/dev/null

	# Return helper in global scope via a heredoc evaluated by caller
	cat <<'EOS'
RLY_WRAPPER_DEFINED=1
EOS
}

fund_relayer_on_chain() {
  local home_dir="$1"
  local chain_id="$2"
  local node_rpc="$3"
  local relayer_addr="$4"

  local output
  log_debug "Funding $relayer_addr on $chain_id from $USER_NAME"
  output=$("$BINARY" tx bank send \
    "$USER_NAME" \
    "$relayer_addr" \
    1000000000000000000${DENOM} \
    --from "$USER_NAME" \
    --keyring-backend "$KEYRING" \
    --home "$home_dir" \
    --node "$node_rpc" \
    --chain-id "$chain_id" \
    --broadcast-mode sync \
    --gas-prices "1000000000${DENOM}" \
    --gas auto \
    --gas-adjustment 1.3 \
    -y \
    -o json 2>/dev/null) || {
    exitcode=$?
    log_error "Bank send command failed with exit code $exitcode"
    return 1
  }
  
  log_debug "Funding tx output: $output"

  local txhash
  txhash=$(echo "$output" | jq -r '.txhash // empty')
  if [ -z "$txhash" ]; then
    log_error "failed to extract txhash from funding tx output"
    log_debug "$output"
    return 1
  fi
  # Check if tx was rejected at broadcast time
  local code_at_broadcast
  code_at_broadcast=$(echo "$output" | jq -r '.code // 0')
  if [ "$code_at_broadcast" != "0" ]; then
    log_error "funding tx rejected at broadcast"
    log_debug "$output"
    return 1
  fi
  # Wait for tx to be included in a block
  local max_wait=30
  local waited=0
  while [ $waited -lt $max_wait ]; do
    local tx_result
    tx_result=$("$BINARY" q tx "$txhash" --node "$node_rpc" -o json 2>/dev/null || true)
    if [ -n "$tx_result" ]; then
      local code
      code=$(echo "$tx_result" | jq -r '.code // 1')
      if [ "$code" = "0" ]; then
		log_success "Funded relayer on $chain_id ($relayer_addr)"
        return 0
      else
        log_error "funding tx failed with code $code"
        log_debug "$tx_result"
        return 1
      fi
    fi
    waited=$((waited + 1))
    sleep 2
  done

  log_error "funding tx ${txhash} not confirmed after ${max_wait}s"
  return 1
}

main() {
  require_cmd "$BINARY"
  require_cmd jq
  require_cmd curl

  log "Workdir: $WORKDIR"

  log_step "Initializing chain A (${CHAIN_A_ID})"
  init_chain "$HOME_A" "$CHAIN_A_ID"
  set_chain_ports "$HOME_A" "$RPC_A" "$P2P_A" "$APP_A" "$API_A" "$GRPC_A" "$GRPC_WEB_A" "$PROM_A" "$PPROF_A"

  log_step "Initializing chain B (${CHAIN_B_ID})"
  init_chain "$HOME_B" "$CHAIN_B_ID"
  set_chain_ports "$HOME_B" "$RPC_B" "$P2P_B" "$APP_B" "$API_B" "$GRPC_B" "$GRPC_WEB_B" "$PROM_B" "$PPROF_B"

  log_step "Starting both chains"
  start_chain "$HOME_A" "$CHAIN_A_EVM_ID" "$WORKDIR/chain-a.log"
  start_chain "$HOME_B" "$CHAIN_B_EVM_ID" "$WORKDIR/chain-b.log"

  wait_for_rpc "$RPC_A"
  wait_for_rpc "$RPC_B"
  log_success "Chains started and RPC ready"

  log_step "Setting up relayer config"

  # Define rly wrapper and configure chains/keys
  eval "$(rly_setup)"

  # Recreate rly wrapper for subsequent calls
  local relayer_pkg="github.com/cosmos/relayer/v2@v2.6.0"
  local rly_cmd
  if command -v rly >/dev/null 2>&1; then
    rly_cmd=(rly)
  else
    rly_cmd=(go run "$relayer_pkg")
  fi
  rly() { "${rly_cmd[@]}" --home "$RLY_HOME" "$@"; }

  local relayer_addr_a
  local relayer_addr_b
  relayer_addr_a=$(rly chains address "${CHAIN_A_ID}")
  relayer_addr_b=$(rly chains address "${CHAIN_B_ID}")

  log_step "Funding relayer accounts"
  fund_relayer_on_chain "$HOME_A" "$CHAIN_A_ID" "http://127.0.0.1:${RPC_A}" "$relayer_addr_a"
  fund_relayer_on_chain "$HOME_B" "$CHAIN_B_ID" "http://127.0.0.1:${RPC_B}" "$relayer_addr_b"

  wait_for_account "http://127.0.0.1:${RPC_A}" "$relayer_addr_a"
  wait_for_account "http://127.0.0.1:${RPC_B}" "$relayer_addr_b"

  local path_name="kudora-local"
  log_step "Creating IBC path: $path_name"
  if output=$(rly paths new "${CHAIN_A_ID}" "${CHAIN_B_ID}" "$path_name" --src-port transfer --dst-port transfer 2>&1); then
    :
  else
    log_error "Failed to create IBC path"
    echo "$output" >&2
    exit 1
  fi

  log_step "Linking chains (create clients/connection/channel)"
  if output=$(rly tx link "$path_name" --src-port transfer --dst-port transfer 2>&1); then
    log_success "IBC Link established"
  else
    log_error "Failed to link chains"
    echo "$output" >&2
    exit 1
  fi

  # Determine channel on chain A
  local channel_a
  channel_a=$("$BINARY" q ibc channel channels --node "http://127.0.0.1:${RPC_A}" -o json | jq -r '.channels[0].channel_id')
  if [ -z "$channel_a" ] || [ "$channel_a" = "null" ]; then
    log_error "could not determine IBC channel on chain A" >&2
    exit 1
  fi
  log "Channel found: ${channel_a}"

  local recv_addr
  recv_addr=$("$BINARY" keys show "$USER_NAME" --keyring-backend "$KEYRING" --home "$HOME_B" -a)

  log_step "Starting relayer (background)"
  rly start "$path_name" >/dev/null 2>&1 &
  local rly_pid=$!

  log_step "Sending ICS-20 transfer from chain A -> chain B via ${channel_a}"
  # Workaround: Kudora does not currently support the relayer's balance/fee queries (it fails with
  # "unknown query path"). We still use the relayer to relay packets/acks, but submit the ICS-20
  # transfer with the chain CLI.
  local transfer_out
  transfer_out=$("$BINARY" tx ibc-transfer transfer transfer "$channel_a" "$recv_addr" "1000${DENOM}" \
    --from "$USER_NAME" \
    --keyring-backend "$KEYRING" \
    --home "$HOME_A" \
    --node "http://127.0.0.1:${RPC_A}" \
    --chain-id "$CHAIN_A_ID" \
    --broadcast-mode sync \
    --gas 200000 \
    --gas-prices "1000000000${DENOM}" \
    -y \
    -o json 2>/dev/null)
  local transfer_txhash
  transfer_txhash=$(echo "$transfer_out" | jq -r '.txhash // empty')
  if [ -z "$transfer_txhash" ]; then
    log_error "failed to extract txhash from ibc-transfer tx output"
    log_debug "$transfer_out"
    exit 1
  fi
  local transfer_code
  transfer_code=$(echo "$transfer_out" | jq -r '.code // 0')
  if [ "$transfer_code" != "0" ]; then
    log_error "ibc-transfer tx rejected at broadcast"
    log_debug "$transfer_out"
    exit 1
  fi
  # Wait for tx to be included
  local max_wait=30
  local waited=0
  log "Waiting for transfer tx confirmation ($transfer_txhash)..."
  while [ $waited -lt $max_wait ]; do
    local tx_result
    tx_result=$("$BINARY" q tx "$transfer_txhash" --node "http://127.0.0.1:${RPC_A}" -o json 2>/dev/null || true)
    if [ -n "$tx_result" ]; then
      local code
      code=$(echo "$tx_result" | jq -r '.code // 1')
      if [ "$code" = "0" ]; then
        log_success "Transfer tx confirmed"
        break
      else
        log_error "ibc-transfer tx failed with code $code"
        log_debug "$tx_result"
        exit 1
      fi
    fi
    waited=$((waited + 1))
    sleep 2
  done
  if [ $waited -ge $max_wait ]; then
    log_error "ibc-transfer tx ${transfer_txhash} not confirmed after ${max_wait}s"
    exit 1
  fi

  # Give the relayer a moment, then force-flush remaining packets/acks
  log "Flushing packets..."
  sleep 5
  rly tx flush "$path_name" "$channel_a" >/dev/null 2>&1 || true

  log_step "Stopping relayer"
  kill "$rly_pid" 2>/dev/null || true
  wait "$rly_pid" 2>/dev/null || true

  log_step "Verifying receiver balance on chain B"
  local balances
  balances=$("$BINARY" q bank balances "$recv_addr" --node "http://127.0.0.1:${RPC_B}" -o json)

  local ibc_coin
  ibc_coin=$(echo "$balances" | jq -r '.balances[] | select(.denom|startswith("ibc/")) | "\(.amount) \(.denom)"' | head -n 1 || true)

  if [ -z "$ibc_coin" ]; then
    log_error "no ibc/* denom found in receiver balances"
    log_debug "$balances"
    exit 1
  fi

  log_success "IBC transfer received: $ibc_coin"
  log_success "Test complete. Logs available in: \n    $WORKDIR/chain-a.log\n    $WORKDIR/chain-b.log"
}

main "$@"
