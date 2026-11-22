package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RedemptionResetLimitKeys stores the quota limit windows a reset card resets.
// Every key matches SubscriptionPlanQuotaLimit.Key() (for example "weekly" or
// "custom:18000").
type RedemptionResetLimitKeys []string

func (k RedemptionResetLimitKeys) Value() (driver.Value, error) {
	if len(k) == 0 {
		return "[]", nil
	}
	data, err := json.Marshal(k)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func (k *RedemptionResetLimitKeys) Scan(value any) error {
	*k = RedemptionResetLimitKeys{}
	raw, err := quotaLimitJSONBytes(value)
	if err != nil || len(raw) == 0 {
		return err
	}
	var parsed RedemptionResetLimitKeys
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return err
	}
	if parsed == nil {
		parsed = RedemptionResetLimitKeys{}
	}
	*k = parsed
	return nil
}

type Redemption struct {
	Id           int    `json:"id"`
	UserId       int    `json:"user_id"`
	Key          string `json:"key" gorm:"type:char(32);uniqueIndex"`
	Status       int    `json:"status" gorm:"default:1"`
	Name         string `json:"name" gorm:"index"`
	Quota        int    `json:"quota" gorm:"default:100"`
	CreatedTime  int64  `json:"created_time" gorm:"bigint"`
	RedeemedTime int64  `json:"redeemed_time" gorm:"bigint"`
	Count        int    `json:"count" gorm:"-:all"` // only for api request
	UsedUserId   int    `json:"used_user_id"`
	// UsedCount 是兑换码已使用的总次数。
	UsedCount int `json:"used_count" gorm:"default:0"`
	// MaxUses 是兑换码最多可被使用的总次数，0 表示不限制。
	MaxUses int `json:"max_uses" gorm:"default:0"`
	// PerUserLimit 是单个账号最多可使用的次数，0 表示不限制。
	PerUserLimit int            `json:"per_user_limit" gorm:"default:0"`
	DeletedAt    gorm.DeletedAt `gorm:"index"`
	ExpiredTime  int64          `json:"expired_time" gorm:"bigint"` // 过期时间，0 表示不过期
	// PlanId 大于 0 表示订阅兑换码：兑换时为用户开通该订阅方案，Quota 不生效。
	PlanId int `json:"plan_id" gorm:"default:0"`
	// ResetPlanId 大于 0 表示订阅重置卡：兑换时重置该套餐下有效订阅的指定限额窗口，Quota / PlanId 不生效。
	ResetPlanId int `json:"reset_plan_id" gorm:"default:0"`
	// ResetQuotaLimitKeys 是重置卡要清零的限额窗口列表，key 与 SubscriptionPlanQuotaLimit.Key() 一致。
	ResetQuotaLimitKeys RedemptionResetLimitKeys `json:"reset_quota_limit_keys" gorm:"type:text"`
	// ResetAdvanceTime 表示兑换时是否推进下一次自动重置时间（与后台订阅重置的选项一致）。
	ResetAdvanceTime bool `json:"reset_advance_time" gorm:"default:true"`
}

// IsSubscription reports whether this code grants a subscription plan instead of wallet quota.
func (redemption *Redemption) IsSubscription() bool {
	return redemption.PlanId > 0
}

// IsResetCard reports whether this code resets quota windows of an existing subscription.
func (redemption *Redemption) IsResetCard() bool {
	return redemption.ResetPlanId > 0
}

// TotalUsesExhausted reports whether the code has reached its total usage limit.
func (redemption *Redemption) TotalUsesExhausted() bool {
	return redemption.MaxUses > 0 && redemption.UsedCount >= redemption.MaxUses
}

// RedemptionUsage records how many times one account has used one redemption code.
type RedemptionUsage struct {
	Id           int   `json:"id"`
	RedemptionId int   `json:"redemption_id" gorm:"uniqueIndex:idx_redemption_usage_code_user;index"`
	UserId       int   `json:"user_id" gorm:"uniqueIndex:idx_redemption_usage_code_user;index"`
	UsedCount    int   `json:"used_count" gorm:"default:0"`
	LastUsedTime int64 `json:"last_used_time" gorm:"bigint"`
}

