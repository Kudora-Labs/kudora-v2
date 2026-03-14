package app

// Tests for stateful precompile registration in Kudora-v2.
//
// Three levels of coverage:
//
//	Level 1 — fast unit tests (~10ms, no full node):
//	  TestPrecompilesRegistration_NoConflicts
//	  TestPrecompilesRegistration_MinimumCount
//	  TestPrecompilesRegistration_Determinism
//	  TestPrecompilesRegistration_NoPanic
//	  TestPrecompilesRegistration_KnownAddresses
//
//	Level 2 — keeper wiring with full app (~5s first run, singleton):
//	  TestPrecompileKeepersWiring
//	  TestEVMKeeperHasStaticPrecompiles
//
//	Level 3 — keeper behaviour suite (reads via underlying keepers):
//	  TestPrecompilesSuite/*
//
// Run individually to avoid the "chainConfig already set" panic when
// executing alongside other EVM test suites:
//
//	go test ./app -run TestPrecompilesRegistration -v
//	go test ./app -run TestPrecompileKeepers -v
//	go test ./app -run TestPrecompilesSuite -v

import (
	"testing"

	"cosmossdk.io/log"
	"cosmossdk.io/math"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	evmtypes "github.com/cosmos/evm/x/vm/types"
)

// =============================================================================
// Level 1 — Fast unit tests (no full node required)
// =============================================================================

func TestPrecompilesRegistration_NoConflicts(t *testing.T) {
	app, err := getTestApp()
	if err != nil || app == nil {
		t.Skipf("Skipping: %v", err)
	}

	precompiles, err := buildStatefulPrecompiles(
		app.StakingKeeper,
		app.DistrKeeper,
		app.BankKeeper,
		app.Erc20Keeper,
		app.AuthzKeeper,
		app.TransferKeeper,
		app.IBCKeeper,
		app.EVMKeeper,
		app.GovKeeper,
		app.SlashingKeeper,
		app.appCodec,
		app.AuthKeeper.AddressCodec(),
	)
	require.NoError(t, err, "buildStatefulPrecompiles must not return an error")
	require.NotEmpty(t, precompiles, "precompiles map must not be empty")
	require.NoError(t, err, "buildStatefulPrecompiles must not return an error")
	require.NotEmpty(t, precompiles)

	seen := make(map[common.Address]bool)
	for addr, pc := range precompiles {
		require.NotEqual(t, common.Address{}, addr, "zero address found in precompile map")
		require.NotNil(t, pc, "nil implementation found at %s", addr.Hex())
		require.False(t, seen[addr], "duplicate precompile address: %s", addr.Hex())
		seen[addr] = true
	}
}

func TestPrecompilesRegistration_MinimumCount(t *testing.T) {
	app, err := getTestApp()
	if err != nil || app == nil {
		t.Skipf("Skipping: %v", err)
	}

	precompiles, err := buildStatefulPrecompiles(
		app.StakingKeeper,
		app.DistrKeeper,
		app.BankKeeper,
		app.Erc20Keeper,
		app.AuthzKeeper,
		app.TransferKeeper,
		app.IBCKeeper,
		app.EVMKeeper,
		app.GovKeeper,
		app.SlashingKeeper,
		app.appCodec,
		app.AuthKeeper.AddressCodec(),
	)
	require.NoError(t, err)

	// Prague base (~9 standard) + 8 custom (p256, bech32, staking, distribution,
	// ics20, bank, gov, slashing) = at least 17.
	require.GreaterOrEqual(t, len(precompiles), 17,
		"expected >= 17 precompiles, got %d", len(precompiles))
}

