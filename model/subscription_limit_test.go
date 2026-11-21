package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeQuotaLimitsValidatesWindows(t *testing.T) {
	limits, err := NormalizeQuotaLimits(SubscriptionPlanQuotaLimits{
		{Period: SubscriptionResetDaily, Amount: 100},
		{Period: SubscriptionResetWeekly, Amount: 0},
		{Period: SubscriptionResetCustom, CustomSeconds: 18000, Amount: 60},
	})
	require.NoError(t, err)
	require.Len(t, limits, 2)
	assert.Equal(t, "daily", limits[0].Key())
	assert.Equal(t, "custom:18000", limits[1].Key())

	_, err = NormalizeQuotaLimits(SubscriptionPlanQuotaLimits{{Period: "hourly", Amount: 1}})
	require.Error(t, err)

	_, err = NormalizeQuotaLimits(SubscriptionPlanQuotaLimits{{Period: SubscriptionResetCustom, Amount: 1}})
	require.Error(t, err)

	_, err = NormalizeQuotaLimits(SubscriptionPlanQuotaLimits{
		{Period: SubscriptionResetDaily, Amount: 1},
		{Period: SubscriptionResetDaily, Amount: 2},
	})
	require.Error(t, err)
}

func TestPreConsumeEnforcesMultipleQuotaWindows(t *testing.T) {
	truncateTables(t)

	plan := &SubscriptionPlan{
		Id:            9701,
		Title:         "Multi window",
		PriceAmount:   10,
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		QuotaLimits: SubscriptionPlanQuotaLimits{
			{Period: SubscriptionResetDaily, Amount: 100},
			{Period: SubscriptionResetCustom, CustomSeconds: 18000, Amount: 60},
		},
	}
	require.NoError(t, DB.Create(plan).Error)

	sub, err := CreateUserSubscriptionFromPlanTx(DB, 501, plan, "test")
	require.NoError(t, err)
	require.Len(t, sub.QuotaLimitUsage, 2)

	consume := func(requestId string, amount int64) error {
		_, err := PreConsumeUserSubscription(requestId, 501, "gpt-test", 0, amount)
		return err
	}

	require.NoError(t, consume("multi-window-1", 50))

	// The 5-hour window only has 10 left, so 20 must be rejected.
	err = consume("multi-window-2", 20)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subscription quota insufficient")
	assert.Contains(t, err.Error(), "5h")

	require.NoError(t, consume("multi-window-3", 10))

	// Simulate the 5-hour window rolling over while the daily window is still
	// within the same day.
	var stored UserSubscription
	require.NoError(t, DB.Where("id = ?", sub.Id).First(&stored).Error)
	for i := range stored.QuotaLimitUsage {
		if stored.QuotaLimitUsage[i].Key == "custom:18000" {
			stored.QuotaLimitUsage[i].WindowStart -= 18000
			stored.QuotaLimitUsage[i].WindowEnd -= 18000
		}
	}
	require.NoError(t, DB.Save(&stored).Error)

	// The 5-hour window restarts from zero, the daily window already used 60.
	require.NoError(t, consume("multi-window-4", 5))

	// 60 + 5 + 40 > 100, blocked by the daily window even though the 5-hour
	// window has room.
	err = consume("multi-window-5", 40)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "daily")
}

func TestPostConsumeAndRefundUpdateQuotaLimitUsage(t *testing.T) {
	truncateTables(t)

	plan := &SubscriptionPlan{
		Id:            9702,
		Title:         "Settle",
		PriceAmount:   10,
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		QuotaLimits: SubscriptionPlanQuotaLimits{
			{Period: SubscriptionResetDaily, Amount: 100},
		},
	}
	require.NoError(t, DB.Create(plan).Error)

	sub, err := CreateUserSubscriptionFromPlanTx(DB, 502, plan, "test")
	require.NoError(t, err)
	require.Len(t, sub.QuotaLimitUsage, 1)

	_, err = PreConsumeUserSubscription("settle-1", 502, "gpt-test", 0, 40)
	require.NoError(t, err)

	require.NoError(t, PostConsumeUserSubscriptionDelta(sub.Id, 10))
	var stored UserSubscription
	require.NoError(t, DB.Where("id = ?", sub.Id).First(&stored).Error)
	require.Len(t, stored.QuotaLimitUsage, 1)
	assert.EqualValues(t, 50, stored.QuotaLimitUsage[0].Used)
	assert.EqualValues(t, 50, stored.AmountUsed)

	require.NoError(t, PostConsumeUserSubscriptionDelta(sub.Id, -20))
	require.NoError(t, DB.Where("id = ?", sub.Id).First(&stored).Error)
	assert.EqualValues(t, 30, stored.QuotaLimitUsage[0].Used)
	assert.EqualValues(t, 30, stored.AmountUsed)
}

func TestPurchaseLimitCountsOnlyActiveSubscriptions(t *testing.T) {
	truncateTables(t)

	plan := &SubscriptionPlan{
		Id:                 9801,
		Title:              "Limited",
		PriceAmount:        10,
		DurationUnit:       SubscriptionDurationMonth,
		DurationValue:      1,
		MaxPurchasePerUser: 1,
	}
	require.NoError(t, DB.Create(plan).Error)

	first, err := CreateUserSubscriptionFromPlanTx(DB, 601, plan, "test")
	require.NoError(t, err)
	require.NotNil(t, first)

	// An active, unexpired subscription blocks a second purchase.
	_, err = CreateUserSubscriptionFromPlanTx(DB, 601, plan, "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Purchase limit")

	count, err := CountUserSubscriptionsByPlan(601, plan.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)

	// Cancelled subscriptions release the purchase-limit slot.
	require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", first.Id).Updates(map[string]any{
		"status":   "cancelled",
		"end_time": GetDBTimestamp() - 1,
	}).Error)
	count, err = CountUserSubscriptionsByPlan(601, plan.Id)
	require.NoError(t, err)
	assert.Zero(t, count)

	second, err := CreateUserSubscriptionFromPlanTx(DB, 601, plan, "test")
	require.NoError(t, err)
	require.NotNil(t, second)

	// An active row whose end time already passed (before the expiry task runs)
	// must not block a purchase either.
	require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", second.Id).
		Update("end_time", GetDBTimestamp()-1).Error)
	_, err = CreateUserSubscriptionFromPlanTx(DB, 601, plan, "test")
	require.NoError(t, err)
}

func TestAdminResetClearsQuotaLimitUsage(t *testing.T) {
	truncateTables(t)

	plan := &SubscriptionPlan{
		Id:            9703,
		Title:         "Reset",
		PriceAmount:   10,
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		QuotaLimits: SubscriptionPlanQuotaLimits{
			{Period: SubscriptionResetDaily, Amount: 100},
		},
	}
	require.NoError(t, DB.Create(plan).Error)

	sub, err := CreateUserSubscriptionFromPlanTx(DB, 503, plan, "test")
	require.NoError(t, err)
	require.NoError(t, PostConsumeUserSubscriptionDelta(sub.Id, 30))

	result, err := AdminResetUserSubscriptionsByPlan(503, plan.Id, false)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.ResetCount)

	var stored UserSubscription
	require.NoError(t, DB.Where("id = ?", sub.Id).First(&stored).Error)
	assert.Zero(t, stored.AmountUsed)
	require.Len(t, stored.QuotaLimitUsage, 1)
	assert.Zero(t, stored.QuotaLimitUsage[0].Used)
}