// usedTimesByUser returns how many times the account already used the code.
func usedTimesByUser(db *gorm.DB, redemptionId int, userId int) (int, error) {
	var usage RedemptionUsage
	err := db.Where("redemption_id = ? AND user_id = ?", redemptionId, userId).First(&usage).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return usage.UsedCount, nil
}

// RedeemResult describes what a successful redemption granted.
type RedeemResult struct {
	Quota     int    `json:"quota"`
	PlanId    int    `json:"plan_id,omitempty"`
	PlanTitle string `json:"plan_title,omitempty"`
	// Reset card result.
	ResetPlanId      int      `json:"reset_plan_id,omitempty"`
	ResetPlanTitle   string   `json:"reset_plan_title,omitempty"`
	ResetLimitKeys   []string `json:"reset_limit_keys,omitempty"`
	ResetCount       int      `json:"reset_count,omitempty"`
	AdvanceResetTime bool     `json:"advance_reset_time,omitempty"`
}

// ResetCardLimit describes one quota window a reset card will clear.
type ResetCardLimit struct {
	Key           string `json:"key"`
	Period        string `json:"period"`
	CustomSeconds int64  `json:"custom_seconds"`
	Amount        int64  `json:"amount"`
}

// ResetCardPreview describes a valid reset card and whether the user can use it.
type ResetCardPreview struct {
	Id                int              `json:"id"`
	PlanId            int              `json:"plan_id"`
	PlanTitle         string           `json:"plan_title"`
	Limits            []ResetCardLimit `json:"limits"`
	AdvanceResetTime  bool             `json:"advance_reset_time"`
	Eligible          bool             `json:"eligible"`
	SubscriptionCount int              `json:"subscription_count"`
}

// NormalizeResetCardLimitKeys validates the selected windows against the plan's
// current quota limits and returns canonical, de-duplicated keys.
func NormalizeResetCardLimitKeys(plan *SubscriptionPlan, keys []string) ([]string, error) {
	if plan == nil {
		return nil, common.NewMessage("Subscription plan not found")
	}
	limits := normalizeQuotaLimitsLenient(plan.QuotaLimits)
	if len(limits) == 0 {
		return nil, common.NewMessage("This subscription plan has no resettable quota limits")
	}
	allowed := make(map[string]SubscriptionPlanQuotaLimit, len(limits))
	for _, limit := range limits {
		allowed[limit.Key()] = limit
	}
	result := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := allowed[key]; !ok {
			return nil, common.NewMessage("Reset card contains an invalid quota limit")
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}
	if len(result) == 0 {
		return nil, common.NewMessage("Select at least one quota limit to reset")
	}
	return result, nil
}

// buildResetCardLimits resolves the stored keys to the plan's limit definitions.
func buildResetCardLimits(plan *SubscriptionPlan, keys []string) []ResetCardLimit {
	if plan == nil {
		return nil
	}
	byKey := make(map[string]SubscriptionPlanQuotaLimit, len(plan.QuotaLimits))
	for _, limit := range normalizeQuotaLimitsLenient(plan.QuotaLimits) {
		byKey[limit.Key()] = limit
	}
	limits := make([]ResetCardLimit, 0, len(keys))
	for _, key := range keys {
		limit, ok := byKey[key]
		if !ok {
			continue
		}
		limits = append(limits, ResetCardLimit{
			Key:           key,
			Period:        limit.Period,
			CustomSeconds: limit.CustomSeconds,
			Amount:        limit.Amount,
		})
	}
	return limits
}

