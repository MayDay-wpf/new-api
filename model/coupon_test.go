package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCouponTest(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&Coupon{}, &UserCoupon{}))
	truncateTables(t)
	t.Cleanup(func() {
		DB.Exec("DELETE FROM coupons")
		DB.Exec("DELETE FROM user_coupons")
	})
}

func issueTestCoupon(t *testing.T, coupon Coupon, userIds ...int) map[int]int {
	t.Helper()
	if coupon.Name == "" {
		coupon.Name = "test"
	}
	if coupon.Scope == "" {
		coupon.Scope = CouponScopeAll
	}
	coupon.TargetType = CouponTargetCustom
	issued, err := IssueCoupon(&coupon, IssueCouponParams{UserIds: userIds})
	require.NoError(t, err)
	require.Equal(t, len(userIds), issued)
	var rows []UserCoupon
	require.NoError(t, DB.Where("coupon_id = ?", coupon.Id).Find(&rows).Error)
	byUser := map[int]int{}
	for _, row := range rows {
		byUser[row.UserId] = row.Id
	}
	return byUser
}

func userCouponStatus(t *testing.T, id int) string {
	t.Helper()
	var row UserCoupon
	require.NoError(t, DB.First(&row, id).Error)
	return row.Status
}

func TestIssueCouponRecipients(t *testing.T) {
	setupCouponTest(t)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "a", AffCode: "coupon-aff-1", Status: common.UserStatusEnabled, Group: "vip"}).Error)
	require.NoError(t, DB.Create(&User{Id: 2, Username: "b", AffCode: "coupon-aff-2", Status: common.UserStatusEnabled, Group: "default"}).Error)
	require.NoError(t, DB.Create(&User{Id: 3, Username: "c", AffCode: "coupon-aff-3", Status: common.UserStatusDisabled, Group: "vip"}).Error)

	all := Coupon{Name: "all", Scope: CouponScopeAll, Discount: 1, TargetType: CouponTargetAll}
	issued, err := IssueCoupon(&all, IssueCouponParams{AllUsers: true})
	require.NoError(t, err)
	assert.Equal(t, 2, issued, "all users means enabled users that exist now")

	custom := Coupon{Name: "custom", Scope: CouponScopeAll, Discount: 1, TargetType: CouponTargetCustom}
	issued, err = IssueCoupon(&custom, IssueCouponParams{UserIds: []int{2}, Groups: []string{"vip"}})
	require.NoError(t, err)
	assert.Equal(t, 2, issued, "user ids and groups are merged; disabled group members are skipped")

	missing := Coupon{Name: "missing", Scope: CouponScopeAll, Discount: 1, TargetType: CouponTargetCustom}
	_, err = IssueCoupon(&missing, IssueCouponParams{UserIds: []int{1, 99}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "99")
	var count int64
	require.NoError(t, DB.Model(&Coupon{}).Where("name = ?", "missing").Count(&count).Error)
	assert.Zero(t, count, "a rejected issue leaves no batch behind")
}

func TestApplyUserCouponsRules(t *testing.T) {
	setupCouponTest(t)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "a", AffCode: "coupon-aff-1", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, DB.Create(&User{Id: 2, Username: "b", AffCode: "coupon-aff-2", Status: common.UserStatusEnabled}).Error)
	now := common.GetTimestamp()

	stackA := issueTestCoupon(t, Coupon{Threshold: 10, Discount: 2, Stackable: true}, 1)[1]
	stackB := issueTestCoupon(t, Coupon{Threshold: 0, Discount: 1.5, Stackable: true}, 1)[1]
	single := issueTestCoupon(t, Coupon{Threshold: 0, Discount: 3}, 1)[1]
	subOnly := issueTestCoupon(t, Coupon{Scope: CouponScopeSubscription, Discount: 1}, 1)[1]
	big := issueTestCoupon(t, Coupon{Discount: 100}, 1)[1]
	expired := issueTestCoupon(t, Coupon{Discount: 1, ExpiresAt: now + 3600}, 1)[1]
	require.NoError(t, DB.Model(&UserCoupon{}).Where("id = ?", expired).Update("expires_at", now-1).Error)
	otherUser := issueTestCoupon(t, Coupon{Discount: 1}, 2)[2]

	tests := []struct {
		name    string
		ids     []int
		usage   string
		amount  float64
		wantErr error
		final   float64
	}{
		{name: "stackable coupons combine", ids: []int{stackA, stackB}, usage: CouponScopeTopUp, amount: 10, final: 6.5},
		{name: "threshold not met", ids: []int{stackA}, usage: CouponScopeTopUp, amount: 9.99, wantErr: ErrCouponThresholdNotMet},
		{name: "non-stackable cannot combine", ids: []int{single, stackB}, usage: CouponScopeTopUp, amount: 20, wantErr: ErrCouponNotStackable},
		{name: "non-stackable alone", ids: []int{single}, usage: CouponScopeTopUp, amount: 20, final: 17},
		{name: "scope mismatch", ids: []int{subOnly}, usage: CouponScopeTopUp, amount: 20, wantErr: ErrCouponScopeMismatch},
		{name: "scope match", ids: []int{subOnly}, usage: CouponScopeSubscription, amount: 20, final: 19},
		{name: "discount floors at zero", ids: []int{big}, usage: CouponScopeSubscription, amount: 20, final: 0},
		{name: "expired", ids: []int{expired}, usage: CouponScopeTopUp, amount: 20, wantErr: ErrCouponExpired},
		{name: "another user's coupon", ids: []int{otherUser}, usage: CouponScopeTopUp, amount: 20, wantErr: ErrCouponUnavailable},
		{name: "too many", ids: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}, usage: CouponScopeTopUp, amount: 20, wantErr: ErrCouponTooMany},
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tradeNo := "RULE" + string(rune('A'+i))
			result, err := LockUserCoupons(1, tc.ids, tc.usage, tc.amount, tradeNo)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				var locked int64
				require.NoError(t, DB.Model(&UserCoupon{}).Where("trade_no = ?", tradeNo).Count(&locked).Error)
				assert.Zero(t, locked, "a rejected payment locks nothing")
				return
			}
			require.NoError(t, err)
			assert.InDelta(t, tc.final, result.FinalAmount, 0.0001)
			require.NoError(t, ReleaseUserCoupons(tradeNo))
		})
	}
}

