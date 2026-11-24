package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Coupon scopes: which purchases a coupon applies to.
const (
	CouponScopeTopUp        = "topup"
	CouponScopeSubscription = "subscription"
	CouponScopeAll          = "all"
)

// Coupon recipient selection recorded on the batch.
const (
	CouponTargetAll    = "all"
	CouponTargetCustom = "custom"
)

const (
	CouponStatusActive  = 1
	CouponStatusRevoked = 2
)

// User coupon states. "expired" is never stored: an available coupon whose
// expires_at has passed is reported as expired.
const (
	UserCouponStatusAvailable = "available"
	UserCouponStatusLocked    = "locked"
	UserCouponStatusUsed      = "used"
	UserCouponStatusRevoked   = "revoked"
	UserCouponStatusExpired   = "expired"
)

const (
	MaxCouponsPerPayment     = 10
	MaxCouponAmount          = 1000000
	MaxCouponTargetUserIds   = 5000
	MaxCouponTargetGroups    = 50
	couponIssueBatchSize     = 500
	couponRecipientPageLimit = 100
)

var (
	ErrCouponUnavailable       error = common.NewMessage("Coupon not found or unavailable")
	ErrCouponExpired           error = common.NewMessage("Coupon has expired")
	ErrCouponScopeMismatch     error = common.NewMessage("Coupon cannot be used for this purchase")
	ErrCouponThresholdNotMet   error = common.NewMessage("Order amount does not meet the coupon threshold")
	ErrCouponNotStackable      error = common.NewMessage("Non-stackable coupons cannot be combined with other coupons")
	ErrCouponTooMany           error = common.NewMessage("Too many coupons selected")
	ErrCouponNoRecipients      error = common.NewMessage("No users matched the selected recipients")
	ErrCouponAlreadyRevoked    error = common.NewMessage("Coupon has already been revoked")
	ErrCouponPaymentAmountZero error = common.NewMessage("Payment amount after coupons is too low")
)

// CouponIntList is a JSON array of user ids stored in a text column.
type CouponIntList []int