// validateRedemptionValue checks the granted value: quota for balance codes, plan for subscription codes.
func (redemption *Redemption) validateRedemptionValue() error {
	if redemption.MaxUses < 0 || redemption.PerUserLimit < 0 {
		return common.NewMessage("Usage limits cannot be negative")
	}
	if redemption.MaxUses > 0 && redemption.MaxUses < redemption.UsedCount {
		return common.NewMessage("Total usage limit cannot be lower than the used count")
	}
	if redemption.IsResetCard() {
		if redemption.IsSubscription() {
			return common.NewMessage("A reset card cannot also grant a subscription plan")
		}
		plan, err := GetSubscriptionPlanById(redemption.ResetPlanId)
		if err != nil {
			return common.NewMessage("Subscription plan not found")
		}
		keys, err := NormalizeResetCardLimitKeys(plan, redemption.ResetQuotaLimitKeys)
		if err != nil {
			return err
		}
		redemption.ResetQuotaLimitKeys = keys
		return nil
	}
	if redemption.IsSubscription() {
		if _, err := GetSubscriptionPlanById(redemption.PlanId); err != nil {
			return common.NewMessage("Subscription plan not found")
		}
		return nil
	}
	if redemption.Quota <= 0 {
		return errors.New("redemption quota must be positive")
	}
	return common.ValidateWalletQuota(redemption.Quota)
}

func GetAllRedemptions(startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
	// 开始事务
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 获取总数
	err = tx.Model(&Redemption{}).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 获取分页数据
	err = tx.Order("id desc").Limit(num).Offset(startIdx).Find(&redemptions).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 提交事务
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return redemptions, total, nil
}

func SearchRedemptions(keyword string, status string, startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&Redemption{})

	if keyword != "" {
		if id, err := strconv.Atoi(keyword); err == nil {
			query = query.Where("id = ? OR name LIKE ?", id, keyword+"%")
		} else {
			query = query.Where("name LIKE ?", keyword+"%")
		}
	}

	if status != "" {
		now := common.GetTimestamp()
		switch status {
		case "expired":
			query = query.Where(
				"status = ? AND expired_time != 0 AND expired_time < ?",
				common.RedemptionCodeStatusEnabled,
				now,
			)
		case strconv.Itoa(common.RedemptionCodeStatusEnabled):
			query = query.Where(
				"status = ? AND (expired_time = 0 OR expired_time >= ?)",
				common.RedemptionCodeStatusEnabled,
				now,
			)
		case strconv.Itoa(common.RedemptionCodeStatusDisabled):
			query = query.Where("status = ?", common.RedemptionCodeStatusDisabled)
		case strconv.Itoa(common.RedemptionCodeStatusUsed):
			query = query.Where("status = ?", common.RedemptionCodeStatusUsed)
		}
	}

	// Get total count
	err = query.Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated data
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&redemptions).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return redemptions, total, nil
}

func GetRedemptionById(id int) (*Redemption, error) {
	if id == 0 {
		return nil, common.NewMessage("ID is empty")
	}
	redemption := Redemption{Id: id}
	var err error = nil
	err = DB.First(&redemption, "id = ?", id).Error
	return &redemption, err
}

func Redeem(key string, userId int) (quota int, err error) {
	result, err := RedeemCode(key, userId)
	if err != nil {
		return 0, err
	}
	return result.Quota, nil
}

