package ante

import (
	corestoretypes "cosmossdk.io/core/store"
	circuitkeeper "cosmossdk.io/x/circuit/keeper"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmTypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/cosmos/cosmos-sdk/codec"
	authante "github.com/cosmos/cosmos-sdk/x/auth/ante"
	signing "cosmossdk.io/x/tx/signing"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	baseevmante "github.com/cosmos/evm/ante"
	evminterfaces "github.com/cosmos/evm/ante/interfaces"
	feemarketkeeper "github.com/cosmos/evm/x/feemarket/keeper"
	evmmodulekeeper "github.com/cosmos/evm/x/vm/keeper"
	ibckeeper "github.com/cosmos/ibc-go/v10/modules/core/keeper"
)

// HandlerOptions extends the SDK ante options with EVM, WASM, and IBC specifics.
type HandlerOptions struct {
	// Core cosmos/auth keepers and configuration
	AccountKeeper          evminterfaces.AccountKeeper
	BankKeeper             bankkeeper.Keeper
	FeegrantKeeper         authante.FeegrantKeeper
	SignModeHandler        *signing.HandlerMap
	SignatureGasConsumer   authante.SignatureVerificationGasConsumer
	TxFeeChecker           authante.TxFeeChecker
	ExtensionOptionChecker authante.ExtensionOptionChecker

	// EVM-specific options
	Cdc               codec.BinaryCodec
	EvmKeeper         *evmmodulekeeper.Keeper
	FeeMarketKeeper   feemarketkeeper.Keeper
	MaxTxGasWanted    uint64
	PendingTxListener baseevmante.PendingTxListener
	IBCKeeper         *ibckeeper.Keeper

	// WASM-specific options
	NodeConfig            *wasmTypes.NodeConfig
	WasmKeeper            *wasmkeeper.Keeper
	TXCounterStoreService corestoretypes.KVStoreService
	CircuitKeeper         *circuitkeeper.Keeper
}