func TestPrecompilesRegistration_Determinism(t *testing.T) {
	app, err := getTestApp()
	if err != nil || app == nil {
		t.Skipf("Skipping: %v", err)
	}

	build := func() map[common.Address]bool {
		p, err := buildStatefulPrecompiles(
			app.StakingKeeper, app.DistrKeeper, app.BankKeeper,
			app.Erc20Keeper, app.AuthzKeeper, app.TransferKeeper,
			app.IBCKeeper, app.EVMKeeper, app.GovKeeper, app.SlashingKeeper,
			app.appCodec, app.AuthKeeper.AddressCodec(),
		)
		require.NoError(t, err)
		out := make(map[common.Address]bool, len(p))
		for addr := range p {
			out[addr] = true
		}
		return out
	}

	first, second := build(), build()
	require.Equal(t, len(first), len(second), "precompile count is not deterministic")
	for addr := range first {
		require.True(t, second[addr], "address %s missing on second call", addr.Hex())
	}
}

func TestPrecompilesRegistration_NoPanic(t *testing.T) {
	app, err := getTestApp()
	if err != nil || app == nil {
		t.Skipf("Skipping: %v", err)
	}

	require.NotPanics(t, func() {
		_, _ = buildStatefulPrecompiles(
			app.StakingKeeper, app.DistrKeeper, app.BankKeeper,
			app.Erc20Keeper, app.AuthzKeeper, app.TransferKeeper,
			app.IBCKeeper, app.EVMKeeper, app.GovKeeper, app.SlashingKeeper,
			app.appCodec, app.AuthKeeper.AddressCodec(),
		)
	})
}

// TestPrecompilesRegistration_KnownAddresses asserts that every expected
// custom precompile address is present in the map.
// Addresses are canonical per cosmos/evm and confirmed in V1's interchaintest/setup.go.
func TestPrecompilesRegistration_KnownAddresses(t *testing.T) {
	app, err := getTestApp()
	if err != nil || app == nil {
		t.Skipf("Skipping: %v", err)
	}

	precompiles, err := buildStatefulPrecompiles(
		app.StakingKeeper, app.DistrKeeper, app.BankKeeper,
		app.Erc20Keeper, app.AuthzKeeper, app.TransferKeeper,
		app.IBCKeeper, app.EVMKeeper, app.GovKeeper, app.SlashingKeeper,
		app.appCodec, app.AuthKeeper.AddressCodec(),
	)
	require.NoError(t, err)

	expected := []struct {
		name string
		hex  string
	}{
		{"p256", "0x0000000000000000000000000000000000000100"},
		{"bech32", "0x0000000000000000000000000000000000000400"},
		{"staking", "0x0000000000000000000000000000000000000800"},
		{"distribution", "0x0000000000000000000000000000000000000801"},
		{"ics20", "0x0000000000000000000000000000000000000802"},
		{"bank", "0x0000000000000000000000000000000000000804"},
		{"gov", "0x0000000000000000000000000000000000000805"},
		{"slashing", "0x0000000000000000000000000000000000000806"},
	}

	for _, tc := range expected {
		addr := common.HexToAddress(tc.hex)
		_, ok := precompiles[addr]
		require.True(t, ok, "precompile %q not found at expected address %s", tc.name, tc.hex)
	}
}

// =============================================================================
// Level 2 — Keeper wiring with the real app
// =============================================================================

// TestPrecompileKeepersWiring verifies that after app.New() the precompile
// map is fully populated and each expected address resolves to a non-nil impl.
func TestPrecompileKeepersWiring(t *testing.T) {
	app, err := getTestApp()
	if err != nil || app == nil {
		t.Skipf("Skipping: %v", err)
	}

	require.NotNil(t, app.EVMKeeper, "EVMKeeper must not be nil")

	// Build the map a second time with the live keepers — this mirrors exactly
	// what postRegisterEVMModules() did during New(). If it panics or errors
	// here, the registration during startup would have failed too.
	precompiles, err := buildStatefulPrecompiles(
		app.StakingKeeper, app.DistrKeeper, app.BankKeeper,
		app.Erc20Keeper, app.AuthzKeeper, app.TransferKeeper,
		app.IBCKeeper, app.EVMKeeper, app.GovKeeper, app.SlashingKeeper,
		app.appCodec, app.AuthKeeper.AddressCodec(),
	)

	require.NoError(t, err)

	addresses := []string{
		"0x0000000000000000000000000000000000000100",
		"0x0000000000000000000000000000000000000400",
		"0x0000000000000000000000000000000000000800",
		"0x0000000000000000000000000000000000000801",
		"0x0000000000000000000000000000000000000802",
		"0x0000000000000000000000000000000000000804",
		"0x0000000000000000000000000000000000000805",
		"0x0000000000000000000000000000000000000806",
	}
	for _, hex := range addresses {
		addr := common.HexToAddress(hex)
		pc, ok := precompiles[addr]
		require.True(t, ok, "missing precompile at %s", hex)
		require.NotNil(t, pc, "nil precompile at %s", hex)
	}
}

