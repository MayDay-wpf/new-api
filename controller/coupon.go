package controller

import (
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

type issueCouponRequest struct {
	Name       string   `json:"name"`
	Scope      string   `json:"scope"`
	Threshold  float64  `json:"threshold"`
	Discount   float64  `json:"discount"`
	Stackable  bool     `json:"stackable"`
	ExpiresAt  int64    `json:"expires_at"`
	TargetType string   `json:"target_type"`
	UserIds    []int    `json:"user_ids"`
	Groups     []string `json:"groups"`
}

// AdminIssueCoupon creates a coupon batch and hands one coupon to every
// recipient.
func AdminIssueCoupon(c *gin.Context) {
	if !operation_setting.IsPaymentComplianceConfirmed() {
		common.ApiErrorT(c, "Payment, redemption, subscription, and invitation reward features are disabled. The administrator must confirm compliance terms before enabling them.")
		return
	}
	var req issueCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if nameLength := utf8.RuneCountInString(req.Name); nameLength == 0 || nameLength > 64 {
		common.ApiErrorT(c, "Coupon name length must be between 1-64")
		return
	}
	if !model.IsValidCouponScope(req.Scope) {
		common.ApiErrorT(c, "Invalid coupon scope")
		return
	}
	threshold := model.NormalizeCouponAmount(req.Threshold)
	discount := model.NormalizeCouponAmount(req.Discount)
	if threshold < 0 {
		common.ApiErrorT(c, "Coupon threshold cannot be negative")
		return
	}
	if discount <= 0 {
		common.ApiErrorT(c, "Coupon discount must be greater than 0")
		return
	}
	if threshold > model.MaxCouponAmount || discount > model.MaxCouponAmount {
		common.ApiErrorT(c, "Coupon amount is too large")
		return
	}
	if req.ExpiresAt < 0 || (req.ExpiresAt > 0 && req.ExpiresAt <= common.GetTimestamp()) {
		common.ApiErrorT(c, "Coupon expiry time must be in the future")
		return
	}

	params := model.IssueCouponParams{}
	var userIds []int
	var groups []string
	switch req.TargetType {
	case model.CouponTargetAll:
		params.AllUsers = true
	case model.CouponTargetCustom:
		seenIds := map[int]bool{}
		for _, id := range req.UserIds {
			if id <= 0 {
				common.ApiErrorT(c, "Invalid parameters")
				return
			}
			if !seenIds[id] {
				seenIds[id] = true
				userIds = append(userIds, id)
			}
		}
		seenGroups := map[string]bool{}
		for _, group := range req.Groups {
			group = strings.TrimSpace(group)
			if group != "" && !seenGroups[group] {
				seenGroups[group] = true
				groups = append(groups, group)
			}
		}
		if len(userIds) == 0 && len(groups) == 0 {
			common.ApiErrorT(c, "Select at least one recipient")
			return
		}
		if len(userIds) > model.MaxCouponTargetUserIds || len(groups) > model.MaxCouponTargetGroups {
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
		params.UserIds = userIds
		params.Groups = groups
	default:
		common.ApiErrorT(c, "Select at least one recipient")
		return
	}

	coupon := &model.Coupon{
		Name:          req.Name,
		Scope:         req.Scope,
		Threshold:     threshold,
		Discount:      discount,
		Stackable:     req.Stackable,
		ExpiresAt:     req.ExpiresAt,
		TargetType:    req.TargetType,
		TargetUserIds: userIds,
		TargetGroups:  groups,
		CreatedBy:     c.GetInt("id"),
	}
	issued, err := model.IssueCoupon(coupon, params)
	if err != nil {
		var message *common.Message
		if errors.As(err, &message) {
			common.ApiError(c, err)
			return
		}
		common.SysError(common.LogText("failed to issue coupon: %s", err.Error()))
		common.ApiErrorT(c, "Failed to issue coupons")
		return
	}
	recordManageAudit(c, "coupon.issue", map[string]any{
		"id":          coupon.Id,
		"name":        coupon.Name,
		"count":       issued,
		"scope":       coupon.Scope,
		"threshold":   coupon.Threshold,
		"discount":    coupon.Discount,
		"stackable":   coupon.Stackable,
		"expires_at":  coupon.ExpiresAt,
		"target_type": coupon.TargetType,
	})
	common.ApiSuccess(c, gin.H{"coupon": coupon, "issued_count": issued})
}

func AdminListCoupons(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	coupons, total, err := model.GetCoupons(c.Query("keyword"), pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(coupons)
	common.ApiSuccess(c, pageInfo)
}

func AdminRevokeCoupon(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	coupon, revoked, err := model.RevokeCoupon(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "coupon.revoke", map[string]any{
		"id":    coupon.Id,
		"name":  coupon.Name,
		"count": revoked,
	})
	common.ApiSuccess(c, gin.H{"revoked_count": revoked})
}

func AdminListCouponRecipients(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	pageInfo := common.GetPageQuery(c)
	recipients, total, err := model.GetCouponRecipients(id, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(recipients)
	common.ApiSuccess(c, pageInfo)
}

// GetSelfCoupons lists the coupons of the signed-in user.
func GetSelfCoupons(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	coupons, total, err := model.GetUserCoupons(c.GetInt("id"), c.Query("status"), c.Query("scope"), pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(coupons)
	common.ApiSuccess(c, pageInfo)
}