func (l CouponIntList) Value() (driver.Value, error) {
	if len(l) == 0 {
		return "[]", nil
	}
	data, err := json.Marshal(l)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func (l *CouponIntList) Scan(value any) error {
	*l = CouponIntList{}
	raw, err := quotaLimitJSONBytes(value)
	if err != nil || len(raw) == 0 {
		return err
	}
	return json.Unmarshal(raw, l)
}

// CouponStringList is a JSON array of group names stored in a text column.
type CouponStringList []string

func (l CouponStringList) Value() (driver.Value, error) {
	if len(l) == 0 {
		return "[]", nil
	}
	data, err := json.Marshal(l)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func (l *CouponStringList) Scan(value any) error {
	*l = CouponStringList{}
	raw, err := quotaLimitJSONBytes(value)
	if err != nil || len(raw) == 0 {
		return err
	}
	return json.Unmarshal(raw, l)
}

// Coupon is one issued batch. Its rule is immutable after issue; each
// recipient holds a UserCoupon row that is used at most once.
type Coupon struct {
	Id            int              `json:"id"`
	Name          string           `json:"name" gorm:"type:varchar(64);not null;index"`
	Scope         string           `json:"scope" gorm:"type:varchar(16);not null"`
	Threshold     float64          `json:"threshold" gorm:"type:decimal(12,2);not null;default:0"`
	Discount      float64          `json:"discount" gorm:"type:decimal(12,2);not null;default:0"`
	Stackable     bool             `json:"stackable"`
	ExpiresAt     int64            `json:"expires_at" gorm:"type:bigint;not null;default:0"`
	TargetType    string           `json:"target_type" gorm:"type:varchar(16);not null"`
	TargetUserIds CouponIntList    `json:"target_user_ids" gorm:"type:text"`
	TargetGroups  CouponStringList `json:"target_groups" gorm:"type:text"`
	Status        int              `json:"status" gorm:"type:int;not null;default:1;index"`
	IssuedCount   int              `json:"issued_count" gorm:"type:int;not null;default:0"`
	UsedCount     int64            `json:"used_count" gorm:"-"`
	CreatedBy     int              `json:"created_by" gorm:"type:int;not null;default:0"`
	CreatedAt     int64            `json:"created_at" gorm:"type:bigint"`
	UpdatedAt     int64            `json:"updated_at" gorm:"type:bigint"`
}

func (c *Coupon) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	c.CreatedAt = now
	c.UpdatedAt = now
	return nil
}

func (c *Coupon) BeforeUpdate(tx *gorm.DB) error {
	c.UpdatedAt = common.GetTimestamp()
	return nil
}

// UserCoupon is one coupon held by one user. TradeNo is the order that holds
// it while locked and the order that used it once used.
type UserCoupon struct {
	Id        int    `json:"id"`
	CouponId  int    `json:"coupon_id" gorm:"not null;index"`
	UserId    int    `json:"user_id" gorm:"not null;index:idx_user_coupon_user_status,priority:1"`
	Status    string `json:"status" gorm:"type:varchar(16);not null;index:idx_user_coupon_user_status,priority:2"`
	ExpiresAt int64  `json:"expires_at" gorm:"type:bigint;not null;default:0"`
	TradeNo   string `json:"trade_no" gorm:"type:varchar(255);not null;default:'';index"`
	LockedAt  int64  `json:"locked_at" gorm:"type:bigint;not null;default:0;index"`
	UsedAt    int64  `json:"used_at" gorm:"type:bigint;not null;default:0"`
	CreatedAt int64  `json:"created_at" gorm:"type:bigint"`
}

func IsValidCouponScope(scope string) bool {
	return scope == CouponScopeTopUp || scope == CouponScopeSubscription || scope == CouponScopeAll
}

func couponScopeAllows(couponScope string, usage string) bool {
	return couponScope == CouponScopeAll || couponScope == usage
}

// NormalizeCouponAmount rounds a coupon amount to cents.
func NormalizeCouponAmount(value float64) float64 {
	return decimal.NewFromFloat(value).Round(2).InexactFloat64()
}

// displayUserCouponStatus folds expiry into the stored status.
func displayUserCouponStatus(status string, expiresAt int64, now int64) string {
	if status == UserCouponStatusAvailable && expiresAt > 0 && expiresAt <= now {
		return UserCouponStatusExpired
	}
	return status
}

func normalizeCouponIds(ids []int) ([]int, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	seen := make(map[int]bool, len(ids))
	normalized := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, ErrCouponUnavailable
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		normalized = append(normalized, id)
	}
	if len(normalized) > MaxCouponsPerPayment {
		return nil, ErrCouponTooMany
	}
	return normalized, nil
}

// CouponApplication is the price outcome of applying coupons to one payment.
type CouponApplication struct {
	UserCouponIds  []int   `json:"user_coupon_ids"`
	OriginalAmount float64 `json:"original_amount"`
	Discount       float64 `json:"discount"`
	FinalAmount    float64 `json:"final_amount"`
}

