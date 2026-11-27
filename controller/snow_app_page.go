package controller

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
)

type snowAppPageConfig struct {
	ClientId            string `json:"client_id"`
	ClientLabel         string `json:"client_label"`
	RedirectUri         string `json:"redirect_uri"`
	State               string `json:"state"`
	CodeChallenge       string `json:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method"`
}

func encodeSnowAppPageConfig(cfg snowAppPageConfig) string {
	data, err := json.Marshal(cfg)
	if err != nil {
		return "{}"
	}
	// json.Marshal 已转义 < > &，这里再兜底一次脚本闭合。
	return strings.ReplaceAll(string(data), "</", "<\\/")
}

// 只接受可安全渲染的 Logo 地址，其余回退到内置雪花图标。
func snowAppLogoURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.HasPrefix(trimmed, "//") {
		return ""
	}
	if strings.HasPrefix(trimmed, "/") {
		return trimmed
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "data:image/") {
		return trimmed
	}
	return ""
}

const snowAppBrandFallbackSVG = `<svg width="30" height="30" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" aria-hidden="true">
      <path d="M12 2v20M4.2 6.5l15.6 11M19.8 6.5L4.2 17.5"/>
      <path d="M12 6.4l2.1-2.1M12 6.4L9.9 4.3M12 17.6l2.1 2.1M12 17.6l-2.1 2.1"/>
      <path d="M7.4 9.2L4.5 8.4M7.4 9.2l.8-2.9M16.6 14.8l2.9.8M16.6 14.8l-.8 2.9"/>
      <path d="M7.4 14.8l-2.9.8M7.4 14.8l.8 2.9M16.6 9.2l2.9-.8M16.6 9.2l-.8-2.9"/>
    </svg>`

func snowAppBrandMark() string {
	if logo := snowAppLogoURL(common.Logo); logo != "" {
		return fmt.Sprintf(
			`<img class="brand-logo" src="%s" alt="" draggable="false">`,
			html.EscapeString(logo),
		)
	}
	return snowAppBrandFallbackSVG
}

// SnowAppAuthorizePage 渲染桌面客户端授权页，不依赖 web 构建产物。
func SnowAppAuthorizePage(c *gin.Context) {
	cfg := snowAppPageConfig{
		ClientId:            strings.TrimSpace(c.Query("client_id")),
		RedirectUri:         c.Query("redirect_uri"),
		State:               c.Query("state"),
		CodeChallenge:       strings.TrimSpace(c.Query("code_challenge")),
		CodeChallengeMethod: strings.TrimSpace(c.Query("code_challenge_method")),
	}
	clientId, _, err := validateSnowAppAuthorizeRequest(&snowAppAuthorizeRequest{
		ClientId:            cfg.ClientId,
		RedirectUri:         cfg.RedirectUri,
		State:               cfg.State,
		CodeChallenge:       cfg.CodeChallenge,
		CodeChallengeMethod: cfg.CodeChallengeMethod,
	})
	if err != nil {
		c.Header("Cache-Control", "no-store")
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(snowAppPageHTML("", "", err.Error())))
		return
	}
	cfg.ClientId = clientId
	cfg.ClientLabel, _ = snowAppClientLabel(clientId)
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(snowAppPageHTML(encodeSnowAppPageConfig(cfg), cfg.ClientLabel, "")))
}

// snowAppPageHTML 生成授权页；clientLabel 用于提示正在授权的客户端。
func snowAppPageHTML(configJSON string, clientLabel string, startupError string) string {
	if strings.TrimSpace(clientLabel) == "" {
		clientLabel = "客户端"
	}
	page := strings.ReplaceAll(snowAppAuthorizePageTemplate, "__SNOW_APP_CONFIG__", configJSON)
	page = strings.ReplaceAll(page, "__SNOW_APP_LOGO__", snowAppBrandMark())
	page = strings.ReplaceAll(page, "__SNOW_APP_CLIENT__", html.EscapeString(clientLabel))
	errorJSON, err := json.Marshal(startupError)
	if err != nil {
		errorJSON = []byte(`""`)
	}
	page = strings.ReplaceAll(page, "__SNOW_APP_ERROR_JSON__", strings.ReplaceAll(string(errorJSON), "</", "<\\/"))
	return page
}

