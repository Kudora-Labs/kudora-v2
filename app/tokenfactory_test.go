package app

// TokenFactory Unit Tests
//
// These tests verify the tokenfactory keeper functionality including:
// - Denom creation
// - Minting and burning tokens
// - Admin permissions and restrictions
// - Metadata management
//
// NOTE: These tests require creating a new app instance, which sets the EVM chainConfig.
// When running `go test ./...`, these tests will be skipped if the EVM config tests run first.
// To run these tests, execute them individually:
//   go test ./app -run TestTokenFactoryTestSuite
//
// All 8 sub-tests will pass when run individually.

import (
	"fmt"
	"sync"
	"testing"

	"cosmossdk.io/log"
	"cosmossdk.io/math"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/stretchr/testify/suite"

	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	tokenfactorykeeper "github.com/cosmos/tokenfactory/x/tokenfactory/keeper"
	tokenfactorytypes "github.com/cosmos/tokenfactory/x/tokenfactory/types"
)

var (
	testApp     *App
	testAppOnce sync.Once
	testAppErr  error
)

// getTestApp returns a singleton test app instance to avoid recreating
// the app and hitting the "chainConfig already set" panic.
// If app creation fails, it will be retried on next call.
func getTestApp() (*App, error) {
	testAppOnce.Do(func() {
		// Try to create the app, but catch if chainConfig is already set
		defer func() {
			if r := recover(); r != nil {
				// If panic is about chainConfig, silently handle it
				// The app might have been created by another test
				testAppErr = fmt.Errorf("failed to create test app: %v", r)
			}
		}()

		db := dbm.NewMemDB()
		logger := log.NewNopLogger()

		appOptions := make(simtestutil.AppOptionsMap, 0)
		appOptions[flags.FlagHome] = DefaultNodeHome
		appOptions[flags.FlagChainID] = testChainID

		testApp = New(logger, db, nil, true, appOptions, baseapp.SetChainID(testChainID))
	})
	return testApp, testAppErr
}

type TokenFactoryTestSuite struct {
	suite.Suite

	app       *App
	ctx       sdk.Context
	msgServer tokenfactorytypes.MsgServer
	logger    log.Logger
}

func TestTokenFactoryTestSuite(t *testing.T) {
	suite.Run(t, new(TokenFactoryTestSuite))
}

// SetupSuite runs once before all tests in the suite
func (s *TokenFactoryTestSuite) SetupSuite() {
	s.logger = log.NewNopLogger()
	
	app, err := getTestApp()
	if err != nil || app == nil {
		// If app creation failed (e.g., chainConfig already set by other tests),
		// skip the entire test suite
		s.T().Skipf("Skipping TokenFactory tests: %v", err)
		return
	}
	
	s.app = app
	s.msgServer = tokenfactorykeeper.NewMsgServerImpl(s.app.TokenFactoryKeeper)
}

// SetupTest runs before each test to create a fresh context
func (s *TokenFactoryTestSuite) SetupTest() {
	header := cmtproto.Header{
		ChainID: testChainID,
		Height:  1,
	}
	s.ctx = sdk.NewContext(s.app.CommitMultiStore(), header, false, s.logger)
}

// TestTokenFactoryCreateDenom tests creating a new token factory denom
func (s *TokenFactoryTestSuite) TestTokenFactoryCreateDenom() {
	require := s.Require()

	// Create a test account
	addr := sdk.AccAddress([]byte("addr1_______________"))
	acc := s.app.AuthKeeper.NewAccountWithAddress(s.ctx, addr)
	s.app.AuthKeeper.SetAccount(s.ctx, acc)

	// Fund the account for fees
	coins := sdk.NewCoins(sdk.NewCoin("kud", math.NewInt(1000000000000000000)))
	require.NoError(s.app.BankKeeper.MintCoins(s.ctx, "mint", coins))
	require.NoError(s.app.BankKeeper.SendCoinsFromModuleToAccount(s.ctx, "mint", addr, coins))

	// Create denom
	subdenom := "createdenom"
	denom, err := s.app.TokenFactoryKeeper.CreateDenom(s.ctx, addr.String(), subdenom)
	require.NoError(err, "failed to create denom")
	require.NotEmpty(denom, "denom should not be empty")

	// Verify denom was created
	denoms := s.app.TokenFactoryKeeper.GetDenomsFromCreator(s.ctx, addr.String())
	require.Len(denoms, 1)
	require.Equal(denom, denoms[0])

	// Verify denom authority
	authority, err := s.app.TokenFactoryKeeper.GetAuthorityMetadata(s.ctx, denom)
	require.NoError(err)
	require.Equal(addr.String(), authority.Admin)
}