// applyUserCouponsTx validates the selected coupons of userId against one
// payment of amount (the settlement amount) for usage, and moves them to
// targetStatus (locked for a pending order, used for an instant purchase)
// bound to tradeNo. The conditional update makes two concurrent payments
// unable to take the same coupon even without row locks (SQLite).
func applyUserCouponsTx(tx *gorm.DB, userId int, couponIds []int, usage string, amount float64, tradeNo string, targetStatus string) (*CouponApplication, error) {
	result := &CouponApplication{OriginalAmount: amount, FinalAmount: amount}
	ids, err := normalizeCouponIds(couponIds)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return result, nil
	}
	if tradeNo == "" {
		return nil, errors.New("coupon trade number is empty")
	}

	var userCoupons []UserCoupon
	if err := lockForUpdate(tx).Where("id IN ? AND user_id = ?", ids, userId).Find(&userCoupons).Error; err != nil {
		return nil, err
	}
	if len(userCoupons) != len(ids) {
		return nil, ErrCouponUnavailable
	}
	couponIdSet := make([]int, 0, len(userCoupons))
	for _, userCoupon := range userCoupons {
		if !slices.Contains(couponIdSet, userCoupon.CouponId) {
			couponIdSet = append(couponIdSet, userCoupon.CouponId)
		}
	}
	var coupons []Coupon
	if err := tx.Where("id IN ?", couponIdSet).Find(&coupons).Error; err != nil {
		return nil, err
	}
	couponById := make(map[int]Coupon, len(coupons))
	for _, coupon := range coupons {
		couponById[coupon.Id] = coupon
	}

	now := common.GetTimestamp()
	dAmount := decimal.NewFromFloat(amount)
	totalDiscount := decimal.Zero
	for _, userCoupon := range userCoupons {
		coupon, ok := couponById[userCoupon.CouponId]
		if !ok || coupon.Status != CouponStatusActive || userCoupon.Status != UserCouponStatusAvailable {
			return nil, ErrCouponUnavailable
		}
		if userCoupon.ExpiresAt > 0 && userCoupon.ExpiresAt <= now {
			return nil, ErrCouponExpired
		}
		if !couponScopeAllows(coupon.Scope, usage) {
			return nil, ErrCouponScopeMismatch
		}
		if len(userCoupons) > 1 && !coupon.Stackable {
			return nil, ErrCouponNotStackable
		}
		if dAmount.LessThan(decimal.NewFromFloat(coupon.Threshold)) {
			return nil, ErrCouponThresholdNotMet
		}
		totalDiscount = totalDiscount.Add(decimal.NewFromFloat(coupon.Discount))
	}

	final := dAmount.Sub(totalDiscount)
	if final.IsNegative() {
		final = decimal.Zero
	}
	final = final.Round(2)

	updates := map[string]any{"status": targetStatus, "trade_no": tradeNo}
	if targetStatus == UserCouponStatusUsed {
		updates["used_at"] = now
	} else {
		updates["locked_at"] = now
	}
	update := tx.Model(&UserCoupon{}).
		Where("id IN ? AND user_id = ? AND status = ?", ids, userId, UserCouponStatusAvailable).
		Updates(updates)
	if update.Error != nil {
		return nil, update.Error
	}
	if update.RowsAffected != int64(len(ids)) {
		return nil, ErrCouponUnavailable
	}

	result.UserCouponIds = ids
	result.Discount = dAmount.Sub(final).Round(2).InexactFloat64()
	result.FinalAmount = final.InexactFloat64()
	return result, nil
}

// LockUserCoupons reserves the selected coupons for a pending order. Without
// coupons it returns the amount unchanged and touches no rows.
func LockUserCoupons(userId int, couponIds []int, usage string, amount float64, tradeNo string) (*CouponApplication, error) {
	if len(couponIds) == 0 {
		return &CouponApplication{OriginalAmount: amount, FinalAmount: amount}, nil
	}
	var result *CouponApplication
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = applyUserCouponsTx(tx, userId, couponIds, usage, amount, tradeNo, UserCouponStatusLocked)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// consumeUserCouponsTx marks the coupons of a completed order as used. A
// coupon whose stale lock was released but not taken by another order still
// carries this trade_no and is consumed too.
func consumeUserCouponsTx(tx *gorm.DB, tradeNo string) error {
	if tradeNo == "" {
		return nil
	}
	return tx.Model(&UserCoupon{}).
		Where("trade_no = ? AND status IN ?", tradeNo, []string{UserCouponStatusLocked, UserCouponStatusAvailable}).
		Updates(map[string]any{"status": UserCouponStatusUsed, "used_at": common.GetTimestamp(), "locked_at": 0}).Error
}

// releaseUserCouponsTx returns the coupons locked by a failed or expired order.
// Coupons of a batch revoked meanwhile become revoked instead of available.
func releaseUserCouponsTx(tx *gorm.DB, tradeNo string) error {
	if tradeNo == "" {
		return nil
	}
	revokedCoupons := tx.Model(&Coupon{}).Select("id").Where("status = ?", CouponStatusRevoked)
	if err := tx.Model(&UserCoupon{}).
		Where("trade_no = ? AND status = ? AND coupon_id IN (?)", tradeNo, UserCouponStatusLocked, revokedCoupons).
		Updates(map[string]any{"status": UserCouponStatusRevoked, "trade_no": "", "locked_at": 0}).Error; err != nil {
		return err
	}
	return tx.Model(&UserCoupon{}).
		Where("trade_no = ? AND status = ?", tradeNo, UserCouponStatusLocked).
		Updates(map[string]any{"status": UserCouponStatusAvailable, "trade_no": "", "locked_at": 0}).Error
}

// ReleaseUserCoupons returns the coupons of an order that never reached the
// payment gateway or was rejected by it.
func ReleaseUserCoupons(tradeNo string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		return releaseUserCouponsTx(tx, tradeNo)
	})
}