const snowAppAuthorizePageTemplate = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Snow AI Router · __SNOW_APP_CLIENT__ 授权</title>
<style>
:root {
  color-scheme: light dark;
  --page-bg: #f4f4f2;
  --card-bg: #ffffff;
  --text: #1c1d1f;
  --text-secondary: #5f6368;
  --text-muted: #8a8f98;
  --border: #e2e3e5;
  --field-bg: #fbfbfa;
  --accent: #26292e;
  --accent-text: #ffffff;
  --error: #b3261e;
  --error-bg: #fdf2f1;
  --ok: #1f6f43;
}
@media (prefers-color-scheme: dark) {
  :root {
    --page-bg: #16181b;
    --card-bg: #1e2125;
    --text: #eceef1;
    --text-secondary: #a7adb6;
    --text-muted: #7c838d;
    --border: #2c3036;
    --field-bg: #171a1e;
    --accent: #e8eaed;
    --accent-text: #16181b;
    --error: #f2b8b5;
    --error-bg: #35201f;
    --ok: #7ee2a8;
  }
}
* { box-sizing: border-box; }
html, body { height: 100%; }
body {
  margin: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 32px 20px;
  background: var(--page-bg);
  color: var(--text);
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif;
  font-size: 14px;
  line-height: 1.6;
  -webkit-font-smoothing: antialiased;
}
.card {
  width: min(420px, 100%);
  padding: 30px 28px 24px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--card-bg);
}
.brand { display: flex; align-items: center; gap: 10px; }
.brand-logo, .brand svg { flex: none; width: 30px; height: 30px; object-fit: contain; color: var(--text); }
.brand-name { font-size: 15px; font-weight: 650; letter-spacing: 0.2px; }
.brand-sub { font-size: 11px; color: var(--text-muted); }
h1 { margin: 22px 0 4px; font-size: 19px; font-weight: 650; letter-spacing: 0.2px; }
.subtitle { margin: 0 0 22px; color: var(--text-secondary); font-size: 13px; }
.field { display: grid; gap: 6px; margin-bottom: 14px; }
.field label { font-size: 12px; color: var(--text-secondary); }
input {
  width: 100%;
  padding: 9px 10px;
  border: 1px solid var(--border);
  border-radius: 4px;
  outline: none;
  background: var(--field-bg);
  color: var(--text);
  font: inherit;
}
input:focus { border-color: var(--text-muted); }
button {
  width: 100%;
  padding: 10px 14px;
  border: 0;
  border-radius: 4px;
  background: var(--accent);
  color: var(--accent-text);
  font: inherit;
  font-weight: 600;
  cursor: pointer;
}
button:disabled { opacity: 0.55; cursor: default; }
button.ghost {
  width: auto;
  padding: 0;
  background: none;
  color: var(--text-secondary);
  font-weight: 400;
  text-decoration: underline;
  text-underline-offset: 3px;
}
.actions { margin-top: 18px; display: grid; gap: 12px; }
.account {
  margin: 18px 0;
  padding: 14px;
  border: 1px solid var(--border);
  border-radius: 4px;
  display: grid;
  gap: 4px;
}
.account-name { font-weight: 600; }
.account-meta { font-size: 12px; color: var(--text-secondary); word-break: break-all; }
.permissions { margin: 0 0 4px; padding: 0; list-style: none; display: grid; gap: 6px; font-size: 12px; color: var(--text-secondary); }
.permissions li { display: flex; gap: 8px; align-items: flex-start; }
.permissions li::before { content: ""; flex: none; width: 4px; height: 4px; margin-top: 8px; border-radius: 50%; background: var(--text-muted); }
.status { min-height: 18px; font-size: 12px; color: var(--text-muted); }
.error { margin: 0 0 14px; padding: 10px 12px; border-radius: 4px; background: var(--error-bg); color: var(--error); font-size: 12px; }
.footnote { margin: 18px 0 0; font-size: 11px; color: var(--text-muted); }
.hidden { display: none !important; }
</style>
</head>
<body>
<main class="card">
  <div class="brand">
    __SNOW_APP_LOGO__
    <div>
      <div class="brand-name">Snow AI Router</div>
      <div class="brand-sub">Console authorization</div>
    </div>
  </div>

  <div id="startupError" class="error hidden"></div>

  <section id="viewLoading">
    <h1>正在检查登录状态</h1>
    <p class="subtitle">请稍候，正在读取当前账号的会话。</p>
  </section>

  <section id="viewLogin" class="hidden">
    <h1>登录你的账号</h1>
    <p class="subtitle">登录后即可授权 __SNOW_APP_CLIENT__ 使用这台路由服务。</p>
    <div id="loginError" class="error hidden"></div>
    <form id="loginForm" autocomplete="on">
      <div class="field">
        <label for="username">用户名或邮箱</label>
        <input id="username" name="username" type="text" autocomplete="username" required>
      </div>
      <div class="field">
        <label for="password">密码</label>
        <input id="password" name="password" type="password" autocomplete="current-password" required>
      </div>
      <div class="actions">
        <button id="loginButton" type="submit">登录并继续</button>
      </div>
    </form>
    <p class="footnote">已启用二次验证的账号请先在新版控制台完成登录，再刷新本页。</p>
  </section>

  <section id="viewAccount" class="hidden">
    <h1>授权 __SNOW_APP_CLIENT__</h1>
    <p class="subtitle">确认后将在 __SNOW_APP_CLIENT__ 中接入你的可用渠道。</p>
    <div id="authorizeError" class="error hidden"></div>
    <div class="account">
      <div id="accountName" class="account-name"></div>
      <div id="accountMeta" class="account-meta"></div>
    </div>
    <ul class="permissions">
      <li>读取账号资料与可用渠道</li>
      <li>创建并读取由 __SNOW_APP_CLIENT__ 生成的 API 密钥</li>
    </ul>
    <div class="actions">
      <button id="authorizeButton" type="button">授权并返回 __SNOW_APP_CLIENT__</button>
      <button id="switchButton" class="ghost" type="button">使用其他账号</button>
    </div>
  </section>

  <p id="status" class="status"></p>
  <p class="footnote">此页面由 Snow AI Router 提供，授权结果直接返回本机的 __SNOW_APP_CLIENT__。</p>