// RedeemCode redeems a balance or subscription code and reports what was granted.
func RedeemCode(key string, userId int) (*RedeemResult, error) {
	if key == "" {
		return nil, errors.New("redemption code not provided")
	}
	if userId == 0 {
		return nil, errors.New("invalid user id")
	}
	redemption := &Redemption{}
	var plan *SubscriptionPlan
	var subscriptionErr *common.Message
	var resetPlan *SubscriptionPlan
	var resetResult *SubscriptionResetResult
	var appliedResetKeys []string
	groupChanged := false

	keyCol := "`key`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		keyCol = `"key"`
	}
	common.RandomSleep()
	err := DB.Transaction(func(tx *gorm.DB) error {
		err := lockForUpdate(tx).Where(keyCol+" = ?", key).First(redemption).Error
		if err != nil {
			return errors.New("invalid redemption code")
		}
		if redemption.Status != common.RedemptionCodeStatusEnabled {
			return errors.New("redemption code already used")
		}
		now := common.GetTimestamp()
		if redemption.ExpiredTime != 0 && redemption.ExpiredTime < now {
			return errors.New("redemption code expired")
		}
		if redemption.TotalUsesExhausted() {
			return errors.New("redemption code already used")
		}
		if redemption.PerUserLimit > 0 {
			used, err := usedTimesByUser(tx, redemption.Id, userId)
			if err != nil {
				return err
			}
			if used >= redemption.PerUserLimit {
				return errors.New("redemption code usage limit reached for this account")
			}
		}
		// Compare-and-swap on used_count: only the transaction that can still
		// consume one use may grant the code's value, so a concurrent redeem of
		// the same code loses here even without a row lock (e.g. on SQLite).
		result := tx.Model(&Redemption{}).
			Where("id = ? AND status = ?", redemption.Id, common.RedemptionCodeStatusEnabled).
			Where("max_uses = 0 OR used_count < max_uses").
			Updates(map[string]any{
				"used_count":    gorm.Expr("used_count + 1"),
				"redeemed_time": now,
				"used_user_id":  userId,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("redemption code already used")
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "redemption_id"}, {Name: "user_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"used_count":     gorm.Expr("used_count + 1"),
				"last_used_time": now,
			}),
		}).Create(&RedemptionUsage{
			RedemptionId: redemption.Id,
			UserId:       userId,
			UsedCount:    1,
			LastUsedTime: now,
		}).Error; err != nil {
			return err
		}
		redemption.UsedCount++
		if redemption.MaxUses > 0 && redemption.UsedCount >= redemption.MaxUses {
			if err := tx.Model(&Redemption{}).Where("id = ?", redemption.Id).
				Update("status", common.RedemptionCodeStatusUsed).Error; err != nil {
				return err
			}
			redemption.Status = common.RedemptionCodeStatusUsed
		}
		if redemption.IsResetCard() {
			resetPlan, err = getSubscriptionPlanByIdTx(tx, redemption.ResetPlanId)
			if err != nil {
				return err
			}
			keys, err := NormalizeResetCardLimitKeys(resetPlan, redemption.ResetQuotaLimitKeys)
			if err != nil {
				var msg *common.Message
				if errors.As(err, &msg) {
					subscriptionErr = msg
				}
				return err
			}
			appliedResetKeys = keys
			resetResult, err = resetUserSubscriptionsByLimitKeysTx(tx, userId, resetPlan, common.GetTimestamp(), keys, redemption.ResetAdvanceTime)
			if err != nil {
				var msg *common.Message
				if errors.As(err, &msg) {
					subscriptionErr = msg
				}
				return err
			}
			return nil
		}
		if !redemption.IsSubscription() {
			return creditTopUpQuota(tx, userId, redemption.Quota, nil)
		}
		// Subscription code: same order as AdminBindSubscription (lock user, then plan stock).
		// The code was flipped to "used" above, so its own reserved slot is already released
		// from the count and the stock check below effectively consumes that slot.
		var userRow User
		if err := lockForUpdate(tx).Select("id").Where("id = ?", userId).First(&userRow).Error; err != nil {
			return err
		}
		plan, err = getSubscriptionPlanByIdTx(tx, redemption.PlanId)
		if err != nil {
			return err
		}
		if err := checkSubscriptionPlanStockFor(tx, plan.Id, true, 1, redemption.Id); err != nil {
			var msg *common.Message
			if errors.As(err, &msg) {
				subscriptionErr = msg
			}
			return err
		}
		subscription, err := CreateUserSubscriptionFromPlanTx(tx, userId, plan, "redemption")
		if err != nil {
			var msg *common.Message
			if errors.As(err, &msg) {
				subscriptionErr = msg
			}
			return err
		}
		groupChanged = subscription.PrevUserGroup != ""
		return nil
	})
	if err != nil {
		common.SysError(common.LogText("redemption failed: %s", err.Error()))
		// Plan-level rejections (sold out, purchase limit) are shown to the user;
		// everything else stays generic so the code state is not leaked.
		if subscriptionErr != nil {
			return nil, subscriptionErr
		}
		return nil, ErrRedeemFailed
	}
	if redemption.IsResetCard() {
		if resetPlan == nil || resetResult == nil {
			return nil, ErrRedeemFailed
		}
		RecordLog(userId, LogTypeTopup, common.NewMessage("Reset {{count}} subscription(s) of plan {{plan}} with a reset card, code ID {{id}}", map[string]any{"count": resetResult.ResetCount, "plan": resetPlan.Title, "id": redemption.Id}))
		return &RedeemResult{
			ResetPlanId:      resetPlan.Id,
			ResetPlanTitle:   resetPlan.Title,
			ResetLimitKeys:   appliedResetKeys,
			ResetCount:       resetResult.ResetCount,
			AdvanceResetTime: redemption.ResetAdvanceTime,
		}, nil
	}
	if redemption.IsSubscription() {
		if groupChanged {
			refreshSubscriptionUserGroupCache(userId, common.LogText("subscription redemption"))
		}
		RecordLog(userId, LogTypeTopup, common.NewMessage("Redeemed subscription plan {{plan}} with a redemption code, code ID {{id}}", map[string]any{"plan": plan.Title, "id": redemption.Id}))
		return &RedeemResult{PlanId: plan.Id, PlanTitle: plan.Title}, nil
	}
	syncCreditUserQuotaCache(userId, redemption.Quota, common.LogText("redemption"))
	RecordLog(userId, LogTypeTopup, common.NewMessage("Topped up {{quota}} with a redemption code, code ID {{id}}", map[string]any{"quota": logger.FormatQuota(redemption.Quota), "id": redemption.Id}))
	return &RedeemResult{Quota: redemption.Quota}, nil
}