// ReleaseStaleCouponLocks returns coupons held longer than olderThanSeconds
// by orders that never completed (an abandoned checkout sends no callback).
// trade_no is kept, so a payment that still completes later marks the coupons
// used as long as no other order has taken them meanwhile.
func ReleaseStaleCouponLocks(olderThanSeconds int64) (int64, error) {
	cutoff := common.GetTimestamp() - olderThanSeconds
	var released int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		revokedCoupons := tx.Model(&Coupon{}).Select("id").Where("status = ?", CouponStatusRevoked)
		if err := tx.Model(&UserCoupon{}).
			Where("status = ? AND locked_at > 0 AND locked_at < ? AND coupon_id IN (?)", UserCouponStatusLocked, cutoff, revokedCoupons).
			Updates(map[string]any{"status": UserCouponStatusRevoked, "locked_at": 0}).Error; err != nil {
			return err
		}
		result := tx.Model(&UserCoupon{}).
			Where("status = ? AND locked_at > 0 AND locked_at < ?", UserCouponStatusLocked, cutoff).
			Updates(map[string]any{"status": UserCouponStatusAvailable, "locked_at": 0})
		released = result.RowsAffected
		return result.Error
	})
	return released, err
}

// IssueCouponParams selects the recipients of a new coupon batch.
type IssueCouponParams struct {
	AllUsers bool
	UserIds  []int
	Groups   []string
}

// IssueCoupon creates the batch and one UserCoupon per recipient. "All users"
// means the enabled users that exist now; later sign-ups get nothing.
func IssueCoupon(coupon *Coupon, params IssueCouponParams) (int, error) {
	if coupon == nil {
		return 0, errors.New("coupon is nil")
	}
	issued := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		recipients, err := resolveCouponRecipientsTx(tx, params)
		if err != nil {
			return err
		}
		if len(recipients) == 0 {
			return ErrCouponNoRecipients
		}
		coupon.Status = CouponStatusActive
		coupon.IssuedCount = len(recipients)
		if err := tx.Create(coupon).Error; err != nil {
			return err
		}
		now := common.GetTimestamp()
		rows := make([]UserCoupon, 0, len(recipients))
		for _, userId := range recipients {
			rows = append(rows, UserCoupon{
				CouponId:  coupon.Id,
				UserId:    userId,
				Status:    UserCouponStatusAvailable,
				ExpiresAt: coupon.ExpiresAt,
				CreatedAt: now,
			})
		}
		if err := tx.CreateInBatches(rows, couponIssueBatchSize).Error; err != nil {
			return err
		}
		issued = len(recipients)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return issued, nil
}

func resolveCouponRecipientsTx(tx *gorm.DB, params IssueCouponParams) ([]int, error) {
	var ids []int
	if params.AllUsers {
		if err := tx.Model(&User{}).Where("status = ?", common.UserStatusEnabled).Order("id").Pluck("id", &ids).Error; err != nil {
			return nil, err
		}
		return ids, nil
	}

	seen := map[int]bool{}
	if len(params.UserIds) > 0 {
		var existing []int
		if err := tx.Model(&User{}).Where("id IN ?", params.UserIds).Pluck("id", &existing).Error; err != nil {
			return nil, err
		}
		for _, id := range existing {
			seen[id] = true
		}
		var missing []string
		for _, id := range params.UserIds {
			if !seen[id] {
				missing = append(missing, strconv.Itoa(id))
			}
		}
		if len(missing) > 0 {
			return nil, common.NewMessage("Users do not exist: {{ids}}", map[string]any{"ids": strings.Join(missing, ", ")})
		}
	}
	if len(params.Groups) > 0 {
		var groupIds []int
		if err := tx.Model(&User{}).
			Where(commonGroupCol+" IN ? AND status = ?", params.Groups, common.UserStatusEnabled).
			Pluck("id", &groupIds).Error; err != nil {
			return nil, err
		}
		for _, id := range groupIds {
			seen[id] = true
		}
	}
	for id := range seen {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids, nil
}

// RevokeCoupon voids a batch: every unused coupon of it becomes revoked.
// Coupons held by a pending order are revoked when that order is released.
func RevokeCoupon(couponId int) (*Coupon, int64, error) {
	var coupon Coupon
	var revoked int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("id = ?", couponId).First(&coupon).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCouponUnavailable
			}
			return err
		}
		if coupon.Status == CouponStatusRevoked {
			return ErrCouponAlreadyRevoked
		}
		coupon.Status = CouponStatusRevoked
		if err := tx.Model(&coupon).Update("status", CouponStatusRevoked).Error; err != nil {
			return err
		}
		result := tx.Model(&UserCoupon{}).
			Where("coupon_id = ? AND status = ?", couponId, UserCouponStatusAvailable).
			Update("status", UserCouponStatusRevoked)
		if result.Error != nil {
			return result.Error
		}
		revoked = result.RowsAffected
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return &coupon, revoked, nil
}

