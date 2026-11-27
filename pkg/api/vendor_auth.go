package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/audit"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/config"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/scheduler"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/store"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/vault"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers/acck"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers/akile"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers/lazycat"
	"github.com/go-chi/chi/v5"
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

	gaCode := strings.TrimSpace(req.GACode)
	if gaCode == "" {
		creds := config.LoadSavedCredentials()
		if creds.Akile != nil && creds.Akile.TOTPSecret != "" {
			if autoCode, err := vault.GenerateTOTP(creds.Akile.TOTPSecret); err == nil && autoCode != "" {
				gaCode = autoCode
				log.Printf("[AkileLogin] 自动利用 Vaultwarden TOTP 密钥生成 2FA 动态码: %s", autoCode)
			}
		}
	}

	token, need2FA, msg, err := akProvider.Login(r.Context(), req.Username, req.Password, gaCode)
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

	gaCode := req.GACode
	if gaCode == "" {
		creds := config.LoadSavedCredentials()
		if creds.LazyCat != nil && creds.LazyCat.TOTPSecret != "" {
			if autoCode, err := vault.GenerateTOTP(creds.LazyCat.TOTPSecret); err == nil && autoCode != "" {
				gaCode = autoCode
				log.Printf("[LazyCatLogin] 自动基于 Vaultwarden/凭据库 TOTP 密钥生成 2FA 动态码: %s", autoCode)
			}
		}
	}

	token, need2FA, msg, err := lzProvider.Login(r.Context(), req.Username, req.Password, gaCode)
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
		Token           string `json:"token"`
		Username        string `json:"username"`
		Password        string `json:"password"`
		GACode          string `json:"ga_code"`
		TOTPSecret      string `json:"totp_secret"`
		SaveCredentials bool   `json:"save_credentials"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的请求体"})
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

	tok := strings.Trim(strings.TrimSpace(req.Token), "\"'")
	username := strings.TrimSpace(req.Username)
	password := req.Password

	// 模式 1: 直接提交 JWT Token 校验并保存
	if tok != "" && username == "" && password == "" {
		acckProvider.SetToken(tok)
		if err := acckProvider.VerifyToken(r.Context()); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "ACCK 凭据校验失败: " + err.Error()})
			return
		}

		_ = config.SaveTokens("", tok, "", "")
		config.SetProviderToken("acck", tok)

		_, expTime, _ := ParseJWTExp(tok)
		audit.Record("rest_api", "acck.login", "acck", nil, "SUCCESS", "用户手动验证并保存 ACCK Token 成功", 0)

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"status":     "success",
			"message":    "ACCK 凭据校验通过并已成功保存",
			"token":      tok,
			"expires_at": expTime.Format("2006-01-02 15:04:05"),
		})
		return
	}

	// 模式 2: 账号密码 + 2FA 自动解算 Altcha 登录
	if username == "" || password == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "请输入有效的 ACCK 账号密码或 Token"})
		return
	}

	creds := config.LoadSavedCredentials()
	gaCode := strings.TrimSpace(req.GACode)
	totpSecret := strings.TrimSpace(req.TOTPSecret)
	if totpSecret == "" && creds.ACCK != nil && strings.EqualFold(strings.TrimSpace(creds.ACCK.Username), username) {
		totpSecret = strings.TrimSpace(creds.ACCK.TOTPSecret)
	}

	totpFn := func() (string, error) {
		if gaCode != "" {
			return gaCode, nil
		}
		if totpSecret != "" {
			code, err := vault.GenerateTOTP(totpSecret)
			if err != nil {
				return "", err
			}
			log.Printf("[AcckLogin] 自动基于 TOTP 密钥生成 2FA 动态码")
			return code, nil
		}
		return "", nil
	}

	loginCtx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()

	newTok, need2FA, msg, err := acckProvider.LoginWithTOTPProvider(loginCtx, username, password, totpFn, false)
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

	_ = config.SaveTokens("", newTok, "", "")
	acckProvider.SetToken(newTok)

	if req.SaveCredentials {
		_ = config.SaveProviderCredentialWithTOTP("acck", username, password, totpSecret, true)
	}

	_, expTime, _ := ParseJWTExp(newTok)
	audit.Record("rest_api", "acck.login", "acck", map[string]interface{}{"username": config.MaskUsername(username)}, "SUCCESS", "用户登录 ACCK 成功并自动保存 Token", 0)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":     "success",
		"message":    "ACCK 登录成功并已自动保存 Token",
		"token":      newTok,
		"expires_at": expTime.Format("2006-01-02 15:04:05"),
	})
}

// GET /api/vendor/health-summary 秒级极速鉴权健康与各服务独立状态
func handleVendorHealthSummary(w http.ResponseWriter, r *http.Request) {
	creds := config.LoadSavedCredentials()
	checkinSummary := scheduler.GetCheckinManager().GetSummary()

	type ProviderHealth struct {
		ProviderID     string                 `json:"provider_id"`
		Configured     bool                   `json:"configured"`
		TokenValid     bool                   `json:"token_valid"`
		IsExpired      bool                   `json:"is_expired"`
		ExpiresAt      string                 `json:"expires_at,omitempty"`
		SecondsLeft    int64                  `json:"seconds_left"`
		HasCredentials bool                   `json:"has_credentials"`
		UsernameMasked string                 `json:"username_masked,omitempty"`
		AutoRefresh    bool                   `json:"auto_refresh"`
		LastSyncedAt   string                 `json:"last_synced_at,omitempty"`
		TodayCheckin   map[string]interface{} `json:"today_checkin,omitempty"`
	}

	summary := make(map[string]ProviderHealth)
	now := time.Now().Unix()

	// 1. Akile
	akTok := config.GetProviderToken("akile")
	akHealth := ProviderHealth{
		ProviderID:   "akile",
		Configured:   akTok != "",
		LastSyncedAt: store.GetRefreshTime("instances:akile"),
	}
	if creds.Akile != nil && creds.Akile.Password != "" {
		akHealth.HasCredentials = true
		akHealth.UsernameMasked = config.MaskUsername(creds.Akile.Username)
		akHealth.AutoRefresh = creds.Akile.AutoRefresh
	}
	if akTok != "" {
		exp, expTime, ok := ParseJWTExp(akTok)
		if ok {
			akHealth.ExpiresAt = expTime.Format("2006-01-02 15:04:05")
			akHealth.SecondsLeft = exp - now
			akHealth.IsExpired = (akHealth.SecondsLeft <= 0)
			akHealth.TokenValid = !akHealth.IsExpired
		} else {
			akHealth.TokenValid = true // 无法按 JWT 解析的非标 Token 默认就绪
		}
	}
	if ck, ok := checkinSummary["akile"].(map[string]interface{}); ok {
		akHealth.TodayCheckin = ck
	}
	summary["akile"] = akHealth

	// 2. LazyCat
	lzTok := config.GetProviderToken("lazycat")
	lzHealth := ProviderHealth{
		ProviderID:   "lazycat",
		Configured:   lzTok != "",
		LastSyncedAt: store.GetRefreshTime("instances:lazycat"),
	}
	if creds.LazyCat != nil && creds.LazyCat.Password != "" {
		lzHealth.HasCredentials = true
		lzHealth.UsernameMasked = config.MaskUsername(creds.LazyCat.Username)
		lzHealth.AutoRefresh = creds.LazyCat.AutoRefresh
	}
	if lzTok != "" {
		exp, expTime, ok := ParseJWTExp(lzTok)
		if ok {
			lzHealth.ExpiresAt = expTime.Format("2006-01-02 15:04:05")
			lzHealth.SecondsLeft = exp - now
			lzHealth.IsExpired = (lzHealth.SecondsLeft <= 0)
			lzHealth.TokenValid = !lzHealth.IsExpired
		} else {
			lzHealth.TokenValid = true
		}
	}
	if ck, ok := checkinSummary["lazycat"].(map[string]interface{}); ok {
		lzHealth.TodayCheckin = ck
	}
	summary["lazycat"] = lzHealth

	// 3. ACCK
	acckTok := config.GetProviderToken("acck")
	acckHealth := ProviderHealth{
		ProviderID:   "acck",
		Configured:   acckTok != "",
		LastSyncedAt: store.GetRefreshTime("instances:acck"),
	}
	if creds.ACCK != nil && creds.ACCK.Password != "" {
		acckHealth.HasCredentials = true
		acckHealth.UsernameMasked = config.MaskUsername(creds.ACCK.Username)
		acckHealth.AutoRefresh = creds.ACCK.AutoRefresh
	}
	if acckTok != "" {
		exp, expTime, ok := ParseJWTExp(acckTok)
		if ok {
			acckHealth.ExpiresAt = expTime.Format("2006-01-02 15:04:05")
			acckHealth.SecondsLeft = exp - now
			acckHealth.IsExpired = (acckHealth.SecondsLeft <= 0)
			acckHealth.TokenValid = !acckHealth.IsExpired
		} else {
			acckHealth.TokenValid = true
		}
	}
	if ck, ok := checkinSummary["acck"].(map[string]interface{}); ok {
		acckHealth.TodayCheckin = ck
	}
	summary["acck"] = acckHealth

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"providers":    summary,
		"refreshed_at": time.Now().Format("2006-01-02 15:04:05"),
	})
}

// POST /api/vendor/quick-login/{provider} 手动一键免密快速登录 (从凭据库读取自动完成登录)
// POST /api/vendor/quick-login/{provider} 手动一键免密快速登录 (从凭据库读取自动完成登录，缺失时自动从 Vaultwarden 穿透解密同步)
func handleVendorQuickLogin(w http.ResponseWriter, r *http.Request) {
	provider := strings.ToLower(chi.URLParam(r, "provider"))
	creds := config.LoadSavedCredentials()

	switch provider {
	case "akile":
		// 若本地尚未保存凭据，尝试先从 Vaultwarden 自动解密同步
		if creds.Akile == nil || creds.Akile.Username == "" || creds.Akile.Password == "" {
			log.Printf("[QuickLogin] 本地凭据库未找到 Akile，正在尝试从 Vaultwarden (192.168.8.11:4743) 自动解密同步...")
			_, _ = vault.PullAndSyncFromVaultwarden(r.Context(), "", "", "")
			creds = config.LoadSavedCredentials()
		}

		if creds.Akile == nil || creds.Akile.Username == "" || creds.Akile.Password == "" {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error": "未能从 Vaultwarden 或本地凭据库获取 Akile 账号密码，请在凭据设置中输入并保存",
			})
			return
		}

		p, ok := providers.Get("akile")
		if !ok {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "akile provider 未注册"})
			return
		}
		akProvider := p.(*akile.AkileProvider)

		// 检查是否有 2FA 动态码传入；若无且保存了 TOTP 密钥，自动计算实时 6 位动态验证码
		var body struct {
			GACode string `json:"ga_code"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gaCode := strings.TrimSpace(body.GACode)
		if gaCode == "" && creds.Akile.TOTPSecret != "" {
			if autoCode, err := vault.GenerateTOTP(creds.Akile.TOTPSecret); err == nil && autoCode != "" {
				gaCode = autoCode
				log.Printf("[QuickLogin] 自动基于 Vaultwarden TOTP 密钥生成 Akile 2FA 动态码: %s", autoCode)
			}
		}

		tok, need2FA, msg, err := akProvider.Login(r.Context(), creds.Akile.Username, creds.Akile.Password, gaCode)
		if need2FA {
			// 若初次尝试需要 2FA 且之前未带 gaCode，尝试即时生成并再次重试
			if gaCode == "" && creds.Akile.TOTPSecret != "" {
				if autoCode, err2 := vault.GenerateTOTP(creds.Akile.TOTPSecret); err2 == nil && autoCode != "" {
					tok, need2FA, msg, err = akProvider.Login(r.Context(), creds.Akile.Username, creds.Akile.Password, autoCode)
				}
			}
		}
		if need2FA {
			respondJSON(w, http.StatusOK, map[string]interface{}{
				"need_2fa": true,
				"message":  msg,
			})
			return
		}
		if err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "一键登录失败: " + err.Error()})
			return
		}

		_ = config.SaveTokens(tok, "", "", "")
		config.Global.AkileToken = tok
		akProvider.SetToken(tok)

		_, expTime, _ := ParseJWTExp(tok)
		audit.Record("rest_api", "akile.quick_login", "akile", map[string]interface{}{"username": creds.Akile.Username, "auto_totp": gaCode != ""}, "SUCCESS", "用户通过凭据库(及Vaultwarden)一键免密登录 Akile 成功", 0)

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"status":     "success",
			"message":    "Akile 免密登录成功！Token 已自动更新生效",
			"token":      tok,
			"expires_at": expTime.Format("2006-01-02 15:04:05"),
		})

	case "lazycat":
		// 若本地尚未保存凭据，尝试先从 Vaultwarden 自动解密同步
		if creds.LazyCat == nil || creds.LazyCat.Username == "" || creds.LazyCat.Password == "" {
			log.Printf("[QuickLogin] 本地凭据库未找到 LazyCat，正在尝试从 Vaultwarden 自动解密同步...")
			_, _ = vault.PullAndSyncFromVaultwarden(r.Context(), "", "", "")
			creds = config.LoadSavedCredentials()
		}

		if creds.LazyCat == nil || creds.LazyCat.Username == "" || creds.LazyCat.Password == "" {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error": "未能从 Vaultwarden 或本地凭据库获取懒猫云账号密码，请先在凭据设置中输入并保存",
			})
			return
		}

		p, ok := providers.Get("lazycat")
		if !ok {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "lazycat provider 未注册"})
			return
		}
		lzProvider := p.(*lazycat.LazyCatProvider)

		var body struct {
			GACode string `json:"ga_code"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		gaCode := body.GACode
		if gaCode == "" && creds.LazyCat.TOTPSecret != "" {
			if autoCode, err := vault.GenerateTOTP(creds.LazyCat.TOTPSecret); err == nil && autoCode != "" {
				gaCode = autoCode
				log.Printf("[QuickLogin] 自动基于 Vaultwarden/凭据库 TOTP 密钥生成 LazyCat 2FA 动态码: %s", autoCode)
			}
		}

		tok, need2FA, msg, err := lzProvider.Login(r.Context(), creds.LazyCat.Username, creds.LazyCat.Password, gaCode)
		if need2FA {
			respondJSON(w, http.StatusOK, map[string]interface{}{
				"need_2fa": true,
				"message":  msg,
			})
			return
		}
		if err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "一键登录失败: " + err.Error()})
			return
		}

		_ = config.SaveTokens("", "", tok, "")
		config.Global.LazyCatToken = tok
		lzProvider.SetToken(tok)

		_, expTime, _ := ParseJWTExp(tok)
		audit.Record("rest_api", "lazycat.quick_login", "lazycat", map[string]interface{}{"username": creds.LazyCat.Username}, "SUCCESS", "用户通过凭据库一键免密登录懒猫云成功", 0)

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"status":     "success",
			"message":    "懒猫云免密登录成功！Token 已自动更新生效",
			"token":      tok,
			"expires_at": expTime.Format("2006-01-02 15:04:05"),
		})

	case "acck":
		// 若本地尚未保存账号密码，尝试先从 Vaultwarden 自动解密同步
		if creds.ACCK == nil || creds.ACCK.Username == "" || creds.ACCK.Password == "" {
			log.Printf("[QuickLogin] 本地凭据库未找到 ACCK 账密，正在尝试从 Vaultwarden 自动解密同步...")
			_, _ = vault.PullAndSyncFromVaultwarden(r.Context(), "", "", "")
			creds = config.LoadSavedCredentials()
		}

		p, ok := providers.Get("acck")
		if !ok {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "acck provider 未注册"})
			return
		}
		acckProvider := p.(*acck.ACCKProvider)

		// 若密码字段存的是直接 JWT Token (兼容旧式录入)，走 Token 校验分支
		if creds.ACCK != nil && strings.HasPrefix(creds.ACCK.Password, "ey") && strings.Count(creds.ACCK.Password, ".") == 2 {
			targetTok := creds.ACCK.Password
			acckProvider.SetToken(targetTok)
			if err := acckProvider.VerifyToken(r.Context()); err == nil {
				_ = config.SaveTokens("", targetTok, "", "")
				config.SetProviderToken("acck", targetTok)
				_, expTime, _ := ParseJWTExp(targetTok)
				audit.Record("rest_api", "acck.quick_login", "acck", nil, "SUCCESS", "用户触发 ACCK Token 快速校验通过", 0)
				respondJSON(w, http.StatusOK, map[string]interface{}{
					"status":     "success",
					"message":    "ACCK Token 快速校验成功且状态有效",
					"token":      targetTok,
					"expires_at": expTime.Format("2006-01-02 15:04:05"),
				})
				return
			}
		}

		if creds.ACCK == nil || creds.ACCK.Username == "" || creds.ACCK.Password == "" {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error": "未能从 Vaultwarden 或本地凭据库获取 ACCK 账号密码，请先在凭据设置中输入并保存",
			})
			return
		}

		var body struct {
			GACode string `json:"ga_code"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		manualGA := strings.TrimSpace(body.GACode)

		loginWithCred := func(c *config.ProviderCredential) (string, bool, string, error) {
			totpFn := func() (string, error) {
				if manualGA != "" {
					return manualGA, nil
				}
				if strings.TrimSpace(c.TOTPSecret) != "" {
					code, err := vault.GenerateTOTP(c.TOTPSecret)
					if err != nil {
						return "", err
					}
					log.Printf("[QuickLogin] 自动基于 Vaultwarden TOTP 密钥生成 ACCK 2FA 动态码")
					return code, nil
				}
				return "", nil
			}
			loginCtx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
			defer cancel()
			return acckProvider.LoginWithTOTPProvider(loginCtx, c.Username, c.Password, totpFn, false)
		}

		tok, need2FA, msg, err := loginWithCred(creds.ACCK)
		if err != nil && errors.Is(err, acck.ErrInvalidCredentials) {
			oldUser, oldPass := creds.ACCK.Username, creds.ACCK.Password
			if _, vErr := vault.PullAndSyncFromVaultwarden(r.Context(), "", "", ""); vErr == nil {
				refreshed := config.LoadSavedCredentials()
				if refreshed.ACCK != nil && refreshed.ACCK.Password != "" && (refreshed.ACCK.Username != oldUser || refreshed.ACCK.Password != oldPass) {
					creds = refreshed
					tok, need2FA, msg, err = loginWithCred(creds.ACCK)
				}
			}
		}

		if need2FA {
			respondJSON(w, http.StatusOK, map[string]interface{}{
				"need_2fa": true,
				"message":  msg,
			})
			return
		}
		if err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "一键登录失败: " + err.Error()})
			return
		}

		_ = config.SaveTokens("", tok, "", "")
		acckProvider.SetToken(tok)

		_, expTime, _ := ParseJWTExp(tok)
		audit.Record("rest_api", "acck.quick_login", "acck", map[string]interface{}{
			"username":  config.MaskUsername(creds.ACCK.Username),
			"auto_totp": manualGA != "" || strings.TrimSpace(creds.ACCK.TOTPSecret) != "",
		}, "SUCCESS", "用户通过凭据库(及Vaultwarden)一键免密登录 ACCK 成功", 0)

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"status":     "success",
			"message":    "ACCK 免密登录成功！Token 已自动更新生效",
			"token":      tok,
			"expires_at": expTime.Format("2006-01-02 15:04:05"),
		})

	default:
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "未知服务商: " + provider})
	}
}

