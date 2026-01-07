package app

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"sync"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	evmtypes "github.com/cosmos/evm/x/vm/types"
)

// ============================================================================
// Chain Identity Constants
// ============================================================================
const (
	// AppName is the application name used in various contexts
	// Name already defined in app.go
	AppName = Name

	// DefaultChainID is the default chain identifier for mainnet
	// Format: {name}_{evm_chain_id}-{revision}
	DefaultChainID = "kudora_12000-1"
)

// ============================================================================
// Token Constants
// ============================================================================
const (
	// BaseDenom is the smallest unit of the native token
	// This is what gets stored on-chain and used in transactions
	BaseDenom = "kud"

	// DisplayDenom is the human-readable denomination
	// Used for display purposes in wallets and explorers
	DisplayDenom = "kudos"

	// BaseDenomUnit represents the number of decimal places
	// Set to 18 for EVM compatibility (same as Ethereum's wei)
	BaseDenomUnit = 18
)

// ============================================================================
// Bech32 Prefix Configuration
// ============================================================================

// These prefixes determine how different types of addresses are encoded
// in the human-readable bech32 format
var (
	// Bech32PrefixAccAddr is the prefix for account addresses
	// Example: kudo1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5lzv7xu
	Bech32PrefixAccAddr = AccountAddressPrefix

	// Bech32PrefixAccPub is the prefix for account public keys
	// Example: kudopub1addwnpepq...
	Bech32PrefixAccPub = AccountAddressPrefix + "pub"

	// Bech32PrefixValAddr is the prefix for validator operator addresses
	// Example: kudovaloper1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5h7r5...
	Bech32PrefixValAddr = AccountAddressPrefix + "valoper"

	// Bech32PrefixValPub is the prefix for validator operator public keys
	// Example: kudovaloperpub1addwnpepq...
	Bech32PrefixValPub = AccountAddressPrefix + "valoperpub"

	// Bech32PrefixConsAddr is the prefix for consensus node addresses
	// Example: kudovalcons1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5m3k...
	Bech32PrefixConsAddr = AccountAddressPrefix + "valcons"

	// Bech32PrefixConsPub is the prefix for consensus node public keys
	// Example: kudovalconspub1addwnpepq...
	Bech32PrefixConsPub = AccountAddressPrefix + "valconspub"
)

// ============================================================================
// EVM Configuration Constants
// ============================================================================
const (
	// EVMChainID is the chain ID used by the EVM
	// This is what MetaMask and other EVM wallets will use
	// Extracted from the chain ID format: kudora_{evm_chain_id}-{revision}
	EVMChainID = 12000

	// CoinType is the BIP-44 coin type for HD wallet derivation
	// 60 is the coin type for Ethereum, required for EVM compatibility
	CoinType = 60
)

// ============================================================================
// EVM Coin Info Configuration
// ============================================================================

// ChainsCoinInfo maps chain IDs to their EVM token configuration
// This is used by the EVM module to properly handle token decimals
// and display names across different network configurations
var ChainsCoinInfo = map[string]evmtypes.EvmCoinInfo{
	// Configuration for mainnet (with -1 suffix)
	DefaultChainID: {
		Denom:         BaseDenom,
		ExtendedDenom: BaseDenom,
		DisplayDenom:  DisplayDenom,
		Decimals:      evmtypes.EighteenDecimals,
	},
	// Configuration for chain ID without revision suffix
	// This allows lookup by just "kudora_12000"
	"kudora_12000": {
		Denom:         BaseDenom,
		ExtendedDenom: BaseDenom,
		DisplayDenom:  DisplayDenom,
		Decimals:      evmtypes.EighteenDecimals,
	},
	// Configuration for local development
	"kudora_9000-1": {
		Denom:         BaseDenom,
		ExtendedDenom: BaseDenom,
		DisplayDenom:  DisplayDenom,
		Decimals:      evmtypes.EighteenDecimals,
	},
}

// ============================================================================
// EVM Configuration State
// ============================================================================

// evmInitOnce ensures EVMAppOptions initialization happens exactly once
// even when called from multiple goroutines concurrently
var evmInitOnce sync.Once

// evmInitErr stores any error that occurred during EVM initialization
var evmInitErr error

// ============================================================================
// EVM Application Options
// ============================================================================

// EVMAppOptions configures the EVM module with Kudora-specific settings
// This function MUST be called during application startup before any EVM operations
// It configures:
// - Token denomination registration with the SDK
// - EVM chain configuration (gas limits, opcodes, etc.)
// - Decimal precision settings for EVM transactions
//
// Thread-safe: Uses sync.Once to ensure initialization happens exactly once,
// even when called concurrently from multiple goroutines.
func EVMAppOptions(chainID string) error {
	// Ensure initialization happens exactly once, thread-safely
	evmInitOnce.Do(func() {
		evmInitErr = initEVM(chainID)
	})
	return evmInitErr
}

