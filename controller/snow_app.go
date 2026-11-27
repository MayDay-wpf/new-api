package controller

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

const (
	snowAppTokenName             = "Snow"
	snowAppLegacyTokenName       = "Snow App"
	snowAppTokenNamePrefix       = "Snow · "
	snowAppLegacyTokenNamePrefix = "Snow App · "
	snowAppAuthCodeTTL           = int64(5 * 60)
	snowAppPatExpiryDays         = 365
	snowAppTokenNameLimit        = 50
)

// 已接入的桌面客户端；client_id 决定授权页展示的名称。
var snowAppClients = map[string]string{
	"snow-app": "Snow App",
	"snow-cli": "Snow CLI",
}

func snowAppClientLabel(clientId string) (string, bool) {
	label, ok := snowAppClients[strings.TrimSpace(clientId)]
	return label, ok
}

func isSnowAppTokenName(name string) bool {
	return name == snowAppTokenName || name == snowAppLegacyTokenName
}

func isSnowAppChannelTokenName(name string) bool {
	return strings.HasPrefix(name, snowAppTokenNamePrefix) ||
		strings.HasPrefix(name, snowAppLegacyTokenNamePrefix)
}

// 桌面客户端只能通过授权换取这些最小权限。
var snowAppTokenScopes = []string{
	"profile:read",
	"api_key:read",
	"api_key:write",
}

type snowAppAuthorizeRequest struct {
	ClientId            string `json:"client_id"`
	RedirectUri         string `json:"redirect_uri"`
	State               string `json:"state"`
	CodeChallenge       string `json:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method"`
}

type snowAppExchangeRequest struct {
	ClientId     string `json:"client_id"`
	Code         string `json:"code"`
	CodeVerifier string `json:"code_verifier"`
	RedirectUri  string `json:"redirect_uri"`
}

type snowAppChannel struct {
	Id          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Ratio       any      `json:"ratio"`
	Models      []string `json:"models"`
	HasKey      bool     `json:"has_key"`
}

// 只接受本机回环地址的回调，避免授权码被重定向到任意站点。
func normalizeSnowAppRedirectUri(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("redirect_uri is required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", errors.New("redirect_uri is invalid")
	}
	if parsed.Scheme != "http" || parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" {
		return "", errors.New("redirect_uri must be a plain loopback http URL")
	}
	switch parsed.Hostname() {
	case "127.0.0.1", "localhost", "::1":
	default:
		return "", errors.New("redirect_uri must point to the local machine")
	}
	if parsed.Port() == "" || parsed.Path == "" || parsed.Path == "/" {
		return "", errors.New("redirect_uri must carry a port and a callback path")
	}
	return parsed.String(), nil
}

func isSnowAppPkceChallenge(value string) bool {
	if len(value) < 43 || len(value) > 128 {
		return false
	}
	for _, ch := range value {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			continue
		}
		if ch == '-' || ch == '_' || ch == '.' || ch == '~' {
			continue
		}
		return false
	}
	return true
}

func validateSnowAppAuthorizeRequest(req *snowAppAuthorizeRequest) (string, string, error) {
	clientId := strings.TrimSpace(req.ClientId)
	if _, ok := snowAppClientLabel(clientId); !ok {
		return "", "", errors.New("unsupported client_id")
	}
	redirectUri, err := normalizeSnowAppRedirectUri(req.RedirectUri)
	if err != nil {
		return "", "", err
	}
	if method := strings.TrimSpace(req.CodeChallengeMethod); method != "" && method != "S256" {
		return "", "", errors.New("only the S256 PKCE method is supported")
	}
	if !isSnowAppPkceChallenge(strings.TrimSpace(req.CodeChallenge)) {
		return "", "", errors.New("code_challenge is invalid")
	}
	if len(strings.TrimSpace(req.State)) > 256 {
		return "", "", errors.New("state is too long")
	}
	return clientId, redirectUri, nil
}

