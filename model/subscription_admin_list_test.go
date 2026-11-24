package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAdminUserSubscriptionsFiltersAndSummarizes(t *testing.T) {
	truncateTables(t)
	_ = PurgeSubscriptionPlanCache()
	t.Cleanup(func() {
		_ = PurgeSubscriptionPlanCache()
	})

	now := time.Now().Unix()
	planAlpha := &SubscriptionPlan{
		Title:         "Alpha Plan",
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		TotalAmount:   1000,
		Enabled:       true,
		QuotaLimits: SubscriptionPlanQuotaLimits{
			{Period: SubscriptionResetCustom, CustomSeconds: 18000, Amount: 1000},
		},
	}
	planBeta := &SubscriptionPlan{
		Title:         "Beta Plan",
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		TotalAmount:   500,
		Enabled:       true,
	}
	require.NoError(t, DB.Create(planAlpha).Error)
	require.NoError(t, DB.Create(planBeta).Error)
	InvalidateSubscriptionPlanCache(planAlpha.Id)
	InvalidateSubscriptionPlanCache(planBeta.Id)

	alice := User{
		Username:    "alice",
		DisplayName: "Alice",
		Email:       "alice@example.com",
		Password:    "unused-password-hash",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AffCode:     "alice-aff",
	}
	bob := User{
		Username:    "bob",
		DisplayName: "Bob",
		Email:       "bob@example.com",
		Password:    "unused-password-hash",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AffCode:     "bob-aff",
	}
	require.NoError(t, DB.Create(&alice).Error)
	require.NoError(t, DB.Create(&bob).Error)

	subs := []UserSubscription{
		{
			UserId:      alice.Id,
			PlanId:      planAlpha.Id,
			Status:      "active",
			StartTime:   now - 100,
			EndTime:     now + 3*24*60*60,
			AmountTotal: 1000,
			AmountUsed:  250,
		},
		{
			UserId:      bob.Id,
			PlanId:      planAlpha.Id,
			Status:      "active",
			StartTime:   now - 100,
			EndTime:     now + 30*24*60*60,
			AmountTotal: 1000,
			AmountUsed:  1000,
		},
		{
			UserId:      alice.Id,
			PlanId:      planBeta.Id,
			Status:      "expired",
			StartTime:   now - 100,
			EndTime:     now - 10,
			AmountTotal: 500,
			AmountUsed:  500,
		},
	}
	require.NoError(t, DB.Create(&subs).Error)

	pageInfo := &common.PageInfo{Page: 1, PageSize: 20}
	rows, total, summary, err := GetAdminUserSubscriptions(AdminUserSubscriptionQuery{}, pageInfo)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	assert.EqualValues(t, 3, summary.Total)
	assert.EqualValues(t, 2, summary.Active)
	assert.EqualValues(t, 1, summary.ExpiringSoon)
	require.Len(t, rows, 3)

	// Newest end_time first: bob's Alpha subscription, then alice's Alpha,
	// then alice's expired Beta subscription.
	assert.Equal(t, "bob", rows[0].Username)
	assert.Equal(t, "Alpha Plan", rows[0].PlanTitle)
	require.Len(t, rows[0].PlanQuotaLimits, 1)
	assert.Equal(t, SubscriptionResetCustom, rows[0].PlanQuotaLimits[0].Period)
	assert.EqualValues(t, 18000, rows[0].PlanQuotaLimits[0].CustomSeconds)
	assert.EqualValues(t, 1000, rows[0].PlanQuotaLimits[0].Amount)
	assert.Equal(t, "alice", rows[1].Username)
	assert.Equal(t, "Alice", rows[1].DisplayName)
	assert.Equal(t, "alice@example.com", rows[1].Email)
	assert.Equal(t, "Alpha Plan", rows[1].PlanTitle)
	assert.EqualValues(t, 250, rows[1].Subscription.AmountUsed)
	assert.Equal(t, "Beta Plan", rows[2].PlanTitle)

	// Keyword searches the user name, display name, email and plan title.
	rows, total, summary, err = GetAdminUserSubscriptions(
		AdminUserSubscriptionQuery{Keyword: "alice"}, pageInfo)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	assert.EqualValues(t, 2, summary.Total)
	require.Len(t, rows, 2)

	_, total, _, err = GetAdminUserSubscriptions(
		AdminUserSubscriptionQuery{Keyword: "Alpha"}, pageInfo)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)

	// Plan filter.
	rows, total, _, err = GetAdminUserSubscriptions(
		AdminUserSubscriptionQuery{PlanId: planBeta.Id}, pageInfo)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, rows, 1)
	assert.Equal(t, "Beta Plan", rows[0].PlanTitle)

	// Status filter keeps the overview counts based on keyword+plan only.
	rows, total, summary, err = GetAdminUserSubscriptions(
		AdminUserSubscriptionQuery{Status: "active"}, pageInfo)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	assert.EqualValues(t, 3, summary.Total)
	assert.EqualValues(t, 2, summary.Active)
	require.Len(t, rows, 2)

	rows, total, _, err = GetAdminUserSubscriptions(
		AdminUserSubscriptionQuery{Status: "expired"}, pageInfo)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, rows, 1)
	assert.Equal(t, "Beta Plan", rows[0].PlanTitle)

	// Pagination.
	pageInfo.Page = 2
	pageInfo.PageSize = 2
	rows, total, _, err = GetAdminUserSubscriptions(AdminUserSubscriptionQuery{}, pageInfo)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	require.Len(t, rows, 1)
	assert.Equal(t, "Beta Plan", rows[0].PlanTitle)
}