// GetCoupons lists coupon batches for administrators. Valid batches (active
// and not expired) come first, then revoked or expired ones; newest first
// inside each group.
func GetCoupons(keyword string, pageInfo *common.PageInfo) ([]*Coupon, int64, error) {
	query := DB.Model(&Coupon{})
	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		pattern, err := sanitizeLikePattern(keyword)
		if err != nil {
			return nil, 0, err
		}
		if !strings.Contains(pattern, "%") {
			pattern = "%" + pattern + "%"
		}
		if id, convErr := strconv.Atoi(keyword); convErr == nil {
			query = query.Where("(id = ? OR name LIKE ? ESCAPE '!')", id, pattern)
		} else {
			query = query.Where("name LIKE ? ESCAPE '!'", pattern)
		}
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var coupons []*Coupon
	// Valid batches first (active and not expired), invalid ones (revoked or
	// expired) after them; within each group the newest batch comes first.
	order := fmt.Sprintf(
		"CASE WHEN status = %d AND (expires_at = 0 OR expires_at > %d) THEN 0 ELSE 1 END, id desc",
		CouponStatusActive, common.GetTimestamp(),
	)
	if err := query.Order(order).Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&coupons).Error; err != nil {
		return nil, 0, err
	}
	if len(coupons) == 0 {
		return coupons, total, nil
	}
	ids := make([]int, 0, len(coupons))
	for _, coupon := range coupons {
		ids = append(ids, coupon.Id)
	}
	var counts []struct {
		CouponId int
		Total    int64
	}
	if err := DB.Model(&UserCoupon{}).Select("coupon_id, count(*) as total").
		Where("coupon_id IN ? AND status = ?", ids, UserCouponStatusUsed).
		Group("coupon_id").Scan(&counts).Error; err != nil {
		return nil, 0, err
	}
	usedById := make(map[int]int64, len(counts))
	for _, count := range counts {
		usedById[count.CouponId] = count.Total
	}
	for _, coupon := range coupons {
		coupon.UsedCount = usedById[coupon.Id]
	}
	return coupons, total, nil
}

// CouponRecipient is one holder of a coupon batch, for the admin detail view.
type CouponRecipient struct {
	Id          int    `json:"id"`
	UserId      int    `json:"user_id"`
	Username    string `json:"username"`
	Status      string `json:"status"`
	UsedAt      int64  `json:"used_at"`
	UsedTradeNo string `json:"used_trade_no"`
	CreatedAt   int64  `json:"created_at"`
}

func GetCouponRecipients(couponId int, pageInfo *common.PageInfo) ([]CouponRecipient, int64, error) {
	query := DB.Model(&UserCoupon{}).Where("coupon_id = ?", couponId)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	pageSize := min(max(pageInfo.GetPageSize(), 1), couponRecipientPageLimit)
	var rows []UserCoupon
	if err := query.Order("id asc").Limit(pageSize).Offset(pageInfo.GetStartIdx()).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	userIds := make([]int, 0, len(rows))
	for _, row := range rows {
		userIds = append(userIds, row.UserId)
	}
	var users []User
	if len(userIds) > 0 {
		if err := DB.Unscoped().Select("id", "username").Where("id IN ?", userIds).Find(&users).Error; err != nil {
			return nil, 0, err
		}
	}
	names := make(map[int]string, len(users))
	for _, user := range users {
		names[user.Id] = user.Username
	}
	now := common.GetTimestamp()
	recipients := make([]CouponRecipient, 0, len(rows))
	for _, row := range rows {
		recipient := CouponRecipient{
			Id:        row.Id,
			UserId:    row.UserId,
			Username:  names[row.UserId],
			Status:    displayUserCouponStatus(row.Status, row.ExpiresAt, now),
			UsedAt:    row.UsedAt,
			CreatedAt: row.CreatedAt,
		}
		if row.Status == UserCouponStatusUsed {
			recipient.UsedTradeNo = row.TradeNo
		}
		recipients = append(recipients, recipient)
	}
	return recipients, total, nil
}

