package app

import (
	"math/big"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// ============================================================================
// Chain Identity Constants
// ============================================================================
const (
	// AppName is the application name used in various contexts
	AppName = "kudora"

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
	BaseDenomUnit int64 = 18
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
