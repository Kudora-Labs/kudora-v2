package ante

import (
	circuitante "cosmossdk.io/x/circuit/ante"
	cosmosante "github.com/cosmos/evm/ante/cosmos"
	evmante "github.com/cosmos/evm/ante/evm"
	evmtypes "github.com/cosmos/evm/x/vm/types"
	ibcante "github.com/cosmos/ibc-go/v10/modules/core/ante"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth/ante"
	sdkvesting "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
)

// NewCosmosAnteHandler creates the ante chain for non-EVM transactions, enriched with WASM decorators.
func NewCosmosAnteHandler(options HandlerOptions) sdk.AnteHandler {
	return func(ctx sdk.Context, tx sdk.Tx, simulate bool) (sdk.Context, error) {
		feemarketParams := options.FeeMarketKeeper.GetParams(ctx)
		txFeeChecker := evmante.NewDynamicFeeChecker(&feemarketParams)

		decorators := []sdk.AnteDecorator{
			cosmosante.NewRejectMessagesDecorator(),
			cosmosante.NewAuthzLimiterDecorator(
				sdk.MsgTypeURL(&evmtypes.MsgEthereumTx{}),
				sdk.MsgTypeURL(&sdkvesting.MsgCreateVestingAccount{}),
			),
			ante.NewSetUpContextDecorator(),
		}

		decorators = append(decorators, wasmDecorators(options)...)

		decorators = append(decorators,
			circuitante.NewCircuitBreakerDecorator(options.CircuitKeeper),
			ante.NewExtensionOptionsDecorator(options.ExtensionOptionChecker),
			ante.NewValidateBasicDecorator(),
			ante.NewTxTimeoutHeightDecorator(),
			ante.NewValidateMemoDecorator(options.AccountKeeper),
			cosmosante.NewMinGasPriceDecorator(&feemarketParams),
			ante.NewConsumeGasForTxSizeDecorator(options.AccountKeeper),
			ante.NewDeductFeeDecorator(options.AccountKeeper, options.BankKeeper, options.FeegrantKeeper, txFeeChecker),
			ante.NewSetPubKeyDecorator(options.AccountKeeper),
			ante.NewValidateSigCountDecorator(options.AccountKeeper),
			ante.NewSigGasConsumeDecorator(options.AccountKeeper, options.SignatureGasConsumer),
			ante.NewSigVerificationDecorator(options.AccountKeeper, options.SignModeHandler),
			ante.NewIncrementSequenceDecorator(options.AccountKeeper),
			ibcante.NewRedundantRelayDecorator(options.IBCKeeper),
			evmante.NewGasWantedDecorator(options.EvmKeeper, options.FeeMarketKeeper, &feemarketParams),
		)

		return sdk.ChainAnteDecorators(decorators...)(ctx, tx, simulate)
	}
}
