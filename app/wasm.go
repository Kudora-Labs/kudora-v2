package app

import (
	"fmt"

	storetypes "cosmossdk.io/store/types"
	"github.com/CosmWasm/wasmd/x/wasm"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/runtime"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
	"github.com/cosmos/cosmos-sdk/x/auth/ante"
	"github.com/cosmos/cosmos-sdk/x/auth/posthandler"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	evmante "github.com/cosmos/evm/ante"
	evmdecorators "github.com/cosmos/evm/ante/evm"
	srvflags "github.com/cosmos/evm/server/flags"
	evmtypes "github.com/cosmos/evm/types"
	"github.com/cosmos/gogoproto/proto"
	porttypes "github.com/cosmos/ibc-go/v10/modules/core/05-port/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cast"
)

// registerWasmModules register CosmWasm keepers and non dependency inject modules.
func (app *App) registerWasmModules(
	appOpts servertypes.AppOptions,
	wasmOpts ...wasmkeeper.Option,
) (porttypes.IBCModule, error) {
	// set up non depinject support modules store keys
	// Only register if not already registered
	if app.GetKey(wasmtypes.StoreKey) == nil {
		if err := app.RegisterStores(
			storetypes.NewKVStoreKey(wasmtypes.StoreKey),
		); err != nil {
			panic(err)
		}
	}

	wasmConfig, err := wasm.ReadNodeConfig(appOpts)
	if err != nil {
		return nil, fmt.Errorf("error while reading wasm config: %s", err)
	}

	// If SimulationGasLimit is not set, use a default value
	// This prevents panic in NewLimitSimulationGasDecorator
	// Using 5 million gas as a reasonable default for simulations
	if wasmConfig.SimulationGasLimit == nil || *wasmConfig.SimulationGasLimit == 0 {
		defaultSimGasLimit := uint64(5_000_000)
		wasmConfig.SimulationGasLimit = &defaultSimGasLimit
	}

	// The last arguments can contain custom message handlers, and custom query handlers,
	// if we want to allow any custom callbacks
	app.WasmKeeper = wasmkeeper.NewKeeper(
		app.AppCodec(),
		runtime.NewKVStoreService(app.GetKey(wasmtypes.StoreKey)),
		app.AuthKeeper,
		app.BankKeeper,
		app.StakingKeeper,
		distrkeeper.NewQuerier(app.DistrKeeper),
		app.IBCKeeper.ChannelKeeper,
		app.IBCKeeper.ChannelKeeper,
		app.TransferKeeper,
		app.MsgServiceRouter(),
		app.GRPCQueryRouter(),
		DefaultNodeHome,
		wasmConfig,
		wasmtypes.VMConfig{},
		wasmkeeper.BuiltInCapabilities(),
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		wasmOpts...,
	)

	// register IBC modules
	if err := app.RegisterModules(
		wasm.NewAppModule(
			app.AppCodec(),
			&app.WasmKeeper,
			app.StakingKeeper,
			app.AuthKeeper,
			app.BankKeeper,
			app.MsgServiceRouter(),
			app.GetSubspace(wasmtypes.ModuleName),
		)); err != nil {
		return nil, err
	}

	if err := app.setAnteHandler(appOpts, app.txConfig, wasmConfig, app.GetKey(wasmtypes.StoreKey)); err != nil {
		return nil, err
	}

	if manager := app.SnapshotManager(); manager != nil {
		err := manager.RegisterExtensions(
			wasmkeeper.NewWasmSnapshotter(app.CommitMultiStore(), &app.WasmKeeper),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to register snapshot extension: %s", err)
		}
	}

	if err := app.setPostHandler(); err != nil {
		return nil, err
	}

	// At startup, after all modules have been registered, check that all proto
	// annotations are correct.
	protoFiles, err := proto.MergedRegistry()
	if err != nil {
		return nil, err
	}
	err = msgservice.ValidateProtoAnnotations(protoFiles)
	if err != nil {
		return nil, err
	}

	// Create fee enabled wasm ibc Stack
	wasmStack := wasm.NewIBCHandler(app.WasmKeeper, app.IBCKeeper.ChannelKeeper, app.IBCKeeper.ChannelKeeper)

	return wasmStack, nil
}

func (app *App) setPostHandler() error {
	postHandler, err := posthandler.NewPostHandler(
		posthandler.HandlerOptions{},
	)
	if err != nil {
		return err
	}
	app.SetPostHandler(postHandler)
	return nil
}

func (app *App) setAnteHandler(appOpts servertypes.AppOptions, txConfig client.TxConfig, wasmConfig wasmtypes.NodeConfig, txCounterStoreKey *storetypes.KVStoreKey) error {
	maxGasWanted := cast.ToUint64(appOpts.Get(srvflags.EVMMaxTxGasWanted))

	anteHandler, err := NewAnteHandler(
		HandlerOptions{
			HandlerOptions: ante.HandlerOptions{
				AccountKeeper:          app.AuthKeeper,
				BankKeeper:             app.BankKeeper,
				SignModeHandler:        txConfig.SignModeHandler(),
				FeegrantKeeper:         app.FeeGrantKeeper,
				ExtensionOptionChecker: evmtypes.HasDynamicFeeExtensionOption,
				SigGasConsumer:         evmante.SigVerificationGasConsumer,
			},
			AccountKeeper:   app.AuthKeeper,
			Cdc:             app.appCodec,
			EvmKeeper:       app.EVMKeeper,
			FeeMarketKeeper: app.FeeMarketKeeper,
			MaxTxGasWanted:  maxGasWanted,
			TxFeeChecker:    evmdecorators.NewDynamicFeeChecker(app.FeeMarketKeeper),
			PendingTxListener: func(hash common.Hash) {
				for _, listener := range app.pendingTxListeners {
					listener(hash)
				}
			},
			ExtensionOptionChecker: evmtypes.HasDynamicFeeExtensionOption,
			IBCKeeper:              app.IBCKeeper,
			NodeConfig:             &wasmConfig,
			WasmKeeper:             &app.WasmKeeper,
			TXCounterStoreService:  runtime.NewKVStoreService(txCounterStoreKey),
			CircuitKeeper:          &app.CircuitBreakerKeeper,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create AnteHandler: %s", err)
	}

	// Set the AnteHandler for the app
	app.SetAnteHandler(anteHandler)
	return nil
}
