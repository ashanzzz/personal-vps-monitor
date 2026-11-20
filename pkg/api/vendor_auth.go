package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/audit"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/config"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/scheduler"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/store"
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
	akTok := config.Global.AkileToken
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
	lzTok := config.Global.LazyCatToken
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
	acckTok := config.Global.ACCKToken
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
func handleVendorQuickLogin(w http.ResponseWriter, r *http.Request) {
	provider := strings.ToLower(chi.URLParam(r, "provider"))
	creds := config.LoadSavedCredentials()

	switch provider {
	case "akile":
		if creds.Akile == nil || creds.Akile.Username == "" || creds.Akile.Password == "" {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error": "尚未保存 Akile 账号密码凭据，请先在凭据设置中输入并保存",
			})
			return
		}

		p, ok := providers.Get("akile")
		if !ok {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "akile provider 未注册"})
			return
		}
		akProvider := p.(*akile.AkileProvider)

		// 检查是否有 2FA 动态码传入 (可选)
		var body struct {
			GACode string `json:"ga_code"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		tok, need2FA, msg, err := akProvider.Login(r.Context(), creds.Akile.Username, creds.Akile.Password, body.GACode)
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
		audit.Record("rest_api", "akile.quick_login", "akile", map[string]interface{}{"username": creds.Akile.Username}, "SUCCESS", "用户通过凭据库一键免密登录 Akile 成功", 0)

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"status":     "success",
			"message":    "Akile 免密登录成功！Token 已自动更新生效",
			"token":      tok,
			"expires_at": expTime.Format("2006-01-02 15:04:05"),
		})

	case "lazycat":
		if creds.LazyCat == nil || creds.LazyCat.Username == "" || creds.LazyCat.Password == "" {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error": "尚未保存懒猫云账号密码凭据，请先在凭据设置中输入并保存",
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

		tok, need2FA, msg, err := lzProvider.Login(r.Context(), creds.LazyCat.Username, creds.LazyCat.Password, body.GACode)
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
		// 如果 ACCK 凭据库已保存 Token 或密码
		p, ok := providers.Get("acck")
		if !ok {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "acck provider 未注册"})
			return
		}
		acckProvider := p.(*acck.ACCKProvider)

		targetTok := config.Global.ACCKToken
		if targetTok == "" && creds.ACCK != nil && creds.ACCK.Password != "" {
			targetTok = creds.ACCK.Password
		}
		if targetTok == "" {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "尚未配置 ACCK Token，请先录入"})
			return
		}

		acckProvider.SetToken(targetTok)
		if err := acckProvider.VerifyToken(r.Context()); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "ACCK 凭据校验失败: " + err.Error()})
			return
		}

		_ = config.SaveTokens("", targetTok, "", "")
		config.Global.ACCKToken = targetTok

		_, expTime, _ := ParseJWTExp(targetTok)
		audit.Record("rest_api", "acck.quick_login", "acck", nil, "SUCCESS", "用户触发 ACCK 凭据快速校验通过", 0)

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"status":     "success",
			"message":    "ACCK 凭据快速校验成功且状态有效",
			"token":      targetTok,
			"expires_at": expTime.Format("2006-01-02 15:04:05"),
		})

	default:
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "未知服务商: " + provider})
	}
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

	pCtx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	insts, err := im.ListInstances(pCtx)
	// 若遇到 401 掉线，尝试自动重登自愈
	if err != nil && (strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "token") || strings.Contains(err.Error(), "unauthorized")) {
		log.Printf("[SingleSync] 服务商 %s 同步遇到 401 掉线，尝试自动重登...", provider)
		if store.TryAutoRecoverProvider(pCtx, provider) {
			pCtx2, cancel2 := context.WithTimeout(r.Context(), 20*time.Second)
			insts, err = im.ListInstances(pCtx2)
			cancel2()
		}
	}

	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("同步失败: %v", err)})
		return
	}

	nowStr := store.RecordRefreshTime("instances:" + provider)
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
