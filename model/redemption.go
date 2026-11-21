package model

import (
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"gorm.io/gorm"
)

type Redemption struct {
	Id           int            `json:"id"`
	UserId       int            `json:"user_id"`
	Key          string         `json:"key" gorm:"type:char(32);uniqueIndex"`
	Status       int            `json:"status" gorm:"default:1"`
	Name         string         `json:"name" gorm:"index"`
	Quota        int            `json:"quota" gorm:"default:100"`
	CreatedTime  int64          `json:"created_time" gorm:"bigint"`
	RedeemedTime int64          `json:"redeemed_time" gorm:"bigint"`
	Count        int            `json:"count" gorm:"-:all"` // only for api request
	UsedUserId   int            `json:"used_user_id"`
	DeletedAt    gorm.DeletedAt `gorm:"index"`
	ExpiredTime  int64          `json:"expired_time" gorm:"bigint"` // 过期时间，0 表示不过期
	// PlanId 大于 0 表示订阅兑换码：兑换时为用户开通该订阅方案，Quota 不生效。
	PlanId int `json:"plan_id" gorm:"default:0"`
}

// IsSubscription reports whether this code grants a subscription plan instead of wallet quota.
func (redemption *Redemption) IsSubscription() bool {
	return redemption.PlanId > 0
}

// RedeemResult describes what a successful redemption granted.
type RedeemResult struct {
	Quota     int    `json:"quota"`
	PlanId    int    `json:"plan_id,omitempty"`
	PlanTitle string `json:"plan_title,omitempty"`
}

// validateRedemptionValue checks the granted value: quota for balance codes, plan for subscription codes.
func (redemption *Redemption) validateRedemptionValue() error {
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
		if redemption.ExpiredTime != 0 && redemption.ExpiredTime < common.GetTimestamp() {
			return errors.New("redemption code expired")
		}
		// Compare-and-swap on status: only the transaction that flips
		// enabled -> used may credit quota, so a concurrent redeem of the
		// same code loses here even without a row lock (e.g. on SQLite).
		result := tx.Model(&Redemption{}).
			Where("id = ? AND status = ?", redemption.Id, common.RedemptionCodeStatusEnabled).
			Updates(map[string]any{
				"redeemed_time": common.GetTimestamp(),
				"status":        common.RedemptionCodeStatusUsed,
				"used_user_id":  userId,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("redemption code already used")
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
		if err := checkSubscriptionPlanStock(tx, plan.Id, true); err != nil {
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
			if redemptionReservesStock(r) {
				need++
			}
		}
		if need > 0 {
			if err := checkSubscriptionPlanStockFor(tx, planId, true, need, 0); err != nil {
				return err
			}
		}
		return tx.Create(&redemptions).Error
	})
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
		// extended, ...) must fit into the remaining stock of its plan.
		if redemptionReservesStock(redemption) {
			var old Redemption
			if err := tx.Where("id = ?", redemption.Id).First(&old).Error; err != nil {
				return err
			}
			alreadyReserved := old.PlanId == redemption.PlanId && redemptionReservesStock(&old)
			if !alreadyReserved {
				if err := checkSubscriptionPlanStockFor(tx, redemption.PlanId, true, 1, redemption.Id); err != nil {
					return err
				}
			}
		}
		return tx.Model(redemption).Select("name", "status", "quota", "plan_id", "redeemed_time", "expired_time").Updates(redemption).Error
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
