package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/samber/hot"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Subscription duration units
const (
	SubscriptionDurationYear   = "year"
	SubscriptionDurationMonth  = "month"
	SubscriptionDurationDay    = "day"
	SubscriptionDurationHour   = "hour"
	SubscriptionDurationCustom = "custom"
)

// Subscription quota reset period
const (
	SubscriptionResetNever   = "never"
	SubscriptionResetDaily   = "daily"
	SubscriptionResetWeekly  = "weekly"
	SubscriptionResetMonthly = "monthly"
	SubscriptionResetCustom  = "custom"
)

var (
	ErrSubscriptionOrderNotFound      = errors.New("subscription order not found")
	ErrSubscriptionOrderStatusInvalid = errors.New("subscription order status invalid")
)

const (
	subscriptionPlanCacheNamespace     = "new-api:subscription_plan:v1"
	subscriptionPlanInfoCacheNamespace = "new-api:subscription_plan_info:v1"
)

var (
	subscriptionPlanCacheOnce     sync.Once
	subscriptionPlanInfoCacheOnce sync.Once

	subscriptionPlanCache     *cachex.HybridCache[SubscriptionPlan]
	subscriptionPlanInfoCache *cachex.HybridCache[SubscriptionPlanInfo]
)

func subscriptionPlanCacheTTL() time.Duration {
	ttlSeconds := common.GetEnvOrDefault("SUBSCRIPTION_PLAN_CACHE_TTL", 300)
	if ttlSeconds <= 0 {
		ttlSeconds = 300
	}
	return time.Duration(ttlSeconds) * time.Second
}

func subscriptionPlanInfoCacheTTL() time.Duration {
	ttlSeconds := common.GetEnvOrDefault("SUBSCRIPTION_PLAN_INFO_CACHE_TTL", 120)
	if ttlSeconds <= 0 {
		ttlSeconds = 120
	}
	return time.Duration(ttlSeconds) * time.Second
}

func subscriptionPlanCacheCapacity() int {
	capacity := common.GetEnvOrDefault("SUBSCRIPTION_PLAN_CACHE_CAP", 5000)
	if capacity <= 0 {
		capacity = 5000
	}
	return capacity
}

func subscriptionPlanInfoCacheCapacity() int {
	capacity := common.GetEnvOrDefault("SUBSCRIPTION_PLAN_INFO_CACHE_CAP", 10000)
	if capacity <= 0 {
		capacity = 10000
	}
	return capacity
}

func getSubscriptionPlanCache() *cachex.HybridCache[SubscriptionPlan] {
	subscriptionPlanCacheOnce.Do(func() {
		ttl := subscriptionPlanCacheTTL()
		subscriptionPlanCache = cachex.NewHybridCache[SubscriptionPlan](cachex.HybridCacheConfig[SubscriptionPlan]{
			Namespace: cachex.Namespace(subscriptionPlanCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.JSONCodec[SubscriptionPlan]{},
			Memory: func() *hot.HotCache[string, SubscriptionPlan] {
				return hot.NewHotCache[string, SubscriptionPlan](hot.LRU, subscriptionPlanCacheCapacity()).
					WithTTL(ttl).
					WithJanitor().
					Build()
			},
		})
	})
	return subscriptionPlanCache
}

func getSubscriptionPlanInfoCache() *cachex.HybridCache[SubscriptionPlanInfo] {
	subscriptionPlanInfoCacheOnce.Do(func() {
		ttl := subscriptionPlanInfoCacheTTL()
		subscriptionPlanInfoCache = cachex.NewHybridCache[SubscriptionPlanInfo](cachex.HybridCacheConfig[SubscriptionPlanInfo]{
			Namespace: cachex.Namespace(subscriptionPlanInfoCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.JSONCodec[SubscriptionPlanInfo]{},
			Memory: func() *hot.HotCache[string, SubscriptionPlanInfo] {
				return hot.NewHotCache[string, SubscriptionPlanInfo](hot.LRU, subscriptionPlanInfoCacheCapacity()).
					WithTTL(ttl).
					WithJanitor().
					Build()
			},
		})
	})
	return subscriptionPlanInfoCache
}

func subscriptionPlanCacheKey(id int) string {
	if id <= 0 {
		return ""
	}
	return strconv.Itoa(id)
}

// InvalidateSubscriptionPlanCache drops a plan written on this node from the
// cache and tells the other nodes to drop their copies.
func InvalidateSubscriptionPlanCache(planId int) {
	if planId <= 0 {
		return
	}
	cache := getSubscriptionPlanCache()
	_, _ = cache.DeleteMany([]string{subscriptionPlanCacheKey(planId)})
	infoCache := getSubscriptionPlanInfoCache()
	_ = infoCache.Purge()
	BumpSyncTopic(SyncTopicSubscriptionPlans)
}

// PurgeSubscriptionPlanCache drops every cached plan, so the next reads load
// them from the database.
func PurgeSubscriptionPlanCache() error {
	if err := getSubscriptionPlanCache().Purge(); err != nil {
		return err
	}
	return getSubscriptionPlanInfoCache().Purge()
}

// SubscriptionPlanQuotaLimit is one independent quota window of a plan.
// A plan may carry several limits at the same time (for example a 5-hour
// window, a weekly window and a monthly window); every window is enforced on
// top of the plan's overall TotalAmount.
type SubscriptionPlanQuotaLimit struct {
	// Period is daily / weekly / monthly / custom.
	Period string `json:"period"`
	// CustomSeconds is the window length when Period is custom (18000 = 5h).
	CustomSeconds int64 `json:"custom_seconds"`
	// Amount is the max quota units usable inside one window (0 = no limit).
	Amount int64 `json:"amount"`
}

type SubscriptionPlanQuotaLimits []SubscriptionPlanQuotaLimit

func (l SubscriptionPlanQuotaLimits) Value() (driver.Value, error) {
	if len(l) == 0 {
		return "[]", nil
	}
	data, err := json.Marshal(l)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func (l *SubscriptionPlanQuotaLimits) Scan(value any) error {
	*l = nil
	raw, err := quotaLimitJSONBytes(value)
	if err != nil || len(raw) == 0 {
		return err
	}
	var parsed SubscriptionPlanQuotaLimits
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return err
	}
	*l = parsed
	return nil
}

// SubscriptionQuotaLimitUsage tracks how much quota one subscription used
// inside the current window of a plan limit.
type SubscriptionQuotaLimitUsage struct {
	Key         string `json:"key"`
	Used        int64  `json:"used"`
	WindowStart int64  `json:"window_start"`
	WindowEnd   int64  `json:"window_end"`
}

type SubscriptionQuotaLimitUsages []SubscriptionQuotaLimitUsage

func (u SubscriptionQuotaLimitUsages) Value() (driver.Value, error) {
	if len(u) == 0 {
		return "[]", nil
	}
	data, err := json.Marshal(u)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func (u *SubscriptionQuotaLimitUsages) Scan(value any) error {
	*u = nil
	raw, err := quotaLimitJSONBytes(value)
	if err != nil || len(raw) == 0 {
		return err
	}
	var parsed SubscriptionQuotaLimitUsages
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return err
	}
	*u = parsed
	return nil
}

func quotaLimitJSONBytes(value any) ([]byte, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	default:
		return nil, fmt.Errorf("unsupported quota limit JSON value type %T", value)
	}
}

// NormalizeQuotaLimitPeriod maps a stored period to its canonical value.
func NormalizeQuotaLimitPeriod(period string) string {
	switch strings.ToLower(strings.TrimSpace(period)) {
	case SubscriptionResetDaily, SubscriptionResetWeekly, SubscriptionResetMonthly:
		return strings.ToLower(strings.TrimSpace(period))
	case SubscriptionResetCustom:
		return SubscriptionResetCustom
	default:
		return ""
	}
}

// Key identifies a limit independently from its amount, so editing the amount
// keeps the usage counter of the same window.
func (l SubscriptionPlanQuotaLimit) Key() string {
	period := NormalizeQuotaLimitPeriod(l.Period)
	if period == SubscriptionResetCustom {
		return fmt.Sprintf("%s:%d", SubscriptionResetCustom, l.CustomSeconds)
	}
	return period
}

// NormalizeQuotaLimits validates and canonicalizes user supplied limits.
func NormalizeQuotaLimits(limits SubscriptionPlanQuotaLimits) (SubscriptionPlanQuotaLimits, error) {
	result := make(SubscriptionPlanQuotaLimits, 0, len(limits))
	seen := make(map[string]struct{}, len(limits))
	for _, limit := range limits {
		period := NormalizeQuotaLimitPeriod(limit.Period)
		if period == "" {
			return nil, errors.New("invalid quota limit period")
		}
		limit.Period = period
		if period == SubscriptionResetCustom {
			if limit.CustomSeconds <= 0 {
				return nil, errors.New("custom quota limit window must be greater than 0 seconds")
			}
		} else {
			limit.CustomSeconds = 0
		}
		if limit.Amount < 0 {
			return nil, errors.New("quota limit amount cannot be negative")
		}
		if limit.Amount == 0 {
			// An empty limit means "no limit for this window".
			continue
		}
		key := limit.Key()
		if _, ok := seen[key]; ok {
			return nil, errors.New("duplicate quota limit window")
		}
		seen[key] = struct{}{}
		result = append(result, limit)
	}
	return result, nil
}

// normalizeQuotaLimitsLenient drops invalid entries instead of failing, so a
// stale cached plan can still be served.
func normalizeQuotaLimitsLenient(limits SubscriptionPlanQuotaLimits) SubscriptionPlanQuotaLimits {
	result := make(SubscriptionPlanQuotaLimits, 0, len(limits))
	seen := make(map[string]struct{}, len(limits))
	for _, limit := range limits {
		period := NormalizeQuotaLimitPeriod(limit.Period)
		if period == "" || limit.Amount <= 0 {
			continue
		}
		limit.Period = period
		if period == SubscriptionResetCustom {
			if limit.CustomSeconds <= 0 {
				continue
			}
		} else {
			limit.CustomSeconds = 0
		}
		key := limit.Key()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, limit)
	}
	return result
}

var (
	quotaWindowLocationOnce  sync.Once
	quotaWindowLocationValue = time.UTC
)

// quotaWindowLocation returns the timezone used as the reference when a monthly
// window is rolled forward (its day of month must be taken from somewhere).
//
// It must be identical on every node: all nodes share the same subscription
// rows, so a node whose process timezone differs would compute a different
// boundary. The timezone is therefore fixed instead of coming from time.Local:
// SUBSCRIPTION_WINDOW_TIMEZONE (IANA name) selects it and it defaults to UTC.
// When running several nodes, configure the very same value on all of them.
func quotaWindowLocation() *time.Location {
	quotaWindowLocationOnce.Do(func() {
		name := strings.TrimSpace(common.GetEnvOrDefaultString("SUBSCRIPTION_WINDOW_TIMEZONE", ""))
		if name == "" {
			return
		}
		loc, err := time.LoadLocation(name)
		if err != nil {
			common.SysLog(common.LogText("invalid SUBSCRIPTION_WINDOW_TIMEZONE %q, falling back to UTC: %s", name, err.Error()))
			return
		}
		quotaWindowLocationValue = loc
	})
	return quotaWindowLocationValue
}

// quotaLimitWindow returns the [start, end) unix range of the window that
// contains now. Every window tumbles forward from the subscription start time
// instead of aligning to a wall-clock boundary: a weekly window always covers
// exactly seven days from the moment the subscription was opened, and the next
// window starts where the previous one ended.
func quotaLimitWindow(limit SubscriptionPlanQuotaLimit, anchor time.Time, now time.Time) (int64, int64) {
	switch NormalizeQuotaLimitPeriod(limit.Period) {
	case SubscriptionResetWeekly:
		return tumbleFixedWindow(anchor.Unix(), now.Unix(), 7*86400)
	case SubscriptionResetMonthly:
		return tumbleMonthWindow(anchor, now)
	case SubscriptionResetCustom:
		window := limit.CustomSeconds
		if window <= 0 {
			window = 86400
		}
		return tumbleFixedWindow(anchor.Unix(), now.Unix(), window)
	default: // daily, and unknown periods degrade to a one day window
		return tumbleFixedWindow(anchor.Unix(), now.Unix(), 86400)
	}
}