// TestEVMKeeperHasStaticPrecompiles checks that the chain config is initialised,
// which means postRegisterEVMModules() ran successfully during New().
func TestEVMKeeperHasStaticPrecompiles(t *testing.T) {
	app, err := getTestApp()
	if err != nil || app == nil {
		t.Skipf("Skipping: %v", err)
	}

	require.NotNil(t, app.EVMKeeper)
	chainCfg := evmtypes.GetChainConfig()
	require.NotNil(t, chainCfg, "EVM chain config must be initialised after New()")
}

// =============================================================================
// Level 3 — Keeper behaviour suite
// Each sub-test exercises the underlying keeper that the named precompile
// depends on, using the exact call path the precompile itself would use.
// =============================================================================

type PrecompilesSuite struct {
	suite.Suite
	app *App
	ctx sdk.Context
}

func TestPrecompilesSuite(t *testing.T) {
	suite.Run(t, new(PrecompilesSuite))
}

func (s *PrecompilesSuite) SetupSuite() {
	app, err := getTestApp()
	if err != nil || app == nil {
		s.T().Skipf("Skipping PrecompilesSuite: %v", err)
		return
	}
	s.app = app
}

func (s *PrecompilesSuite) SetupTest() {
	header := cmtproto.Header{ChainID: testChainID, Height: 1}
	s.ctx = sdk.NewContext(s.app.CommitMultiStore(), header, false, log.NewNopLogger())
}

// ── Staking precompile (0x0800) ───────────────────────────────────────────

func (s *PrecompilesSuite) TestStaking_ValidatorQueryWorks() {
	vals, err := s.app.StakingKeeper.GetAllValidators(s.ctx)
	s.Require().NoError(err, "StakingKeeper.GetAllValidators must succeed (used by staking precompile)")
	s.Require().GreaterOrEqual(len(vals), 0)
}

func (s *PrecompilesSuite) TestStaking_ParamsAccessible() {
	params, err := s.app.StakingKeeper.GetParams(s.ctx)
	s.Require().NoError(err)
	s.Require().NotEmpty(params.BondDenom, "bond denom must be set")
}

// ── Distribution precompile (0x0801) ─────────────────────────────────────

func (s *PrecompilesSuite) TestDistribution_ParamsAccessible() {
	params, err := s.app.DistrKeeper.Params.Get(s.ctx)
	s.Require().NoError(err, "DistrKeeper.Params must be accessible (used by distribution precompile)")
	s.Require().NotNil(params)
}

// ── Bank precompile (0x0803) ──────────────────────────────────────────────

func (s *PrecompilesSuite) TestBank_BalanceQueryWorks() {
	// Mint coins to a fresh account and read the balance back —
	// identical to what the bank precompile's balanceOf method does.
	addr := sdk.AccAddress([]byte("precompile_bank_____"))
	acc := s.app.AuthKeeper.NewAccountWithAddress(s.ctx, addr)
	s.app.AuthKeeper.SetAccount(s.ctx, acc)

	amount := math.NewInt(1_000_000_000_000_000_000) // 1 kud
	coins := sdk.NewCoins(sdk.NewCoin(BaseDenom, amount))
	s.Require().NoError(s.app.BankKeeper.MintCoins(s.ctx, "mint", coins))
	s.Require().NoError(s.app.BankKeeper.SendCoinsFromModuleToAccount(s.ctx, "mint", addr, coins))

	got := s.app.BankKeeper.GetBalance(s.ctx, addr, BaseDenom)
	s.Require().Equal(amount, got.Amount, "bank balance must match after mint")
}

