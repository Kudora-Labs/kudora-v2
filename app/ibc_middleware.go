// app/ibc_middleware.go
package app

import (
	"cosmossdk.io/core/appmodule"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	packetforward "github.com/cosmos/ibc-apps/middleware/packet-forward-middleware/v10/packetforward"
	packetforwardkeeper "github.com/cosmos/ibc-apps/middleware/packet-forward-middleware/v10/packetforward/keeper"
	packetforwardtypes "github.com/cosmos/ibc-apps/middleware/packet-forward-middleware/v10/packetforward/types"

	ratelimit "github.com/cosmos/ibc-apps/modules/rate-limiting/v10"
	ratelimitkeeper "github.com/cosmos/ibc-apps/modules/rate-limiting/v10/keeper"
	ratelimittypes "github.com/cosmos/ibc-apps/modules/rate-limiting/v10/types"
)

// initIBCMiddlewareKeepers initializes the IBC middleware keepers
// This function should be called AFTER IBCKeeper and TransferKeeper are initialized
// but BEFORE the IBC router is configured
func (app *App) initIBCMiddlewareKeepers() error {
    // Get governance module address for authority
    govModuleAddr, err := app.AuthKeeper.AddressCodec().BytesToString(
        authtypes.NewModuleAddress(govtypes.ModuleName),
    )
    if err != nil {
        return err
    }
    
    // =========================================
    // Initialize Rate Limit Keeper
    // =========================================
    // Rate limiting protects against bridge exploits by limiting
    // the amount of tokens that can flow through IBC channels
    // within a specified time window
    app.RateLimitKeeper = ratelimitkeeper.NewKeeper(
        app.appCodec,
        runtime.NewKVStoreService(app.GetKey(ratelimittypes.StoreKey)),
        app.GetSubspace(ratelimittypes.ModuleName), // Required in v10
        govModuleAddr,
        app.BankKeeper,
        app.IBCKeeper.ChannelKeeper,
        app.IBCKeeper.ClientKeeper,     // Required in v10
		nil,
    )
    
    // =========================================
    // Initialize Packet Forward Middleware Keeper
    // =========================================
    // PFM enables multi-hop IBC transfers (A -> B -> C)
    // using the memo field to specify forwarding instructions
    app.PacketForwardKeeper = packetforwardkeeper.NewKeeper(
        app.appCodec,
        runtime.NewKVStoreService(app.GetKey(packetforwardtypes.StoreKey)),
        app.TransferKeeper,             // Now passed directly in v10
        app.IBCKeeper.ChannelKeeper,
        app.BankKeeper,
        nil,  // Required in v10
        govModuleAddr,
    )
    
    return nil
}

func RegisterPacketForward(codec codec.Codec) map[string]appmodule.AppModule{
    modules := map[string]appmodule.AppModule{
        packetforwardtypes.ModuleName: packetforward.NewAppModule(
            &packetforwardkeeper.Keeper{}, // Empty keeper for CLI registration
            nil,                         // Subspace not needed for CLI
        ),
    }

    // Register interfaces for proper encoding/decoding
    for _, m := range modules {
        if mr, ok := m.(interface {
            RegisterInterfaces(codectypes.InterfaceRegistry)
        }); ok {
            mr.RegisterInterfaces(codec.InterfaceRegistry())
        }
    }

    return modules
}

// RegisterRateLimit registers the ratelimit module for CLI.
// This is needed because ratelimit doesn't support depinject yet.
func RegisterRateLimit(codec codec.Codec) map[string]appmodule.AppModule{
    modules := map[string]appmodule.AppModule{
        ratelimittypes.ModuleName: ratelimit.NewAppModule(
            codec,
            ratelimitkeeper.Keeper{}, // Empty keeper for CLI registration
        ),
    }

    // Register interfaces for proper encoding/decoding
    for _, m := range modules {
        if mr, ok := m.(interface {
            RegisterInterfaces(codectypes.InterfaceRegistry)
        }); ok {
            mr.RegisterInterfaces(codec.InterfaceRegistry())
        }
    }

    return modules
}