package controller

import (
	"errors"
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// redemptionRequest marks usage limits as optional: missing max_uses /
// per_user_limit keeps the old single-use default on create and leaves the
// existing values unchanged on update, while an explicit 0 means unlimited.
type redemptionRequest struct {
	model.Redemption
	MaxUses      *int `json:"max_uses"`
	PerUserLimit *int `json:"per_user_limit"`
}

func GetAllRedemptions(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	redemptions, total, err := model.GetAllRedemptions(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(redemptions)
	common.ApiSuccess(c, pageInfo)
	return
}

func SearchRedemptions(c *gin.Context) {
	keyword := c.Query("keyword")
	status := c.Query("status")
	pageInfo := common.GetPageQuery(c)
	redemptions, total, err := model.SearchRedemptions(keyword, status, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(redemptions)
	common.ApiSuccess(c, pageInfo)
	return
}

func GetRedemption(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	redemption, err := model.GetRedemptionById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    redemption,
	})
	return
}

func AddRedemption(c *gin.Context) {
	if !operation_setting.IsPaymentComplianceConfirmed() {
		common.ApiErrorT(c, "Payment, redemption, subscription, and invitation reward features are disabled. The administrator must confirm compliance terms before enabling them.")
		return
	}

	payload := redemptionRequest{}
	err := c.ShouldBindJSON(&payload)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	redemption := payload.Redemption
	redemption.MaxUses = 1
	redemption.PerUserLimit = 1
	if payload.MaxUses != nil {
		redemption.MaxUses = *payload.MaxUses
	}
	if payload.PerUserLimit != nil {
		redemption.PerUserLimit = *payload.PerUserLimit
	}
	if utf8.RuneCountInString(redemption.Name) == 0 || utf8.RuneCountInString(redemption.Name) > 20 {
		common.ApiErrorT(c, "Redemption code name length must be between 1-20")
		return
	}
	if redemption.Count <= 0 {
		common.ApiErrorT(c, "Redemption code count must be greater than 0")
		return
	}
	if redemption.Count > 100 {
		common.ApiErrorT(c, "Maximum 100 redemption codes can be generated at once")
		return
	}
	if redemption.MaxUses < 0 || redemption.PerUserLimit < 0 {
		common.ApiErrorT(c, "Usage limits cannot be negative")
		return
	}
	var planTitle string
	if redemption.ResetPlanId > 0 {
		if redemption.PlanId > 0 {
			common.ApiErrorT(c, "A reset card cannot also grant a subscription plan")
			return
		}
		resetPlan, err := model.GetSubscriptionPlanById(redemption.ResetPlanId)
		if err != nil {
			common.ApiErrorT(c, "Subscription plan not found")
			return
		}
		keys, err := model.NormalizeResetCardLimitKeys(resetPlan, redemption.ResetQuotaLimitKeys)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		redemption.ResetQuotaLimitKeys = keys
		redemption.PlanId = 0
		redemption.Quota = 0
		planTitle = resetPlan.Title
	} else if redemption.PlanId > 0 {
		plan, err := model.GetSubscriptionPlanById(redemption.PlanId)
		if err != nil {
			common.ApiErrorT(c, "Subscription plan not found")
			return
		}
		planTitle = plan.Title
		redemption.Quota = 0
	} else {
		if redemption.Quota <= 0 {
			common.ApiErrorT(c, "Redemption quota must be positive")
			return
		}
		if err := common.ValidateWalletQuota(redemption.Quota); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	if err := validateExpiredTime(redemption.ExpiredTime); err != nil {
		common.ApiError(c, err)
		return
	}
	var keys []string
	if redemption.ResetPlanId > 0 {
		// Reset cards grant no quota and reserve no plan stock, so the batch can be
		// inserted directly.
		batch := make([]*model.Redemption, 0, redemption.Count)
		for i := 0; i < redemption.Count; i++ {
			batch = append(batch, &model.Redemption{
				UserId:              c.GetInt("id"),
				Name:                redemption.Name,
				Key:                 common.GetUUID(),
				Status:              common.RedemptionCodeStatusEnabled,
				CreatedTime:         common.GetTimestamp(),
				ExpiredTime:         redemption.ExpiredTime,
				MaxUses:             redemption.MaxUses,
				PerUserLimit:        redemption.PerUserLimit,
				ResetPlanId:         redemption.ResetPlanId,
				ResetQuotaLimitKeys: redemption.ResetQuotaLimitKeys,
				ResetAdvanceTime:    redemption.ResetAdvanceTime,
			})
		}
		if err := model.InsertRedemptions(batch); err != nil {
			common.SysError(common.LogText("failed to insert reset card redemptions: %s", err.Error()))
			common.ApiErrorT(c, "Failed to create redemption code, please try again later")
			return
		}
		for _, r := range batch {
			keys = append(keys, r.Key)
		}
	} else if redemption.PlanId > 0 {
		// Subscription codes reserve plan stock, so the batch is created atomically.
		batch := make([]*model.Redemption, 0, redemption.Count)
		for i := 0; i < redemption.Count; i++ {
			batch = append(batch, &model.Redemption{
				UserId:       c.GetInt("id"),
				Name:         redemption.Name,
				Key:          common.GetUUID(),
				Status:       common.RedemptionCodeStatusEnabled,
				CreatedTime:  common.GetTimestamp(),
				PlanId:       redemption.PlanId,
				ExpiredTime:  redemption.ExpiredTime,
				MaxUses:      redemption.MaxUses,
				PerUserLimit: redemption.PerUserLimit,
			})
		}
		if err := model.InsertSubscriptionRedemptions(batch); err != nil {
			common.SysError(common.LogText("failed to insert subscription redemptions: %s", err.Error()))
			if errors.Is(err, model.ErrSubscriptionPlanSoldOut) {
				common.ApiErrorT(c, "Insufficient plan stock for this many redemption codes")
				return
			}
			common.ApiErrorT(c, "Failed to create redemption code, please try again later")
			return
		}
		for _, r := range batch {
			keys = append(keys, r.Key)
		}
	}
	for i := 0; redemption.PlanId <= 0 && redemption.ResetPlanId <= 0 && i < redemption.Count; i++ {
		key := common.GetUUID()
		cleanRedemption := model.Redemption{
			UserId:       c.GetInt("id"),
			Name:         redemption.Name,
			Key:          key,
			CreatedTime:  common.GetTimestamp(),
			Quota:        redemption.Quota,
			PlanId:       redemption.PlanId,
			ExpiredTime:  redemption.ExpiredTime,
			MaxUses:      redemption.MaxUses,
			PerUserLimit: redemption.PerUserLimit,
		}
		err = cleanRedemption.Insert()
		if err != nil {
			common.SysError(common.LogText("failed to insert redemption: %s", err.Error()))
			common.ApiErrorStatus(c, http.StatusOK, common.NewMessage("Failed to create redemption code, please try again later"), gin.H{"data": keys})
			return
		}
		keys = append(keys, key)
	}
	auditDetails := map[string]any{
		"name":           redemption.Name,
		"count":          redemption.Count,
		"max_uses":       redemption.MaxUses,
		"per_user_limit": redemption.PerUserLimit,
	}
	if redemption.ResetPlanId > 0 {
		auditDetails["reset_plan_id"] = redemption.ResetPlanId
		auditDetails["reset_plan_title"] = planTitle
		auditDetails["reset_limit_keys"] = redemption.ResetQuotaLimitKeys
		auditDetails["advance_reset_time"] = redemption.ResetAdvanceTime
	} else if redemption.PlanId > 0 {
		auditDetails["plan_id"] = redemption.PlanId
		auditDetails["plan_title"] = planTitle
	} else {
		auditDetails["quota"] = logger.FormatQuota(redemption.Quota)
	}
	recordManageAudit(c, "redemption.create", auditDetails)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    keys,
	})
	return
}

