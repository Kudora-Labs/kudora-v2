package app

import (
	"cosmossdk.io/core/appmodule"
	storetypes "cosmossdk.io/store/types"
	"github.com/CosmWasm/wasmd/x/wasm"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/runtime"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	erc20 "github.com/cosmos/evm/x/erc20"
	erc20v2 "github.com/cosmos/evm/x/erc20/v2"
	ibctransferevm "github.com/cosmos/evm/x/ibc/transfer"
	ibctransferkeeper "github.com/cosmos/evm/x/ibc/transfer/keeper"
	ibctransferv2evm "github.com/cosmos/evm/x/ibc/transfer/v2"
	"github.com/cosmos/ibc-apps/middleware/packet-forward-middleware/v10/packetforward"
	packetforwardkeeper "github.com/cosmos/ibc-apps/middleware/packet-forward-middleware/v10/packetforward/keeper"
	packetforwardtypes "github.com/cosmos/ibc-apps/middleware/packet-forward-middleware/v10/packetforward/types"
	ratelimit "github.com/cosmos/ibc-apps/modules/rate-limiting/v10"
	ratelimittypes "github.com/cosmos/ibc-apps/modules/rate-limiting/v10/types"
	icamodule "github.com/cosmos/ibc-go/v10/modules/apps/27-interchain-accounts"
	icacontroller "github.com/cosmos/ibc-go/v10/modules/apps/27-interchain-accounts/controller"
	icacontrollerkeeper "github.com/cosmos/ibc-go/v10/modules/apps/27-interchain-accounts/controller/keeper"
	icacontrollertypes "github.com/cosmos/ibc-go/v10/modules/apps/27-interchain-accounts/controller/types"
	icahost "github.com/cosmos/ibc-go/v10/modules/apps/27-interchain-accounts/host"
	icahostkeeper "github.com/cosmos/ibc-go/v10/modules/apps/27-interchain-accounts/host/keeper"
	icahosttypes "github.com/cosmos/ibc-go/v10/modules/apps/27-interchain-accounts/host/types"
	icatypes "github.com/cosmos/ibc-go/v10/modules/apps/27-interchain-accounts/types"
	ibctransfer "github.com/cosmos/ibc-go/v10/modules/apps/transfer"
	ibctransfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	ibc "github.com/cosmos/ibc-go/v10/modules/core"
	ibcclienttypes "github.com/cosmos/ibc-go/v10/modules/core/02-client/types" // nolint:staticcheck // Deprecated: params key table is needed for params migration
	ibcconnectiontypes "github.com/cosmos/ibc-go/v10/modules/core/03-connection/types"
	porttypes "github.com/cosmos/ibc-go/v10/modules/core/05-port/types"
	ibcapi "github.com/cosmos/ibc-go/v10/modules/core/api"
	ibcexported "github.com/cosmos/ibc-go/v10/modules/core/exported"
	ibckeeper "github.com/cosmos/ibc-go/v10/modules/core/keeper"
	solomachine "github.com/cosmos/ibc-go/v10/modules/light-clients/06-solomachine"
	ibctm "github.com/cosmos/ibc-go/v10/modules/light-clients/07-tendermint"
	bindings "github.com/cosmos/tokenfactory/x/tokenfactory/bindings"
)

