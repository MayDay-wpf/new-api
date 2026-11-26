package operation_setting

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/setting/config"
)

const (
	BillingDiscountMatchAny = "any" // 用户命中 或 UA 命中
	BillingDiscountMatchAll = "all" // 用户命中 且 UA 命中

	BillingDiscountUAContains = "contains" // 包含匹配（不区分大小写）
	BillingDiscountUAExact    = "exact"    // 完全相等（不区分大小写）

	// MaxBillingDiscountRatio bounds the multiplier so a typo cannot turn a
	// discount into an extreme surcharge.
	MaxBillingDiscountRatio = 10.0
)

// BillingDiscountSchedule 描述规则的重复生效时间窗口。
// 所有字段均可不填：schedule 为空（或规则没有 schedule）表示无条件立即生效。
// weekdays 与 month_days 同时填写时取交集；calendar 条件按窗口开始那天判断，
// 因此跨零点的窗口（如 22:00-06:00）凌晨部分仍属于前一天。
type BillingDiscountSchedule struct {
	Timezone  string `json:"timezone,omitempty"`   // IANA 时区名，空 = 服务器本地时区
	Weekdays  []int  `json:"weekdays,omitempty"`   // 0=周日 … 6=周六，空 = 不限
	MonthDays []int  `json:"month_days,omitempty"` // 1…31，空 = 不限
	StartTime string `json:"start_time,omitempty"` // "HH:MM"，含起始时刻
	EndTime   string `json:"end_time,omitempty"`   // "HH:MM"，不含结束时刻，可跨零点
}

// BillingDiscountRule 是一条折扣规则：命中后最终计费乘以 Ratio。
// 条件只统计非空列表：any 模式任一条件命中即生效，all 模式要求全部命中；
// 没有任何条件的规则永不命中。渠道条件按请求实际使用的渠道判断，
// 且渠道列表始终是硬性限制：请求落在列表之外的渠道时规则一律不生效
// （列表只在渠道尚未确定时留给其它条件判断）。
type BillingDiscountRule struct {
	Name       string                   `json:"name"`
	Enabled    bool                     `json:"enabled"`
	Ratio      float64                  `json:"ratio"`
	UserIds    []int                    `json:"user_ids"`
	ChannelIds []int                    `json:"channel_ids"`
	UserAgents []string                 `json:"user_agents"`
	MatchMode  string                   `json:"match_mode"`
	UAMatch    string                   `json:"ua_match"`
	Schedule   *BillingDiscountSchedule `json:"schedule,omitempty"`
}

type BillingDiscountSetting struct {
	Rules []BillingDiscountRule `json:"rules"`
}

var billingDiscountSetting = BillingDiscountSetting{
	Rules: []BillingDiscountRule{},
}

func init() {
	config.GlobalConfig.Register("billing_discount_setting", &billingDiscountSetting)
}

// ValidateBillingDiscountRules checks every rule before it is stored, so the
// billing path never sees a non-positive or unbounded multiplier.
func ValidateBillingDiscountRules(rules []BillingDiscountRule) error {
	for i, rule := range rules {
		if math.IsNaN(rule.Ratio) || rule.Ratio <= 0 || rule.Ratio > MaxBillingDiscountRatio {
			return fmt.Errorf("rule %d: ratio must be greater than 0 and at most %g", i+1, MaxBillingDiscountRatio)
		}
		switch rule.MatchMode {
		case "", BillingDiscountMatchAny, BillingDiscountMatchAll:
		default:
			return fmt.Errorf("rule %d: invalid match mode %q", i+1, rule.MatchMode)
		}
		switch rule.UAMatch {
		case "", BillingDiscountUAContains, BillingDiscountUAExact:
		default:
			return fmt.Errorf("rule %d: invalid user agent match %q", i+1, rule.UAMatch)
		}
		if err := rule.Schedule.Validate(); err != nil {
			return fmt.Errorf("rule %d: %w", i+1, err)
		}
	}
	return nil
}

// IsEmpty reports whether the schedule imposes any restriction at all.
func (schedule *BillingDiscountSchedule) IsEmpty() bool {
	if schedule == nil {
		return true
	}
	return strings.TrimSpace(schedule.Timezone) == "" &&
		len(schedule.Weekdays) == 0 &&
		len(schedule.MonthDays) == 0 &&
		strings.TrimSpace(schedule.StartTime) == "" &&
		strings.TrimSpace(schedule.EndTime) == ""
}

// Validate rejects a schedule that could never be interpreted consistently, so
// the billing path only ever sees well-formed windows.
func (schedule *BillingDiscountSchedule) Validate() error {
	if schedule == nil {
		return nil
	}
	if timezone := strings.TrimSpace(schedule.Timezone); timezone != "" {
		if _, err := time.LoadLocation(timezone); err != nil {
			return fmt.Errorf("invalid timezone %q", timezone)
		}
	}
	for _, weekday := range schedule.Weekdays {
		if weekday < 0 || weekday > 6 {
			return fmt.Errorf("weekday must be between 0 (Sunday) and 6 (Saturday), got %d", weekday)
		}
	}
	for _, day := range schedule.MonthDays {
		if day < 1 || day > 31 {
			return fmt.Errorf("month day must be between 1 and 31, got %d", day)
		}
	}
	start, hasStart, startOK := parseBillingDiscountClock(schedule.StartTime)
	end, hasEnd, endOK := parseBillingDiscountClock(schedule.EndTime)
	if !startOK {
		return fmt.Errorf("invalid start time %q, expected HH:MM", schedule.StartTime)
	}
	if !endOK {
		return fmt.Errorf("invalid end time %q, expected HH:MM", schedule.EndTime)
	}
	if hasStart != hasEnd {
		return fmt.Errorf("start time and end time must be set together")
	}
	if hasStart && start == end {
		return fmt.Errorf("start time and end time must differ")
	}
	return nil
}