// PreviewResetCard validates a reset card without consuming it, so the UI can
// show a confirmation dialog before the reset takes effect. A valid card is
// returned with Eligible=false when the user has no active subscription of the
// target plan; such a card must stay usable.
func PreviewResetCard(key string, userId int) (*ResetCardPreview, error) {
	if strings.TrimSpace(key) == "" {
		return nil, common.NewMessage("Please enter a redemption code")
	}
	if userId <= 0 {
		return nil, errors.New("invalid user id")
	}
	keyCol := "`key`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		keyCol = `"key"`
	}
	redemption := &Redemption{}
	if err := DB.Where(keyCol+" = ?", key).First(redemption).Error; err != nil {
		return nil, common.NewMessage("Invalid redemption code")
	}
	if redemption.Status != common.RedemptionCodeStatusEnabled {
		return nil, common.NewMessage("Redemption code already used")
	}
	if redemption.ExpiredTime != 0 && redemption.ExpiredTime < common.GetTimestamp() {
		return nil, common.NewMessage("Redemption code expired")
	}
	if redemption.TotalUsesExhausted() {
		return nil, common.NewMessage("Redemption code already used")
	}
	if redemption.PerUserLimit > 0 {
		used, err := usedTimesByUser(DB, redemption.Id, userId)
		if err != nil {
			return nil, err
		}
		if used >= redemption.PerUserLimit {
			return nil, common.NewMessage("Redemption code usage limit reached for this account")
		}
	}
	if !redemption.IsResetCard() {
		return nil, common.NewMessage("This is not a subscription reset card")
	}
	plan, err := GetSubscriptionPlanById(redemption.ResetPlanId)
	if err != nil {
		return nil, common.NewMessage("Subscription plan not found")
	}
	keys, err := NormalizeResetCardLimitKeys(plan, redemption.ResetQuotaLimitKeys)
	if err != nil {
		return nil, err
	}
	var count int64
	if err := DB.Model(&UserSubscription{}).
		Where("user_id = ? AND plan_id = ? AND status = ? AND end_time > ?", userId, plan.Id, "active", common.GetTimestamp()).
		Count(&count).Error; err != nil {
		return nil, err
	}
	return &ResetCardPreview{
		Id:                redemption.Id,
		PlanId:            plan.Id,
		PlanTitle:         plan.Title,
		Limits:            buildResetCardLimits(plan, keys),
		AdvanceResetTime:  redemption.ResetAdvanceTime,
		Eligible:          count > 0,
		SubscriptionCount: int(count),
	}, nil
}