// registerIBCModules register IBC keepers and non dependency inject modules.
func (app *App) registerIBCModules(appOpts servertypes.AppOptions) error {
	// set up non depinject support modules store keys
	if err := app.RegisterStores(
		storetypes.NewKVStoreKey(ibcexported.StoreKey),
		storetypes.NewKVStoreKey(ibctransfertypes.StoreKey),
		storetypes.NewKVStoreKey(icahosttypes.StoreKey),
		storetypes.NewKVStoreKey(icacontrollertypes.StoreKey),
		storetypes.NewKVStoreKey(packetforwardtypes.StoreKey),
        storetypes.NewKVStoreKey(ratelimittypes.StoreKey),
	); err != nil {
		return err
	}

	// register the key tables for legacy param subspaces
	keyTable := ibcclienttypes.ParamKeyTable()
	keyTable.RegisterParamSet(&ibcconnectiontypes.Params{})
	app.ParamsKeeper.Subspace(ibcexported.ModuleName).WithKeyTable(keyTable)
	app.ParamsKeeper.Subspace(ibctransfertypes.ModuleName).WithKeyTable(ibctransfertypes.ParamKeyTable())
	app.ParamsKeeper.Subspace(icacontrollertypes.SubModuleName).WithKeyTable(icacontrollertypes.ParamKeyTable())
	app.ParamsKeeper.Subspace(icahosttypes.SubModuleName).WithKeyTable(icahosttypes.ParamKeyTable())
	app.ParamsKeeper.Subspace(packetforwardtypes.ModuleName)
	app.ParamsKeeper.Subspace(ratelimittypes.ModuleName)

	govModuleAddr, _ := app.AuthKeeper.AddressCodec().BytesToString(authtypes.NewModuleAddress(govtypes.ModuleName))

	// Create IBC keeper
	app.IBCKeeper = ibckeeper.NewKeeper(
		app.appCodec,
		runtime.NewKVStoreService(app.GetKey(ibcexported.StoreKey)),
		app.GetSubspace(ibcexported.ModuleName),
		app.UpgradeKeeper,
		govModuleAddr,
	)

	// Create IBC transfer keeper
	app.TransferKeeper = ibctransferkeeper.NewKeeper(
		app.appCodec,
		runtime.NewKVStoreService(app.GetKey(ibctransfertypes.StoreKey)),
		app.GetSubspace(ibctransfertypes.ModuleName),
		app.IBCKeeper.ChannelKeeper,
		app.IBCKeeper.ChannelKeeper,
		app.MsgServiceRouter(),
		app.AuthKeeper,
		app.BankKeeper, app.Erc20Keeper,

		govModuleAddr,
	)

	if err := app.initIBCMiddlewareKeepers(); err != nil {
        return err
    }

	// Create interchain account keepers
	app.ICAHostKeeper = icahostkeeper.NewKeeper(
		app.appCodec,
		runtime.NewKVStoreService(app.GetKey(icahosttypes.StoreKey)),
		app.GetSubspace(icahosttypes.SubModuleName),
		app.IBCKeeper.ChannelKeeper, // ICS4Wrapper
		app.IBCKeeper.ChannelKeeper,
		app.AuthKeeper,
		app.MsgServiceRouter(),
		app.GRPCQueryRouter(),
		govModuleAddr,
	)

	app.ICAControllerKeeper = icacontrollerkeeper.NewKeeper(
		app.appCodec,
		runtime.NewKVStoreService(app.GetKey(icacontrollertypes.StoreKey)),
		app.GetSubspace(icacontrollertypes.SubModuleName),
		app.IBCKeeper.ChannelKeeper,
		app.IBCKeeper.ChannelKeeper,
		app.MsgServiceRouter(),
		govModuleAddr,
	)

	app.configureIBCMiddlewareStacks(appOpts)
	
	// this line is used by starport scaffolding # ibc/app/module

	clientKeeper := app.IBCKeeper.ClientKeeper
	storeProvider := clientKeeper.GetStoreProvider()

	tmLightClientModule := ibctm.NewLightClientModule(app.appCodec, storeProvider)
	clientKeeper.AddRoute(ibctm.ModuleName, &tmLightClientModule)

	soloLightClientModule := solomachine.NewLightClientModule(app.appCodec, storeProvider)
	clientKeeper.AddRoute(solomachine.ModuleName, &soloLightClientModule)

	// register IBC modules
	if err := app.RegisterModules(
		ibc.NewAppModule(app.IBCKeeper),
		ibctransferevm.NewAppModule(app.TransferKeeper), // ibc transfer evm compatible
		icamodule.NewAppModule(&app.ICAControllerKeeper, &app.ICAHostKeeper),
		ibctm.NewAppModule(tmLightClientModule),
		solomachine.NewAppModule(soloLightClientModule),
		packetforward.NewAppModule(
     		app.PacketForwardKeeper,
        	app.GetSubspace(packetforwardtypes.ModuleName),
    	),
    	ratelimit.NewAppModule(app.appCodec, *app.RateLimitKeeper),
	); err != nil {
		return err
	}

	return nil
}