// POST /api/vendor/vault/sync 手动从 Vaultwarden (192.168.8.11:4743) 触发解密同步至本地凭据库
func handleVaultSync(w http.ResponseWriter, r *http.Request) {
	items, err := vault.PullAndSyncFromVaultwarden(r.Context(), "", "", "")
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("从 Vaultwarden 同步解密失败: %v", err),
		})
		return
	}

	creds := config.LoadSavedCredentials()
	type ItemSummary struct {
		Name     string `json:"name"`
		Username string `json:"username"`
		HasTOTP  bool   `json:"has_totp"`
	}
	var res []ItemSummary
	for _, it := range items {
		res = append(res, ItemSummary{
			Name:     it.Name,
			Username: config.MaskUsername(it.Username),
			HasTOTP:  it.TOTPSecret != "",
		})
	}

	audit.Record("security", "vaultwarden.sync", "vaultwarden", map[string]interface{}{"matched_count": len(items)}, "SUCCESS", fmt.Sprintf("成功从 Vaultwarden 解密提取 %d 家服务商凭据", len(items)), 0)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":        "success",
		"message":       fmt.Sprintf("成功从 Vaultwarden 自动解密提取并更新了 %d 家厂商凭据！", len(items)),
		"matched_count": len(items),
		"items":         res,
		"credentials":   creds,
	})
}

