package controller

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/thanhpk/randstr"
)

type SubscriptionResetPayRequest struct {
	PlanId        int    `json:"plan_id"`
	PaymentMethod string `json:"payment_method"`
	CouponIds     []int  `json:"coupon_ids"`
}

// resolveSubscriptionResetTarget validates the plan and the user's eligibility
// for a paid quota reset. On failure the error response has been written.
func resolveSubscriptionResetTarget(c *gin.Context, planId int) (*model.SubscriptionPlan, bool) {
	if planId <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return nil, false
	}
	plan, err := model.GetSubscriptionPlanById(planId)
	if err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	if !plan.Enabled {
		common.ApiErrorT(c, "Subscription plan is not enabled")
		return nil, false
	}
	if !plan.SupportsPaidReset() {
		common.ApiErrorT(c, "This plan does not support paid quota reset")
		return nil, false
	}
	hasActive, err := model.HasActiveUserSubscriptionForPlan(c.GetInt("id"), plan.Id)
	if err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	if !hasActive {
		common.ApiErrorT(c, "You have no active subscription for this plan")
		return nil, false
	}
	return plan, true
}

// SubscriptionRequestBalanceReset charges the wallet and resets the quota
// counters of the user's active subscription of the plan.
func SubscriptionRequestBalanceReset(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}

	var req SubscriptionResetPayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := model.PurchaseSubscriptionQuotaResetWithBalance(c.GetInt("id"), req.PlanId, req.CouponIds); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// SubscriptionRequestEpayReset starts an epay checkout for a paid quota reset.
func SubscriptionRequestEpayReset(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}

	var req SubscriptionResetPayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	plan, ok := resolveSubscriptionResetTarget(c, req.PlanId)
	if !ok {
		return
	}
	resetPrice := plan.CalculateResetPrice()
	if resetPrice < 0.01 {
		common.ApiErrorT(c, "Plan amount is too low")
		return
	}
	if !operation_setting.ContainsPayMethod(req.PaymentMethod) {
		common.ApiErrorT(c, "Payment method does not exist")
		return
	}

	userId := c.GetInt("id")
	callBackAddress := service.GetCallbackAddress()
	returnUrl, err := url.Parse(callBackAddress + "/api/subscription/epay/return")
	if err != nil {
		common.ApiErrorT(c, "Callback URL configuration error")
		return
	}
	notifyUrl, err := url.Parse(callBackAddress + "/api/subscription/epay/notify")
	if err != nil {
		common.ApiErrorT(c, "Callback URL configuration error")
		return
	}

	tradeNo := fmt.Sprintf("SUBRESETUSR%dNO%s%d", userId, common.GetRandomString(6), time.Now().Unix())

	client := GetEpayClient()
	if client == nil {
		common.ApiErrorT(c, "Payment information has not been configured by administrator")
		return
	}

	payMoney, ok := lockSubscriptionCoupons(c, userId, req.CouponIds, resetPrice, tradeNo)
	if !ok {
		return
	}
	order := &model.SubscriptionOrder{
		UserId:          userId,
		PlanId:          plan.Id,
		Money:           payMoney,
		Kind:            model.SubscriptionOrderKindReset,
		TradeNo:         tradeNo,
		PaymentMethod:   req.PaymentMethod,
		PaymentProvider: model.PaymentProviderEpay,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := order.Insert(); err != nil {
		releaseCouponsOrLog(c, tradeNo)
		common.ApiErrorT(c, "Failed to create order")
		return
	}
	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type:           req.PaymentMethod,
		ServiceTradeNo: tradeNo,
		Name:           fmt.Sprintf("RESET:%s", plan.Title),
		Money:          strconv.FormatFloat(payMoney, 'f', 2, 64),
		Device:         epay.PC,
		NotifyUrl:      notifyUrl,
		ReturnUrl:      returnUrl,
	})
	if err != nil {
		_ = model.ExpireSubscriptionOrder(tradeNo, model.PaymentProviderEpay)
		common.ApiErrorT(c, "Failed to start payment")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": params, "url": uri})
}

