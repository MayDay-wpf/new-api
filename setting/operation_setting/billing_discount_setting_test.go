package operation_setting

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingDiscountRuleMatches(t *testing.T) {
	tests := []struct {
		name      string
		rule      BillingDiscountRule
		userId    int
		channelId int
		userAgent string
		want      bool
	}{
		{"user hit in any mode", BillingDiscountRule{Enabled: true, UserIds: []int{7}, UserAgents: []string{"claude-cli"}}, 7, 0, "curl/8", true},
		{"ua contains is case-insensitive", BillingDiscountRule{Enabled: true, UserAgents: []string{"Claude-CLI"}}, 1, 0, "claude-cli/2.0.1 (external)", true},
		{"ua exact rejects longer value", BillingDiscountRule{Enabled: true, UserAgents: []string{"claude-cli"}, UAMatch: BillingDiscountUAExact}, 1, 0, "claude-cli/2.0.1", false},
		{"ua exact accepts same value ignoring case", BillingDiscountRule{Enabled: true, UserAgents: []string{"Claude-CLI/2.0.1"}, UAMatch: BillingDiscountUAExact}, 1, 0, "claude-cli/2.0.1", true},
		{"all mode needs both", BillingDiscountRule{Enabled: true, UserIds: []int{7}, UserAgents: []string{"claude-cli"}, MatchMode: BillingDiscountMatchAll}, 7, 0, "curl/8", false},
		{"all mode with both hits", BillingDiscountRule{Enabled: true, UserIds: []int{7}, UserAgents: []string{"claude-cli"}, MatchMode: BillingDiscountMatchAll}, 7, 0, "claude-cli/2", true},
		{"all mode ignores empty condition", BillingDiscountRule{Enabled: true, UserIds: []int{7}, MatchMode: BillingDiscountMatchAll}, 7, 0, "", true},
		{"rule without conditions never matches", BillingDiscountRule{Enabled: true, UserAgents: []string{"  "}}, 7, 0, "anything", false},
		{"disabled rule never matches", BillingDiscountRule{UserIds: []int{7}}, 7, 0, "", false},
		{"empty request ua does not match", BillingDiscountRule{Enabled: true, UserAgents: []string{"x"}}, 1, 0, "", false},
		{"channel hit in any mode", BillingDiscountRule{Enabled: true, ChannelIds: []int{3, 5}}, 1, 5, "", true},
		{"channel miss in any mode", BillingDiscountRule{Enabled: true, ChannelIds: []int{3, 5}}, 1, 6, "", false},
		{"unknown channel never hits a channel condition", BillingDiscountRule{Enabled: true, ChannelIds: []int{3}}, 1, 0, "", false},
		{"all mode needs user and channel", BillingDiscountRule{Enabled: true, UserIds: []int{7}, ChannelIds: []int{5}, MatchMode: BillingDiscountMatchAll}, 7, 6, "", false},
		{"all mode with user and channel hits", BillingDiscountRule{Enabled: true, UserIds: []int{7}, ChannelIds: []int{5}, MatchMode: BillingDiscountMatchAll}, 7, 5, "", true},
		{"all mode also counts user agent", BillingDiscountRule{Enabled: true, UserIds: []int{7}, ChannelIds: []int{5}, UserAgents: []string{"cli"}, MatchMode: BillingDiscountMatchAll}, 7, 5, "curl/8", false},
		{"channel list restricts any mode", BillingDiscountRule{Enabled: true, UserAgents: []string{"claude"}, ChannelIds: []int{3}}, 1, 5, "claude-cli/2.0.1", false},
		{"channel list restricts any mode with user hit", BillingDiscountRule{Enabled: true, UserIds: []int{7}, ChannelIds: []int{3}}, 7, 5, "", false},
		{"channel list allows any mode on a listed channel", BillingDiscountRule{Enabled: true, UserAgents: []string{"claude"}, ChannelIds: []int{3}}, 1, 3, "claude-cli/2.0.1", true},
		{"unknown channel leaves other conditions in charge", BillingDiscountRule{Enabled: true, UserAgents: []string{"claude"}, ChannelIds: []int{3}}, 1, 0, "claude-cli/2.0.1", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.rule.Matches(tt.userId, tt.channelId, tt.userAgent))
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

	ratio, name, ok := ResolveBillingDiscount(7, 0, "claude-cli/2.0.1")
	require.True(t, ok)
	assert.Equal(t, 0.5, ratio)
	assert.Equal(t, "cli", name)

	ratio, name, ok = ResolveBillingDiscount(7, 0, "curl/8")
	require.True(t, ok)
	assert.Equal(t, 0.8, ratio)
	assert.Equal(t, "vip", name)

	ratio, _, ok = ResolveBillingDiscount(8, 0, "curl/8")
	assert.False(t, ok)
	assert.Equal(t, 1.0, ratio)
}

func TestResolveBillingDiscountUsesChannelOfTheAttempt(t *testing.T) {
	saved := billingDiscountSetting.Rules
	t.Cleanup(func() { billingDiscountSetting.Rules = saved })
	billingDiscountSetting.Rules = []BillingDiscountRule{
		{Name: "user-only", Enabled: true, Ratio: 0.9, UserIds: []int{7}},
		{Name: "channel-3", Enabled: true, Ratio: 0.6, ChannelIds: []int{3}},
		{Name: "channel-5", Enabled: true, Ratio: 0.4, ChannelIds: []int{5}},
	}

	// Before channel selection, channel-scoped rules must not match.
	ratio, name, ok := ResolveBillingDiscount(7, 0, "")
	require.True(t, ok)
	assert.Equal(t, 0.9, ratio)
	assert.Equal(t, "user-only", name)

	ratio, name, ok = ResolveBillingDiscount(7, 3, "")
	require.True(t, ok)
	assert.Equal(t, 0.6, ratio)
	assert.Equal(t, "channel-3", name)

	// The lowest ratio among the matching rules wins.
	ratio, name, ok = ResolveBillingDiscount(7, 5, "")
	require.True(t, ok)
	assert.Equal(t, 0.4, ratio)
	assert.Equal(t, "channel-5", name)

	// A user outside every rule still gets the channel discount.
	ratio, name, ok = ResolveBillingDiscount(8, 5, "")
	require.True(t, ok)
	assert.Equal(t, 0.4, ratio)
	assert.Equal(t, "channel-5", name)
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

func TestBillingDiscountScheduleIsEmpty(t *testing.T) {
	assert.True(t, (*BillingDiscountSchedule)(nil).IsEmpty())
	assert.True(t, (&BillingDiscountSchedule{}).IsEmpty())
	assert.True(t, (&BillingDiscountSchedule{Timezone: " ", StartTime: " "}).IsEmpty())
	assert.False(t, (&BillingDiscountSchedule{StartTime: "09:00", EndTime: "18:00"}).IsEmpty())
	assert.False(t, (&BillingDiscountSchedule{Weekdays: []int{1}}).IsEmpty())
}

func TestBillingDiscountScheduleAlwaysActiveWhenUnset(t *testing.T) {
	now := time.Date(2026, 11, 23, 3, 15, 0, 0, time.UTC)
	assert.True(t, (*BillingDiscountSchedule)(nil).IsActiveAt(now))
	assert.True(t, (&BillingDiscountSchedule{}).IsActiveAt(now))
}

func TestBillingDiscountScheduleDailyWindow(t *testing.T) {
	schedule := &BillingDiscountSchedule{Timezone: "UTC", StartTime: "09:00", EndTime: "18:00"}
	cases := []struct {
		name string
		now  time.Time
		want bool
	}{
		{"before start", time.Date(2026, 11, 23, 8, 59, 0, 0, time.UTC), false},
		{"at start", time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC), true},
		{"last minute inside", time.Date(2026, 11, 23, 17, 59, 0, 0, time.UTC), true},
		{"at end is exclusive", time.Date(2026, 11, 23, 18, 0, 0, 0, time.UTC), false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, schedule.IsActiveAt(tt.now))
		})
	}
}

