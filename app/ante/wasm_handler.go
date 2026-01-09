package ante

import (
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// wasmDecorators builds the WASM-specific ante decorators used in the Cosmos chain.
func wasmDecorators(options HandlerOptions) []sdk.AnteDecorator {
	return []sdk.AnteDecorator{
		wasmkeeper.NewLimitSimulationGasDecorator(options.NodeConfig.SimulationGasLimit),
		wasmkeeper.NewCountTXDecorator(options.TXCounterStoreService),
		wasmkeeper.NewGasRegisterDecorator(options.WasmKeeper.GetGasRegister()),
		wasmkeeper.NewTxContractsDecorator(),
	}
}
