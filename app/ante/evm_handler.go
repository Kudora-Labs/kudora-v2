package ante

import (
	baseevmante "github.com/cosmos/evm/ante"
	evmante "github.com/cosmos/evm/ante/evm"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// NewMonoEVMAnteHandler creates the sdk.AnteHandler implementation for EVM transactions.
func NewMonoEVMAnteHandler(options HandlerOptions) sdk.AnteHandler {
	return func(ctx sdk.Context, tx sdk.Tx, simulate bool) (sdk.Context, error) {
		evmParams := options.EvmKeeper.GetParams(ctx)
		feemarketParams := options.FeeMarketKeeper.GetParams(ctx)

		decorators := []sdk.AnteDecorator{
			evmante.NewEVMMonoDecorator(
				options.AccountKeeper,
				options.FeeMarketKeeper,
				options.EvmKeeper,
				options.MaxTxGasWanted,
				&evmParams,
				&feemarketParams,
			),
			baseevmante.NewTxListenerDecorator(options.PendingTxListener),
		}

		return sdk.ChainAnteDecorators(decorators...)(ctx, tx, simulate)
	}
}