func snowAppPkceChallenge(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

// SnowAppAuthorize 在浏览器已登录的前提下签发一次性授权码。
func SnowAppAuthorize(c *gin.Context) {
	userId := c.GetInt("id")
	if userId <= 0 {
		common.ApiErrorStatus(c, http.StatusUnauthorized, errors.New("a dashboard login session is required"))
		return
	}
	var req snowAppAuthorizeRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}
	clientId, redirectUri, err := validateSnowAppAuthorizeRequest(&req)
	if err != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, err)
		return
	}

	now := common.GetTimestamp()
	_ = model.DeleteExpiredSnowAppAuthCodes(now)
	code, err := common.GenerateRandomCharsKey(48)
	if err != nil {
		common.ApiErrorStatus(c, http.StatusInternalServerError, errors.New("failed to generate the authorization code"))
		return
	}
	record := &model.SnowAppAuthCode{
		CodeHash:      model.AccessTokenFingerprint(code),
		UserId:        userId,
		ClientId:      clientId,
		RedirectUri:   redirectUri,
		CodeChallenge: strings.TrimSpace(req.CodeChallenge),
		ExpiresAt:     now + snowAppAuthCodeTTL,
		CreatedAt:     now,
	}
	if err := model.CreateSnowAppAuthCode(record); err != nil {
		common.ApiErrorStatus(c, http.StatusInternalServerError, errors.New("failed to store the authorization code"))
		return
	}
	common.ApiSuccess(c, gin.H{
		"code":       code,
		"expires_at": record.ExpiresAt,
	})
}

// SnowAppExchangeToken 用授权码 + PKCE 换取长期访问令牌。
func SnowAppExchangeToken(c *gin.Context) {
	var req snowAppExchangeRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}
	clientId := strings.TrimSpace(req.ClientId)
	if _, ok := snowAppClientLabel(clientId); !ok {
		common.ApiErrorStatus(c, http.StatusBadRequest, errors.New("unsupported client_id"))
		return
	}
	verifier := strings.TrimSpace(req.CodeVerifier)
	if !isSnowAppPkceChallenge(verifier) {
		common.ApiErrorStatus(c, http.StatusBadRequest, errors.New("code_verifier is invalid"))
		return
	}
	redirectUri, err := normalizeSnowAppRedirectUri(req.RedirectUri)
	if err != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, err)
		return
	}
	now := common.GetTimestamp()
	record, err := model.ConsumeSnowAppAuthCode(model.AccessTokenFingerprint(strings.TrimSpace(req.Code)), now)
	if err != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, model.ErrSnowAppAuthCodeInvalid)
		return
	}
	if record.ClientId != "" && record.ClientId != clientId {
		common.ApiErrorStatus(c, http.StatusBadRequest, errors.New("client_id does not match the authorization request"))
		return
	}
	if record.RedirectUri != redirectUri {
		common.ApiErrorStatus(c, http.StatusBadRequest, errors.New("redirect_uri does not match the authorization request"))
		return
	}
	challenge := snowAppPkceChallenge(verifier)
	if subtle.ConstantTimeCompare([]byte(challenge), []byte(record.CodeChallenge)) != 1 {
		common.ApiErrorStatus(c, http.StatusBadRequest, errors.New("code_verifier does not match the authorization request"))
		return
	}

	user, err := model.GetSelfUserById(record.UserId)
	if err != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, errors.New("the authorized account no longer exists"))
		return
	}
	if user.Status != common.UserStatusEnabled {
		common.ApiErrorStatus(c, http.StatusForbidden, errors.New("the authorized account has been disabled"))
		return
	}

	// 同一账号重复授权时不再堆积历史令牌，只保留最新一枚。
	if tokens, listErr := model.ListUserAccessTokens(user.Id); listErr == nil {
		for i := range tokens {
			if !isSnowAppTokenName(tokens[i].Name) {
				continue
			}
			_, _ = model.DeleteUserAccessToken(user.Id, tokens[i].Id)
		}
	}

	suffix, err := common.GenerateRandomCharsKey(43)
	if err != nil {
		common.ApiErrorStatus(c, http.StatusInternalServerError, errors.New("failed to generate the access token"))
		return
	}
	raw := model.AccessTokenPrefix + suffix
	token := &model.UserAccessToken{
		Name:      snowAppTokenName,
		TokenHash: model.AccessTokenFingerprint(raw),
		TokenHint: model.AccessTokenHint(raw),
		ExpiresAt: now + int64(snowAppPatExpiryDays)*24*60*60,
		CreatedAt: now,
	}
	if err := token.SetScopes(snowAppTokenScopes); err != nil {
		common.ApiErrorStatus(c, http.StatusInternalServerError, errors.New("failed to grant access token scopes"))
		return
	}
	if err := model.CreateUserAccessToken(user.Id, token, service.AccessTokenMaxPerUser); err != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, err)
		return
	}

	common.ApiSuccess(c, gin.H{
		"access_token": raw,
		"token_type":   "Bearer",
		"expires_at":   token.ExpiresAt,
		"user": gin.H{
			"id":           user.Id,
			"username":     user.Username,
			"display_name": user.DisplayName,
			"email":        user.Email,
			"group":        user.Group,
			"role":         user.Role,
		},
	})
}

