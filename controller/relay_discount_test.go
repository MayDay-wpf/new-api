package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A channel pinned by the distributor is the attempt's channel, so a
// channel-scoped billing discount must be resolved when the attempt starts and
// not only after a retry selects a channel again.
func TestGetChannelAppliesChannelScopedDiscount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := config.GlobalConfig.ExportAllConfigs()
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(previous)) })
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"group_ratio_setting.group_ratio": `{"default":1}`,
		"billing_discount_setting.rules":  `[{"name":"channel-3","enabled":true,"ratio":0.5,"user_ids":[],"channel_ids":[3],"user_agents":["claude"],"match_mode":"all","ua_match":"contains"}]`,
	}))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("User-Agent", "claude-cli/2.0.1")
	ctx.Request = req
	ctx.Set("channel_id", 3)
	ctx.Set("channel_type", 1)
	ctx.Set("channel_name", "pinned")

	info := &relaycommon.RelayInfo{UserId: 1, UserGroup: "default", UsingGroup: "default"}
	channel, apiErr := getChannel(ctx, info, &service.RetryParam{Ctx: ctx, Retry: common.GetPointer(0)})
	require.Nil(t, apiErr)
	require.NotNil(t, channel)
	assert.Equal(t, 3, channel.Id)
	assert.Equal(t, 0.5, info.PriceData.GroupRatioInfo.GroupRatio)
	assert.Equal(t, 0.5, info.PriceData.GroupRatioInfo.DiscountRatio)
	assert.Equal(t, "channel-3", info.PriceData.GroupRatioInfo.DiscountName)
}
