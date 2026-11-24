package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAffAuditTest(t *testing.T) (*gorm.DB, *model.User) {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, i18n.Init())
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AuditLog{}))
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		common.RedisEnabled = previousRedis
	})
	user := &model.User{
		Username:    "aff-owner",
		Password:    "placeholder",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AffCode:     "aff-owner",
		AuthVersion: 1,
	}
	require.NoError(t, db.Create(user).Error)
	return db, user
}

func newAffAuditRouter(user *model.User) *gin.Engine {
	router := gin.New()
	router.POST("/api/user/aff_transfer", func(c *gin.Context) {
		c.Set("id", user.Id)
		c.Set("role", user.Role)
		c.Set("username", user.Username)
		TransferAffQuota(c)
	})
	return router
}

func auditParam(t *testing.T, audit model.AuditLog, key string) string {
	t.Helper()
	require.NotNil(t, audit.Other.Op)
	raw, ok := audit.Other.Op.Params[key]
	require.True(t, ok, "missing audit param %s", key)
	data, ok := raw.(json.RawMessage)
	require.True(t, ok, "audit param %s has unexpected type %T", key, raw)
	var value string
	require.NoError(t, json.Unmarshal(data, &value))
	return value
}

func TestTransferAffQuotaWritesSecurityAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	confirmPaymentComplianceForTest(t)
	db, user := setupAffAuditTest(t)

	transferQuota := int(common.QuotaPerUnit)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("aff_quota", transferQuota*2).Error)

	request := httptest.NewRequest(http.MethodPost, "/api/user/aff_transfer", strings.NewReader(fmt.Sprintf(`{"quota":%d}`, transferQuota)))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = "192.0.2.9:1234"
	response := httptest.NewRecorder()
	newAffAuditRouter(user).ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"success":true`)

	var audit model.AuditLog
	require.NoError(t, db.Where("action = ?", "user.aff_transfer").Order("id desc").First(&audit).Error)
	require.Equal(t, model.AuditCategorySecurity, audit.Category)
	require.Equal(t, user.Id, audit.UserId)
	require.True(t, audit.Success)
	require.Equal(t, "192.0.2.9", audit.Ip)
	require.Equal(t, "/api/user/aff_transfer", audit.Route)
	require.Equal(t, fmt.Sprintf("Moved %s from invitation rewards to balance", logger.FormatQuota(transferQuota)), audit.Content)
	require.Equal(t, logger.FormatQuota(transferQuota), auditParam(t, audit, "quota"))
	require.Equal(t, logger.FormatQuota(transferQuota*2), auditParam(t, audit, "aff_quota_before"))
	require.Equal(t, logger.FormatQuota(transferQuota), auditParam(t, audit, "aff_quota_after"))
	require.Equal(t, logger.FormatQuota(transferQuota), auditParam(t, audit, "wallet_quota_after"))
}

func TestTransferAffQuotaAuditsRejectedTransfer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	confirmPaymentComplianceForTest(t)
	db, user := setupAffAuditTest(t)

	request := httptest.NewRequest(http.MethodPost, "/api/user/aff_transfer", strings.NewReader(fmt.Sprintf(`{"quota":%d}`, int(common.QuotaPerUnit))))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	newAffAuditRouter(user).ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"success":false`)

	var audit model.AuditLog
	require.NoError(t, db.Where("action = ?", "user.aff_transfer").Order("id desc").First(&audit).Error)
	require.False(t, audit.Success)
	require.Contains(t, audit.Content, "Moved")
	require.Equal(t, "insufficient_aff_quota", auditParam(t, audit, "failure_reason"))
}

func TestRecordInviteAuditWritesSecurityEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	confirmPaymentComplianceForTest(t)
	db, inviter := setupAffAuditTest(t)

	originalInviterReward, originalInviteeReward := common.QuotaForInviter, common.QuotaForInvitee
	common.QuotaForInviter, common.QuotaForInvitee = 1000, 500
	t.Cleanup(func() {
		common.QuotaForInviter, common.QuotaForInvitee = originalInviterReward, originalInviteeReward
	})

	invitee := &model.User{
		Username:    "aff-invitee",
		Password:    "placeholder",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AffCode:     "aff-invitee",
		AuthVersion: 1,
	}
	require.NoError(t, db.Create(invitee).Error)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/register", nil)
	c.Request.RemoteAddr = "198.51.100.7:4321"
	recordInviteAudit(c, inviter.Id, invitee)

	var audit model.AuditLog
	require.NoError(t, db.Where("action = ?", "user.aff_invite").Order("id desc").First(&audit).Error)
	require.Equal(t, model.AuditCategorySecurity, audit.Category)
	require.Equal(t, invitee.Id, audit.UserId)
	require.True(t, audit.Success)
	require.Equal(t, "198.51.100.7", audit.Ip)
	require.Equal(t, inviter.Username, auditParam(t, audit, "inviter_username"))
	require.Equal(t, invitee.Username, auditParam(t, audit, "invitee_username"))
	require.Equal(t, logger.FormatQuota(1000), auditParam(t, audit, "inviter_reward"))
	require.Equal(t, logger.FormatQuota(500), auditParam(t, audit, "invitee_reward"))
}