func snowAppTokenNameFor(group string) string {
	group = strings.TrimSpace(group)
	if group == "" {
		group = "default"
	}
	name := snowAppTokenNamePrefix + group
	if utf8.RuneCountInString(name) <= snowAppTokenNameLimit {
		return name
	}
	keep := snowAppTokenNameLimit - utf8.RuneCountInString(snowAppTokenNamePrefix)
	if keep < 1 {
		keep = 1
	}
	runes := []rune(group)
	if len(runes) > keep {
		runes = runes[:keep]
	}
	return snowAppTokenNamePrefix + string(runes)
}

// snowAppTokensByGroup 返回该账号由 Snow 客户端创建的 API 密钥（按分组索引）。
func snowAppTokensByGroup(userId int) map[string]*model.Token {
	result := make(map[string]*model.Token)
	tokens, err := model.GetAllUserTokens(userId, 0, 1000)
	if err != nil {
		return result
	}
	for _, token := range tokens {
		if token == nil || !isSnowAppChannelTokenName(token.Name) {
			continue
		}
		if _, exists := result[token.Group]; !exists {
			result[token.Group] = token
		}
	}
	return result
}

// SnowAppChannels 列出当前账号可用的分组（渠道）与其中的模型。
func SnowAppChannels(c *gin.Context) {
	userId := c.GetInt("id")
	userGroup, err := model.GetUserGroup(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	usableGroups := service.GetUserUsableGroups(userGroup)
	existing := snowAppTokensByGroup(userId)

	ids := make([]string, 0, len(usableGroups))
	for group := range usableGroups {
		ids = append(ids, group)
	}
	sort.Strings(ids)

	channels := make([]snowAppChannel, 0, len(ids))
	for _, group := range ids {
		description := strings.TrimSpace(usableGroups[group])
		name := description
		if name == "" {
			name = group
		}
		var ratio any = service.GetUserGroupRatio(userGroup, group)
		modelGroups := []string{group}
		if group == "auto" {
			ratio = "auto"
			if description == "" {
				name = "auto"
			}
			modelGroups = service.GetUserAutoGroup(userGroup)
		}
		models := service.GetGroupsEnabledModels(modelGroups)
		// 没有模型的渠道无法用于对话，不出现在可接入列表中。
		if len(models) == 0 {
			continue
		}
		channels = append(channels, snowAppChannel{
			Id:          group,
			Name:        name,
			Description: description,
			Ratio:       ratio,
			Models:      models,
			HasKey:      existing[group] != nil,
		})
	}
	common.ApiSuccess(c, gin.H{"channels": channels})
}

// SnowAppEnsureKey 为指定分组复用或创建一枚 API 密钥。
func SnowAppEnsureKey(c *gin.Context) {
	userId := c.GetInt("id")
	var req struct {
		Group string `json:"group"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}
	group := strings.TrimSpace(req.Group)
	if group == "" {
		common.ApiErrorStatus(c, http.StatusBadRequest, errors.New("group is required"))
		return
	}
	userGroup, err := model.GetUserGroup(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if _, ok := service.GetUserUsableGroups(userGroup)[group]; !ok {
		common.ApiErrorStatus(c, http.StatusForbidden, errors.New("the group is not available to this account"))
		return
	}

	if token := snowAppTokensByGroup(userId)[group]; token != nil {
		common.ApiSuccess(c, gin.H{
			"key":      token.GetFullKey(),
			"created":  false,
			"token_id": token.Id,
			"name":     token.Name,
			"group":    token.Group,
		})
		return
	}

	count, err := model.CountUserTokens(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if int(count) >= operation_setting.GetMaxUserTokens() {
		common.ApiErrorStatus(c, http.StatusBadRequest, errors.New("the account has reached the maximum number of API keys"))
		return
	}
	key, err := common.GenerateKey()
	if err != nil {
		common.ApiErrorStatus(c, http.StatusInternalServerError, errors.New("failed to generate the API key"))
		return
	}
	now := common.GetTimestamp()
	token := &model.Token{
		UserId:              userId,
		Name:                snowAppTokenNameFor(group),
		Key:                 key,
		CreatedTime:         now,
		AccessedTime:        now,
		ExpiredTime:         -1,
		UnlimitedQuota:      true,
		Group:               group,
		AllowUserQuotaQuery: true,
	}
	if err := token.Insert(); err != nil {
		common.ApiErrorStatus(c, http.StatusInternalServerError, errors.New("failed to create the API key"))
		return
	}
	common.ApiSuccess(c, gin.H{
		"key":      key,
		"created":  true,
		"token_id": token.Id,
		"name":     token.Name,
		"group":    token.Group,
	})
}
