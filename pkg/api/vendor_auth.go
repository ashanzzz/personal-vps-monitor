package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/audit"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/config"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers/acck"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers/akile"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers/lazycat"
)

// ParseJWTExp 从 JWT 字符串中解析出 exp 到期时间戳
func ParseJWTExp(tok string) (int64, time.Time, bool) {
	tok = strings.Trim(strings.TrimSpace(tok), "\"'")
	if strings.HasPrefix(strings.ToLower(tok), "bearer ") {
		tok = strings.TrimSpace(tok[7:])
	}
	parts := strings.Split(tok, ".")
	if len(parts) < 2 {
		return 0, time.Time{}, false
	}

	payload := parts[1]
	// 补全 base64 padding
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}

	data, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		data, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return 0, time.Time{}, false
		}
	}

	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(data, &claims); err != nil || claims.Exp <= 0 {
		return 0, time.Time{}, false
	}

	expTime := time.Unix(claims.Exp, 0)
	return claims.Exp, expTime, true
}

// POST /api/akile/login
func handleAkileLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username        string `json:"username"`
		Password        string `json:"password"`
		GACode          string `json:"ga_code"`
		SaveCredentials bool   `json:"save_credentials"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的登录请求体"})
		return
	}

	p, ok := providers.Get("akile")
	if !ok {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "akile provider 未注册"})
		return
	}
	akProvider, ok := p.(*akile.AkileProvider)
	if !ok {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "akile provider 类型异常"})
		return
	}

	token, need2FA, msg, err := akProvider.Login(r.Context(), req.Username, req.Password, req.GACode)
	if need2FA {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"need_2fa": true,
			"message":  msg,
		})
		return
	}
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// 登录成功，持久化保存 Token
	_ = config.SaveTokens(token, "", "", "")
	config.Global.AkileToken = token
	akProvider.SetToken(token)

	// 若用户勾选记住凭据，保存至 credentials.json 供定时免密续期使用
	if req.SaveCredentials && req.Username != "" && req.Password != "" {
		_ = config.SaveProviderCredential("akile", req.Username, req.Password, true)
	}

	_, expTime, _ := ParseJWTExp(token)
	audit.Record("rest_api", "akile.login", "akile", map[string]interface{}{"username": req.Username}, "SUCCESS", "用户手动登录 Akile 成功并自动保存 Token", 0)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":     "success",
		"message":    "Akile 登录成功并已自动保存 Token",
		"token":      token,
		"expires_at": expTime.Format("2006-01-02 15:04:05"),
	})
}

// POST /api/akile/refresh-token
func handleAkileRefreshToken(w http.ResponseWriter, r *http.Request) {
	p, ok := providers.Get("akile")
	if !ok {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "akile provider 未注册"})
		return
	}
	akProvider, ok := p.(*akile.AkileProvider)
	if !ok {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "akile provider 类型异常"})
		return
	}

	newToken, err := akProvider.RefreshToken(r.Context())
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "续期失败: " + err.Error()})
		return
	}

	_ = config.SaveTokens(newToken, "", "", "")
	config.Global.AkileToken = newToken
	akProvider.SetToken(newToken)

	_, expTime, _ := ParseJWTExp(newToken)
	audit.Record("rest_api", "akile.refresh_token", "akile", nil, "SUCCESS", "用户手动触发 Akile Token 续期成功", 0)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":     "success",
		"message":    "Token 续期成功，有效期已重置为 24 小时",
		"token":      newToken,
		"expires_at": expTime.Format("2006-01-02 15:04:05"),
	})
}

// POST /api/lazycat/login
func handleLazyCatLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username        string `json:"username"`
		Password        string `json:"password"`
		GACode          string `json:"ga_code"`
		SaveCredentials bool   `json:"save_credentials"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的登录请求体"})
		return
	}

	p, ok := providers.Get("lazycat")
	if !ok {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "lazycat provider 未注册"})
		return
	}
	lzProvider, ok := p.(*lazycat.LazyCatProvider)
	if !ok {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "lazycat provider 类型异常"})
		return
	}

	token, need2FA, msg, err := lzProvider.Login(r.Context(), req.Username, req.Password, req.GACode)
	if need2FA {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"need_2fa": true,
			"message":  msg,
		})
		return
	}
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// 持久化保存 Token
	_ = config.SaveTokens("", "", token, "")
	config.Global.LazyCatToken = token
	lzProvider.SetToken(token)

	// 若用户勾选记住凭据，保存至 credentials.json 供定时免密续期使用
	if req.SaveCredentials && req.Username != "" && req.Password != "" {
		_ = config.SaveProviderCredential("lazycat", req.Username, req.Password, true)
	}

	_, expTime, _ := ParseJWTExp(token)
	audit.Record("rest_api", "lazycat.login", "lazycat", map[string]interface{}{"username": req.Username}, "SUCCESS", "用户手动登录懒猫云成功并自动保存 Token", 0)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":     "success",
		"message":    "懒猫云登录成功并已自动保存 Token",
		"token":      token,
		"expires_at": expTime.Format("2006-01-02 15:04:05"),
	})
}

// POST /api/acck/login
func handleAcckLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的请求体"})
		return
	}

	tok := strings.Trim(strings.TrimSpace(req.Token), "\"'")
	if tok == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "请输入有效的 ACCK Token"})
		return
	}

	p, ok := providers.Get("acck")
	if !ok {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "acck provider 未注册"})
		return
	}
	acckProvider, ok := p.(*acck.ACCKProvider)
	if !ok {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "acck provider 类型异常"})
		return
	}

	acckProvider.SetToken(tok)
	if err := acckProvider.VerifyToken(r.Context()); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "ACCK 凭据校验失败: " + err.Error()})
		return
	}

	// 校验通过，持久化保存
	_ = config.SaveTokens("", tok, "", "")
	config.Global.ACCKToken = tok

	_, expTime, _ := ParseJWTExp(tok)
	audit.Record("rest_api", "acck.login", "acck", nil, "SUCCESS", "用户手动验证并保存 ACCK Token 成功", 0)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":     "success",
		"message":    "ACCK 凭据校验通过并已成功保存",
		"token":      tok,
		"expires_at": expTime.Format("2006-01-02 15:04:05"),
	})
}