// RegisterIBC Since the IBC modules don't support dependency injection,
// we need to manually register the modules on the client side.
// This needs to be removed after IBC supports App Wiring.
// DO NOT USE THIS MAP ELSEWHERE: is strictly for interface registration
func RegisterIBC(cdc codec.Codec) map[string]appmodule.AppModule {
	modules := map[string]appmodule.AppModule{
		ibcexported.ModuleName:      ibc.AppModule{},
		ibctransfertypes.ModuleName: ibctransfer.AppModule{},
		icatypes.ModuleName:         icamodule.AppModule{},
		ibctm.ModuleName:            ibctm.AppModule{},
		solomachine.ModuleName:      solomachine.AppModule{},
		wasmtypes.ModuleName:        wasm.AppModule{},
	}

	for _, m := range modules {
		if mr, ok := m.(module.AppModuleBasic); ok {
			mr.RegisterInterfaces(cdc.InterfaceRegistry())
		}
	}

	return modules
}

// configureIBCMiddlewareStacks configures IBC middleware stacks for both IBC v1 (Classic) and v2 (Eureka)
func (app *App) configureIBCMiddlewareStacks(appOpts servertypes.AppOptions) {
	// =========================================
	// IBC Classic (v1) Transfer Stack
	// Order: ERC20 -> RateLimit -> PFM -> Transfer
	// =========================================
	
	// Layer 1 (Bottom): Transfer base application
	// Using cosmos/evm transfer module for ERC20 compatibility
	var transferStack porttypes.IBCModule
	transferStack = ibctransferevm.NewIBCModule(app.TransferKeeper)
	
	// Layer 2: Packet Forward Middleware
	// Enables multi-hop transfers (A -> B -> C)
	transferStack = packetforward.NewIBCMiddleware(
		transferStack,
		app.PacketForwardKeeper,
		0, // Number of retries on timeout (0 = no retries)
		packetforwardkeeper.DefaultForwardTransferPacketTimeoutTimestamp,
	)
	
	// Layer 3: Rate Limit Middleware
	// Protects against bridge exploits
	transferStack = ratelimit.NewIBCMiddleware(
		*app.RateLimitKeeper,
		transferStack,
	)
	
	// Layer 4 (Top): ERC20 Middleware
	// Converts IBC tokens to ERC20 representation
	// MUST be outermost to execute AFTER ICS20 OnRecvPacket
	transferStack = erc20.NewIBCMiddleware(
		app.Erc20Keeper,
		transferStack,
	)
	
	// =========================================
	// IBC Classic (v1) ICA Stacks
	// =========================================
	
	// ICA Controller Stack
	var icaControllerStack porttypes.IBCModule
	icaControllerStack = icacontroller.NewIBCMiddleware(app.ICAControllerKeeper)
	
	// ICA Host Stack
	var icaHostStack porttypes.IBCModule
	icaHostStack = icahost.NewIBCModule(app.ICAHostKeeper)
	
	// =========================================
	// Wasm IBC Stack
	// =========================================
	wasmOpts := bindings.RegisterCustomPlugins(app.BankKeeper, &app.TokenFactoryKeeper)
	wasmStack, err := app.registerWasmModules(appOpts, wasmOpts...)
	if err != nil {
		panic(err)
	}
	
	// =========================================
	// Configure IBC v1 Router
	// =========================================
	ibcRouter := porttypes.NewRouter().
		AddRoute(ibctransfertypes.ModuleName, transferStack).
		AddRoute(icacontrollertypes.SubModuleName, icaControllerStack).
		AddRoute(icahosttypes.SubModuleName, icaHostStack).
		AddRoute(wasmtypes.ModuleName, wasmStack)
	
	app.IBCKeeper.SetRouter(ibcRouter)
	
	// =========================================
	// IBC v2 (Eureka) Transfer Stack
	// Note: PFM and RateLimit do NOT support IBC v2 yet
	// =========================================
	var transferStackV2 ibcapi.IBCModule
	transferStackV2 = ibctransferv2evm.NewIBCModule(app.TransferKeeper)
	
	// Add ERC20 v2 middleware
	transferStackV2 = erc20v2.NewIBCMiddleware(transferStackV2, app.Erc20Keeper)
	
	// Configure IBC v2 Router
	ibcv2Router := ibcapi.NewRouter().
		AddRoute(ibctransfertypes.PortID, transferStackV2)
	
	app.IBCKeeper.SetRouterV2(ibcv2Router)
}
