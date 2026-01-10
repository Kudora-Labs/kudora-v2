package app

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/std"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	ratelimittypes "github.com/cosmos/ibc-apps/modules/rate-limiting/v10/types"
)

const msgAddRateLimitJSON = `{
  "@type": "/ratelimit.v1.MsgAddRateLimit",
  "authority": "kudo10d07y265gmmuvt4z0w9aw880jnsr700juqe799",
  "denom": "kud",
  "channel_or_client_id": "channel-0",
  "max_percent_send": "10",
  "max_percent_recv": "10",
  "duration_hours": "24"
}`

func TestRateLimitKeeperInitialized(t *testing.T) {
	app, err := getTestApp()
	if err != nil || app == nil {
		t.Skipf("Skipping RateLimit tests: %v", err)
		return
	}

	require.NotNil(t, app.RateLimitKeeper, "RateLimitKeeper should be initialized")
	require.NotNil(t, app.GetKey(ratelimittypes.StoreKey), "ratelimit store key should be registered")
}

func TestRateLimitCodecDecodesMsgAddRateLimit(t *testing.T) {
	app, err := getTestApp()
	if err != nil || app == nil {
		t.Skipf("Skipping RateLimit tests: %v", err)
		return
	}

	var any codectypes.Any
	require.NoError(t, app.AppCodec().UnmarshalJSON([]byte(msgAddRateLimitJSON), &any))

	var msg sdk.Msg
	require.NoError(t, app.AppCodec().UnpackAny(&any, &msg))

	unpacked, ok := msg.(*ratelimittypes.MsgAddRateLimit)
	require.True(t, ok)
	require.Equal(t, "kud", unpacked.Denom)
	require.Equal(t, "channel-0", unpacked.ChannelOrClientId)
	require.Equal(t, "10", unpacked.MaxPercentSend.String())
	require.Equal(t, "10", unpacked.MaxPercentRecv.String())
	require.Equal(t, uint64(24), unpacked.DurationHours)
}

func TestRegisterRateLimit_RegistersInterfacesForJSONDecoding(t *testing.T) {
	ir := codectypes.NewInterfaceRegistry()
	std.RegisterInterfaces(ir)
	cdc := codec.NewProtoCodec(ir)

	_ = RegisterRateLimit(cdc)

	var any codectypes.Any
	require.NoError(t, cdc.UnmarshalJSON([]byte(msgAddRateLimitJSON), &any))

	var msg sdk.Msg
	require.NoError(t, cdc.UnpackAny(&any, &msg))

	unpacked, ok := msg.(*ratelimittypes.MsgAddRateLimit)
	require.True(t, ok)
	require.Equal(t, "kud", unpacked.Denom)
}
