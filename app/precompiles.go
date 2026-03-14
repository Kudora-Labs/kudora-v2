package app

import (
	"fmt"
	"maps"

	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	distributionkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"
	slashingkeeper "github.com/cosmos/cosmos-sdk/x/slashing/keeper"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"

	bankprecompile "github.com/cosmos/evm/precompiles/bank"
	"github.com/cosmos/evm/precompiles/bech32"
	distprecompile "github.com/cosmos/evm/precompiles/distribution"
	govprecompile "github.com/cosmos/evm/precompiles/gov"
	ics20precompile "github.com/cosmos/evm/precompiles/ics20"
	"github.com/cosmos/evm/precompiles/p256"
	slashingprecompile "github.com/cosmos/evm/precompiles/slashing"
	stakingprecompile "github.com/cosmos/evm/precompiles/staking"
	erc20keeper "github.com/cosmos/evm/x/erc20/keeper"
	evmkeeper "github.com/cosmos/evm/x/vm/keeper"
	ibctransferkeeper "github.com/cosmos/ibc-go/v10/modules/apps/transfer/keeper"
	"github.com/ethereum/go-ethereum/common"
	gethvm "github.com/ethereum/go-ethereum/core/vm"

	ibckeeper "github.com/cosmos/ibc-go/v10/modules/core/keeper"

	"cosmossdk.io/core/address"
	"github.com/cosmos/cosmos-sdk/codec"
)

const kudoraBech32PrecompileBaseGas = 6_000

// buildStatefulPrecompiles constructs the complete precompile map for Kudora.
//
// Registered precompiles (addresses per interchaintest/setup.go):
//
//	0x0100  p256        secp256r1 (EIP-7212), stateless
//	0x0400  bech32      Cosmos address encoding, stateless
//	0x0800  staking     delegate / undelegate / queries
//	0x0801  distribution withdraw rewards / queries
//	0x0802  ics20       IBC transfer from Solidity
//	0x0804  bank        native coin balances / send
//	0x0805  gov         on-chain governance voting
//	0x0806  slashing    jail status / unjail queries
//
// Note: the evidence precompile was removed upstream in cosmos/evm (PR #305).
// It is intentionally absent here.
func buildStatefulPrecompiles(
	stakingKeeper *stakingkeeper.Keeper,
	distributionKeeper distributionkeeper.Keeper,
	bankKeeper bankkeeper.Keeper,
	erc20Keeper erc20keeper.Keeper,
	authzKeeper authzkeeper.Keeper,
	transferKeeper ibctransferkeeper.Keeper,
	ibcKeeper *ibckeeper.Keeper,
	evmKeeper *evmkeeper.Keeper,
	govKeeper *govkeeper.Keeper,
	slashingKeeper slashingkeeper.Keeper,
	appCodec codec.Codec,
	addrCdc address.Codec,
) (map[common.Address]gethvm.PrecompiledContract, error) {
	// Clone Prague as the base — V2 uses gethvm modern fork.
	// V1 used Berlin; Prague is the correct base for cosmos/evm >= v0.4.0.
	precompiles := maps.Clone(gethvm.PrecompiledContractsPrague)

	// ── Stateless ───────────────────────────────────────────────────────────

	p256Precompile := &p256.Precompile{}

	bech32Precompile, err := bech32.NewPrecompile(kudoraBech32PrecompileBaseGas)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate bech32 precompile: %w", err)
	}

	// ── Stateful ────────────────────────────────────────────────────────────

	// Staking: delegate, undelegate, redelegate, validator queries.
	// Required by liquid staking protocols calling these methods from Solidity.
	stakingPrecompile := stakingprecompile.NewPrecompile(
		*stakingKeeper,
		stakingkeeper.NewMsgServerImpl(stakingKeeper),
		stakingkeeper.NewQuerier(stakingKeeper),
		bankKeeper,
		addrCdc,
	)

	// Distribution: withdraw delegator rewards, validator commission queries.
	// Required by auto-compounding protocols and reward dashboards.
	distributionPrecompile := distprecompile.NewPrecompile(
		distributionKeeper,
		distributionkeeper.NewMsgServerImpl(distributionKeeper),
		distributionkeeper.NewQuerier(distributionKeeper),
		*stakingKeeper,
		bankKeeper,
		addrCdc,
	)

	// ICS20: initiate IBC transfers from Solidity in the same tx as a swap.
	// ibc-go v10: ChannelKeeper is accessed via ibcKeeper.ChannelKeeper,
	// not as a standalone injected keeper.
	ibcTransferPrecompile := ics20precompile.NewPrecompile(
		bankKeeper,
		*stakingKeeper,
		transferKeeper,
		ibcKeeper.ChannelKeeper,
		erc20Keeper,
	)

	// Bank: query native coin balances and send coins from Solidity.
	// Bridges the ERC20 world and the native Cosmos bank module.
	bankPrecompile := bankprecompile.NewPrecompile(bankKeeper, erc20Keeper)

	// Gov: vote on on-chain proposals from a Solidity DAO contract.
	govPrecompile := govprecompile.NewPrecompile(
		govkeeper.NewMsgServerImpl(govKeeper),
		govkeeper.NewQueryServer(govKeeper),
		bankKeeper,
		appCodec,
		addrCdc,
	)

	// Slashing: query jail status, signing info. Used by validator dashboards.
	slashingPrecompile := slashingprecompile.NewPrecompile(
		slashingKeeper,
		slashingkeeper.NewMsgServerImpl(slashingKeeper),
		bankKeeper,
		stakingKeeper.ValidatorAddressCodec(),
		stakingKeeper.ConsensusAddressCodec(),
	)

	// ── Registration ────────────────────────────────────────────────────────

	precompiles[p256Precompile.Address()] = p256Precompile
	precompiles[bech32Precompile.Address()] = bech32Precompile
	precompiles[stakingPrecompile.Address()] = stakingPrecompile
	precompiles[distributionPrecompile.Address()] = distributionPrecompile
	precompiles[ibcTransferPrecompile.Address()] = ibcTransferPrecompile
	precompiles[bankPrecompile.Address()] = bankPrecompile
	precompiles[govPrecompile.Address()] = govPrecompile
	precompiles[slashingPrecompile.Address()] = slashingPrecompile

	return precompiles, nil
}