</main>

<script>
(function () {
  var CONFIG = __SNOW_APP_CONFIG__;
  var startupError = __SNOW_APP_ERROR_JSON__;
  var accessToken = "";

  var views = {
    loading: document.getElementById("viewLoading"),
    login: document.getElementById("viewLogin"),
    account: document.getElementById("viewAccount")
  };
  var statusEl = document.getElementById("status");
  var startupErrorEl = document.getElementById("startupError");

  function showView(name) {
    Object.keys(views).forEach(function (key) {
      views[key].classList.toggle("hidden", key !== name);
    });
  }

  function setStatus(text) {
    statusEl.textContent = text || "";
  }

  function showError(id, message) {
    var el = document.getElementById(id);
    el.textContent = message || "";
    el.classList.toggle("hidden", !message);
  }

  function request(path, body, token) {
    var headers = { "Content-Type": "application/json" };
    if (token) {
      headers.Authorization = "Bearer " + token;
    }
    return fetch(path, {
      method: "POST",
      credentials: "same-origin",
      headers: headers,
      body: JSON.stringify(body || {})
    }).then(function (response) {
      return response.json().catch(function () { return null; }).then(function (data) {
        return { status: response.status, ok: response.ok, data: data };
      });
    });
  }

  function errorMessage(result, fallback) {
    if (result && result.data && result.data.message) {
      return result.data.message;
    }
    if (result && result.status) {
      return fallback + " (HTTP " + result.status + ")";
    }
    return fallback;
  }

  function renderAccount(user) {
    user = user || {};
    document.getElementById("accountName").textContent =
      user.display_name || user.username || "已登录账号";
    var meta = [];
    if (user.username) meta.push("@" + user.username);
    if (user.email) meta.push(user.email);
    if (user.group) meta.push("分组 " + user.group);
    document.getElementById("accountMeta").textContent = meta.join(" · ");
  }

  function refreshSession() {
    showView("loading");
    setStatus("正在读取会话…");
    return request("/api/user/auth/refresh", {}, "").then(function (result) {
      if (result.data && result.data.success && result.data.data && result.data.data.access_token) {
        accessToken = result.data.data.access_token;
        renderAccount(result.data.data.user);
        setStatus("");
        showView("account");
        return;
      }
      accessToken = "";
      showView("login");
      setStatus("");
      window.setTimeout(function () {
        var input = document.getElementById("username");
        if (input) input.focus();
      }, 30);
    }).catch(function () {
      showView("login");
      setStatus("");
    });
  }

  function handleLogin(event) {
    event.preventDefault();
    showError("loginError", "");
    var button = document.getElementById("loginButton");
    button.disabled = true;
    setStatus("正在登录…");
    request("/api/user/login", {
      username: document.getElementById("username").value.trim(),
      password: document.getElementById("password").value
    }, "").then(function (result) {
      var payload = result.data || {};
      if (payload.success && payload.data && payload.data.access_token) {
        accessToken = payload.data.access_token;
        document.getElementById("password").value = "";
        renderAccount(payload.data.user);
        setStatus("");
        showView("account");
        return;
      }
      if (payload.success && payload.data && payload.data.require_verification) {
        showError("loginError", "该账号需要二次验证。请先在控制台完成登录与验证，然后刷新本页。");
        return;
      }
      showError("loginError", errorMessage(result, "登录失败，请检查用户名或密码"));
    }).catch(function () {
      showError("loginError", "网络异常，请稍后重试");
    }).then(function () {
      button.disabled = false;
      setStatus("");
    });
  }

  function authorize() {
    showError("authorizeError", "");
    var button = document.getElementById("authorizeButton");
    button.disabled = true;
    setStatus("正在授权…");
    request("/api/snow-app/authorize", {
      client_id: CONFIG.client_id,
      redirect_uri: CONFIG.redirect_uri,
      state: CONFIG.state,
      code_challenge: CONFIG.code_challenge,
      code_challenge_method: CONFIG.code_challenge_method
    }, accessToken).then(function (result) {
      var payload = result.data || {};
      if (!payload.success || !payload.data || !payload.data.code) {
        showError("authorizeError", errorMessage(result, "授权失败，请重新登录后再试"));
        return;
      }
      setStatus("授权成功，正在返回 " + (CONFIG.client_label || "客户端") + "…");
      var target = CONFIG.redirect_uri + "?code=" + encodeURIComponent(payload.data.code);
      if (CONFIG.state) {
        target += "&state=" + encodeURIComponent(CONFIG.state);
      }
      window.location.replace(target);
    }).catch(function () {
      showError("authorizeError", "网络异常，请稍后重试");
    }).then(function () {
      button.disabled = false;
    });
  }

  function switchAccount() {
    setStatus("正在退出当前账号…");
    request("/api/user/auth/logout", {}, accessToken).catch(function () {}).then(function () {
      accessToken = "";
      document.getElementById("username").value = "";
      document.getElementById("password").value = "";
      showView("login");
      setStatus("");
    });
  }

  document.getElementById("loginForm").addEventListener("submit", handleLogin);
  document.getElementById("authorizeButton").addEventListener("click", authorize);
  document.getElementById("switchButton").addEventListener("click", switchAccount);

  if (startupError) {
    startupErrorEl.textContent = startupError;
    startupErrorEl.classList.remove("hidden");
    showView("login");
    document.getElementById("loginForm").classList.add("hidden");
    setStatus("");
    return;
  }
  if (!CONFIG || !CONFIG.redirect_uri) {
    startupErrorEl.textContent = "授权请求缺少必要参数，请从客户端重新发起登录。";
    startupErrorEl.classList.remove("hidden");
    showView("loading");
    return;
  }
  refreshSession();
})();
</script>
</body>
</html>`
