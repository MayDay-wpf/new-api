package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// GetBillingDiscountUserAgents lists distinct User-Agents from recent consume
// logs so administrators can pick them when editing billing discount rules.
func GetBillingDiscountUserAgents(c *gin.Context) {
	userAgents, err := model.GetRecentLogUserAgents(5000, 200)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, userAgents)
}

// GetBillingDiscountChannels lists channels so administrators can restrict a
// billing discount rule to a set of channels.
func GetBillingDiscountChannels(c *gin.Context) {
	channels, err := model.GetBillingDiscountChannels()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, channels)
}