func TestCouponLockLifecycle(t *testing.T) {
	setupCouponTest(t)
	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })
	user := insertUserForPaymentGuardTest(t, 7, 0)
	couponId := issueTestCoupon(t, Coupon{Scope: CouponScopeTopUp, Discount: 1}, user.Id)[user.Id]

	_, err := LockUserCoupons(user.Id, []int{couponId}, CouponScopeTopUp, 5, "ORDER1")
	require.NoError(t, err)
	assert.Equal(t, UserCouponStatusLocked, userCouponStatus(t, couponId))

	_, err = LockUserCoupons(user.Id, []int{couponId}, CouponScopeTopUp, 5, "ORDER2")
	require.ErrorIs(t, err, ErrCouponUnavailable, "a locked coupon cannot be used by another order")

	// A failed order gives the coupon back.
	createEpayTestOrder(t, user.Id, "ORDER1", PaymentProviderEpay, common.TopUpStatusPending)
	require.NoError(t, UpdatePendingTopUpStatus("ORDER1", PaymentProviderEpay, common.TopUpStatusFailed))
	assert.Equal(t, UserCouponStatusAvailable, userCouponStatus(t, couponId))

	// A completed order consumes it exactly once.
	_, err = LockUserCoupons(user.Id, []int{couponId}, CouponScopeTopUp, 5, "ORDER3")
	require.NoError(t, err)
	createEpayTestOrder(t, user.Id, "ORDER3", PaymentProviderEpay, common.TopUpStatusPending)
	_, err = RechargeEpay("ORDER3", "alipay", "127.0.0.1")
	require.NoError(t, err)
	assert.Equal(t, UserCouponStatusUsed, userCouponStatus(t, couponId))

	views, total, err := GetUserCoupons(user.Id, UserCouponStatusUsed, CouponScopeTopUp, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	assert.Equal(t, "ORDER3", views[0].UsedTradeNo)
}

func TestRevokeCouponAndStaleLocks(t *testing.T) {
	setupCouponTest(t)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "a", AffCode: "coupon-aff-1", Status: common.UserStatusEnabled}).Error)
	coupon := Coupon{Name: "revoke", Scope: CouponScopeAll, Discount: 1, TargetType: CouponTargetCustom}
	_, err := IssueCoupon(&coupon, IssueCouponParams{UserIds: []int{1}})
	require.NoError(t, err)
	stale := issueTestCoupon(t, Coupon{Discount: 1}, 1)[1]

	var held UserCoupon
	require.NoError(t, DB.Where("coupon_id = ?", coupon.Id).First(&held).Error)
	_, err = LockUserCoupons(1, []int{held.Id}, CouponScopeTopUp, 5, "HELD")
	require.NoError(t, err)

	_, revoked, err := RevokeCoupon(coupon.Id)
	require.NoError(t, err)
	assert.Zero(t, revoked, "a coupon held by a pending order is revoked on release")
	_, _, err = RevokeCoupon(coupon.Id)
	require.ErrorIs(t, err, ErrCouponAlreadyRevoked)

	require.NoError(t, ReleaseUserCoupons("HELD"))
	assert.Equal(t, UserCouponStatusRevoked, userCouponStatus(t, held.Id))

	_, err = LockUserCoupons(1, []int{stale}, CouponScopeTopUp, 5, "ABANDONED")
	require.NoError(t, err)
	require.NoError(t, DB.Model(&UserCoupon{}).Where("id = ?", stale).Update("locked_at", common.GetTimestamp()-7200).Error)
	released, err := ReleaseStaleCouponLocks(3600)
	require.NoError(t, err)
	assert.EqualValues(t, 1, released)
	assert.Equal(t, UserCouponStatusAvailable, userCouponStatus(t, stale))

	views, _, err := GetUserCoupons(1, UserCouponStatusExpired, "", &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, UserCouponStatusRevoked, views[0].Status)
}