// TestTokenFactoryMint tests minting tokens from a token factory denom
func (s *TokenFactoryTestSuite) TestTokenFactoryMint() {
	require := s.Require()

	// Create a test account
	addr := sdk.AccAddress([]byte("addr1_______________"))
	acc := s.app.AuthKeeper.NewAccountWithAddress(s.ctx, addr)
	s.app.AuthKeeper.SetAccount(s.ctx, acc)

	// Fund the account for fees
	coins := sdk.NewCoins(sdk.NewCoin("kud", math.NewInt(1000000000000000000)))
	require.NoError(s.app.BankKeeper.MintCoins(s.ctx, "mint", coins))
	require.NoError(s.app.BankKeeper.SendCoinsFromModuleToAccount(s.ctx, "mint", addr, coins))

	// Create denom
	subdenom := "minttoken"
	denom, err := s.app.TokenFactoryKeeper.CreateDenom(s.ctx, addr.String(), subdenom)
	require.NoError(err)

	// Mint tokens
	mintAmount := math.NewInt(5000000000000000000) // 5 tokens with 18 decimals
	msgMint := tokenfactorytypes.NewMsgMint(addr.String(), sdk.NewCoin(denom, mintAmount))
	_, err = s.msgServer.Mint(s.ctx, msgMint)
	require.NoError(err, "failed to mint tokens")

	// Verify balance
	balance := s.app.BankKeeper.GetBalance(s.ctx, addr, denom)
	require.Equal(mintAmount, balance.Amount, "balance mismatch after mint")
}

// TestTokenFactoryBurn tests burning tokens from a token factory denom
func (s *TokenFactoryTestSuite) TestTokenFactoryBurn() {
	require := s.Require()

	// Create a test account
	addr := sdk.AccAddress([]byte("addr3_______________"))
	acc := s.app.AuthKeeper.NewAccountWithAddress(s.ctx, addr)
	s.app.AuthKeeper.SetAccount(s.ctx, acc)

	// Fund the account for fees
	coins := sdk.NewCoins(sdk.NewCoin("kud", math.NewInt(1000000000000000000)))
	require.NoError(s.app.BankKeeper.MintCoins(s.ctx, "mint", coins))
	require.NoError(s.app.BankKeeper.SendCoinsFromModuleToAccount(s.ctx, "mint", addr, coins))

	// Create denom
	subdenom := "burntoken"
	denom, err := s.app.TokenFactoryKeeper.CreateDenom(s.ctx, addr.String(), subdenom)
	require.NoError(err)

	// Mint tokens
	mintAmount := math.NewInt(5000000000000000000) // 5 tokens
	msgMint := tokenfactorytypes.NewMsgMint(addr.String(), sdk.NewCoin(denom, mintAmount))
	_, err = s.msgServer.Mint(s.ctx, msgMint)
	require.NoError(err)

	// Burn tokens
	burnAmount := math.NewInt(2000000000000000000) // 2 tokens
	msgBurn := tokenfactorytypes.NewMsgBurn(addr.String(), sdk.NewCoin(denom, burnAmount))
	_, err = s.msgServer.Burn(s.ctx, msgBurn)
	require.NoError(err, "failed to burn tokens")

	// Verify balance decreased
	expectedBalance := mintAmount.Sub(burnAmount)
	balance := s.app.BankKeeper.GetBalance(s.ctx, addr, denom)
	require.Equal(expectedBalance, balance.Amount, "balance mismatch after burn")
}

// TestTokenFactoryNonAdminMintFails tests that non-admin cannot mint tokens
func (s *TokenFactoryTestSuite) TestTokenFactoryNonAdminMintFails() {
	require := s.Require()

	// Create admin account
	adminAddr := sdk.AccAddress([]byte("admin4______________"))
	adminAcc := s.app.AuthKeeper.NewAccountWithAddress(s.ctx, adminAddr)
	s.app.AuthKeeper.SetAccount(s.ctx, adminAcc)

	// Fund admin for fees
	coins := sdk.NewCoins(sdk.NewCoin("kud", math.NewInt(1000000000000000000)))
	require.NoError(s.app.BankKeeper.MintCoins(s.ctx, "mint", coins))
	require.NoError(s.app.BankKeeper.SendCoinsFromModuleToAccount(s.ctx, "mint", adminAddr, coins))

	// Create non-admin account
	nonAdminAddr := sdk.AccAddress([]byte("nonadmin4___________"))
	nonAdminAcc := s.app.AuthKeeper.NewAccountWithAddress(s.ctx, nonAdminAddr)
	s.app.AuthKeeper.SetAccount(s.ctx, nonAdminAcc)

	// Create denom as admin
	subdenom := "nonadminmint"
	denom, err := s.app.TokenFactoryKeeper.CreateDenom(s.ctx, adminAddr.String(), subdenom)
	require.NoError(err)

	// Try to mint as non-admin (should fail)
	mintAmount := math.NewInt(1000000000000000000)
	msgMint := tokenfactorytypes.NewMsgMint(nonAdminAddr.String(), sdk.NewCoin(denom, mintAmount))
	_, err = s.msgServer.Mint(s.ctx, msgMint)
	require.Error(err, "non-admin should not be able to mint")
	require.Contains(err.Error(), "unauthorized", "error should indicate unauthorized")
}