// IsActiveAt reports whether now falls inside the schedule's activation window.
// A nil or empty schedule is always active. A stored window that cannot be
// interpreted is treated as inactive so a broken config never grants a discount
// outside its intended time.
func (schedule *BillingDiscountSchedule) IsActiveAt(now time.Time) bool {
	if schedule.IsEmpty() {
		return true
	}
	location := time.Local
	if timezone := strings.TrimSpace(schedule.Timezone); timezone != "" {
		loaded, err := time.LoadLocation(timezone)
		if err != nil {
			return false
		}
		location = loaded
	}
	local := now.In(location)

	start, hasStart, startOK := parseBillingDiscountClock(schedule.StartTime)
	end, hasEnd, endOK := parseBillingDiscountClock(schedule.EndTime)
	if !startOK || !endOK || hasStart != hasEnd || (hasStart && start == end) {
		return false
	}

	// 跨零点的窗口凌晨部分属于前一天，日历条件要按窗口开始那天判断。
	calendarDay := local
	if hasStart && end < start && local.Hour()*60+local.Minute() < end {
		calendarDay = local.AddDate(0, 0, -1)
	}
	if len(schedule.Weekdays) > 0 && !slices.Contains(schedule.Weekdays, int(calendarDay.Weekday())) {
		return false
	}
	if len(schedule.MonthDays) > 0 && !slices.Contains(schedule.MonthDays, calendarDay.Day()) {
		return false
	}
	if !hasStart {
		return true
	}
	minutes := local.Hour()*60 + local.Minute()
	if end > start {
		return minutes >= start && minutes < end
	}
	return minutes >= start || minutes < end
}

// parseBillingDiscountClock parses "HH:MM" into minutes since midnight. The
// second return value reports whether a time was configured at all; the third
// reports whether the value is valid.
func parseBillingDiscountClock(value string) (int, bool, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false, true
	}
	parts := strings.Split(value, ":")
	if len(parts) != 2 || len(parts[0]) != 2 || len(parts[1]) != 2 {
		return 0, false, false
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, false, false
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, false, false
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, false, false
	}
	return hour*60 + minute, true, true
}

func (rule BillingDiscountRule) matchesUserAgent(userAgent string) bool {
	userAgent = strings.ToLower(strings.TrimSpace(userAgent))
	if userAgent == "" {
		return false
	}
	for _, pattern := range rule.UserAgents {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		if rule.UAMatch == BillingDiscountUAExact {
			if userAgent == pattern {
				return true
			}
		} else if strings.Contains(userAgent, pattern) {
			return true
		}
	}
	return false
}

// Matches reports whether the rule applies to the given user, channel and
// User-Agent right now.
func (rule BillingDiscountRule) Matches(userId int, channelId int, userAgent string) bool {
	return rule.MatchesAt(userId, channelId, userAgent, time.Now())
}

// MatchesAt is Matches with an explicit clock so callers can evaluate every
// rule against the same instant (and tests stay deterministic).
func (rule BillingDiscountRule) MatchesAt(userId int, channelId int, userAgent string, now time.Time) bool {
	if !rule.Enabled {
		return false
	}
	if !rule.Schedule.IsActiveAt(now) {
		return false
	}
	// A channel list always restricts the rule, whatever the condition logic:
	// a request served by a channel outside the list never gets the discount.
	// While the serving channel is still unknown (channelId 0) the list cannot
	// be judged, so the remaining conditions decide.
	if len(rule.ChannelIds) > 0 && channelId > 0 && !slices.Contains(rule.ChannelIds, channelId) {
		return false
	}
	conditions := 0
	hits := 0
	if len(rule.UserIds) > 0 {
		conditions++
		if slices.Contains(rule.UserIds, userId) {
			hits++
		}
	}
	if len(rule.ChannelIds) > 0 {
		conditions++
		if channelId > 0 && slices.Contains(rule.ChannelIds, channelId) {
			hits++
		}
	}
	if slices.ContainsFunc(rule.UserAgents, func(s string) bool { return strings.TrimSpace(s) != "" }) {
		conditions++
		if rule.matchesUserAgent(userAgent) {
			hits++
		}
	}
	if conditions == 0 {
		return false
	}
	if rule.MatchMode == BillingDiscountMatchAll {
		return hits == conditions
	}
	return hits > 0
}

// ResolveBillingDiscount returns the lowest ratio among matching rules, or
// (1, "", false) when nothing matches. Invalid stored ratios are ignored.
// channelId is 0 while the request has no channel yet; the channel conditions of
// a rule are then left undecided and its other conditions decide.
func ResolveBillingDiscount(userId int, channelId int, userAgent string) (float64, string, bool) {
	return resolveBillingDiscountAt(userId, channelId, userAgent, time.Now())
}

func resolveBillingDiscountAt(userId int, channelId int, userAgent string, now time.Time) (float64, string, bool) {
	ratio := 1.0
	name := ""
	matched := false
	for _, rule := range billingDiscountSetting.Rules {
		if math.IsNaN(rule.Ratio) || rule.Ratio <= 0 || rule.Ratio > MaxBillingDiscountRatio {
			continue
		}
		if !rule.MatchesAt(userId, channelId, userAgent, now) {
			continue
		}
		if !matched || rule.Ratio < ratio {
			ratio = rule.Ratio
			name = rule.Name
			matched = true
		}
	}
	return ratio, name, matched
}
