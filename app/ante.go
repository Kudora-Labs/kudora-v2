package app

import (
	"errors"

	corestoretypes "cosmossdk.io/core/store"
	errorsmod "cosmossdk.io/errors"
	circuitante "cosmossdk.io/x/circuit/ante"
	circuitkeeper "cosmossdk.io/x/circuit/keeper"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmTypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	authante "github.com/cosmos/cosmos-sdk/x/auth/ante"
	sdkvesting "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	baseevmante "github.com/cosmos/evm/ante"
	cosmosante "github.com/cosmos/evm/ante/cosmos"
	evmante "github.com/cosmos/evm/ante/evm"
	evminterfaces "github.com/cosmos/evm/ante/interfaces"
	feemarketkeeper "github.com/cosmos/evm/x/feemarket/keeper"
	evmmodulekeeper "github.com/cosmos/evm/x/vm/keeper"
	evmmoduletypes "github.com/cosmos/evm/x/vm/types"
	ibcante "github.com/cosmos/ibc-go/v10/modules/core/ante"
	ibckeeper "github.com/cosmos/ibc-go/v10/modules/core/keeper"
	
)

// HandlerOptions extend the SDK's AnteHandler options by requiring the IBC
// channel keeper.
type HandlerOptions struct {
	authante.HandlerOptions

	// EVM-specific options
	Cdc               codec.BinaryCodec
	EvmKeeper         *evmmodulekeeper.Keeper
	FeeMarketKeeper   feemarketkeeper.Keeper
	MaxTxGasWanted    uint64
	TxFeeChecker      authante.TxFeeChecker
	PendingTxListener baseevmante.PendingTxListener
	IBCKeeper         *ibckeeper.Keeper
	ExtensionOptionChecker authante.ExtensionOptionChecker
	AccountKeeper evminterfaces.AccountKeeper

	// WASM-specific options
	NodeConfig            *wasmTypes.NodeConfig
	WasmKeeper            *wasmkeeper.Keeper
	TXCounterStoreService corestoretypes.KVStoreService
	CircuitKeeper         *circuitkeeper.Keeper
}

// NewAnteHandler constructor
func NewAnteHandler(options HandlerOptions) (sdk.AnteHandler, error) {
	if options.AccountKeeper == nil {
		return nil, errors.New("account keeper is required for ante builder")
	}
	if options.BankKeeper == nil {
		return nil, errors.New("bank keeper is required for ante builder")
	}
	if options.SignModeHandler == nil {
		return nil, errors.New("sign mode handler is required for ante builder")
	}
	if options.ExtensionOptionChecker == nil {
		return nil, errors.New("extension option checker is required for ante builder")
	}
	if options.TxFeeChecker == nil {
		return nil, errors.New("tx fee checker is required for ante builder")
	}
	if options.SigGasConsumer == nil {
		return nil, errors.New("sig gas consumer is required for ante builder")
	}
	if options.Cdc == nil {
		return nil, errors.New("codec is required for ante builder")
	}
	if options.EvmKeeper == nil {
		return nil, errors.New("evm keeper is required for ante builder")
	}
	if options.NodeConfig == nil {
		return nil, errors.New("wasm config is required for ante builder")
	}
	if options.TXCounterStoreService == nil {
		return nil, errors.New("wasm store service is required for ante builder")
	}
	if options.WasmKeeper == nil {
		return nil, errors.New("wasm keeper is required for ante builder")
	}
	if options.CircuitKeeper == nil {
		return nil, errors.New("circuit keeper is required for ante builder")
	}
	if options.IBCKeeper == nil {
		return nil, errors.New("ibc keeper is required for ante builder")
	}

	// Cosmos (non-EVM) ante chain with WASM decorators
	cosmosDecorators := []sdk.AnteDecorator{
		cosmosante.NewRejectMessagesDecorator(),
		cosmosante.NewAuthzLimiterDecorator(
			sdk.MsgTypeURL(&evmmoduletypes.MsgEthereumTx{}),
			sdk.MsgTypeURL(&sdkvesting.MsgCreateVestingAccount{}),
		),
		authante.NewSetUpContextDecorator(),
		wasmkeeper.NewLimitSimulationGasDecorator(options.NodeConfig.SimulationGasLimit),
		wasmkeeper.NewCountTXDecorator(options.TXCounterStoreService),
		wasmkeeper.NewGasRegisterDecorator(options.WasmKeeper.GetGasRegister()),
		wasmkeeper.NewTxContractsDecorator(),
		circuitante.NewCircuitBreakerDecorator(options.CircuitKeeper),
		authante.NewExtensionOptionsDecorator(options.ExtensionOptionChecker),
		authante.NewValidateBasicDecorator(),
		authante.NewTxTimeoutHeightDecorator(),
		authante.NewValidateMemoDecorator(options.AccountKeeper),
		cosmosante.NewMinGasPriceDecorator(options.FeeMarketKeeper, options.EvmKeeper),
		authante.NewConsumeGasForTxSizeDecorator(options.AccountKeeper),
		authante.NewDeductFeeDecorator(options.AccountKeeper, options.BankKeeper, options.FeegrantKeeper, options.TxFeeChecker),
		authante.NewSetPubKeyDecorator(options.AccountKeeper),
		authante.NewValidateSigCountDecorator(options.AccountKeeper),
		authante.NewSigGasConsumeDecorator(options.AccountKeeper, options.SigGasConsumer),
		authante.NewSigVerificationDecorator(options.AccountKeeper, options.SignModeHandler),
		authante.NewIncrementSequenceDecorator(options.AccountKeeper),
		ibcante.NewRedundantRelayDecorator(options.IBCKeeper),
		evmante.NewGasWantedDecorator(options.EvmKeeper, options.FeeMarketKeeper),
	}
	cosmosAnteHandler := sdk.ChainAnteDecorators(cosmosDecorators...)

	// EVM ante chain
	evmDecorators := []sdk.AnteDecorator{
		evmante.NewEVMMonoDecorator(
			options.AccountKeeper,
			options.FeeMarketKeeper,
			options.EvmKeeper,
			options.MaxTxGasWanted,
		),
		baseevmante.NewTxListenerDecorator(options.PendingTxListener),
	}
	evmAnteHandler := sdk.ChainAnteDecorators(evmDecorators...)

	return func(ctx sdk.Context, tx sdk.Tx, simulate bool) (sdk.Context, error) {
		txWithExtensions, ok := tx.(authante.HasExtensionOptionsTx)
		if ok {
			opts := txWithExtensions.GetExtensionOptions()
			if len(opts) > 0 {
				switch typeURL := opts[0].GetTypeUrl(); typeURL {
				case "/cosmos.evm.vm.v1.ExtensionOptionsEthereumTx":
					return evmAnteHandler(ctx, tx, simulate)
				case "/cosmos.evm.types.v1.ExtensionOptionDynamicFeeTx":
					return cosmosAnteHandler(ctx, tx, simulate)
				default:
					return ctx, errorsmod.Wrapf(
						errortypes.ErrUnknownExtensionOptions,
						"rejecting tx with unsupported extension option: %s", typeURL,
					)
				}
			}
		}

		return cosmosAnteHandler(ctx, tx, simulate)
	}, nil
}