// initEVM performs the actual EVM initialization
// This function is called exactly once via sync.Once
func initEVM(chainID string) error {
	// Use default chain ID if none provided
	if chainID == "" {
		chainID = DefaultChainID
	}

	// Extract the base chain ID without revision suffix
	// Example: "kudora_12000-1" -> "kudora_12000"
	baseID := strings.Split(chainID, "-")[0]

	// Look up coin info, first by base ID, then by full chain ID
	coinInfo, found := ChainsCoinInfo[baseID]
	if !found {
		coinInfo, found = ChainsCoinInfo[chainID]
		if !found {
			return fmt.Errorf("unknown chain id: %s (not found in ChainsCoinInfo)", chainID)
		}
	}

	// Register token denominations with the Cosmos SDK
	// This enables proper conversion between base and display units
	if err := setBaseDenom(coinInfo); err != nil {
		return fmt.Errorf("failed to set base denom: %w", err)
	}

	// Get the default Ethereum chain configuration (expects uint64 EVM chain id)
	evmChainID, err := parseEVMChainID(chainID)
	if err != nil {
		return fmt.Errorf("failed to parse evm chain id from %q: %w", chainID, err)
	}
	ethCfg := evmtypes.DefaultChainConfig(evmChainID)

	cfg := evmtypes.NewEVMConfigurator().
		WithChainConfig(ethCfg).
		WithEVMCoinInfo(coinInfo)

	// Configure the EVM with our settings
	err = cfg.Configure()
	if err != nil {
		return fmt.Errorf("failed to configure EVM: %w", err)
	}

	return nil
}

// setBaseDenom registers the token denominations with the Cosmos SDK
// This establishes the relationship between base units (kud) and display units (kudos)
func setBaseDenom(ci evmtypes.EvmCoinInfo) error {
	// Register the display denomination (1 kudos = 1.0)
	if err := sdk.RegisterDenom(ci.DisplayDenom, math.LegacyOneDec()); err != nil {
		return fmt.Errorf("failed to register display denom %s: %w", ci.DisplayDenom, err)
	}

	// Register the base denomination with 18 decimal places
	// 1 kud = 0.000000000000000001 kudos (10^-18)
	baseDenomPrecision := math.LegacyNewDecWithPrec(1, int64(ci.Decimals))
	if err := sdk.RegisterDenom(ci.Denom, baseDenomPrecision); err != nil {
		return fmt.Errorf("failed to register base denom %s: %w", ci.Denom, err)
	}

	return nil
}

// parseEVMChainID extracts the numeric EVM chain id from a Cosmos chain-id.
// Examples:
// - "kudora_12000-1" -> 12000
// - "12000"         -> 12000
func parseEVMChainID(chainID string) (uint64, error) {
	chainID = strings.TrimSpace(chainID)
	if chainID == "" {
		return 0, fmt.Errorf("empty chain id")
	}

	// If chainID is already numeric, accept it.
	if n, err := strconv.ParseUint(chainID, 10, 64); err == nil {
		return n, nil
	}

	// Typical format: "<name>_<evmChainID>-<revision>"
	parts := strings.Split(chainID, "_")
	if len(parts) < 2 {
		return 0, fmt.Errorf("cannot extract evm chain id from %q", chainID)
	}

	last := parts[len(parts)-1]         // e.g. "12000-1"
	evmPart := strings.Split(last, "-")[0] // e.g. "12000"

	n, err := strconv.ParseUint(evmPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid evm chain id %q in %q: %w", evmPart, chainID, err)
	}
	return n, nil
}

func init() {
	// Set bond denom
	sdk.DefaultBondDenom = BaseDenom

	// Configure the default power reduction for staking calculations.
	// This must match the token decimals (BaseDenomUnit) for correct staking math.
	// Power = TokenAmount / 10^BaseDenomUnit
	sdk.DefaultPowerReduction = math.NewIntFromBigInt(
		new(big.Int).Exp(big.NewInt(10), big.NewInt(BaseDenomUnit), nil),
	)

	// Set address prefixes
	accountPubKeyPrefix := Bech32PrefixAccPub
	validatorAddressPrefix := Bech32PrefixValAddr
	validatorPubKeyPrefix := Bech32PrefixValPub
	consNodeAddressPrefix := Bech32PrefixConsAddr
	consNodePubKeyPrefix := Bech32PrefixConsPub

	// Set and seal config
	config := sdk.GetConfig()

	// SetCoinConfig configures the SDK with Kudora-specific coin settings
	config.SetCoinType(CoinType)
	config.SetPurpose(44) // Standard BIP-44 purpose

	// SetBech32Prefixes configures the SDK with Kudora-specific bech32 prefixes
	config.SetBech32PrefixForAccount(AccountAddressPrefix, accountPubKeyPrefix)
	config.SetBech32PrefixForValidator(validatorAddressPrefix, validatorPubKeyPrefix)
	config.SetBech32PrefixForConsensusNode(consNodeAddressPrefix, consNodePubKeyPrefix)
	config.Seal()
}
