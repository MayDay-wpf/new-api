package controller

import (
	"slices"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type BatchManageUsersRequest struct {
	Ids    []int  `json:"ids"`
	Action string `json:"action"`
}

type BatchManageUsersResult struct {
	Succeeded int   `json:"succeeded"`
	FailedIds []int `json:"failed_ids"`
}

// BatchManageUsers applies one bulk action (delete or disable) to a set of
// users. Every target is validated before the step-up proof is consumed, so a
// single unmanageable account rejects the whole request without side effects.
// The proof is bound to the exact action and normalized ID set.
func BatchManageUsers(c *gin.Context) {
	var req BatchManageUsersRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if !slices.Contains(service.AdminUserBatchActions, req.Action) {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if len(req.Ids) > service.MaxAdminUserBatchSize {
		common.ApiErrorT(c, "Too many items in batch request, maximum is {{max}}", map[string]any{"max": service.MaxAdminUserBatchSize})
		return
	}
	ids, ok := service.NormalizeAdminUserBatchIDs(req.Ids)
	if !ok {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}

	var users []model.User
	if err := model.DB.Omit("password", "access_token").Where("id IN ?", ids).Find(&users).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if len(users) != len(ids) {
		common.ApiErrorT(c, "User does not exist")
		return
	}
	myId := c.GetInt("id")
	myRole := c.GetInt("role")
	for _, user := range users {
		if user.Role == common.RoleRootUser {
			if req.Action == "delete" {
				common.ApiErrorT(c, "Cannot delete super administrator account")
			} else {
				common.ApiErrorT(c, "Cannot disable super administrator user")
			}
			return
		}
		if user.Id == myId || myRole <= user.Role {
			common.ApiErrorT(c, "No permission to update users of same or higher permission level")
			return
		}
	}

	authorization := requireAdminUserProof(c, service.VerificationScopeAdminUserBatch, service.AdminUserBatchContext{Action: req.Action, UserIDs: ids})
	if authorization == nil {
		return
	}

	result := BatchManageUsersResult{FailedIds: []int{}}
	for i := range users {
		user := users[i]
		var err error
		if req.Action == "delete" {
			err = batchDeleteUser(c, &user, authorization.Method)
		} else {
			err = batchDisableUser(c, &user, authorization.Method)
		}
		if err != nil {
			common.SysLog(common.LogText("batch %s failed for user %d: %s", req.Action, user.Id, err.Error()))
			result.FailedIds = append(result.FailedIds, user.Id)
			continue
		}
		result.Succeeded++
	}
	common.ApiSuccess(c, result)
}

func batchDeleteUser(c *gin.Context, user *model.User, verificationMethod string) error {
	revokedAccessTokens, err := model.HardDeleteUserById(user.Id)
	if err != nil {
		return err
	}
	recordManageAuditFor(c, user.Id, "user.delete", map[string]any{
		"username":              user.Username,
		"id":                    user.Id,
		"batch":                 true,
		"verification_method":   verificationMethod,
		"revoked_access_tokens": revokedAccessTokens,
	})
	return nil
}

func batchDisableUser(c *gin.Context, user *model.User, verificationMethod string) error {
	if user.Status == common.UserStatusDisabled {
		return nil
	}
	update := model.User{Id: user.Id, Status: common.UserStatusDisabled}
	if err := update.Update(false); err != nil {
		return err
	}
	// Update has already published the new user hash and revoked browser
	// sessions; PAT/relay token caches still need an explicit invalidation.
	if err := model.InvalidateUserTokensCache(user.Id); err != nil {
		common.SysLog(common.LogText("failed to invalidate tokens cache for user %d: %s", user.Id, err.Error()))
	}
	recordManageAuditFor(c, user.Id, "user.manage", map[string]any{
		"action":              "disable",
		"username":            user.Username,
		"id":                  user.Id,
		"batch":               true,
		"verification_method": verificationMethod,
	})
	return nil
}
