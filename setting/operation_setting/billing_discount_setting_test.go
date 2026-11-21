package operation_setting

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingDiscountRuleMatches(t *testing.T) {
	tests := []struct {
		name      string
		rule      BillingDiscountRule
		userId    int
		userAgent string
		want      bool
	}{
		{"user hit in any mode", BillingDiscountRule{Enabled: true, UserIds: []int{7}, UserAgents: []string{"claude-cli"}}, 7, "curl/8", true},
		{"ua contains is case-insensitive", BillingDiscountRule{Enabled: true, UserAgents: []string{"Claude-CLI"}}, 1, "claude-cli/2.0.1 (external)", true},
		{"ua exact rejects longer value", BillingDiscountRule{Enabled: true, UserAgents: []string{"claude-cli"}, UAMatch: BillingDiscountUAExact}, 1, "claude-cli/2.0.1", false},
		{"ua exact accepts same value ignoring case", BillingDiscountRule{Enabled: true, UserAgents: []string{"Claude-CLI/2.0.1"}, UAMatch: BillingDiscountUAExact}, 1, "claude-cli/2.0.1", true},
		{"all mode needs both", BillingDiscountRule{Enabled: true, UserIds: []int{7}, UserAgents: []string{"claude-cli"}, MatchMode: BillingDiscountMatchAll}, 7, "curl/8", false},
		{"all mode with both hits", BillingDiscountRule{Enabled: true, UserIds: []int{7}, UserAgents: []string{"claude-cli"}, MatchMode: BillingDiscountMatchAll}, 7, "claude-cli/2", true},
		{"all mode ignores empty condition", BillingDiscountRule{Enabled: true, UserIds: []int{7}, MatchMode: BillingDiscountMatchAll}, 7, "", true},
		{"rule without conditions never matches", BillingDiscountRule{Enabled: true, UserAgents: []string{"  "}}, 7, "anything", false},
		{"disabled rule never matches", BillingDiscountRule{UserIds: []int{7}}, 7, "", false},
		{"empty request ua does not match", BillingDiscountRule{Enabled: true, UserAgents: []string{"x"}}, 1, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.rule.Matches(tt.userId, tt.userAgent))
		})
	}
}

func TestResolveBillingDiscountPicksLowestValidRatio(t *testing.T) {
	saved := billingDiscountSetting.Rules
	t.Cleanup(func() { billingDiscountSetting.Rules = saved })
	billingDiscountSetting.Rules = []BillingDiscountRule{
		{Name: "vip", Enabled: true, Ratio: 0.8, UserIds: []int{7}},
		{Name: "cli", Enabled: true, Ratio: 0.5, UserAgents: []string{"claude-cli"}},
		{Name: "broken", Enabled: true, Ratio: 0, UserIds: []int{7}},
		{Name: "nan", Enabled: true, Ratio: math.NaN(), UserIds: []int{7}},
	}

	ratio, name, ok := ResolveBillingDiscount(7, "claude-cli/2.0.1")
	require.True(t, ok)
	assert.Equal(t, 0.5, ratio)
	assert.Equal(t, "cli", name)

	ratio, name, ok = ResolveBillingDiscount(7, "curl/8")
	require.True(t, ok)
	assert.Equal(t, 0.8, ratio)
	assert.Equal(t, "vip", name)

	ratio, _, ok = ResolveBillingDiscount(8, "curl/8")
	assert.False(t, ok)
	assert.Equal(t, 1.0, ratio)
}

func TestValidateBillingDiscountRulesRejectsUnsafeValues(t *testing.T) {
	assert.NoError(t, ValidateBillingDiscountRules([]BillingDiscountRule{{Ratio: 0.5}, {Ratio: 1.2, MatchMode: "all", UAMatch: "exact"}}))
	for _, rule := range []BillingDiscountRule{
		{Ratio: 0},
		{Ratio: -1},
		{Ratio: math.NaN()},
		{Ratio: math.Inf(1)},
		{Ratio: MaxBillingDiscountRatio + 1},
		{Ratio: 0.5, MatchMode: "xor"},
		{Ratio: 0.5, UAMatch: "regex"},
	} {
		assert.Error(t, ValidateBillingDiscountRules([]BillingDiscountRule{rule}))
	}
}
