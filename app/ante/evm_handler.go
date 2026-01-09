package ante

import (
	baseevmante "github.com/cosmos/evm/ante"
	evmanute "github.com/cosmos/evm/ante/evm"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// NewMonoEVMAnteHandler creates the sdk.AnteHandler implementation for EVM transactions.
func NewMonoEVMAnteHandler(options HandlerOptions) sdk.AnteHandler {
	decorators := []sdk.AnteDecorator{
		evmanute.NewEVMMonoDecorator(
			options.AccountKeeper,
			options.FeeMarketKeeper,
			options.EvmKeeper,
			options.MaxTxGasWanted,
		),
		baseevmante.NewTxListenerDecorator(options.PendingTxListener),
	}

	return sdk.ChainAnteDecorators(decorators...)
}
