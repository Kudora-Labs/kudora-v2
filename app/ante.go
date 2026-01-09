package app

import (
	"errors"

	antehandlers "kudora/app/ante"

	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	authante "github.com/cosmos/cosmos-sdk/x/auth/ante"
)

// Re-export HandlerOptions locally for convenience within the app package.
type HandlerOptions = antehandlers.HandlerOptions

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
	if options.SignatureGasConsumer == nil {
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

	cosmosAnteHandler := antehandlers.NewCosmosAnteHandler(options)
	evmAnteHandler := antehandlers.NewMonoEVMAnteHandler(options)

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