// POST /api/vendor/sync/{provider} 单个服务商独立增量同步
func handleVendorSingleSync(w http.ResponseWriter, r *http.Request) {
	provider := strings.ToLower(chi.URLParam(r, "provider"))
	p, ok := providers.Get(provider)
	if !ok {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "未知服务商: " + provider})
		return
	}

	im, ok := p.(providers.InstanceManager)
	if !ok {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "该服务商不支持实例同步"})
		return
	}

	pCtx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()

	insts, err := im.ListInstances(pCtx)
	// 若遇到 401 掉线，尝试自动重登自愈
	if err != nil && isAuthError(err) {
		log.Printf("[SingleSync] 服务商 %s 同步遇到 401 掉线，尝试自动重登...", provider)
		if store.TryAutoRecoverProvider(pCtx, provider) {
			pCtx2, cancel2 := context.WithTimeout(r.Context(), 35*time.Second)
			insts, err = im.ListInstances(pCtx2)
			cancel2()
		}
	}

	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("同步失败: %v", err)})
		return
	}

	nowStr := store.RecordRefreshTime("instances:" + provider)
	_ = store.UpdateProviderInstancesInCache(provider, insts)
	audit.Record("rest_api", "vendor.single_sync", provider, map[string]interface{}{"count": len(insts)}, "SUCCESS", fmt.Sprintf("单服务商 %s 强制同步成功，获取 %d 台实例", provider, len(insts)), 0)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":         "success",
		"provider_id":    provider,
		"instances":      insts,
		"count":          len(insts),
		"last_synced_at": nowStr,
	})
}

