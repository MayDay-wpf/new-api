package controller

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createBatchTarget(t *testing.T, name string, role int) *model.User {
	t.Helper()
	user := &model.User{Username: name, Password: "x", Role: role, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: name + "-aff"}
	require.NoError(t, model.DB.Create(user).Error)
	return user
}

func batchUsersProof(t *testing.T, identity service.AuthIdentity, action string, ids ...int) string {
	t.Helper()
	context, err := common.Marshal(service.AdminUserBatchContext{Action: action, UserIDs: ids})
	require.NoError(t, err)
	return issueSecurityEnrollmentProof(t, identity, service.VerificationOperation{Scope: service.VerificationScopeAdminUserBatch, Context: context}, service.VerificationMethodPassword)
}

func batchUsersBody(action string, ids ...int) string {
	raw, _ := common.Marshal(BatchManageUsersRequest{Ids: ids, Action: action})
	return string(raw)
}

func TestBatchManageUsersDisableAndDelete(t *testing.T) {
	_, identity, target := setupAdminUserTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ExternalIdentityClaim{}, &model.Token{}))
	other := createBatchTarget(t, "batch-other", common.RoleCommonUser)

	// Proof IDs are a set: order and duplicates must not matter.
	proof := batchUsersProof(t, identity, "disable", other.Id, target.Id)
	response := adminUserRequest(http.MethodPost, "/api/user/batch", batchUsersBody("disable", target.Id, other.Id, target.Id), proof, identity, common.RoleRootUser, nil, BatchManageUsers)
	var result securityEnrollmentResponse
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	require.True(t, result.Success, response.Body.String())
	var data BatchManageUsersResult
	require.NoError(t, common.Unmarshal(result.Data, &data))
	assert.Equal(t, 2, data.Succeeded)
	assert.Empty(t, data.FailedIds)
	for _, id := range []int{target.Id, other.Id} {
		stored, err := model.GetUserById(id, false)
		require.NoError(t, err)
		assert.Equal(t, common.UserStatusDisabled, stored.Status)
	}

	proof = batchUsersProof(t, identity, "delete", target.Id, other.Id)
	response = adminUserRequest(http.MethodPost, "/api/user/batch", batchUsersBody("delete", target.Id, other.Id), proof, identity, common.RoleRootUser, nil, BatchManageUsers)
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	require.True(t, result.Success, response.Body.String())
	var count int64
	require.NoError(t, model.DB.Unscoped().Model(&model.User{}).Where("id IN ?", []int{target.Id, other.Id}).Count(&count).Error)
	assert.Zero(t, count)
	var audits int64
	require.NoError(t, model.LOG_DB.Model(&model.AuditLog{}).Where("action = ?", "user.delete").Count(&audits).Error)
	assert.EqualValues(t, 2, audits)
}

func TestBatchManageUsersProofIsBoundToActionAndTargets(t *testing.T) {
	_, identity, target := setupAdminUserTest(t)
	other := createBatchTarget(t, "batch-other", common.RoleCommonUser)
	proof := batchUsersProof(t, identity, "disable", target.Id)

	for _, test := range []struct{ name, body string }{
		{"different action", batchUsersBody("delete", target.Id)},
		{"extra target", batchUsersBody("disable", target.Id, other.Id)},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := adminUserRequest(http.MethodPost, "/api/user/batch", test.body, proof, identity, common.RoleRootUser, nil, BatchManageUsers)
			var result securityEnrollmentResponse
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
			assert.Equal(t, http.StatusForbidden, response.Code)
			assert.Equal(t, "SECURITY_PROOF_CONTEXT_MISMATCH", result.Code)
		})
	}
	for _, id := range []int{target.Id, other.Id} {
		stored, err := model.GetUserById(id, false)
		require.NoError(t, err)
		assert.Equal(t, common.UserStatusEnabled, stored.Status)
	}
}

func TestBatchManageUsersRejectsUnmanageableTargetsBeforeMutation(t *testing.T) {
	operator, identity, target := setupAdminUserTest(t)
	root := createBatchTarget(t, "batch-root", common.RoleRootUser)
	admin := createBatchTarget(t, "batch-admin", common.RoleAdminUser)

	for _, test := range []struct {
		name   string
		action string
		ids    []int
		role   int
	}{
		{"root target", "delete", []int{target.Id, root.Id}, common.RoleRootUser},
		{"self", "disable", []int{target.Id, operator.Id}, common.RoleRootUser},
		{"same level admin", "disable", []int{target.Id, admin.Id}, common.RoleAdminUser},
		{"missing user", "delete", []int{target.Id, 999999}, common.RoleRootUser},
		{"unknown action", "promote", []int{target.Id}, common.RoleRootUser},
		{"empty", "delete", []int{}, common.RoleRootUser},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := adminUserRequest(http.MethodPost, "/api/user/batch", batchUsersBody(test.action, test.ids...), "", identity, test.role, nil, BatchManageUsers)
			var result securityEnrollmentResponse
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
			assert.False(t, result.Success, response.Body.String())
			assert.Empty(t, result.Code, "must fail validation before asking for a proof")
		})
	}
	stored, err := model.GetUserById(target.Id, false)
	require.NoError(t, err)
	assert.Equal(t, common.UserStatusEnabled, stored.Status)

	ids := make([]int, service.MaxAdminUserBatchSize+1)
	for i := range ids {
		ids[i] = i + 1
	}
	response := adminUserRequest(http.MethodPost, "/api/user/batch", batchUsersBody("delete", ids...), "", identity, common.RoleRootUser, nil, BatchManageUsers)
	var result securityEnrollmentResponse
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	assert.False(t, result.Success)
}

func TestAdminUserBatchVerificationContext(t *testing.T) {
	for _, test := range []struct {
		name    string
		context string
		valid   bool
	}{
		{"delete", `{"action":"delete","user_ids":[3,1]}`, true},
		{"disable", `{"action":"disable","user_ids":[1]}`, true},
		{"enable not allowed", `{"action":"enable","user_ids":[1]}`, false},
		{"empty ids", `{"action":"delete","user_ids":[]}`, false},
		{"zero id", `{"action":"delete","user_ids":[0,1]}`, false},
		{"missing ids", `{"action":"delete"}`, false},
		{"extra field", `{"action":"delete","user_ids":[1],"user_id":1}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.BindVerificationOperation(service.VerificationOperation{Scope: service.VerificationScopeAdminUserBatch, Context: []byte(test.context)})
			if test.valid {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, service.ErrVerificationContextInvalid)
			}
		})
	}
	a, err := service.BindVerificationOperation(service.VerificationOperation{Scope: service.VerificationScopeAdminUserBatch, Context: []byte(`{"action":"delete","user_ids":[3,1,3]}`)})
	require.NoError(t, err)
	b, err := service.BindVerificationOperation(service.VerificationOperation{Scope: service.VerificationScopeAdminUserBatch, Context: []byte(fmt.Sprintf(`{"user_ids":[1,3],"action":%q}`, "delete"))})
	require.NoError(t, err)
	assert.Equal(t, a.ContextHash, b.ContextHash)
}
