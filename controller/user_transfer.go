package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

type TransferQuotaRequest struct {
	TargetUserId int `json:"target_user_id"`
	Quota        int `json:"quota"`
}

// TransferQuotaToUser moves part of the caller's balance to another user.
// It requires a dashboard session: a personal access token must not be able to
// move wallet funds.
func TransferQuotaToUser(c *gin.Context) {
	if c.GetBool("use_access_token") {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "This operation requires dashboard session authentication. API access token is not allowed.",
		})
		return
	}

	var req TransferQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if req.TargetUserId <= 0 || req.Quota <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}

	transfer, err := model.TransferUserQuota(c.GetInt("id"), req.TargetUserId, req.Quota, c.ClientIP())
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccessT(c, "Transfer successful", gin.H{
		"target_user_id":  transfer.ToUserId,
		"target_username": transfer.ToUsername,
		"quota":           transfer.Quota,
	})
}