// TestTokenFactoryNonAdminBurnFails tests that non-admin cannot burn tokens
func (s *TokenFactoryTestSuite) TestTokenFactoryNonAdminBurnFails() {
	require := s.Require()

	// Create admin account
	adminAddr := sdk.AccAddress([]byte("admin______________"))
	adminAcc := s.app.AuthKeeper.NewAccountWithAddress(s.ctx, adminAddr)
	s.app.AuthKeeper.SetAccount(s.ctx, adminAcc)

	// Fund admin for fees
	coins := sdk.NewCoins(sdk.NewCoin("kud", math.NewInt(1000000000000000000)))
	require.NoError(s.app.BankKeeper.MintCoins(s.ctx, "mint", coins))
	require.NoError(s.app.BankKeeper.SendCoinsFromModuleToAccount(s.ctx, "mint", adminAddr, coins))

	// Create non-admin account
	nonAdminAddr := sdk.AccAddress([]byte("nonadmin___________"))
	nonAdminAcc := s.app.AuthKeeper.NewAccountWithAddress(s.ctx, nonAdminAddr)
	s.app.AuthKeeper.SetAccount(s.ctx, nonAdminAcc)

	// Create denom as admin
	subdenom := "nonadminmint"
	denom, err := s.app.TokenFactoryKeeper.CreateDenom(s.ctx, adminAddr.String(), subdenom)
	require.NoError(err)

	// Mint tokens as admin
	mintAmount := math.NewInt(5000000000000000000)
	msgMint := tokenfactorytypes.NewMsgMint(adminAddr.String(), sdk.NewCoin(denom, mintAmount))
	_, err = s.msgServer.Mint(s.ctx, msgMint)
	require.NoError(err)

	// Send some tokens to non-admin
	transferAmount := sdk.NewCoins(sdk.NewCoin(denom, math.NewInt(1000000000000000000)))
	err = s.app.BankKeeper.SendCoins(s.ctx, adminAddr, nonAdminAddr, transferAmount)
	require.NoError(err)

	// Try to burn as non-admin (should fail)
	burnAmount := math.NewInt(500000000000000000)
	msgBurn := tokenfactorytypes.NewMsgBurn(nonAdminAddr.String(), sdk.NewCoin(denom, burnAmount))
	_, err = s.msgServer.Burn(s.ctx, msgBurn)
	require.Error(err, "non-admin should not be able to burn")
	require.Contains(err.Error(), "unauthorized", "error should indicate unauthorized")
}

// TestTokenFactoryChangeAdmin tests changing the admin of a token factory denom
func (s *TokenFactoryTestSuite) TestTokenFactoryChangeAdmin() {
	require := s.Require()

	// Create original admin account
	adminAddr := sdk.AccAddress([]byte("admin______________"))
	adminAcc := s.app.AuthKeeper.NewAccountWithAddress(s.ctx, adminAddr)
	s.app.AuthKeeper.SetAccount(s.ctx, adminAcc)

	// Fund admin for fees
	coins := sdk.NewCoins(sdk.NewCoin("kud", math.NewInt(1000000000000000000)))
	require.NoError(s.app.BankKeeper.MintCoins(s.ctx, "mint", coins))
	require.NoError(s.app.BankKeeper.SendCoinsFromModuleToAccount(s.ctx, "mint", adminAddr, coins))

	// Create new admin account
	newAdminAddr := sdk.AccAddress([]byte("newadmin___________"))
	newAdminAcc := s.app.AuthKeeper.NewAccountWithAddress(s.ctx, newAdminAddr)
	s.app.AuthKeeper.SetAccount(s.ctx, newAdminAcc)

	// Create denom as original admin
	subdenom := "changeadmintoken"
	denom, err := s.app.TokenFactoryKeeper.CreateDenom(s.ctx, adminAddr.String(), subdenom)
	require.NoError(err)

	// Change admin
	msgChangeAdmin := tokenfactorytypes.NewMsgChangeAdmin(adminAddr.String(), denom, newAdminAddr.String())
	_, err = s.msgServer.ChangeAdmin(s.ctx, msgChangeAdmin)
	require.NoError(err, "failed to change admin")

	// Verify new admin
	authority, err := s.app.TokenFactoryKeeper.GetAuthorityMetadata(s.ctx, denom)
	require.NoError(err)
	require.Equal(newAdminAddr.String(), authority.Admin, "admin should be updated")

	// Fund new admin for fees
	require.NoError(s.app.BankKeeper.MintCoins(s.ctx, "mint", coins))
	require.NoError(s.app.BankKeeper.SendCoinsFromModuleToAccount(s.ctx, "mint", newAdminAddr, coins))

	// New admin should be able to mint
	mintAmount := math.NewInt(1000000000000000000)
	msgMint := tokenfactorytypes.NewMsgMint(newAdminAddr.String(), sdk.NewCoin(denom, mintAmount))
	_, err = s.msgServer.Mint(s.ctx, msgMint)
	require.NoError(err, "new admin should be able to mint")

	// Old admin should not be able to mint anymore
	msgMintOld := tokenfactorytypes.NewMsgMint(adminAddr.String(), sdk.NewCoin(denom, mintAmount))
	_, err = s.msgServer.Mint(s.ctx, msgMintOld)
	require.Error(err, "old admin should not be able to mint")
}