func TestBillingDiscountScheduleWeeklyOvernightWindow(t *testing.T) {
	// 2026-11-27 is a Friday, so a Friday 22:00-06:00 window runs into Saturday.
	schedule := &BillingDiscountSchedule{
		Timezone:  "UTC",
		Weekdays:  []int{5},
		StartTime: "22:00",
		EndTime:   "06:00",
	}
	assert.True(t, schedule.IsActiveAt(time.Date(2026, 11, 27, 23, 0, 0, 0, time.UTC)))
	assert.True(t, schedule.IsActiveAt(time.Date(2026, 11, 28, 3, 0, 0, 0, time.UTC)))
	assert.False(t, schedule.IsActiveAt(time.Date(2026, 11, 28, 6, 0, 0, 0, time.UTC)))
	assert.False(t, schedule.IsActiveAt(time.Date(2026, 11, 28, 23, 0, 0, 0, time.UTC)))
	assert.False(t, schedule.IsActiveAt(time.Date(2026, 11, 27, 21, 59, 0, 0, time.UTC)))
}

func TestBillingDiscountScheduleWeeklyWithoutTimeRange(t *testing.T) {
	schedule := &BillingDiscountSchedule{Timezone: "UTC", Weekdays: []int{1}}
	assert.True(t, schedule.IsActiveAt(time.Date(2026, 11, 23, 4, 0, 0, 0, time.UTC)))
	assert.False(t, schedule.IsActiveAt(time.Date(2026, 11, 24, 4, 0, 0, 0, time.UTC)))
}

