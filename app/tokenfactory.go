package app

import (
	"cosmossdk.io/core/appmodule"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	// Token Factory imports from cosmos/tokenfactory
	tokenfactory "github.com/cosmos/tokenfactory/x/tokenfactory"
	tokenfactorykeeper "github.com/cosmos/tokenfactory/x/tokenfactory/keeper"
	tokenfactorytypes "github.com/cosmos/tokenfactory/x/tokenfactory/types"
)

// Define capabilities for Token Factory module
var tokenFactoryCapabilities = []string{
	tokenfactorytypes.EnableBurnFrom,
	tokenfactorytypes.EnableForceTransfer,
	tokenfactorytypes.EnableSetMetadata,
	tokenfactorytypes.EnableCommunityPoolFeeFunding,
}

// registerTokenFactoryModule registers the Token Factory keeper and module.
// This follows the same pattern as registerIBCModules and registerEVMModules.
func (app *App) registerTokenFactoryModule(appOpts servertypes.AppOptions) error {
	// Step 1: Register the store key for Token Factory
	if err := app.RegisterStores(
		storetypes.NewKVStoreKey(tokenfactorytypes.StoreKey),
	); err != nil {
		return err
	}

	// Step 2: Register params subspace for legacy param handling
	tokenfactorysubspace := app.ParamsKeeper.Subspace(tokenfactorytypes.ModuleName)

	// Step 3: Get the governance module address for authority
	govModuleAddr, err := app.AuthKeeper.AddressCodec().BytesToString(
		authtypes.NewModuleAddress(govtypes.ModuleName),
	)
	if err != nil {
		return err
	}

	// Step 4: Create the Token Factory keeper
	app.TokenFactoryKeeper = tokenfactorykeeper.NewKeeper(
		app.appCodec,
		app.GetKey(tokenfactorytypes.StoreKey),
		GetMaccPerms(),
		app.AuthKeeper,
		app.BankKeeper,
		app.DistrKeeper,
		tokenFactoryCapabilities,
		govModuleAddr,
	)

	// Step 5: Register the module
	if err := app.RegisterModules(
		tokenfactory.NewAppModule(
			app.TokenFactoryKeeper,
			app.AuthKeeper,
			app.BankKeeper,
			tokenfactorysubspace,
		),
	); err != nil {
		return err
	}

	return nil
}

// RegisterTokenFactory registers the TokenFactory module for CLI.
// This is needed because tokenfactory doesn't support depinject yet.
func RegisterTokenFactory(cdc codec.Codec) map[string]appmodule.AppModule {
	modules := map[string]appmodule.AppModule{
		tokenfactorytypes.ModuleName: tokenfactory.NewAppModule(
			tokenfactorykeeper.Keeper{}, // Empty keeper for CLI registration
			nil,                         // AccountKeeper not needed for CLI
			nil,                         // BankKeeper not needed for CLI
			nil,                         // Subspace not needed for CLI
		),
	}

	// Register interfaces for proper encoding/decoding
	for _, m := range modules {
		if mr, ok := m.(interface {
			RegisterInterfaces(codectypes.InterfaceRegistry)
		}); ok {
			mr.RegisterInterfaces(cdc.InterfaceRegistry())
		}
	}

	return modules
}
