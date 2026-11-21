package operation_setting

import (
	"fmt"
	"math"
	"slices"
	"strings"

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

// BillingDiscountRule 是一条折扣规则：命中后最终计费乘以 Ratio，不区分渠道。
// 条件只统计非空列表：any 模式任一条件命中即生效，all 模式要求全部命中；
// 没有任何条件的规则永不命中。
type BillingDiscountRule struct {
	Name       string   `json:"name"`
	Enabled    bool     `json:"enabled"`
	Ratio      float64  `json:"ratio"`
	UserIds    []int    `json:"user_ids"`
	UserAgents []string `json:"user_agents"`
	MatchMode  string   `json:"match_mode"`
	UAMatch    string   `json:"ua_match"`
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
	}
	return nil
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

// Matches reports whether the rule applies to the given user and User-Agent.
func (rule BillingDiscountRule) Matches(userId int, userAgent string) bool {
	if !rule.Enabled {
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
func ResolveBillingDiscount(userId int, userAgent string) (float64, string, bool) {
	ratio := 1.0
	name := ""
	matched := false
	for _, rule := range billingDiscountSetting.Rules {
		if math.IsNaN(rule.Ratio) || rule.Ratio <= 0 || rule.Ratio > MaxBillingDiscountRatio {
			continue
		}
		if !rule.Matches(userId, userAgent) {
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