// tumbleFixedWindow returns the fixed length window that contains now, starting
// at anchor and repeating every window seconds.
func tumbleFixedWindow(anchorUnix, nowUnix, window int64) (int64, int64) {
	if window <= 0 {
		window = 86400
	}
	elapsed := nowUnix - anchorUnix
	if elapsed < 0 {
		elapsed = 0
	}
	start := anchorUnix + (elapsed/window)*window
	return start, start + window
}

// tumbleMonthWindow returns the calendar month window that contains now. Months
// keep the day of the subscription start (clamped to the last day of shorter
// months) and are evaluated in quotaWindowLocation so every node agrees.
func tumbleMonthWindow(anchor time.Time, now time.Time) (int64, int64) {
	base := anchor.In(quotaWindowLocation())
	if now.Unix() < base.Unix() {
		return base.Unix(), addMonthsClamped(base, 1).Unix()
	}
	baseYear, baseMonth, _ := base.Date()
	nowYear, nowMonth, _ := now.In(base.Location()).Date()
	months := (nowYear-baseYear)*12 + int(nowMonth-baseMonth)
	if months < 0 {
		months = 0
	}
	for months > 0 {
		start := addMonthsClamped(base, months)
		if start.Unix() <= now.Unix() {
			return start.Unix(), addMonthsClamped(base, months+1).Unix()
		}
		months--
	}
	return base.Unix(), addMonthsClamped(base, 1).Unix()
}

// addMonthsClamped adds n calendar months to base, keeping its day of month and
// clamping to the last day when the target month is shorter (Jan 31 -> Feb 28).
func addMonthsClamped(base time.Time, n int) time.Time {
	year, month, day := base.Date()
	hour, minute, sec := base.Clock()
	loc := base.Location()
	target := time.Date(year, month+time.Month(n), 1, hour, minute, sec, 0, loc)
	lastDay := time.Date(target.Year(), target.Month()+1, 0, hour, minute, sec, 0, loc).Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(target.Year(), target.Month(), day, hour, minute, sec, 0, loc)
}

// refreshQuotaLimitUsages rebuilds the usage list so it matches the plan's
// current limits and the windows containing now. A stored window that still
// covers now keeps its usage even when its recorded boundaries drifted (for
// example after upgrading from calendar aligned windows), while the boundaries
// themselves always follow the subscription start; an elapsed window restarts at
// zero. Shorter windows always accumulate into the longer windows they overlap
// (see applyQuotaLimitAccumulation). The returned bool reports whether the
// persisted value changed.
func refreshQuotaLimitUsages(sub *UserSubscription, plan *SubscriptionPlan, now time.Time) (SubscriptionQuotaLimitUsages, bool) {
	if sub == nil || plan == nil {
		return nil, false
	}
	limits := normalizeQuotaLimitsLenient(plan.QuotaLimits)
	existing := make(map[string]SubscriptionQuotaLimitUsage, len(sub.QuotaLimitUsage))
	for _, usage := range sub.QuotaLimitUsage {
		existing[usage.Key] = usage
	}
	anchor := time.Unix(sub.StartTime, 0)
	nowUnix := now.Unix()
	result := make(SubscriptionQuotaLimitUsages, 0, len(limits))
	for _, limit := range limits {
		key := limit.Key()
		start, end := quotaLimitWindow(limit, anchor, now)
		usage, ok := existing[key]
		if !ok || usage.WindowEnd <= usage.WindowStart || usage.WindowEnd <= nowUnix {
			// A new window (first tracked, unusable range, or the stored one
			// already elapsed): start counting from zero.
			usage = SubscriptionQuotaLimitUsage{Key: key, WindowStart: start, WindowEnd: end}
		} else {
			// Still the same window: keep the usage, but follow the boundaries
			// computed from the subscription start so the next reset time stays
			// anchored to it even when an older boundary was stored.
			usage.WindowStart = start
			usage.WindowEnd = end
		}
		result = append(result, usage)
	}
	applyQuotaLimitAccumulation(result)
	changed := len(sub.QuotaLimitUsage) != len(result)
	if !changed {
		for i := range result {
			stored, ok := existing[result[i].Key]
			if !ok || stored != result[i] {
				changed = true
				break
			}
		}
	}
	return result, changed
}

// quotaLimitUsageSpan returns the length of a tracked window in seconds.
func quotaLimitUsageSpan(usage SubscriptionQuotaLimitUsage) int64 {
	return usage.WindowEnd - usage.WindowStart
}

// quotaLimitUsagesOverlap reports whether two tracked windows share any instant.
func quotaLimitUsagesOverlap(a, b SubscriptionQuotaLimitUsage) bool {
	if a.WindowStart <= 0 || b.WindowStart <= 0 ||
		quotaLimitUsageSpan(a) <= 0 || quotaLimitUsageSpan(b) <= 0 {
		return false
	}
	return a.WindowStart < b.WindowEnd && b.WindowStart < a.WindowEnd
}

// applyQuotaLimitAccumulation makes every longer window include the usage of the
// shorter windows it overlaps, so quota consumed inside a 5-hour window also
// counts toward the weekly window and weekly usage counts toward the monthly
// window. It only ever raises a longer window, which means a counter that lost
// usage (for example after a node ran with a different timezone) heals itself
// instead of reporting less usage than one of its own sub-windows.
func applyQuotaLimitAccumulation(usages SubscriptionQuotaLimitUsages) bool {
	changed := false
	for {
		raised := false
		for i := range usages {
			for j := range usages {
				if i == j || usages[j].Used <= usages[i].Used {
					continue
				}
				if quotaLimitUsageSpan(usages[i]) <= quotaLimitUsageSpan(usages[j]) {
					continue
				}
				if !quotaLimitUsagesOverlap(usages[i], usages[j]) {
					continue
				}
				usages[i].Used = usages[j].Used
				raised = true
			}
		}
		if !raised {
			break
		}
		changed = true
	}
	return changed
}

func applyQuotaLimitUsage(usages SubscriptionQuotaLimitUsages, delta int64) {
	if delta == 0 {
		return
	}
	for i := range usages {
		used := usages[i].Used + delta
		if used < 0 {
			used = 0
		}
		usages[i].Used = used
	}
}

// QuotaLimitWindowLabel renders a short human readable window name for logs
// and user facing quota errors (for example "5h" or "weekly").
func QuotaLimitWindowLabel(limit SubscriptionPlanQuotaLimit) string {
	switch NormalizeQuotaLimitPeriod(limit.Period) {
	case SubscriptionResetDaily:
		return "daily"
	case SubscriptionResetWeekly:
		return "weekly"
	case SubscriptionResetMonthly:
		return "monthly"
	case SubscriptionResetCustom:
		seconds := limit.CustomSeconds
		switch {
		case seconds > 0 && seconds%3600 == 0:
			return fmt.Sprintf("%dh", seconds/3600)
		case seconds > 0 && seconds%60 == 0:
			return fmt.Sprintf("%dm", seconds/60)
		default:
			return fmt.Sprintf("%ds", seconds)
		}
	default:
		return "limit"
	}
}

// quotaLimitsInsufficient reports whether amount cannot fit in every window.
func quotaLimitsInsufficient(limits SubscriptionPlanQuotaLimits, usages SubscriptionQuotaLimitUsages, amount int64) (bool, string) {
	for i, limit := range limits {
		if limit.Amount <= 0 || i >= len(usages) {
			continue
		}
		if usages[i].Used+amount > limit.Amount {
			return true, QuotaLimitWindowLabel(limit)
		}
	}
	return false, ""
}

// Subscription plan
type SubscriptionPlan struct {
	Id int `json:"id"`

	Title    string `json:"title" gorm:"type:varchar(128);not null"`
	Subtitle string `json:"subtitle" gorm:"type:varchar(255);default:''"`

	// Display money amount (follow existing code style: float64 for money)
	PriceAmount float64 `json:"price_amount" gorm:"type:decimal(10,6);not null;default:0"`
	Currency    string  `json:"currency" gorm:"type:varchar(8);not null;default:'USD'"`

	DurationUnit  string `json:"duration_unit" gorm:"type:varchar(16);not null;default:'month'"`
	DurationValue int    `json:"duration_value" gorm:"type:int;not null;default:1"`
	CustomSeconds int64  `json:"custom_seconds" gorm:"type:bigint;not null;default:0"`

	Enabled   bool `json:"enabled" gorm:"default:true"`
	SortOrder int  `json:"sort_order" gorm:"type:int;default:0"`

	AllowBalancePay *bool `json:"allow_balance_pay"`

	// Allow falling back to wallet balance after subscription quota is exhausted (empty = true)
	AllowWalletOverflow *bool `json:"allow_wallet_overflow"`

	StripePriceId         string `json:"stripe_price_id" gorm:"type:varchar(128);default:''"`
	CreemProductId        string `json:"creem_product_id" gorm:"type:varchar(128);default:''"`
	WaffoPancakeProductId string `json:"waffo_pancake_product_id" gorm:"type:varchar(128);default:''"`

	// Max purchases per user (0 = unlimited)
	MaxPurchasePerUser int `json:"max_purchase_per_user" gorm:"type:int;default:0"`

	// Total stock across all users (0 = unlimited). Every issued user subscription of this plan
	// (paid orders, balance purchases and admin grants) consumes one unit.
	StockTotal int `json:"stock_total" gorm:"type:int;not null;default:0"`
	// Units currently occupied: active, unexpired user_subscriptions of this plan (computed, not persisted)
	StockSold int `json:"stock_sold" gorm:"-"`

	// Upgrade user group after purchase (empty = no change)
	UpgradeGroup string `json:"upgrade_group" gorm:"type:varchar(64);default:''"`

	// Downgrade user group on expiry (empty = revert to the group held before purchase)
	DowngradeGroup string `json:"downgrade_group" gorm:"type:varchar(64);default:''"`

	// Total quota (amount in quota units, 0 = unlimited)
	TotalAmount int64 `json:"total_amount" gorm:"type:bigint;not null;default:0"`

	// Quota reset period for plan (legacy single reset cycle)
	QuotaResetPeriod        string `json:"quota_reset_period" gorm:"type:varchar(16);default:'never'"`
	QuotaResetCustomSeconds int64  `json:"quota_reset_custom_seconds" gorm:"type:bigint;default:0"`

	// Independent quota windows enforced at the same time (e.g. 5h + weekly +
	// monthly). Each entry resets on its own window while TotalAmount remains
	// the lifetime cap.
	QuotaLimits SubscriptionPlanQuotaLimits `json:"quota_limits" gorm:"type:text"`

	CreatedAt int64 `json:"created_at" gorm:"bigint"`
	UpdatedAt int64 `json:"updated_at" gorm:"bigint"`
}

func (p *SubscriptionPlan) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	p.CreatedAt = now
	p.UpdatedAt = now
	return nil
}

func (p *SubscriptionPlan) BeforeUpdate(tx *gorm.DB) error {
	p.UpdatedAt = common.GetTimestamp()
	return nil
}

// IsSoldOut reports whether the plan has a stock limit and it has been used up.
// StockSold must have been filled by FillSubscriptionPlanStockSold.
func (p *SubscriptionPlan) IsSoldOut() bool {
	return p != nil && p.StockTotal > 0 && p.StockSold >= p.StockTotal
}

var ErrSubscriptionPlanSoldOut = common.NewMessage("This plan is sold out")

// countSubscriptionPlanIssued counts the plan's subscriptions that still occupy stock
// (active and not yet ended; expired / cancelled ones release their unit),
// plus outstanding subscription redemption codes that still reserve stock.
func countSubscriptionPlanIssued(db *gorm.DB, planId int) (int64, error) {
	return countSubscriptionPlanIssuedExcluding(db, planId, 0)
}