// SubscriptionRequestWaffoPancakeReset starts a Waffo Pancake checkout for a
// paid quota reset. The plan's Pancake product is used only to reach the
// store; the charged amount is the reset price snapshot.
func SubscriptionRequestWaffoPancakeReset(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}

	var req SubscriptionResetPayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	plan, ok := resolveSubscriptionResetTarget(c, req.PlanId)
	if !ok {
		return
	}
	if strings.TrimSpace(plan.WaffoPancakeProductId) == "" {
		common.ApiErrorT(c, "WaffoPancakeProductId is not configured for this plan")
		return
	}
	if strings.TrimSpace(setting.WaffoPancakeMerchantID) == "" ||
		strings.TrimSpace(setting.WaffoPancakePrivateKey) == "" {
		common.ApiErrorT(c, "Waffo Pancake is not configured or key is invalid")
		return
	}
	resetPrice := plan.CalculateResetPrice()
	if resetPrice < 0.01 {
		common.ApiErrorT(c, "Plan amount is too low")
		return
	}

	userId := c.GetInt("id")
	user, err := model.GetUserById(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if user == nil {
		common.ApiErrorT(c, "User does not exist")
		return
	}

	// The WAFFO_PANCAKE_SUB- prefix drives webhook dispatch to the subscription
	// order branch, which resolves this reset order by its trade_no.
	tradeNo := fmt.Sprintf("WAFFO_PANCAKE_SUB-RESET-%d-%d-%s", userId, time.Now().UnixMilli(), randstr.String(6))

	payMoney, ok := lockSubscriptionCoupons(c, userId, req.CouponIds, resetPrice, tradeNo)
	if !ok {
		return
	}
	order := &model.SubscriptionOrder{
		UserId:          userId,
		PlanId:          plan.Id,
		Money:           payMoney,
		Kind:            model.SubscriptionOrderKindReset,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodWaffoPancake,
		PaymentProvider: model.PaymentProviderWaffoPancake,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := order.Insert(); err != nil {
		logger.LogError(c.Request.Context(), common.LogText("Waffo Pancake failed to create subscription reset order user_id=%d plan_id=%d trade_no=%s error=%q", userId, plan.Id, tradeNo, err.Error()))
		releaseCouponsOrLog(c, tradeNo)
		paymentError(c, common.NewMessage("Failed to create order"))
		return
	}

	expiresInSeconds := 45 * 60
	session, err := service.CreateWaffoPancakeCheckoutSession(c.Request.Context(), &service.WaffoPancakeCreateSessionParams{
		ProductID:     plan.WaffoPancakeProductId,
		BuyerIdentity: service.WaffoPancakeBuyerIdentityFromUserID(user.Id),
		PriceSnapshot: &service.WaffoPancakePriceSnapshot{
			Amount:      decimal.NewFromFloat(payMoney).StringFixed(2),
			TaxCategory: "saas",
		},
		BuyerEmail:              getWaffoPancakeBuyerEmail(user),
		ExpiresInSeconds:        &expiresInSeconds,
		OrderMerchantExternalID: tradeNo,
	})
	if err != nil {
		logger.LogError(c.Request.Context(), common.LogText("Waffo Pancake failed to create subscription reset checkout session user_id=%d plan_id=%d trade_no=%s error=%q", userId, plan.Id, tradeNo, err.Error()))
		order.Status = common.TopUpStatusFailed
		_ = order.Update()
		releaseCouponsOrLog(c, tradeNo)
		paymentError(c, common.NewMessage("Failed to start payment"))
		return
	}
	logger.LogInfo(c.Request.Context(), common.LogText("Waffo Pancake subscription reset order created user_id=%d plan_id=%d trade_no=%s session_id=%s money=%.2f", userId, plan.Id, tradeNo, session.SessionID, payMoney))

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"checkout_url":     session.CheckoutURL,
			"session_id":       session.SessionID,
			"expires_at":       session.ExpiresAt,
			"order_id":         tradeNo,
			"token":            session.Token,
			"token_expires_at": session.TokenExpiresAt,
		},
	})
}