// POST /api/vendor/credentials/save 保存或更新服务商免密凭据
func handleSaveProviderCredentials(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider    string `json:"provider"`
		Username    string `json:"username"`
		Password    string `json:"password"`
		AutoRefresh bool   `json:"auto_refresh"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的请求参数"})
		return
	}

	prov := strings.ToLower(strings.TrimSpace(req.Provider))
	if prov != "akile" && prov != "lazycat" && prov != "acck" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "不支持的服务商: " + prov})
		return
	}

	if err := config.SaveProviderCredential(prov, req.Username, req.Password, req.AutoRefresh); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "保存凭据失败: " + err.Error()})
		return
	}

	audit.Record("rest_api", "vendor.save_credentials", prov, map[string]interface{}{
		"username":     config.MaskUsername(req.Username),
		"auto_refresh": req.AutoRefresh,
	}, "SUCCESS", fmt.Sprintf("成功保存服务商 %s 的免密登录账号凭据至凭据库", prov), 0)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": fmt.Sprintf("服务商 %s 账号凭据已安全保存至凭据库，已启用免密登录与掉线自愈！", prov),
	})
}

// POST /api/vendor/vault/ciphers 获取 Vaultwarden 中解密后的所有密码条目摘要
func handleVaultListCiphers(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Server   string `json:"server"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	server := strings.TrimSpace(req.Server)
	username := strings.TrimSpace(req.Username)
	password := req.Password

	// 如果传入了 server 或 username，静默记住为默认值
	if server != "" || username != "" {
		_ = config.SaveVaultwardenConfig(server, username)
	}

	ciphers, err := vault.ListVaultCiphers(r.Context(), server, username, password)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("连接 Vaultwarden 失败: %v", err),
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"ciphers": ciphers,
		"count":   len(ciphers),
	})
}