func countSubscriptionPlanIssuedExcluding(db *gorm.DB, planId int, excludeRedemptionId int) (int64, error) {
	var count int64
	if err := db.Model(&UserSubscription{}).
		Where("plan_id = ? AND status = ? AND end_time > ?", planId, "active", common.GetTimestamp()).
		Count(&count).Error; err != nil {
		return 0, err
	}
	reserved, err := countSubscriptionPlanReservedRedemptions(db, planId, excludeRedemptionId)
	if err != nil {
		return 0, err
	}
	return count + reserved, nil
}

// redemptionReservesStockQuery filters subscription codes that still hold a stock slot:
// not yet redeemed (enabled or disabled) and not expired. Soft-deleted codes are excluded by gorm.
func redemptionReservesStockQuery(db *gorm.DB) *gorm.DB {
	return db.Model(&Redemption{}).Where("plan_id > 0 AND status <> ? AND (expired_time = 0 OR expired_time >= ?)",
		common.RedemptionCodeStatusUsed, common.GetTimestamp())
}

// redemptionReservesStock mirrors redemptionReservesStockQuery for a single code.
func redemptionReservesStock(r *Redemption) bool {
	if r == nil || r.PlanId <= 0 || r.Status == common.RedemptionCodeStatusUsed {
		return false
	}
	return r.ExpiredTime == 0 || r.ExpiredTime >= common.GetTimestamp()
}

// redemptionReservedSlotsExpr is the SQL expression for how many plan stock slots
// one redemption code still reserves. Codes with a total usage limit reserve their
// remaining uses; unlimited codes reserve one slot.
const redemptionReservedSlotsExpr = "CASE WHEN max_uses > 0 THEN CASE WHEN max_uses > used_count THEN max_uses - used_count ELSE 0 END ELSE 1 END"

// redemptionReservedSlotCount mirrors redemptionReservedSlotsExpr for a single code.
func redemptionReservedSlotCount(r *Redemption) int {
	if !redemptionReservesStock(r) {
		return 0
	}
	if r.MaxUses > 0 {
		return max(r.MaxUses-r.UsedCount, 0)
	}
	return 1
}

func countSubscriptionPlanReservedRedemptions(db *gorm.DB, planId int, excludeRedemptionId int) (int64, error) {
	var result struct {
		Total int64
	}
	query := redemptionReservesStockQuery(db).Where("plan_id = ?", planId)
	if excludeRedemptionId > 0 {
		query = query.Where("id <> ?", excludeRedemptionId)
	}
	if err := query.
		Select("COALESCE(SUM(" + redemptionReservedSlotsExpr + "), 0) AS total").
		Scan(&result).Error; err != nil {
		return 0, err
	}
	return result.Total, nil
}

// checkSubscriptionPlanStock compares stock_total (read from the database, bypassing the
// plan cache) with the number of issued subscriptions. When lock is true the plan row is
// locked so concurrent issuers inside transactions are serialized per plan.
func checkSubscriptionPlanStock(db *gorm.DB, planId int, lock bool) error {
	return checkSubscriptionPlanStockFor(db, planId, lock, 1, 0)
}

// checkSubscriptionPlanStockFor verifies that `need` more slots are available.
// excludeRedemptionId skips one redemption code from the reserved count (used when editing it).
func checkSubscriptionPlanStockFor(db *gorm.DB, planId int, lock bool, need int, excludeRedemptionId int) error {
	if db == nil || planId <= 0 {
		return errors.New("invalid plan id")
	}
	query := db
	if lock {
		query = lockForUpdate(db)
	}
	var plan SubscriptionPlan
	if err := query.Select("id", "stock_total").Where("id = ?", planId).First(&plan).Error; err != nil {
		return err
	}
	if plan.StockTotal <= 0 {
		return nil
	}
	count, err := countSubscriptionPlanIssuedExcluding(db, planId, excludeRedemptionId)
	if err != nil {
		return err
	}
	if count+int64(need) > int64(plan.StockTotal) {
		return ErrSubscriptionPlanSoldOut
	}
	return nil
}

// CheckSubscriptionPlanStock is the pre-order check used before creating a payment order.
func CheckSubscriptionPlanStock(planId int) error {
	return checkSubscriptionPlanStock(DB, planId, false)
}

// FillSubscriptionPlanStockSold sets StockSold of each plan to the number of its
// subscriptions still occupying stock (active and not yet ended).
func FillSubscriptionPlanStockSold(plans []SubscriptionPlan) error {
	if len(plans) == 0 {
		return nil
	}
	ids := make([]int, 0, len(plans))
	for _, p := range plans {
		ids = append(ids, p.Id)
	}
	var rows []struct {
		PlanId int
		Cnt    int64
	}
	if err := DB.Model(&UserSubscription{}).
		Select("plan_id, COUNT(*) AS cnt").
		Where("plan_id IN ? AND status = ? AND end_time > ?", ids, "active", common.GetTimestamp()).
		Group("plan_id").
		Scan(&rows).Error; err != nil {
		return err
	}
	counts := make(map[int]int64, len(rows))
	for _, r := range rows {
		counts[r.PlanId] = r.Cnt
	}
	// Outstanding subscription redemption codes also occupy stock.
	var reservedRows []struct {
		PlanId int
		Cnt    int64
	}
	if err := redemptionReservesStockQuery(DB).
		Select("plan_id, COALESCE(SUM("+redemptionReservedSlotsExpr+"), 0) AS cnt").
		Where("plan_id IN ?", ids).
		Group("plan_id").
		Scan(&reservedRows).Error; err != nil {
		return err
	}
	for _, r := range reservedRows {
		counts[r.PlanId] += r.Cnt
	}
	for i := range plans {
		plans[i].StockSold = int(counts[plans[i].Id])
	}
	return nil
}

func (p *SubscriptionPlan) NormalizeDefaults() {
	if p.AllowBalancePay == nil {
		p.AllowBalancePay = common.GetPointer(true)
	}
	if p.AllowWalletOverflow == nil {
		p.AllowWalletOverflow = common.GetPointer(true)
	}
	p.QuotaLimits = normalizeQuotaLimitsLenient(p.QuotaLimits)
}