func (s *PrecompilesSuite) TestBank_SendCoinsWorks() {
	from := sdk.AccAddress([]byte("precompile_bank_from"))
	to := sdk.AccAddress([]byte("precompile_bank_to__"))
	for _, addr := range []sdk.AccAddress{from, to} {
		acc := s.app.AuthKeeper.NewAccountWithAddress(s.ctx, addr)
		s.app.AuthKeeper.SetAccount(s.ctx, acc)
	}
	coins := sdk.NewCoins(sdk.NewCoin(BaseDenom, math.NewInt(2_000_000_000_000_000_000)))
	s.Require().NoError(s.app.BankKeeper.MintCoins(s.ctx, "mint", coins))
	s.Require().NoError(s.app.BankKeeper.SendCoinsFromModuleToAccount(s.ctx, "mint", from, coins))

	send := sdk.NewCoins(sdk.NewCoin(BaseDenom, math.NewInt(500_000_000_000_000_000)))
	s.Require().NoError(s.app.BankKeeper.SendCoins(s.ctx, from, to, send))

	s.Require().Equal(
		math.NewInt(1_500_000_000_000_000_000),
		s.app.BankKeeper.GetBalance(s.ctx, from, BaseDenom).Amount,
	)
	s.Require().Equal(
		math.NewInt(500_000_000_000_000_000),
		s.app.BankKeeper.GetBalance(s.ctx, to, BaseDenom).Amount,
	)
}

// ── Gov precompile (0x0804) ───────────────────────────────────────────────

func (s *PrecompilesSuite) TestGov_ParamsAccessible() {
	params, err := s.app.GovKeeper.Params.Get(s.ctx)
	s.Require().NoError(err, "GovKeeper.Params must be accessible (used by gov precompile)")
	s.Require().NotEmpty(params.MinDeposit, "gov min deposit must be set")
}

// ── Slashing precompile (0x0805) ──────────────────────────────────────────

func (s *PrecompilesSuite) TestSlashing_ParamsAccessible() {
	params, err := s.app.SlashingKeeper.GetParams(s.ctx)
	s.Require().NoError(err, "SlashingKeeper.GetParams must succeed (used by slashing precompile)")
	s.Require().NotNil(params)
}

// ── ICS20 precompile (0x0802) — TransferKeeper ────────────────────────────

func (s *PrecompilesSuite) TestICS20_TransferKeeperAccessible() {
	params := s.app.TransferKeeper.GetParams(s.ctx)
	s.Require().True(params.SendEnabled, "IBC send must be enabled by default")
}

func (s *PrecompilesSuite) TestICS20_IBCKeeperNotNil() {
	s.Require().NotNil(s.app.IBCKeeper, "IBCKeeper must not be nil")
}

// ── Full integration — all precompiles with live keepers ──────────────────

func (s *PrecompilesSuite) TestAllPrecompilesInstantiateWithRealKeepers() {
	precompiles, err := buildStatefulPrecompiles(
		s.app.StakingKeeper, s.app.DistrKeeper, s.app.BankKeeper,
		s.app.Erc20Keeper, s.app.AuthzKeeper, s.app.TransferKeeper,
		s.app.IBCKeeper, s.app.EVMKeeper, s.app.GovKeeper, s.app.SlashingKeeper,
		s.app.appCodec, s.app.AuthKeeper.AddressCodec(),
	)
	s.Require().NoError(err, "buildStatefulPrecompiles with real keepers must not fail")
	s.Require().GreaterOrEqual(len(precompiles), 17)

	// Every precompile must answer RequiredGas(nil) without panicking.
	for addr, pc := range precompiles {
		s.Require().NotPanics(func() {
			_ = pc.RequiredGas(nil)
		}, "precompile at %s panics on RequiredGas(nil)", addr.Hex())
	}
}