// UserCouponView is a coupon as its holder sees it.
type UserCouponView struct {
	Id          int     `json:"id"`
	CouponId    int     `json:"coupon_id"`
	Name        string  `json:"name"`
	Scope       string  `json:"scope"`
	Threshold   float64 `json:"threshold"`
	Discount    float64 `json:"discount"`
	Stackable   bool    `json:"stackable"`
	ExpiresAt   int64   `json:"expires_at"`
	Status      string  `json:"status"`
	UsedAt      int64   `json:"used_at"`
	UsedTradeNo string  `json:"used_trade_no"`
	CreatedAt   int64   `json:"created_at"`
}

// GetUserCoupons lists a user's coupons. status: available (includes coupons
// held by a pending order), used, or expired (includes revoked); empty lists
// all. scope: topup or subscription keeps the coupons usable there.
func GetUserCoupons(userId int, status string, scope string, pageInfo *common.PageInfo) ([]UserCouponView, int64, error) {
	now := common.GetTimestamp()
	query := DB.Model(&UserCoupon{}).Where("user_id = ?", userId)
	switch status {
	case "":
	case UserCouponStatusAvailable:
		query = query.Where("(status = ? OR (status = ? AND (expires_at = 0 OR expires_at > ?)))",
			UserCouponStatusLocked, UserCouponStatusAvailable, now)
	case UserCouponStatusUsed:
		query = query.Where("status = ?", UserCouponStatusUsed)
	case UserCouponStatusExpired:
		query = query.Where("(status = ? OR (status = ? AND expires_at > 0 AND expires_at <= ?))",
			UserCouponStatusRevoked, UserCouponStatusAvailable, now)
	default:
		return nil, 0, common.NewMessage("Invalid parameters")
	}
	switch scope {
	case "":
	case CouponScopeTopUp, CouponScopeSubscription:
		scopedCoupons := DB.Model(&Coupon{}).Select("id").Where("scope IN ?", []string{scope, CouponScopeAll})
		query = query.Where("coupon_id IN (?)", scopedCoupons)
	default:
		return nil, 0, common.NewMessage("Invalid coupon scope")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []UserCoupon
	order := "id desc"
	if status == UserCouponStatusUsed {
		order = "used_at desc, id desc"
	}
	if err := query.Order(order).Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	couponIds := make([]int, 0, len(rows))
	for _, row := range rows {
		if !slices.Contains(couponIds, row.CouponId) {
			couponIds = append(couponIds, row.CouponId)
		}
	}
	var coupons []Coupon
	if len(couponIds) > 0 {
		if err := DB.Where("id IN ?", couponIds).Find(&coupons).Error; err != nil {
			return nil, 0, err
		}
	}
	couponById := make(map[int]Coupon, len(coupons))
	for _, coupon := range coupons {
		couponById[coupon.Id] = coupon
	}
	views := make([]UserCouponView, 0, len(rows))
	for _, row := range rows {
		coupon := couponById[row.CouponId]
		view := UserCouponView{
			Id:        row.Id,
			CouponId:  row.CouponId,
			Name:      coupon.Name,
			Scope:     coupon.Scope,
			Threshold: coupon.Threshold,
			Discount:  coupon.Discount,
			Stackable: coupon.Stackable,
			ExpiresAt: row.ExpiresAt,
			Status:    displayUserCouponStatus(row.Status, row.ExpiresAt, now),
			UsedAt:    row.UsedAt,
			CreatedAt: row.CreatedAt,
		}
		if row.Status == UserCouponStatusUsed {
			view.UsedTradeNo = row.TradeNo
		}
		views = append(views, view)
	}
	return views, total, nil
}