// POST /api/vendor/vault/bind 从密码库中选中具体一条 Cipher，测试登录并绑定至指定厂商
func handleVaultBindCipher(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
		CipherID string `json:"cipher_id"`
		Server   string `json:"server"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.CipherID == "" || req.Provider == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "请提供有效的 provider 和 cipher_id",
		})
		return
	}

	prov := strings.ToLower(strings.TrimSpace(req.Provider))
	p, ok := providers.Get(prov)
	if !ok {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "不支持的服务商: " + prov,
		})
		return
	}

	item, err := vault.GetVaultCipherByID(r.Context(), req.Server, req.Username, req.Password, req.CipherID)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("提取条目详情失败: %v", err),
		})
		return
	}

	// 执行真实测试登录与绑定
	token, expStr, err := executeProviderLoginTest(r.Context(), prov, p, item.Username, item.Password, item.TOTPSecret)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("测试登录失败: %v", err),
		})
		return
	}

	// 登录测试成功，持久化保存凭据
	_ = config.SaveProviderCredentialWithTOTP(prov, item.Username, item.Password, item.TOTPSecret, true)
	if req.Server != "" || req.Username != "" {
		_ = config.SaveVaultwardenConfig(req.Server, req.Username)
	}

	audit.Record("security", "vendor.vault_bind", prov, map[string]interface{}{
		"cipher_id": req.CipherID,
		"name":      item.Name,
		"username":  config.MaskUsername(item.Username),
		"has_totp":  item.TOTPSecret != "",
	}, "SUCCESS", fmt.Sprintf("成功从 Vaultwarden 绑定条目 [%s] 至服务商 %s 并完成测试登录", item.Name, prov), 0)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":          "success",
		"message":         fmt.Sprintf("成功关联条目 [%s]！测试登录通过，已开启免密自愈。", item.Name),
		"token":           token,
		"expires_at":      expStr,
		"username_masked": config.MaskUsername(item.Username),
	})
}

// POST /api/vendor/test-and-save 手动账号密码 + 2FA 原始代码 (TOTP Secret) 测试并保存
func handleVendorTestAndSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider    string `json:"provider"`
		Username    string `json:"username"`
		Password    string `json:"password"`
		TOTPSecret  string `json:"totp_secret"`
		AutoRefresh *bool  `json:"auto_refresh"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的请求体"})
		return
	}

	prov := strings.ToLower(strings.TrimSpace(req.Provider))
	p, ok := providers.Get(prov)
	if !ok {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "不支持的服务商: " + prov})
		return
	}

	username := strings.TrimSpace(req.Username)
	password := req.Password
	totpSecret := strings.TrimSpace(req.TOTPSecret)
	autoRefresh := true
	if req.AutoRefresh != nil {
		autoRefresh = *req.AutoRefresh
	}

	if username == "" || password == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "用户名和密码不能为空"})
		return
	}

	// 执行真实测试登录
	token, expStr, err := executeProviderLoginTest(r.Context(), prov, p, username, password, totpSecret)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("测试登录失败: %v", err),
		})
		return
	}

	// 测试登录成功，保存至凭据库
	_ = config.SaveProviderCredentialWithTOTP(prov, username, password, totpSecret, autoRefresh)

	audit.Record("security", "vendor.test_and_save", prov, map[string]interface{}{
		"username": config.MaskUsername(username),
		"has_totp": totpSecret != "",
	}, "SUCCESS", fmt.Sprintf("服务商 %s 手动测试登录成功并已安全存入凭据库", prov), 0)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":          "success",
		"message":         fmt.Sprintf("测试登录成功！Token 已自动更新生效，有效期至 %s", expStr),
		"token":           token,
		"expires_at":      expStr,
		"username_masked": config.MaskUsername(username),
	})
}