func (redemption *Redemption) Insert() error {
	if err := redemption.validateRedemptionValue(); err != nil {
		return err
	}
	var err error
	err = DB.Create(redemption).Error
	return err
}

// InsertSubscriptionRedemptions creates subscription codes for one plan atomically.
// Every unredeemed code reserves one slot of the plan stock, so the whole batch is
// checked against the remaining stock under the plan row lock.
func InsertSubscriptionRedemptions(redemptions []*Redemption) error {
	if len(redemptions) == 0 {
		return nil
	}
	planId := redemptions[0].PlanId
	for _, r := range redemptions {
		if r.PlanId <= 0 || r.PlanId != planId {
			return errors.New("subscription redemptions must share one plan")
		}
	}
	if err := redemptions[0].validateRedemptionValue(); err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		need := 0
		for _, r := range redemptions {
			need += redemptionReservedSlotCount(r)
		}
		if need > 0 {
			if err := checkSubscriptionPlanStockFor(tx, planId, true, need, 0); err != nil {
				return err
			}
		}
		return tx.Create(&redemptions).Error
	})
}

// InsertRedemptions stores a batch of codes that do not reserve plan stock
// (balance codes and subscription reset cards).
func InsertRedemptions(redemptions []*Redemption) error {
	if len(redemptions) == 0 {
		return nil
	}
	if err := redemptions[0].validateRedemptionValue(); err != nil {
		return err
	}
	return DB.Create(&redemptions).Error
}

func (redemption *Redemption) SelectUpdate() error {
	// This can update zero values
	return DB.Model(redemption).Select("redeemed_time", "status").Updates(redemption).Error
}

// Update Make sure your token's fields is completed, because this will update non-zero values
func (redemption *Redemption) Update() error {
	if err := redemption.validateRedemptionValue(); err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		// A subscription code that newly starts holding stock (plan changed, expiry
		// extended, usage limit raised, ...) must fit into the remaining stock of its plan.
		if need := redemptionReservedSlotCount(redemption); need > 0 {
			if err := checkSubscriptionPlanStockFor(tx, redemption.PlanId, true, need, redemption.Id); err != nil {
				return err
			}
		}
		return tx.Model(redemption).Select("name", "status", "quota", "max_uses", "per_user_limit", "plan_id", "reset_plan_id", "reset_quota_limit_keys", "reset_advance_time", "redeemed_time", "expired_time").Updates(redemption).Error
	})
}

func (redemption *Redemption) Delete() error {
	var err error
	err = DB.Delete(redemption).Error
	return err
}

func DeleteRedemptionById(id int) (err error) {
	if id == 0 {
		return common.NewMessage("ID is empty")
	}
	redemption := Redemption{Id: id}
	err = DB.Where(redemption).First(&redemption).Error
	if err != nil {
		return err
	}
	return redemption.Delete()
}

func DeleteInvalidRedemptions() (int64, error) {
	now := common.GetTimestamp()
	result := DB.Where("status IN ? OR (status = ? AND expired_time != 0 AND expired_time < ?)", []int{common.RedemptionCodeStatusUsed, common.RedemptionCodeStatusDisabled}, common.RedemptionCodeStatusEnabled, now).Delete(&Redemption{})
	return result.RowsAffected, result.Error
}

// BatchDeleteRedemptions soft-deletes the selected codes in one statement.
func BatchDeleteRedemptions(ids []int) (int64, error) {
	if len(ids) == 0 || len(ids) > 1000 {
		return 0, errors.New("select between 1 and 1000 redemption codes")
	}
	for _, id := range ids {
		if id <= 0 {
			return 0, errors.New("redemption IDs must be positive")
		}
	}
	result := DB.Where("id IN ?", ids).Delete(&Redemption{})
	return result.RowsAffected, result.Error
}