// Subscription order (payment -> webhook -> create UserSubscription)
type SubscriptionOrder struct {
	Id     int     `json:"id"`
	UserId int     `json:"user_id" gorm:"index"`
	PlanId int     `json:"plan_id" gorm:"index"`
	Money  float64 `json:"money"`

	TradeNo         string `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod   string `json:"payment_method" gorm:"type:varchar(50)"`
	PaymentProvider string `json:"payment_provider" gorm:"type:varchar(50);default:''"`
	Status          string `json:"status"`
	CreateTime      int64  `json:"create_time"`
	CompleteTime    int64  `json:"complete_time"`

	ProviderPayload string `json:"provider_payload" gorm:"type:text"`
}

func (o *SubscriptionOrder) Insert() error {
	if o.CreateTime == 0 {
		o.CreateTime = common.GetTimestamp()
	}
	return DB.Create(o).Error
}

func (o *SubscriptionOrder) Update() error {
	return DB.Save(o).Error
}

func GetSubscriptionOrderByTradeNo(tradeNo string) *SubscriptionOrder {
	if tradeNo == "" {
		return nil
	}
	var order SubscriptionOrder
	if err := DB.Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
		return nil
	}
	return &order
}

// User subscription instance
type UserSubscription struct {
	Id     int `json:"id"`
	UserId int `json:"user_id" gorm:"index;index:idx_user_sub_active,priority:1"`
	PlanId int `json:"plan_id" gorm:"index"`

	AmountTotal int64 `json:"amount_total" gorm:"type:bigint;not null;default:0"`
	AmountUsed  int64 `json:"amount_used" gorm:"type:bigint;not null;default:0"`

	StartTime int64  `json:"start_time" gorm:"bigint"`
	EndTime   int64  `json:"end_time" gorm:"bigint;index;index:idx_user_sub_active,priority:3"`
	Status    string `json:"status" gorm:"type:varchar(32);index;index:idx_user_sub_active,priority:2"` // active/expired/cancelled

	Source string `json:"source" gorm:"type:varchar(32);default:'order'"` // order/admin

	LastResetTime int64 `json:"last_reset_time" gorm:"type:bigint;default:0"`
	NextResetTime int64 `json:"next_reset_time" gorm:"type:bigint;default:0;index"`

	// Per-window usage of the plan's independent quota limits. Windows reset
	// independently and never touch AmountUsed.
	QuotaLimitUsage SubscriptionQuotaLimitUsages `json:"quota_limit_usage" gorm:"type:text"`

	UpgradeGroup  string `json:"upgrade_group" gorm:"type:varchar(64);default:''"`
	PrevUserGroup string `json:"prev_user_group" gorm:"type:varchar(64);default:''"`

	// Downgrade target group on expiry (snapshot from plan; empty = revert to PrevUserGroup)
	DowngradeGroup string `json:"downgrade_group" gorm:"type:varchar(64);default:''"`

	// Whether wallet fallback is allowed after this subscription's quota is exhausted (snapshot from plan)
	AllowWalletOverflow bool `json:"allow_wallet_overflow"`

	CreatedAt int64 `json:"created_at" gorm:"bigint"`
	UpdatedAt int64 `json:"updated_at" gorm:"bigint"`
}

func (s *UserSubscription) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	s.CreatedAt = now
	s.UpdatedAt = now
	return nil
}

func (s *UserSubscription) BeforeUpdate(tx *gorm.DB) error {
	s.UpdatedAt = common.GetTimestamp()
	return nil
}

type SubscriptionSummary struct {
	Subscription *UserSubscription `json:"subscription"`
}

// SubscriptionResetScope selects the quota counters an administrative reset
// clears. ResetTotal clears the lifetime AmountUsed counter; LimitKeys lists the
// independent quota windows (SubscriptionPlanQuotaLimit.Key) to clear. A nil
// scope keeps the legacy behaviour: every counter of the plan is reset.
type SubscriptionResetScope struct {
	ResetTotal bool     `json:"reset_total"`
	LimitKeys  []string `json:"limit_keys"`
}

type SubscriptionResetResult struct {
	PlanId           int      `json:"plan_id"`
	MatchedCount     int      `json:"matched_count"`
	ResetCount       int      `json:"reset_count"`
	UserCount        int      `json:"user_count"`
	AdvanceResetTime bool     `json:"advance_reset_time"`
	ResetTotal       bool     `json:"reset_total"`
	ResetLimitKeys   []string `json:"reset_limit_keys"`
	PlanTitle        string   `json:"-"`
	AffectedUserIds  []int    `json:"-"`
}

// normalizeResetScope resolves nil to the full scope and validates that every
// requested window still exists in the plan's current configuration, so a stale
// dialog cannot silently reset nothing.
func normalizeResetScope(plan *SubscriptionPlan, scope *SubscriptionResetScope) (*SubscriptionResetScope, error) {
	if plan == nil {
		return nil, errors.New("invalid plan")
	}
	available := normalizeQuotaLimitsLenient(plan.QuotaLimits)
	allKeys := make([]string, 0, len(available))
	known := make(map[string]struct{}, len(available))
	for _, limit := range available {
		key := limit.Key()
		allKeys = append(allKeys, key)
		known[key] = struct{}{}
	}
	if scope == nil {
		return &SubscriptionResetScope{ResetTotal: true, LimitKeys: allKeys}, nil
	}
	result := &SubscriptionResetScope{ResetTotal: scope.ResetTotal}
	seen := make(map[string]struct{}, len(scope.LimitKeys))
	for _, rawKey := range scope.LimitKeys {
		key := strings.TrimSpace(rawKey)
		if key == "" {
			continue
		}
		if _, ok := known[key]; !ok {
			return nil, common.NewMessage("Selected quota limit is no longer configured for this plan")
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result.LimitKeys = append(result.LimitKeys, key)
	}
	if !result.ResetTotal && len(result.LimitKeys) == 0 {
		return nil, common.NewMessage("Select at least one quota limit to reset")
	}
	return result, nil
}

func calcPlanEndTime(start time.Time, plan *SubscriptionPlan) (int64, error) {
	if plan == nil {
		return 0, errors.New("plan is nil")
	}
	if plan.DurationValue <= 0 && plan.DurationUnit != SubscriptionDurationCustom {
		return 0, errors.New("duration_value must be > 0")
	}
	switch plan.DurationUnit {
	case SubscriptionDurationYear:
		return start.AddDate(plan.DurationValue, 0, 0).Unix(), nil
	case SubscriptionDurationMonth:
		return start.AddDate(0, plan.DurationValue, 0).Unix(), nil
	case SubscriptionDurationDay:
		return start.Add(time.Duration(plan.DurationValue) * 24 * time.Hour).Unix(), nil
	case SubscriptionDurationHour:
		return start.Add(time.Duration(plan.DurationValue) * time.Hour).Unix(), nil
	case SubscriptionDurationCustom:
		if plan.CustomSeconds <= 0 {
			return 0, errors.New("custom_seconds must be > 0")
		}
		return start.Add(time.Duration(plan.CustomSeconds) * time.Second).Unix(), nil
	default:
		return 0, fmt.Errorf("invalid duration_unit: %s", plan.DurationUnit)
	}
}

func NormalizeResetPeriod(period string) string {
	switch strings.TrimSpace(period) {
	case SubscriptionResetDaily, SubscriptionResetWeekly, SubscriptionResetMonthly, SubscriptionResetCustom:
		return strings.TrimSpace(period)
	default:
		return SubscriptionResetNever
	}
}

func calcNextResetTime(base time.Time, plan *SubscriptionPlan, endUnix int64) int64 {
	if plan == nil {
		return 0
	}
	period := NormalizeResetPeriod(plan.QuotaResetPeriod)
	if period == SubscriptionResetNever {
		return 0
	}
	// The reset cycle follows the subscription instead of a wall-clock boundary:
	// exactly one period is added to base (the subscription start or the previous
	// reset), so the next reset always lands one period after the subscription
	// was opened rather than at the next midnight / Monday / month start.
	var next time.Time
	switch period {
	case SubscriptionResetDaily:
		next = base.Add(24 * time.Hour)
	case SubscriptionResetWeekly:
		next = base.Add(7 * 24 * time.Hour)
	case SubscriptionResetMonthly:
		next = addMonthsClamped(base.In(quotaWindowLocation()), 1)
	case SubscriptionResetCustom:
		if plan.QuotaResetCustomSeconds <= 0 {
			return 0
		}
		next = base.Add(time.Duration(plan.QuotaResetCustomSeconds) * time.Second)
	default:
		return 0
	}
	if endUnix > 0 && next.Unix() > endUnix {
		return 0
	}
	return next.Unix()
}

func GetSubscriptionPlanById(id int) (*SubscriptionPlan, error) {
	return getSubscriptionPlanByIdTx(nil, id)
}

func getSubscriptionPlanByIdTx(tx *gorm.DB, id int) (*SubscriptionPlan, error) {
	if id <= 0 {
		return nil, errors.New("invalid plan id")
	}
	key := subscriptionPlanCacheKey(id)
	if key != "" {
		if cached, found, err := getSubscriptionPlanCache().Get(key); err == nil && found {
			cached.NormalizeDefaults()
			return &cached, nil
		}
	}
	var plan SubscriptionPlan
	query := DB
	if tx != nil {
		query = tx
	}
	if err := query.Where("id = ?", id).First(&plan).Error; err != nil {
		return nil, err
	}
	plan.NormalizeDefaults()
	_ = getSubscriptionPlanCache().SetWithTTL(key, plan, subscriptionPlanCacheTTL())
	return &plan, nil
}

// countActiveUserSubscriptionsByPlan counts the subscriptions that still occupy
// a purchase-limit slot: active and not yet ended. Expired or cancelled
// subscriptions release their slot so the user can buy the plan again.
func countActiveUserSubscriptionsByPlan(db *gorm.DB, userId int, planId int, now int64) (int64, error) {
	if db == nil {
		db = DB
	}
	var count int64
	if err := db.Model(&UserSubscription{}).
		Where("user_id = ? AND plan_id = ? AND status = ? AND end_time > ?", userId, planId, "active", now).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func CountUserSubscriptionsByPlan(userId int, planId int) (int64, error) {
	if userId <= 0 || planId <= 0 {
		return 0, errors.New("invalid userId or planId")
	}
	return countActiveUserSubscriptionsByPlan(DB, userId, planId, GetDBTimestamp())
}

// getRenewableUserSubscriptionTx returns the active, unexpired subscription of
// the same plan that a new purchase extends. Buying a plan the user already
// holds renews that subscription: its validity is prolonged instead of opening
// a second row, so the plan's stock is not consumed again.
func getRenewableUserSubscriptionTx(db *gorm.DB, userId int, planId int, now int64) (*UserSubscription, error) {
	if db == nil {
		db = DB
	}
	var sub UserSubscription
	err := lockForUpdate(db).
		Where("user_id = ? AND plan_id = ? AND status = ? AND end_time > ?", userId, planId, "active", now).
		Order("end_time desc, id desc").
		First(&sub).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &sub, nil
}

// isUserSubscriptionRenewalTx reports whether the user already holds an active,
// unexpired subscription of the plan. It only reads, so callers can use it
// before they decide whether a stock / purchase-limit check is required.
func isUserSubscriptionRenewalTx(db *gorm.DB, userId int, planId int) (bool, error) {
	if db == nil {
		db = DB
	}
	if userId <= 0 || planId <= 0 {
		return false, nil
	}
	var count int64
	if err := db.Model(&UserSubscription{}).
		Where("user_id = ? AND plan_id = ? AND status = ? AND end_time > ?", userId, planId, "active", GetDBTimestamp()).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// IsUserSubscriptionRenewal reports whether buying planId for userId renews an
// existing active subscription. Renewals neither consume a plan stock slot nor
// count against MaxPurchasePerUser.
func IsUserSubscriptionRenewal(userId int, planId int) (bool, error) {
	return isUserSubscriptionRenewalTx(DB, userId, planId)
}

// getConflictingActiveSubscriptionTx returns the user's active, unexpired
// subscription of another plan. A user holds at most one active subscription
// at a time, so a second plan must wait until that one ends.
func getConflictingActiveSubscriptionTx(db *gorm.DB, userId int, planId int, now int64) (*UserSubscription, error) {
	if db == nil {
		db = DB
	}
	var sub UserSubscription
	err := db.
		Where("user_id = ? AND plan_id <> ? AND status = ? AND end_time > ?", userId, planId, "active", now).
		Order("end_time desc, id desc").
		First(&sub).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &sub, nil
}

// ensureNoConflictingActiveSubscription blocks buying another plan while the
// user already has an active subscription. Renewing the running plan stays
// allowed; changing plans is an administrator action (cancel the current
// subscription, then grant the new one).
func ensureNoConflictingActiveSubscription(db *gorm.DB, userId int, planId int, now int64) error {
	conflict, err := getConflictingActiveSubscriptionTx(db, userId, planId, now)
	if err != nil {
		return err
	}
	if conflict == nil {
		return nil
	}
	return common.NewMessage("You already have an active subscription. Only renewal or switching via the administrator is allowed.")
}

// CheckSubscriptionPurchaseAllowed validates plan stock and the per-user
// purchase limit before a payment order is created. A renewal skips both
// checks: it extends the existing subscription and therefore takes no new stock
// slot. Buying a different plan while any subscription is still running is
// rejected: the user may only renew it or ask an administrator to switch.
func CheckSubscriptionPurchaseAllowed(userId int, plan *SubscriptionPlan) error {
	if plan == nil || plan.Id <= 0 {
		return errors.New("invalid plan")
	}
	renewal, err := IsUserSubscriptionRenewal(userId, plan.Id)
	if err != nil {
		return err
	}
	if renewal {
		return nil
	}
	if err := ensureNoConflictingActiveSubscription(DB, userId, plan.Id, GetDBTimestamp()); err != nil {
		return err
	}
	if err := CheckSubscriptionPlanStock(plan.Id); err != nil {
		return err
	}
	if plan.MaxPurchasePerUser > 0 {
		count, err := CountUserSubscriptionsByPlan(userId, plan.Id)
		if err != nil {
			return err
		}
		if count >= int64(plan.MaxPurchasePerUser) {
			return common.NewMessage("Purchase limit for this plan has been reached")
		}
	}
	return nil
}

func getUserGroupByIdTx(tx *gorm.DB, userId int) (string, error) {
	if userId <= 0 {
		return "", errors.New("invalid userId")
	}
	if tx == nil {
		tx = DB
	}
	var group string
	if err := lockForUpdate(tx).Model(&User{}).Where("id = ?", userId).Select(commonGroupCol).Find(&group).Error; err != nil {
		return "", err
	}
	return group, nil
}

func downgradeUserGroupForSubscriptionTx(tx *gorm.DB, sub *UserSubscription, now int64) (string, error) {
	if tx == nil || sub == nil {
		return "", errors.New("invalid downgrade args")
	}
	downgradeGroup := strings.TrimSpace(sub.DowngradeGroup)
	upgradeGroup := strings.TrimSpace(sub.UpgradeGroup)
	// Nothing to do if neither an explicit downgrade target nor an upgrade snapshot exists.
	if downgradeGroup == "" && upgradeGroup == "" {
		return "", nil
	}
	currentGroup, err := getUserGroupByIdTx(tx, sub.UserId)
	if err != nil {
		return "", err
	}
	// If another active upgraded subscription exists, keep the current group.
	var activeSub UserSubscription
	activeQuery := tx.Where("user_id = ? AND status = ? AND end_time > ? AND id <> ? AND upgrade_group <> ''",
		sub.UserId, "active", now, sub.Id).
		Order("end_time desc, id desc").
		Limit(1).
		Find(&activeSub)
	if activeQuery.Error == nil && activeQuery.RowsAffected > 0 {
		return "", nil
	}
	// Determine the downgrade target: an explicit downgrade group takes precedence,
	// otherwise revert to the group held before purchase (legacy behavior).
	target := downgradeGroup
	if target == "" {
		// Legacy behavior: only revert when the subscription actually elevated the user.
		if currentGroup != upgradeGroup {
			return "", nil
		}
		target = strings.TrimSpace(sub.PrevUserGroup)
	}
	if target == "" || target == currentGroup {
		return "", nil
	}
	if err := tx.Model(&User{}).Where("id = ?", sub.UserId).
		Update("group", target).Error; err != nil {
		return "", err
	}
	return target, nil
}

// renewUserSubscriptionTx prolongs an existing subscription by one plan period
// (the period starts where the current one ends) and opens a fresh quota cycle:
// the lifetime usage and every quota window (for example the monthly, weekly and
// 5-hour limits) restart at zero. No row is inserted, so plan stock is untouched.
func renewUserSubscriptionTx(tx *gorm.DB, sub *UserSubscription, plan *SubscriptionPlan, periodEndUnix int64) (*UserSubscription, error) {
	if tx == nil || sub == nil || plan == nil {
		return nil, errors.New("invalid renewal args")
	}
	nowUnix := GetDBTimestamp()
	now := time.Unix(nowUnix, 0)
	periodSeconds := periodEndUnix - nowUnix
	if periodSeconds <= 0 {
		return nil, errors.New("invalid renewal period")
	}
	newEnd := sub.EndTime + periodSeconds
	updates := map[string]any{
		"end_time":        newEnd,
		"amount_used":     0,
		"last_reset_time": nowUnix,
		"updated_at":      common.GetTimestamp(),
	}
	sub.AmountUsed = 0
	sub.LastResetTime = nowUnix
	if plan.TotalAmount > 0 {
		updates["amount_total"] = plan.TotalAmount
		sub.AmountTotal = plan.TotalAmount
	}
	// Restart every independent quota window: the renewed period starts with a
	// full allowance for the monthly / weekly / custom windows too.
	usages, _ := refreshQuotaLimitUsages(sub, plan, now)
	for i := range usages {
		usages[i].Used = 0
	}
	updates["quota_limit_usage"] = usages
	sub.QuotaLimitUsage = usages
	// Restart the legacy single reset cycle from the renewal moment so the next
	// reset lands one period later and still fits inside the extended validity.
	nextReset := calcNextResetTime(now, plan, newEnd)
	updates["next_reset_time"] = nextReset
	sub.NextResetTime = nextReset
	if err := tx.Model(&UserSubscription{}).Where("id = ?", sub.Id).Updates(updates).Error; err != nil {
		return nil, err
	}
	sub.EndTime = newEnd
	sub.UpdatedAt = common.GetTimestamp()
	return sub, nil
}

func CreateUserSubscriptionFromPlanTx(tx *gorm.DB, userId int, plan *SubscriptionPlan, source string) (*UserSubscription, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}
	if plan == nil || plan.Id == 0 {
		return nil, errors.New("invalid plan")
	}
	if userId <= 0 {
		return nil, errors.New("invalid user id")
	}
	nowUnix := GetDBTimestamp()
	now := time.Unix(nowUnix, 0)
	endUnix, err := calcPlanEndTime(now, plan)
	if err != nil {
		return nil, err
	}
	// A purchase for a plan the user already holds renews that subscription:
	// the validity is prolonged instead of inserting a new row, so the existing
	// stock slot keeps covering the user.
	renewable, err := getRenewableUserSubscriptionTx(tx, userId, plan.Id, nowUnix)
	if err != nil {
		return nil, err
	}
	if renewable != nil {
		return renewUserSubscriptionTx(tx, renewable, plan, endUnix)
	}
	if plan.MaxPurchasePerUser > 0 {
		count, err := countActiveUserSubscriptionsByPlan(tx, userId, plan.Id, nowUnix)
		if err != nil {
			return nil, err
		}
		if count >= int64(plan.MaxPurchasePerUser) {
			return nil, common.NewMessage("Purchase limit for this plan has been reached")
		}
	}
	resetBase := now
	nextReset := calcNextResetTime(resetBase, plan, endUnix)
	lastReset := int64(0)
	if nextReset > 0 {
		lastReset = now.Unix()
	}
	upgradeGroup := strings.TrimSpace(plan.UpgradeGroup)
	prevGroup := ""
	if upgradeGroup != "" {
		currentGroup, err := getUserGroupByIdTx(tx, userId)
		if err != nil {
			return nil, err
		}
		if currentGroup != upgradeGroup {
			prevGroup = currentGroup
			if err := tx.Model(&User{}).Where("id = ?", userId).
				Update("group", upgradeGroup).Error; err != nil {
				return nil, err
			}
		}
	}
	allowWalletOverflow := true
	if plan.AllowWalletOverflow != nil {
		allowWalletOverflow = *plan.AllowWalletOverflow
	}
	sub := &UserSubscription{
		UserId:              userId,
		PlanId:              plan.Id,
		AmountTotal:         plan.TotalAmount,
		AmountUsed:          0,
		StartTime:           now.Unix(),
		EndTime:             endUnix,
		Status:              "active",
		Source:              source,
		LastResetTime:       lastReset,
		NextResetTime:       nextReset,
		UpgradeGroup:        upgradeGroup,
		PrevUserGroup:       prevGroup,
		DowngradeGroup:      strings.TrimSpace(plan.DowngradeGroup),
		AllowWalletOverflow: allowWalletOverflow,
		CreatedAt:           common.GetTimestamp(),
		UpdatedAt:           common.GetTimestamp(),
	}
	if usages, _ := refreshQuotaLimitUsages(sub, plan, now); len(usages) > 0 {
		sub.QuotaLimitUsage = usages
	}
	if err := tx.Create(sub).Error; err != nil {
		return nil, err
	}
	return sub, nil
}

func refreshSubscriptionUserGroupCache(userId int, operation string) {
	if err := RefreshUserGroupCache(userId); err != nil {
		common.SysError(common.LogText("failed to refresh user group cache after %s for user %d: %v", operation, userId, err))
	}
}

// Complete a subscription order (idempotent). Creates a UserSubscription snapshot from the plan.
// expectedPaymentProvider guards against cross-gateway callback attacks (empty skips the check).
// actualPaymentMethod updates the order's PaymentMethod to reflect the real payment type used (empty skips update).
func CompleteSubscriptionOrder(tradeNo string, providerPayload string, expectedPaymentProvider string, actualPaymentMethod string) error {
	if tradeNo == "" {
		return errors.New("tradeNo is empty")
	}
	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}
	var logUserId int
	var logPlanTitle string
	var logMoney float64
	var logPaymentMethod string
	var upgradeGroup string
	err := DB.Transaction(func(tx *gorm.DB) error {
		var order SubscriptionOrder
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(&order).Error; err != nil {
			return ErrSubscriptionOrderNotFound
		}
		if expectedPaymentProvider != "" && order.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if order.Status == common.TopUpStatusSuccess {
			return nil
		}
		if order.Status != common.TopUpStatusPending {
			return ErrSubscriptionOrderStatusInvalid
		}
		plan, err := GetSubscriptionPlanById(order.PlanId)
		if err != nil {
			return err
		}
		if !plan.Enabled {
			// still allow completion for already purchased orders
		}
		// 锁定用户行：并发完成同一用户的不同订单（包括多实例部署下）时，
		// 使 CreateUserSubscriptionFromPlanTx 的 MaxPurchasePerUser 检查按用户串行。
		var userRow User
		if err := lockForUpdate(tx).Select("id").Where("id = ?", order.UserId).First(&userRow).Error; err != nil {
			return err
		}
		// Lock the plan row so stock counting stays serialized with balance purchases and
		// admin grants. The order is already paid, so it is completed even if stock ran out
		// concurrently; new orders are blocked by CheckSubscriptionPlanStock.
		var planRow SubscriptionPlan
		if err := lockForUpdate(tx).Select("id").Where("id = ?", plan.Id).First(&planRow).Error; err != nil {
			return err
		}
		subscription, err := CreateUserSubscriptionFromPlanTx(tx, order.UserId, plan, "order")
		if err != nil {
			return err
		}
		if subscription.PrevUserGroup != "" {
			upgradeGroup = strings.TrimSpace(subscription.UpgradeGroup)
		}
		if err := upsertSubscriptionTopUpTx(tx, &order); err != nil {
			return err
		}
		order.Status = common.TopUpStatusSuccess
		order.CompleteTime = common.GetTimestamp()
		if providerPayload != "" {
			order.ProviderPayload = providerPayload
		}
		if actualPaymentMethod != "" && order.PaymentMethod != actualPaymentMethod {
			order.PaymentMethod = actualPaymentMethod
		}
		if err := tx.Save(&order).Error; err != nil {
			return err
		}
		logUserId = order.UserId
		logPlanTitle = plan.Title
		logMoney = order.Money
		logPaymentMethod = order.PaymentMethod
		return nil
	})
	if err != nil {
		return err
	}
	if upgradeGroup != "" && logUserId > 0 {
		refreshSubscriptionUserGroupCache(logUserId, common.LogText("subscription payment completion"))
	}
	if logUserId > 0 {
		RecordLog(logUserId, LogTypeTopup, common.NewMessage("Subscription purchased, plan: {{plan}}, payment amount: {{amount}}, payment method: {{method}}", map[string]any{
			"plan":   logPlanTitle,
			"amount": fmt.Sprintf("%.2f", logMoney),
			"method": logPaymentMethod,
		}))
	}
	return nil
}

func upsertSubscriptionTopUpTx(tx *gorm.DB, order *SubscriptionOrder) error {
	if tx == nil || order == nil {
		return errors.New("invalid subscription order")
	}
	now := common.GetTimestamp()
	var topup TopUp
	if err := tx.Where("trade_no = ?", order.TradeNo).First(&topup).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			topup = TopUp{
				UserId:        order.UserId,
				Amount:        0,
				Money:         order.Money,
				TradeNo:       order.TradeNo,
				PaymentMethod: order.PaymentMethod,
				CreateTime:    order.CreateTime,
				CompleteTime:  now,
				Status:        common.TopUpStatusSuccess,
			}
			return tx.Create(&topup).Error
		}
		return err
	}
	topup.Money = order.Money
	if topup.PaymentMethod == "" {
		topup.PaymentMethod = order.PaymentMethod
	} else if topup.PaymentMethod != order.PaymentMethod {
		return ErrPaymentMethodMismatch
	}
	if topup.CreateTime == 0 {
		topup.CreateTime = order.CreateTime
	}
	topup.CompleteTime = now
	topup.Status = common.TopUpStatusSuccess
	return tx.Save(&topup).Error
}

func ExpireSubscriptionOrder(tradeNo string, expectedPaymentProvider string) error {
	if tradeNo == "" {
		return errors.New("tradeNo is empty")
	}
	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var order SubscriptionOrder
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(&order).Error; err != nil {
			return ErrSubscriptionOrderNotFound
		}
		if expectedPaymentProvider != "" && order.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if order.Status != common.TopUpStatusPending {
			return nil
		}
		order.Status = common.TopUpStatusExpired
		order.CompleteTime = common.GetTimestamp()
		return tx.Save(&order).Error
	})
}

// Admin bind (no payment). Creates a UserSubscription from a plan.
func AdminBindSubscription(userId int, planId int, sourceNote string) (*common.Message, error) {
	if userId <= 0 || planId <= 0 {
		return nil, errors.New("invalid userId or planId")
	}
	plan, err := GetSubscriptionPlanById(planId)
	if err != nil {
		return nil, err
	}
	groupChanged := false
	err = DB.Transaction(func(tx *gorm.DB) error {
		// 与 CompleteSubscriptionOrder 一致：先锁用户行，再做购买次数检查。
		var userRow User
		if err := lockForUpdate(tx).Select("id").Where("id = ?", userId).First(&userRow).Error; err != nil {
			return err
		}
		// Admin grants also consume plan stock, except when they renew the user's
		// existing subscription of the same plan (no new slot is taken).
		renewal, err := isUserSubscriptionRenewalTx(tx, userId, plan.Id)
		if err != nil {
			return err
		}
		if !renewal {
			if err := checkSubscriptionPlanStock(tx, plan.Id, true); err != nil {
				return err
			}
		}
		subscription, err := CreateUserSubscriptionFromPlanTx(tx, userId, plan, "admin")
		if err == nil {
			groupChanged = subscription.PrevUserGroup != ""
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if groupChanged {
		refreshSubscriptionUserGroupCache(userId, common.LogText("admin subscription creation"))
		return common.NewMessage("User group will be upgraded to {{group}}", map[string]any{"group": plan.UpgradeGroup}), nil
	}
	return nil, nil
}

func calcSubscriptionBalanceQuota(priceAmount float64) (int, error) {
	if priceAmount <= 0 {
		return 0, nil
	}
	if common.QuotaPerUnit <= 0 {
		return 0, common.NewMessage("Quota unit is misconfigured")
	}
	quota := decimal.NewFromFloat(priceAmount).
		Mul(decimal.NewFromFloat(common.QuotaPerUnit)).
		Ceil()
	return common.WalletQuotaFromDecimalStrict(quota)
}

// PurchaseSubscriptionWithBalance creates a subscription by deducting the user's wallet quota.
func PurchaseSubscriptionWithBalance(userId int, planId int) error {
	if userId <= 0 || planId <= 0 {
		return errors.New("invalid userId or planId")
	}

	var logPlanTitle string
	var logMoney float64
	var chargedQuota int
	var upgradeGroup string
	err := DB.Transaction(func(tx *gorm.DB) error {
		plan, err := getSubscriptionPlanByIdTx(tx, planId)
		if err != nil {
			return err
		}
		if !plan.Enabled {
			return common.NewMessage("Subscription plan is not enabled")
		}
		if plan.PriceAmount < 0 {
			return common.NewMessage("Price cannot be negative")
		}
		if plan.AllowBalancePay != nil && !*plan.AllowBalancePay {
			return common.NewMessage("This plan does not allow balance redemption")
		}

		requiredQuota, err := calcSubscriptionBalanceQuota(plan.PriceAmount)
		if err != nil {
			return err
		}

		var user User
		if err := lockForUpdate(tx).Where("id = ?", userId).First(&user).Error; err != nil {
			return err
		}
		if requiredQuota > 0 && user.Quota < requiredQuota {
			return common.NewMessage("Insufficient balance")
		}
		// Lock the plan row and count issued subscriptions before charging; fails
		// when sold out. A renewal keeps the user's existing stock slot. While the
		// user still holds another running plan, buying this one is rejected: only
		// renewal or an administrator switch is allowed.
		renewal, err := isUserSubscriptionRenewalTx(tx, userId, plan.Id)
		if err != nil {
			return err
		}
		if !renewal {
			if err := ensureNoConflictingActiveSubscription(tx, userId, plan.Id, GetDBTimestamp()); err != nil {
				return err
			}
			if err := checkSubscriptionPlanStock(tx, plan.Id, true); err != nil {
				return err
			}
		}
		if requiredQuota > 0 {
			if err := tx.Model(&User{}).Where("id = ?", userId).
				Update("quota", gorm.Expr("quota - ?", requiredQuota)).Error; err != nil {
				return err
			}
		}

		subscription, err := CreateUserSubscriptionFromPlanTx(tx, userId, plan, PaymentMethodBalance)
		if err != nil {
			return err
		}

		now := common.GetTimestamp()
		tradeNo := fmt.Sprintf("SUBBALUSR%dNO%s%d", userId, common.GetRandomString(6), time.Now().UnixNano())
		order := &SubscriptionOrder{
			UserId:          userId,
			PlanId:          plan.Id,
			Money:           plan.PriceAmount,
			TradeNo:         tradeNo,
			PaymentMethod:   PaymentMethodBalance,
			PaymentProvider: PaymentProviderBalance,
			Status:          common.TopUpStatusSuccess,
			CreateTime:      now,
			CompleteTime:    now,
			ProviderPayload: fmt.Sprintf("charged_quota=%d", requiredQuota),
		}
		if err := tx.Create(order).Error; err != nil {
			return err
		}

		logPlanTitle = plan.Title
		logMoney = plan.PriceAmount
		chargedQuota = requiredQuota
		if subscription.PrevUserGroup != "" {
			upgradeGroup = strings.TrimSpace(subscription.UpgradeGroup)
		}
		return nil
	})
	if err != nil {
		return err
	}

	if chargedQuota > 0 {
		if err := cacheDecrUserQuota(userId, int64(chargedQuota)); err != nil {
			common.SysLog(common.LogText("failed to decrease user quota cache after subscription balance purchase: %s", err.Error()))
		}
	}
	if upgradeGroup != "" {
		refreshSubscriptionUserGroupCache(userId, common.LogText("subscription balance purchase"))
	}
	RecordLog(userId, LogTypeTopup, common.NewMessage("Subscription purchased with balance, plan: {{plan}}, payment amount: {{amount}}, quota deducted: {{quota}}", map[string]any{
		"plan":   logPlanTitle,
		"amount": fmt.Sprintf("%.2f", logMoney),
		"quota":  chargedQuota,
	}))
	return nil
}

// GetAllActiveUserSubscriptions returns all active subscriptions for a user.
func GetAllActiveUserSubscriptions(userId int) ([]SubscriptionSummary, error) {
	if userId <= 0 {
		return nil, errors.New("invalid userId")
	}
	now := common.GetTimestamp()
	var subs []UserSubscription
	err := DB.Where("user_id = ? AND status = ? AND end_time > ?", userId, "active", now).
		Order("end_time desc, id desc").
		Find(&subs).Error
	if err != nil {
		return nil, err
	}
	return buildSubscriptionSummaries(subs), nil
}

// HasActiveUserSubscription returns whether the user has any active subscription.
// This is a lightweight existence check to avoid heavy pre-consume transactions.
func HasActiveUserSubscription(userId int) (bool, error) {
	if userId <= 0 {
		return false, errors.New("invalid userId")
	}
	now := common.GetTimestamp()
	var count int64
	if err := DB.Model(&UserSubscription{}).
		Where("user_id = ? AND status = ? AND end_time > ?", userId, "active", now).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// UserActiveSubscriptionsAllowWalletOverflow returns whether wallet balance may be used
// after the user's subscription quota is exhausted. A single active subscription that
// disallows wallet overflow (allow_wallet_overflow = false) blocks the fallback.
func UserActiveSubscriptionsAllowWalletOverflow(userId int) (bool, error) {
	if userId <= 0 {
		return false, errors.New("invalid userId")
	}
	now := common.GetTimestamp()
	var strictCount int64
	if err := DB.Model(&UserSubscription{}).
		Where("user_id = ? AND status = ? AND end_time > ? AND allow_wallet_overflow = ?",
			userId, "active", now, false).
		Count(&strictCount).Error; err != nil {
		return false, err
	}
	return strictCount == 0, nil
}

// GetAllUserSubscriptions returns all subscriptions (active and expired) for a user.
func GetAllUserSubscriptions(userId int) ([]SubscriptionSummary, error) {
	if userId <= 0 {
		return nil, errors.New("invalid userId")
	}
	var subs []UserSubscription
	err := DB.Where("user_id = ?", userId).
		Order("end_time desc, id desc").
		Find(&subs).Error
	if err != nil {
		return nil, err
	}
	return buildSubscriptionSummaries(subs), nil
}

func buildSubscriptionSummaries(subs []UserSubscription) []SubscriptionSummary {
	if len(subs) == 0 {
		return []SubscriptionSummary{}
	}
	now := time.Now()
	plans := make(map[int]*SubscriptionPlan, 2)
	result := make([]SubscriptionSummary, 0, len(subs))
	for _, sub := range subs {
		subCopy := sub
		plan, ok := plans[subCopy.PlanId]
		if !ok {
			if loaded, err := GetSubscriptionPlanById(subCopy.PlanId); err == nil {
				plan = loaded
			}
			plans[subCopy.PlanId] = plan
		}
		if plan != nil {
			if usages, _ := refreshQuotaLimitUsages(&subCopy, plan, now); len(usages) > 0 {
				subCopy.QuotaLimitUsage = usages
			} else {
				subCopy.QuotaLimitUsage = nil
			}
		}
		result = append(result, SubscriptionSummary{
			Subscription: &subCopy,
		})
	}
	return result
}

// AdminInvalidateUserSubscription marks a user subscription as cancelled and ends it immediately.
func AdminInvalidateUserSubscription(userSubscriptionId int) (*common.Message, error) {
	if userSubscriptionId <= 0 {
		return nil, errors.New("invalid userSubscriptionId")
	}
	now := common.GetTimestamp()
	cacheGroup := ""
	downgradeGroup := ""
	var userId int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var sub UserSubscription
		if err := lockForUpdate(tx).
			Where("id = ?", userSubscriptionId).First(&sub).Error; err != nil {
			return err
		}
		userId = sub.UserId
		if err := tx.Model(&sub).Updates(map[string]any{
			"status":     "cancelled",
			"end_time":   now,
			"updated_at": now,
		}).Error; err != nil {
			return err
		}
		target, err := downgradeUserGroupForSubscriptionTx(tx, &sub, now)
		if err != nil {
			return err
		}
		if target != "" {
			cacheGroup = target
			downgradeGroup = target
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if cacheGroup != "" && userId > 0 {
		refreshSubscriptionUserGroupCache(userId, common.LogText("admin subscription update"))
	}
	if downgradeGroup != "" {
		return common.NewMessage("User group will be reverted to {{group}}", map[string]any{"group": downgradeGroup}), nil
	}
	return nil, nil
}

// AdminDeleteUserSubscription hard-deletes a user subscription.
func AdminDeleteUserSubscription(userSubscriptionId int) (*common.Message, error) {
	if userSubscriptionId <= 0 {
		return nil, errors.New("invalid userSubscriptionId")
	}
	now := common.GetTimestamp()
	cacheGroup := ""
	downgradeGroup := ""
	var userId int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var sub UserSubscription
		if err := lockForUpdate(tx).
			Where("id = ?", userSubscriptionId).First(&sub).Error; err != nil {
			return err
		}
		userId = sub.UserId
		target, err := downgradeUserGroupForSubscriptionTx(tx, &sub, now)
		if err != nil {
			return err
		}
		if target != "" {
			cacheGroup = target
			downgradeGroup = target
		}
		if err := tx.Where("id = ?", userSubscriptionId).Delete(&UserSubscription{}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if cacheGroup != "" && userId > 0 {
		refreshSubscriptionUserGroupCache(userId, common.LogText("admin subscription deletion"))
	}
	if downgradeGroup != "" {
		return common.NewMessage("User group will be reverted to {{group}}", map[string]any{"group": downgradeGroup}), nil
	}
	return nil, nil
}

// resetUserSubscriptionTx clears the counters selected by scope. A nil scope
// keeps the legacy behaviour of clearing AmountUsed and every configured quota
// window. Shorter windows contained in a reset window are cleared as well,
// because their usage accumulates into the longer window and would otherwise
// restore it on the next refresh.
func resetUserSubscriptionTx(tx *gorm.DB, sub *UserSubscription, plan *SubscriptionPlan, now int64, scope *SubscriptionResetScope, advanceResetTime bool) error {
	if tx == nil || sub == nil || plan == nil {
		return errors.New("invalid reset args")
	}
	if scope == nil {
		scope = &SubscriptionResetScope{ResetTotal: true}
		for _, limit := range normalizeQuotaLimitsLenient(plan.QuotaLimits) {
			scope.LimitKeys = append(scope.LimitKeys, limit.Key())
		}
	}
	if scope.ResetTotal {
		sub.AmountUsed = 0
	}
	usages, _ := refreshQuotaLimitUsages(sub, plan, time.Unix(now, 0))
	if len(usages) > 0 {
		keys := make(map[string]struct{}, len(scope.LimitKeys))
		for _, key := range scope.LimitKeys {
			keys[key] = struct{}{}
		}
		resetWindows := make(SubscriptionQuotaLimitUsages, 0, len(usages))
		for i := range usages {
			if _, ok := keys[usages[i].Key]; ok {
				usages[i].Used = 0
				resetWindows = append(resetWindows, usages[i])
			}
		}
		for i := range usages {
			if usages[i].Used == 0 {
				continue
			}
			for _, reset := range resetWindows {
				if quotaLimitUsageSpan(usages[i]) < quotaLimitUsageSpan(reset) &&
					quotaLimitUsagesOverlap(usages[i], reset) {
					usages[i].Used = 0
					break
				}
			}
		}
		sub.QuotaLimitUsage = usages
	} else {
		sub.QuotaLimitUsage = nil
	}
	if advanceResetTime {
		nextReset := calcNextResetTime(time.Unix(now, 0), plan, sub.EndTime)
		sub.NextResetTime = nextReset
		if nextReset > 0 {
			sub.LastResetTime = now
		} else {
			sub.LastResetTime = 0
		}
	}
	return tx.Save(sub).Error
}

// resetUserSubscriptionLimitKeysTx clears the usage of the selected quota
// windows of one subscription and leaves the lifetime total and every other
// window untouched. When advanceResetTime is true the next automatic reset time
// is recalculated from now, matching the admin reset behaviour.
func resetUserSubscriptionLimitKeysTx(tx *gorm.DB, sub *UserSubscription, plan *SubscriptionPlan, now int64, limitKeys []string, advanceResetTime bool) error {
	if tx == nil || sub == nil || plan == nil {
		return errors.New("invalid reset args")
	}
	if len(limitKeys) == 0 {
		return common.NewMessage("Select at least one quota limit to reset")
	}
	return resetUserSubscriptionTx(tx, sub, plan, now, &SubscriptionResetScope{LimitKeys: limitKeys}, advanceResetTime)
}

// resetUserSubscriptionsByLimitKeysTx resets the selected windows of every
// active subscription the user holds for the plan. It fails (so the caller's
// transaction rolls back, keeping the reset card unused) when no active
// subscription matches.
func resetUserSubscriptionsByLimitKeysTx(tx *gorm.DB, userId int, plan *SubscriptionPlan, now int64, limitKeys []string, advanceResetTime bool) (*SubscriptionResetResult, error) {
	if tx == nil || plan == nil {
		return nil, errors.New("invalid reset args")
	}
	if len(limitKeys) == 0 {
		return nil, common.NewMessage("Select at least one quota limit to reset")
	}
	var subs []UserSubscription
	if err := lockForUpdate(tx).
		Where("user_id = ? AND plan_id = ? AND status = ? AND end_time > ?", userId, plan.Id, "active", now).
		Order("end_time asc, id asc").
		Find(&subs).Error; err != nil {
		return nil, err
	}
	if len(subs) == 0 {
		return nil, common.NewMessage("You have no active subscription for this plan, so this reset card cannot be used")
	}
	for i := range subs {
		if err := resetUserSubscriptionLimitKeysTx(tx, &subs[i], plan, now, limitKeys, advanceResetTime); err != nil {
			return nil, err
		}
	}
	return buildSubscriptionResetResult(plan, subs, &SubscriptionResetScope{LimitKeys: limitKeys}, advanceResetTime), nil
}

func buildSubscriptionResetResult(plan *SubscriptionPlan, subs []UserSubscription, scope *SubscriptionResetScope, advanceResetTime bool) *SubscriptionResetResult {
	userIds := make([]int, 0, len(subs))
	seenUsers := make(map[int]struct{}, len(subs))
	for _, sub := range subs {
		if _, ok := seenUsers[sub.UserId]; ok {
			continue
		}
		seenUsers[sub.UserId] = struct{}{}
		userIds = append(userIds, sub.UserId)
	}
	result := &SubscriptionResetResult{
		PlanId:           plan.Id,
		MatchedCount:     len(subs),
		ResetCount:       len(subs),
		UserCount:        len(userIds),
		AdvanceResetTime: advanceResetTime,
		PlanTitle:        plan.Title,
		AffectedUserIds:  userIds,
	}
	if scope != nil {
		result.ResetTotal = scope.ResetTotal
		result.ResetLimitKeys = append([]string{}, scope.LimitKeys...)
	}
	return result
}

func adminResetUserSubscriptionsByPlanTx(tx *gorm.DB, userId int, plan *SubscriptionPlan, now int64, scope *SubscriptionResetScope, advanceResetTime bool) (*SubscriptionResetResult, error) {
	if tx == nil || plan == nil {
		return nil, errors.New("invalid reset args")
	}
	var subs []UserSubscription
	if err := lockForUpdate(tx).
		Where("user_id = ? AND plan_id = ? AND status = ? AND end_time > ?", userId, plan.Id, "active", now).
		Order("end_time asc, id asc").
		Find(&subs).Error; err != nil {
		return nil, err
	}
	if len(subs) == 0 {
		return nil, common.NewMessage("This user has no active subscription to this plan")
	}
	for i := range subs {
		if err := resetUserSubscriptionTx(tx, &subs[i], plan, now, scope, advanceResetTime); err != nil {
			return nil, err
		}
	}
	return buildSubscriptionResetResult(plan, subs, scope, advanceResetTime), nil
}

func adminResetPlanSubscriptionsTx(tx *gorm.DB, plan *SubscriptionPlan, now int64, scope *SubscriptionResetScope, advanceResetTime bool) (*SubscriptionResetResult, error) {
	if tx == nil || plan == nil {
		return nil, errors.New("invalid reset args")
	}
	var subs []UserSubscription
	if err := lockForUpdate(tx).
		Where("plan_id = ? AND status = ? AND end_time > ?", plan.Id, "active", now).
		Order("user_id asc, end_time asc, id asc").
		Find(&subs).Error; err != nil {
		return nil, err
	}
	for i := range subs {
		if err := resetUserSubscriptionTx(tx, &subs[i], plan, now, scope, advanceResetTime); err != nil {
			return nil, err
		}
	}
	return buildSubscriptionResetResult(plan, subs, scope, advanceResetTime), nil
}

// AdminResetUserSubscriptionsByPlan resets the counters selected by scope for
// every active subscription the user holds for the plan. A nil scope resets
// every counter (legacy behaviour).
func AdminResetUserSubscriptionsByPlan(userId int, planId int, scope *SubscriptionResetScope, advanceResetTime bool) (*SubscriptionResetResult, error) {
	if userId <= 0 || planId <= 0 {
		return nil, errors.New("invalid userId or planId")
	}
	var result *SubscriptionResetResult
	now := GetDBTimestamp()
	err := DB.Transaction(func(tx *gorm.DB) error {
		plan, err := getSubscriptionPlanByIdTx(tx, planId)
		if err != nil {
			return err
		}
		normalized, err := normalizeResetScope(plan, scope)
		if err != nil {
			return err
		}
		result, err = adminResetUserSubscriptionsByPlanTx(tx, userId, plan, now, normalized, advanceResetTime)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// AdminResetPlanSubscriptions resets the counters selected by scope for every
// active subscription of the plan. A nil scope resets every counter (legacy
// behaviour).
func AdminResetPlanSubscriptions(planId int, scope *SubscriptionResetScope, advanceResetTime bool) (*SubscriptionResetResult, error) {
	if planId <= 0 {
		return nil, errors.New("invalid planId")
	}
	var result *SubscriptionResetResult
	now := GetDBTimestamp()
	err := DB.Transaction(func(tx *gorm.DB) error {
		plan, err := getSubscriptionPlanByIdTx(tx, planId)
		if err != nil {
			return err
		}
		normalized, err := normalizeResetScope(plan, scope)
		if err != nil {
			return err
		}
		result, err = adminResetPlanSubscriptionsTx(tx, plan, now, normalized, advanceResetTime)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

type SubscriptionPreConsumeResult struct {
	UserSubscriptionId int
	PreConsumed        int64
	AmountTotal        int64
	AmountUsedBefore   int64
	AmountUsedAfter    int64
}

// ExpireDueSubscriptions marks expired subscriptions and handles group downgrade.
func ExpireDueSubscriptions(limit int) (int, error) {
	if limit <= 0 {
		limit = 200
	}
	now := GetDBTimestamp()
	var subs []UserSubscription
	if err := DB.Where("status = ? AND end_time > 0 AND end_time <= ?", "active", now).
		Order("end_time asc, id asc").
		Limit(limit).
		Find(&subs).Error; err != nil {
		return 0, err
	}
	if len(subs) == 0 {
		return 0, nil
	}
	expiredCount := 0
	userIds := make(map[int]struct{}, len(subs))
	for _, sub := range subs {
		if sub.UserId > 0 {
			userIds[sub.UserId] = struct{}{}
		}
	}
	for userId := range userIds {
		cacheGroup := ""
		err := DB.Transaction(func(tx *gorm.DB) error {
			res := tx.Model(&UserSubscription{}).
				Where("user_id = ? AND status = ? AND end_time > 0 AND end_time <= ?", userId, "active", now).
				Updates(map[string]any{
					"status":     "expired",
					"updated_at": common.GetTimestamp(),
				})
			if res.Error != nil {
				return res.Error
			}
			expiredCount += int(res.RowsAffected)

			// If there's an active upgraded subscription, keep current group.
			var activeSub UserSubscription
			activeQuery := tx.Where("user_id = ? AND status = ? AND end_time > ? AND upgrade_group <> ''",
				userId, "active", now).
				Order("end_time desc, id desc").
				Limit(1).
				Find(&activeSub)
			if activeQuery.Error == nil && activeQuery.RowsAffected > 0 {
				return nil
			}

			// Find the most recently expired subscription that defines a group transition
			// (an explicit downgrade target or an upgrade snapshot to revert).
			var lastExpired UserSubscription
			expiredQuery := tx.Where("user_id = ? AND status = ? AND (downgrade_group <> '' OR upgrade_group <> '')",
				userId, "expired").
				Order("end_time desc, id desc").
				Limit(1).
				Find(&lastExpired)
			if expiredQuery.Error != nil || expiredQuery.RowsAffected == 0 {
				return nil
			}
			currentGroup, err := getUserGroupByIdTx(tx, userId)
			if err != nil {
				return err
			}
			// An explicit downgrade group takes precedence; otherwise revert to the
			// group held before purchase (legacy behavior, only when the subscription
			// actually elevated the user).
			target := strings.TrimSpace(lastExpired.DowngradeGroup)
			if target == "" {
				upgradeGroup := strings.TrimSpace(lastExpired.UpgradeGroup)
				prevGroup := strings.TrimSpace(lastExpired.PrevUserGroup)
				if upgradeGroup == "" || prevGroup == "" {
					return nil
				}
				if currentGroup != upgradeGroup {
					return nil
				}
				target = prevGroup
			}
			if target == "" || target == currentGroup {
				return nil
			}
			if err := tx.Model(&User{}).Where("id = ?", userId).
				Update("group", target).Error; err != nil {
				return err
			}
			cacheGroup = target
			return nil
		})
		if err != nil {
			return expiredCount, err
		}
		if cacheGroup != "" {
			refreshSubscriptionUserGroupCache(userId, common.LogText("subscription expiration"))
		}
	}
	return expiredCount, nil
}

// SubscriptionPreConsumeRecord stores idempotent pre-consume operations per request.
type SubscriptionPreConsumeRecord struct {
	Id                 int    `json:"id"`
	RequestId          string `json:"request_id" gorm:"type:varchar(64);uniqueIndex"`
	UserId             int    `json:"user_id" gorm:"index"`
	UserSubscriptionId int    `json:"user_subscription_id" gorm:"index"`
	PreConsumed        int64  `json:"pre_consumed" gorm:"type:bigint;not null;default:0"`
	Status             string `json:"status" gorm:"type:varchar(32);index"` // consumed/refunded
	CreatedAt          int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt          int64  `json:"updated_at" gorm:"bigint;index"`
}

func (r *SubscriptionPreConsumeRecord) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	r.CreatedAt = now
	r.UpdatedAt = now
	return nil
}

func (r *SubscriptionPreConsumeRecord) BeforeUpdate(tx *gorm.DB) error {
	r.UpdatedAt = common.GetTimestamp()
	return nil
}

func maybeResetUserSubscriptionWithPlanTx(tx *gorm.DB, sub *UserSubscription, plan *SubscriptionPlan, now int64) error {
	if tx == nil || sub == nil || plan == nil {
		return errors.New("invalid reset args")
	}
	usages, usageChanged := refreshQuotaLimitUsages(sub, plan, time.Unix(now, 0))
	if usageChanged {
		sub.QuotaLimitUsage = usages
	}
	saveUsageIfChanged := func() error {
		if !usageChanged {
			return nil
		}
		return tx.Save(sub).Error
	}
	if sub.NextResetTime > 0 && sub.NextResetTime > now {
		return saveUsageIfChanged()
	}
	if NormalizeResetPeriod(plan.QuotaResetPeriod) == SubscriptionResetNever {
		return saveUsageIfChanged()
	}
	baseUnix := sub.LastResetTime
	if baseUnix <= 0 {
		baseUnix = sub.StartTime
	}
	base := time.Unix(baseUnix, 0)
	next := calcNextResetTime(base, plan, sub.EndTime)
	advanced := false
	for next > 0 && next <= now {
		advanced = true
		base = time.Unix(next, 0)
		next = calcNextResetTime(base, plan, sub.EndTime)
	}
	if !advanced {
		if sub.NextResetTime == 0 && next > 0 {
			sub.NextResetTime = next
			sub.LastResetTime = base.Unix()
			return tx.Save(sub).Error
		}
		return saveUsageIfChanged()
	}
	sub.AmountUsed = 0
	sub.LastResetTime = base.Unix()
	sub.NextResetTime = next
	return tx.Save(sub).Error
}

// PreConsumeUserSubscription pre-consumes from any active subscription total quota.
func PreConsumeUserSubscription(requestId string, userId int, modelName string, quotaType int, amount int64) (*SubscriptionPreConsumeResult, error) {
	if userId <= 0 {
		return nil, errors.New("invalid userId")
	}
	if strings.TrimSpace(requestId) == "" {
		return nil, errors.New("requestId is empty")
	}
	if amount <= 0 {
		return nil, errors.New("amount must be > 0")
	}
	now := GetDBTimestamp()

	returnValue := &SubscriptionPreConsumeResult{}

	err := DB.Transaction(func(tx *gorm.DB) error {
		var existing SubscriptionPreConsumeRecord
		query := tx.Where("request_id = ?", requestId).Limit(1).Find(&existing)
		if query.Error != nil {
			return query.Error
		}
		if query.RowsAffected > 0 {
			if existing.Status == "refunded" {
				return errors.New("subscription pre-consume already refunded")
			}
			var sub UserSubscription
			if err := tx.Where("id = ?", existing.UserSubscriptionId).First(&sub).Error; err != nil {
				return err
			}
			returnValue.UserSubscriptionId = sub.Id
			returnValue.PreConsumed = existing.PreConsumed
			returnValue.AmountTotal = sub.AmountTotal
			returnValue.AmountUsedBefore = sub.AmountUsed
			returnValue.AmountUsedAfter = sub.AmountUsed
			return nil
		}

		var subs []UserSubscription
		if err := lockForUpdate(tx).
			Where("user_id = ? AND status = ? AND end_time > ?", userId, "active", now).
			Order("end_time asc, id asc").
			Find(&subs).Error; err != nil {
			return errors.New("no active subscription")
		}
		if len(subs) == 0 {
			return errors.New("no active subscription")
		}
		lastLimitReason := ""
		for _, candidate := range subs {
			sub := candidate
			plan, err := getSubscriptionPlanByIdTx(tx, sub.PlanId)
			if err != nil {
				return err
			}
			if err := maybeResetUserSubscriptionWithPlanTx(tx, &sub, plan, now); err != nil {
				return err
			}
			limits := normalizeQuotaLimitsLenient(plan.QuotaLimits)
			if len(limits) > 0 {
				usages, _ := refreshQuotaLimitUsages(&sub, plan, time.Unix(now, 0))
				sub.QuotaLimitUsage = usages
				if blocked, limitKey := quotaLimitsInsufficient(limits, usages, amount); blocked {
					lastLimitReason = limitKey
					continue
				}
			}
			usedBefore := sub.AmountUsed
			if sub.AmountTotal > 0 {
				remain := sub.AmountTotal - usedBefore
				if remain < amount {
					continue
				}
			}
			record := &SubscriptionPreConsumeRecord{
				RequestId:          requestId,
				UserId:             userId,
				UserSubscriptionId: sub.Id,
				PreConsumed:        amount,
				Status:             "consumed",
			}
			if err := tx.Create(record).Error; err != nil {
				var dup SubscriptionPreConsumeRecord
				if err2 := tx.Where("request_id = ?", requestId).First(&dup).Error; err2 == nil {
					if dup.Status == "refunded" {
						return errors.New("subscription pre-consume already refunded")
					}
					returnValue.UserSubscriptionId = sub.Id
					returnValue.PreConsumed = dup.PreConsumed
					returnValue.AmountTotal = sub.AmountTotal
					returnValue.AmountUsedBefore = sub.AmountUsed
					returnValue.AmountUsedAfter = sub.AmountUsed
					return nil
				}
				return err
			}
			sub.AmountUsed += amount
			applyQuotaLimitUsage(sub.QuotaLimitUsage, amount)
			if err := tx.Save(&sub).Error; err != nil {
				return err
			}
			returnValue.UserSubscriptionId = sub.Id
			returnValue.PreConsumed = amount
			returnValue.AmountTotal = sub.AmountTotal
			returnValue.AmountUsedBefore = usedBefore
			returnValue.AmountUsedAfter = sub.AmountUsed
			return nil
		}
		if lastLimitReason != "" {
			return fmt.Errorf("subscription quota insufficient, need=%d (limit %s reached)", amount, lastLimitReason)
		}
		return fmt.Errorf("subscription quota insufficient, need=%d", amount)
	})
	if err != nil {
		return nil, err
	}
	return returnValue, nil
}

// RefundSubscriptionPreConsume is idempotent and refunds pre-consumed subscription quota by requestId.
func RefundSubscriptionPreConsume(requestId string) error {
	if strings.TrimSpace(requestId) == "" {
		return errors.New("requestId is empty")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var record SubscriptionPreConsumeRecord
		if err := lockForUpdate(tx).
			Where("request_id = ?", requestId).First(&record).Error; err != nil {
			return err
		}
		if record.Status == "refunded" {
			return nil
		}
		if record.PreConsumed <= 0 {
			record.Status = "refunded"
			return tx.Save(&record).Error
		}
		if err := PostConsumeUserSubscriptionDelta(record.UserSubscriptionId, -record.PreConsumed); err != nil {
			return err
		}
		record.Status = "refunded"
		return tx.Save(&record).Error
	})
}

// ResetDueSubscriptions resets subscriptions whose next_reset_time has passed.
func ResetDueSubscriptions(limit int) (int, error) {
	if limit <= 0 {
		limit = 200
	}
	now := GetDBTimestamp()
	var subs []UserSubscription
	if err := DB.Where("next_reset_time > 0 AND next_reset_time <= ? AND status = ?", now, "active").
		Order("next_reset_time asc").
		Limit(limit).
		Find(&subs).Error; err != nil {
		return 0, err
	}
	if len(subs) == 0 {
		return 0, nil
	}
	resetCount := 0
	for _, sub := range subs {
		subCopy := sub
		plan, err := getSubscriptionPlanByIdTx(nil, sub.PlanId)
		if err != nil || plan == nil {
			continue
		}
		err = DB.Transaction(func(tx *gorm.DB) error {
			var locked UserSubscription
			if err := lockForUpdate(tx).
				Where("id = ? AND next_reset_time > 0 AND next_reset_time <= ?", subCopy.Id, now).
				First(&locked).Error; err != nil {
				return nil
			}
			if err := maybeResetUserSubscriptionWithPlanTx(tx, &locked, plan, now); err != nil {
				return err
			}
			resetCount++
			return nil
		})
		if err != nil {
			return resetCount, err
		}
	}
	return resetCount, nil
}

// CleanupSubscriptionPreConsumeRecords removes old idempotency records to keep table small.
func CleanupSubscriptionPreConsumeRecords(olderThanSeconds int64) (int64, error) {
	if olderThanSeconds <= 0 {
		olderThanSeconds = 7 * 24 * 3600
	}
	cutoff := GetDBTimestamp() - olderThanSeconds
	res := DB.Where("updated_at < ?", cutoff).Delete(&SubscriptionPreConsumeRecord{})
	return res.RowsAffected, res.Error
}

// SubscriptionQuotaLimitSnapshot describes one plan limit and its usage after
// a request. It is persisted in the consumption log detail.
type SubscriptionQuotaLimitSnapshot struct {
	Key           string `json:"key"`
	Period        string `json:"period"`
	CustomSeconds int64  `json:"custom_seconds"`
	Limit         int64  `json:"limit"`
	Used          int64  `json:"used"`
	Remain        int64  `json:"remain"`
	WindowStart   int64  `json:"window_start"`
	WindowEnd     int64  `json:"window_end"`
}

// GetSubscriptionQuotaLimitSnapshots returns the independent quota windows of
// a subscription with their current usage. Returns nil when the plan has no
// quota limits, so legacy logs keep their single remaining field.
func GetSubscriptionQuotaLimitSnapshots(userSubscriptionId int) ([]SubscriptionQuotaLimitSnapshot, error) {
	if userSubscriptionId <= 0 || DB == nil {
		return nil, nil
	}
	var sub UserSubscription
	if err := DB.Where("id = ?", userSubscriptionId).First(&sub).Error; err != nil {
		return nil, err
	}
	plan, err := getSubscriptionPlanByIdTx(nil, sub.PlanId)
	if err != nil {
		return nil, err
	}
	limits := normalizeQuotaLimitsLenient(plan.QuotaLimits)
	if len(limits) == 0 {
		return nil, nil
	}
	usages, _ := refreshQuotaLimitUsages(&sub, plan, time.Now())
	result := make([]SubscriptionQuotaLimitSnapshot, 0, len(limits))
	for i, limit := range limits {
		used, windowStart, windowEnd := int64(0), int64(0), int64(0)
		if i < len(usages) {
			used = usages[i].Used
			windowStart = usages[i].WindowStart
			windowEnd = usages[i].WindowEnd
		}
		result = append(result, SubscriptionQuotaLimitSnapshot{
			Key:           limit.Key(),
			Period:        limit.Period,
			CustomSeconds: limit.CustomSeconds,
			Limit:         limit.Amount,
			Used:          used,
			Remain:        max(limit.Amount-used, 0),
			WindowStart:   windowStart,
			WindowEnd:     windowEnd,
		})
	}
	return result, nil
}

type SubscriptionPlanInfo struct {
	PlanId    int
	PlanTitle string
}

func GetSubscriptionPlanInfoByUserSubscriptionId(userSubscriptionId int) (*SubscriptionPlanInfo, error) {
	if userSubscriptionId <= 0 {
		return nil, errors.New("invalid userSubscriptionId")
	}
	cacheKey := fmt.Sprintf("sub:%d", userSubscriptionId)
	if cached, found, err := getSubscriptionPlanInfoCache().Get(cacheKey); err == nil && found {
		return &cached, nil
	}
	var sub UserSubscription
	if err := DB.Where("id = ?", userSubscriptionId).First(&sub).Error; err != nil {
		return nil, err
	}
	plan, err := getSubscriptionPlanByIdTx(nil, sub.PlanId)
	if err != nil {
		return nil, err
	}
	info := &SubscriptionPlanInfo{
		PlanId:    sub.PlanId,
		PlanTitle: plan.Title,
	}
	_ = getSubscriptionPlanInfoCache().SetWithTTL(cacheKey, *info, subscriptionPlanInfoCacheTTL())
	return info, nil
}

// Update subscription used amount by delta (positive consume more, negative refund).
func PostConsumeUserSubscriptionDelta(userSubscriptionId int, delta int64) error {
	if userSubscriptionId <= 0 {
		return errors.New("invalid userSubscriptionId")
	}
	if delta == 0 {
		return nil
	}
	now := time.Unix(common.GetTimestamp(), 0)
	return DB.Transaction(func(tx *gorm.DB) error {
		var sub UserSubscription
		if err := lockForUpdate(tx).
			Where("id = ?", userSubscriptionId).
			First(&sub).Error; err != nil {
			return err
		}
		newUsed := max(sub.AmountUsed+delta, 0)
		if sub.AmountTotal > 0 && newUsed > sub.AmountTotal {
			return fmt.Errorf("subscription used exceeds total, used=%d total=%d", newUsed, sub.AmountTotal)
		}
		sub.AmountUsed = newUsed
		if plan, err := getSubscriptionPlanByIdTx(tx, sub.PlanId); err == nil && plan != nil {
			usages, _ := refreshQuotaLimitUsages(&sub, plan, now)
			applyQuotaLimitUsage(usages, delta)
			sub.QuotaLimitUsage = usages
		}
		return tx.Save(&sub).Error
	})
}