// executeProviderLoginTest 统一核心：向服务商发起真实测试登录验证
func executeProviderLoginTest(ctx context.Context, provider string, p providers.Provider, username, password, totpSecret string) (token string, expStr string, err error) {
	switch provider {
	case "akile":
		ak, ok := p.(*akile.AkileProvider)
		if !ok {
			return "", "", errors.New("Akile 驱动异常")
		}
		gaCode := ""
		if totpSecret != "" {
			code, err := vault.GenerateTOTP(totpSecret)
			if err != nil {
				return "", "", fmt.Errorf("2FA 原始代码计算动态码失败: %w", err)
			}
			gaCode = code
		}
		tok, need2FA, msg, err := ak.Login(ctx, username, password, gaCode)
		if need2FA {
			return "", "", errors.New("该账号已开启两步验证，请在 2FA 原始代码框中填入密钥 (TOTP Secret) 进行绑定")
		}
		if err != nil {
			return "", "", fmt.Errorf("Akile 登录失败: %w", err)
		}
		if tok == "" {
			return "", "", fmt.Errorf("登录未返回有效 Token: %s", msg)
		}

		_ = config.SaveTokens(tok, "", "", "")
		config.Global.AkileToken = tok
		ak.SetToken(tok)

		_, expTime, ok := ParseJWTExp(tok)
		if ok && !expTime.IsZero() {
			expStr = expTime.Format("2006-01-02 15:04:05")
		}
		return tok, expStr, nil

	case "lazycat":
		lz, ok := p.(*lazycat.LazyCatProvider)
		if !ok {
			return "", "", errors.New("懒猫云驱动异常")
		}
		gaCode := ""
		if totpSecret != "" {
			code, err := vault.GenerateTOTP(totpSecret)
			if err != nil {
				return "", "", fmt.Errorf("2FA 原始代码计算动态码失败: %w", err)
			}
			gaCode = code
		}
		tok, need2FA, msg, err := lz.Login(ctx, username, password, gaCode)
		if need2FA {
			return "", "", errors.New("该账号已开启两步验证，请输入 2FA 原始代码密钥 (TOTP Secret)")
		}
		if err != nil {
			return "", "", fmt.Errorf("懒猫云登录失败: %w", err)
		}
		if tok == "" {
			return "", "", fmt.Errorf("登录未返回有效 Token: %s", msg)
		}

		_ = config.SaveTokens("", "", tok, "")
		config.Global.LazyCatToken = tok
		lz.SetToken(tok)

		_, expTime, ok := ParseJWTExp(tok)
		if ok && !expTime.IsZero() {
			expStr = expTime.Format("2006-01-02 15:04:05")
		}
		return tok, expStr, nil

	case "acck":
		acckP, ok := p.(*acck.ACCKProvider)
		if !ok {
			return "", "", errors.New("ACCK 驱动异常")
		}
		// 若密码字段直接填入的是 JWT，兼容走校验保存
		if strings.HasPrefix(password, "ey") && strings.Count(password, ".") == 2 {
			tok := password
			acckP.SetToken(tok)
			if err := acckP.VerifyToken(ctx); err != nil {
				return "", "", fmt.Errorf("ACCK Token 校验失败: %w", err)
			}
			_ = config.SaveTokens("", tok, "", "")
			config.SetProviderToken("acck", tok)
			_, expTime, ok := ParseJWTExp(tok)
			if ok && !expTime.IsZero() {
				expStr = expTime.Format("2006-01-02 15:04:05")
			}
			return tok, expStr, nil
		}

		totpFn := func() (string, error) {
			if strings.TrimSpace(totpSecret) == "" {
				return "", nil
			}
			code, err := vault.GenerateTOTP(totpSecret)
			if err != nil {
				return "", fmt.Errorf("2FA 原始代码计算动态码失败: %w", err)
			}
			return code, nil
		}
		loginCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
		defer cancel()

		tok, need2FA, msg, err := acckP.LoginWithTOTPProvider(loginCtx, username, password, totpFn, false)
		if need2FA {
			return "", "", errors.New("该账号已开启两步验证，请在 2FA 原始代码框中填入密钥 (TOTP Secret) 进行绑定")
		}
		if err != nil {
			return "", "", fmt.Errorf("ACCK 登录失败: %w", err)
		}
		if tok == "" {
			return "", "", fmt.Errorf("登录未返回有效 Token: %s", msg)
		}

		_ = config.SaveTokens("", tok, "", "")
		config.SetProviderToken("acck", tok)
		acckP.SetToken(tok)

		_, expTime, ok := ParseJWTExp(tok)
		if ok && !expTime.IsZero() {
			expStr = expTime.Format("2006-01-02 15:04:05")
		}
		return tok, expStr, nil
	}

	return "", "", fmt.Errorf("未知服务商: %s", provider)
}