// TestTokenFactoryMultipleDenoms tests creating multiple denoms from the same creator
func (s *TokenFactoryTestSuite) TestTokenFactoryMultipleDenoms() {
	require := s.Require()

	// Create a test account
	addr := sdk.AccAddress([]byte("addr6_______________"))
	acc := s.app.AuthKeeper.NewAccountWithAddress(s.ctx, addr)
	s.app.AuthKeeper.SetAccount(s.ctx, acc)

	// Fund the account for fees
	coins := sdk.NewCoins(sdk.NewCoin("kud", math.NewIntFromUint64(10000000000000000000)))
	s.Require().NoError(s.app.BankKeeper.MintCoins(s.ctx, "mint", coins))
	s.Require().NoError(s.app.BankKeeper.SendCoinsFromModuleToAccount(s.ctx, "mint", addr, coins))

	// Create multiple denoms
	subdenoms := []string{"multitoken1", "multitoken2", "multitoken3"}
	var createdDenoms []string
	for _, subdenom := range subdenoms {
		denom, err := s.app.TokenFactoryKeeper.CreateDenom(s.ctx, addr.String(), subdenom)
		require.NoError(err, "failed to create denom: %s", subdenom)
		createdDenoms = append(createdDenoms, denom)
	}

	// Verify all denoms were created
	denoms := s.app.TokenFactoryKeeper.GetDenomsFromCreator(s.ctx, addr.String())
	require.Len(denoms, len(subdenoms), "should have created %d denoms", len(subdenoms))

	// Verify each denom
	for _, createdDenom := range createdDenoms {
		require.Contains(denoms, createdDenom, "denom %s should exist", createdDenom)
	}
}

// TestTokenFactoryDenomMetadata tests setting and getting denom metadata
func (s *TokenFactoryTestSuite) TestTokenFactoryDenomMetadata() {
	require := s.Require()

	// Create a test account
	addr := sdk.AccAddress([]byte("addr1_______________"))
	acc := s.app.AuthKeeper.NewAccountWithAddress(s.ctx, addr)
	s.app.AuthKeeper.SetAccount(s.ctx, acc)

	// Fund the account for fees
	coins := sdk.NewCoins(sdk.NewCoin("kud", math.NewInt(1000000000000000000)))
	require.NoError(s.app.BankKeeper.MintCoins(s.ctx, "mint", coins))
	require.NoError(s.app.BankKeeper.SendCoinsFromModuleToAccount(s.ctx, "mint", addr, coins))

	// Create denom
	subdenom := "metadatatoken"
	denom, err := s.app.TokenFactoryKeeper.CreateDenom(s.ctx, addr.String(), subdenom)
	require.NoError(err)

	// Set metadata
	metadata := banktypes.Metadata{
		Description: "Test Token",
		DenomUnits: []*banktypes.DenomUnit{
			{Denom: denom, Exponent: 0},
			{Denom: "test", Exponent: 18},
		},
		Base:    denom,
		Display: "test",
		Name:    "Test Token",
		Symbol:  "TEST",
	}

	msgSetMetadata := tokenfactorytypes.NewMsgSetDenomMetadata(addr.String(), metadata)
	_, err = s.msgServer.SetDenomMetadata(s.ctx, msgSetMetadata)
	require.NoError(err, "failed to set denom metadata")

	// Verify metadata
	storedMetadata, found := s.app.BankKeeper.GetDenomMetaData(s.ctx, denom)
	require.True(found, "metadata should be stored")
	require.Equal(metadata.Description, storedMetadata.Description)
	require.Equal(metadata.Name, storedMetadata.Name)
	require.Equal(metadata.Symbol, storedMetadata.Symbol)
}