func DeleteRedemption(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	err := model.DeleteRedemptionById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func UpdateRedemption(c *gin.Context) {
	statusOnly := c.Query("status_only")
	payload := redemptionRequest{}
	err := c.ShouldBindJSON(&payload)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	redemption := payload.Redemption
	cleanRedemption, err := model.GetRedemptionById(redemption.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if statusOnly == "" {
		if redemption.ResetPlanId > 0 {
			if redemption.PlanId > 0 {
				common.ApiErrorT(c, "A reset card cannot also grant a subscription plan")
				return
			}
			resetPlan, err := model.GetSubscriptionPlanById(redemption.ResetPlanId)
			if err != nil {
				common.ApiErrorT(c, "Subscription plan not found")
				return
			}
			keys, err := model.NormalizeResetCardLimitKeys(resetPlan, redemption.ResetQuotaLimitKeys)
			if err != nil {
				common.ApiError(c, err)
				return
			}
			redemption.Quota = 0
			redemption.PlanId = 0
			redemption.ResetQuotaLimitKeys = keys
		} else if redemption.PlanId > 0 {
			if _, err := model.GetSubscriptionPlanById(redemption.PlanId); err != nil {
				common.ApiErrorT(c, "Subscription plan not found")
				return
			}
			redemption.Quota = 0
			redemption.ResetPlanId = 0
			redemption.ResetQuotaLimitKeys = nil
		} else {
			if redemption.Quota <= 0 {
				common.ApiErrorT(c, "Redemption quota must be positive")
				return
			}
			if err := common.ValidateWalletQuota(redemption.Quota); err != nil {
				common.ApiError(c, err)
				return
			}
			redemption.ResetPlanId = 0
			redemption.ResetQuotaLimitKeys = nil
		}
		if err := validateExpiredTime(redemption.ExpiredTime); err != nil {
			common.ApiError(c, err)
			return
		}
		// If you add more fields, please also update redemption.Update()
		cleanRedemption.Name = redemption.Name
		cleanRedemption.Quota = redemption.Quota
		cleanRedemption.PlanId = redemption.PlanId
		cleanRedemption.ResetPlanId = redemption.ResetPlanId
		cleanRedemption.ResetQuotaLimitKeys = redemption.ResetQuotaLimitKeys
		cleanRedemption.ResetAdvanceTime = redemption.ResetAdvanceTime
		cleanRedemption.ExpiredTime = redemption.ExpiredTime
		if payload.MaxUses != nil {
			cleanRedemption.MaxUses = *payload.MaxUses
		}
		if payload.PerUserLimit != nil {
			cleanRedemption.PerUserLimit = *payload.PerUserLimit
		}
	}
	if statusOnly != "" {
		cleanRedemption.Status = redemption.Status
	}
	err = cleanRedemption.Update()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    cleanRedemption,
	})
	return
}

func DeleteInvalidRedemption(c *gin.Context) {
	rows, err := model.DeleteInvalidRedemptions()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    rows,
	})
	return
}

func validateExpiredTime(expired int64) error {
	if expired != 0 && expired < common.GetTimestamp() {
		return common.NewMessage("Expiration time cannot be earlier than current time")
	}
	return nil
}

func DeleteRedemptionBatch(c *gin.Context) {
	var request struct {
		Ids []int `json:"ids" binding:"required,min=1,max=1000,dive,gt=0"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	count, err := model.BatchDeleteRedemptions(request.Ids)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "redemption.delete_batch", map[string]any{
		"count":                    count,
		"total":                    len(request.Ids),
		"requested_redemption_ids": request.Ids,
	})
	common.ApiSuccess(c, count)
}