func TestBillingDiscountScheduleMonthlyWindow(t *testing.T) {
	schedule := &BillingDiscountSchedule{
		Timezone:  "UTC",
		MonthDays: []int{1, 15},
		StartTime: "10:00",
		EndTime:   "12:00",
	}
	assert.True(t, schedule.IsActiveAt(time.Date(2026, 11, 1, 11, 0, 0, 0, time.UTC)))
	assert.True(t, schedule.IsActiveAt(time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)))
	assert.False(t, schedule.IsActiveAt(time.Date(2026, 11, 2, 11, 0, 0, 0, time.UTC)))
	assert.False(t, schedule.IsActiveAt(time.Date(2026, 11, 15, 12, 30, 0, 0, time.UTC)))
}

func TestBillingDiscountScheduleTimezone(t *testing.T) {
	schedule := &BillingDiscountSchedule{
		Timezone:  "Asia/Shanghai",
		StartTime: "09:00",
		EndTime:   "18:00",
	}
	assert.True(t, schedule.IsActiveAt(time.Date(2026, 11, 23, 1, 0, 0, 0, time.UTC)))
	assert.True(t, schedule.IsActiveAt(time.Date(2026, 11, 23, 9, 30, 0, 0, time.UTC)))
	assert.False(t, schedule.IsActiveAt(time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)))
}

func TestBillingDiscountScheduleInvalidStoredWindowIsInactive(t *testing.T) {
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	assert.False(t, (&BillingDiscountSchedule{StartTime: "09:00"}).IsActiveAt(now))
	assert.False(t, (&BillingDiscountSchedule{StartTime: "9am", EndTime: "18:00"}).IsActiveAt(now))
	assert.False(t, (&BillingDiscountSchedule{Timezone: "Mars/Olympus", StartTime: "09:00", EndTime: "18:00"}).IsActiveAt(now))
}

func TestBillingDiscountRuleRespectsSchedule(t *testing.T) {
	rule := BillingDiscountRule{
		Enabled:  true,
		Ratio:    0.5,
		UserIds:  []int{7},
		Schedule: &BillingDiscountSchedule{Timezone: "UTC", StartTime: "09:00", EndTime: "18:00"},
	}
	assert.True(t, rule.MatchesAt(7, 0, "", time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)))
	assert.False(t, rule.MatchesAt(7, 0, "", time.Date(2026, 11, 23, 20, 0, 0, 0, time.UTC)))

	// A rule without a schedule stays unconditional.
	always := BillingDiscountRule{Enabled: true, Ratio: 0.5, UserIds: []int{7}}
	assert.True(t, always.MatchesAt(7, 0, "", time.Date(2026, 11, 23, 20, 0, 0, 0, time.UTC)))
}

func TestValidateBillingDiscountSchedule(t *testing.T) {
	valid := []*BillingDiscountSchedule{
		nil,
		{},
		{Timezone: "Asia/Shanghai", Weekdays: []int{0, 6}, StartTime: "00:00", EndTime: "23:59"},
		{MonthDays: []int{1, 31}, StartTime: "22:00", EndTime: "06:00"},
	}
	for _, schedule := range valid {
		assert.NoError(t, schedule.Validate())
	}

	invalid := []*BillingDiscountSchedule{
		{Timezone: "Mars/Olympus"},
		{Weekdays: []int{7}},
		{Weekdays: []int{-1}},
		{MonthDays: []int{0}},
		{MonthDays: []int{32}},
		{StartTime: "09:00"},
		{EndTime: "18:00"},
		{StartTime: "09:00", EndTime: "09:00"},
		{StartTime: "24:00", EndTime: "18:00"},
		{StartTime: "9:0", EndTime: "18:00"},
		{StartTime: "9am", EndTime: "18:00"},
	}
	for _, schedule := range invalid {
		assert.Error(t, schedule.Validate(), "schedule %+v should be rejected", schedule)
	}
}
