package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/audit"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/config"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/scheduler"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/store"
	"github.com/ashanzzz/personal-vps-monitor/pkg/mcp"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter() http.Handler {
	// 初始化后台智能巡检调度器
	scheduler.Init()

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(SecurityMiddleware)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "healthy",
			"time":   time.Now().Format(time.RFC3339),
		})
	})

	r.Get("/version", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"service": "personal-vps-monitor",
			"version": "v2.0.0-modular",
			"arch":    "go-static-headless",
		})
	})

	// 自动挂载 AI 原生 MCP SSE 协议端点 (/sse, /message) 并强制身份鉴权拦截
	if srv, err := mcp.New(); err == nil {
		srv.AttachSSE(r, RequireAuthMiddleware)
	} else {
		log.Printf("[Router] Warning: failed to mount MCP SSE endpoints: %v", err)
	}

	r.Route("/api", func(r chi.Router) {
		// 管理员认证公开接口 (/api/auth/status, /api/auth/login, /api/auth/setup, /api/auth/logout)
		registerAuthRoutes(r)

		// 业务接口全量身份拦截保护 (无论是局域网还是公网，未认证一律 401 拦截)
		r.Group(func(r chi.Router) {
			r.Use(RequireAuthMiddleware)

			// 密码修改接口
			r.Post("/auth/change-password", handleChangePassword)

			// 网关控制端点
			registerGatewayRoutes(r)

			// 基础系统与告警兼容端点
			r.Get("/system/unraid-status", func(w http.ResponseWriter, r *http.Request) {
				report := config.ProbeUnraidContainers(r.Context())
				respondJSON(w, http.StatusOK, report)
			})
			r.Get("/events", func(w http.ResponseWriter, r *http.Request) {
				respondJSON(w, http.StatusOK, []interface{}{})
			})
			r.Get("/settings", func(w http.ResponseWriter, r *http.Request) {
				respondJSON(w, http.StatusOK, map[string]interface{}{
					"refresh_interval":   60,
					"auto_refresh_token": true,
					"request_timeout":    10,
					"request_retries":    3,
				})
			})
			// 智能巡检调度器状态与极速突发控制
			r.Get("/scheduler/status", handleGetSchedulerStatus)
			r.Get("/refresh/status", handleGetSchedulerStatus) // 兼容旧接口
			r.Post("/scheduler/burst", handleTriggerBurst)

			// 购买计划与智能狙击规则 (Purchase Plans)
			r.Get("/plans", handleListPlans)
			r.Post("/plans", handleCreatePlan)
			r.Put("/plans/{id}", handleUpdatePlan)
			r.Delete("/plans/{id}", handleDeletePlan)
			r.Post("/plans/{id}/toggle", handleTogglePlan)
			r.Post("/plans/buy-item", handleBuyMarketItemDirect)

			// 市场宏观数据与历史走势
			r.Get("/market/stats", handleGetMarketStats)

			// 查询已激活驱动列表
			r.Get("/providers", handleListProviders)

			// 聚合查询全网实例
			r.Get("/instances", handleUnifiedInstances)

			// 聚合查询二手集市转让商品
			r.Get("/market", handleUnifiedMarket)

			// 官方新机在线商店
			r.Get("/store", handleUnifiedStore)

			// 全局模块刷新时间戳
			r.Get("/system/refresh-times", handleGetRefreshTimes)

			// 批量触发每日签到 / 指定厂商单点签到
			r.Post("/checkin", handleUnifiedCheckin)
			r.Post("/checkin/{provider}", handleSingleCheckin)

			// 行为审计日志接口 (查询过去 30 天)
			r.Get("/audit", handleGetAudit)
			r.Post("/audit/prune", handlePruneAudit)

			// 厂商 API 凭证与 Token 配置热加载
			r.Get("/settings/tokens", handleGetTokens)
			r.Post("/settings/tokens", handleSaveTokens)

			// 厂商快捷登录与免密鉴权接口 (支持 2FA 动态码与免密自动续期)
			r.Post("/akile/login", handleAkileLogin)
			r.Post("/akile/refresh-token", handleAkileRefreshToken)
			r.Post("/lazycat/login", handleLazyCatLogin)
			r.Post("/acck/login", handleAcckLogin)

			// 极速鉴权健康摘要、一键免密登录、单服务独立同步与凭据管理
			r.Get("/vendor/health-summary", handleVendorHealthSummary)
			r.Post("/vendor/quick-login/{provider}", handleVendorQuickLogin)
			r.Post("/vendor/sync/{provider}", handleVendorSingleSync)
			r.Post("/vendor/credentials/save", handleSaveProviderCredentials)
			r.Post("/vendor/vault/sync", handleVaultSync)
			r.Post("/vendor/vault/ciphers", handleVaultListCiphers)
			r.Post("/vendor/vault/bind", handleVaultBindCipher)
			r.Post("/vendor/test-and-save", handleVendorTestAndSave)

			// 懒猫云特化端点
			r.Route("/lazycat", func(r chi.Router) {
				r.Get("/instances", handleLazyCatInstances)
				r.Get("/ports", handleLazyCatPorts)
				r.Post("/ports", handleLazyCatAddPort)
				r.Delete("/ports/{id}", handleLazyCatDeletePort)
				r.Get("/detail", handleLazyCatInstanceDetail)
				r.Get("/terminal/ws", handleLazyCatTerminalWS)
				r.Post("/terminal/exec", handleLazyCatTerminalExec)
				r.Get("/fleet-probe", handleLazyCatFleetProbe)
				r.Get("/container/egress-nodes", handleLazyCatEgressNodes)
				r.Post("/sync", handleLazyCatSync)
				r.Post("/checkin", handleLazyCatCheckin)
			})

			// 普通独立主机代理生命周期管理与 SSH 执行引擎 (REST API & 链式代理导出)
			registerInstanceExecRoutes(r)
		})
	})

	// 挂载 React SPA 前端静态服务 (自动 fallback 至 index.html)
	mountFrontend(r)

	return r
}

func handleListProviders(w http.ResponseWriter, r *http.Request) {
	all := providers.GetAll()
	type item struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	res := make([]item, 0, len(all))
	for _, p := range all {
		res = append(res, item{ID: p.ID(), Name: p.Name()})
	}
	respondJSON(w, http.StatusOK, res)
}

func handleUnifiedInstances(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	targetProvider := r.URL.Query().Get("provider")
	forceRefresh := r.URL.Query().Get("refresh") == "true" || r.URL.Query().Get("force") == "true"

	cache, err := store.SyncAllInstancesDaily(ctx, forceRefresh)
	if err != nil && cache == nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "获取每日资产缓存失败: " + err.Error()})
		return
	}

	ident := GetIdentity(ctx)
	isReadOnly := ident != nil && ident.Role == "read"

	var filtered []providers.Instance
	for _, inst := range cache.Instances {
		if targetProvider != "" && targetProvider != "all" && !strings.EqualFold(inst.ProviderID, targetProvider) {
			continue
		}
		if isReadOnly {
			inst.SSHPassword = ""  // 只读密钥自动脱敏，不泄漏 root 密码
			inst.RootPassword = "" // 彻底抹除 root 初始密码
		}
		filtered = append(filtered, inst)
	}

	providerErrors := make(map[string]string)
	for id, st := range cache.ProviderStatus {
		if st.Status == "error" {
			providerErrors[id] = st.Message
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"total":                  len(filtered),
		"instances":              filtered,
		"provider_status":        cache.ProviderStatus,
		"provider_errors":        providerErrors,
		"last_synced_at":         cache.LastSyncedAt,
		"next_scheduled_sync":    cache.NextScheduledSync,
		"refreshed_at":           time.Now().Format("2006-01-02 15:04:05"),
		"provider_refresh_times": store.GetAllRefreshTimes(),
	})
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "401") || strings.Contains(s, "token") || strings.Contains(s, "unauthorized") || strings.Contains(s, "token无效")
}

func handleUnifiedMarket(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	targetProvider := r.URL.Query().Get("provider")
	all := providers.GetAll()
	var results []providers.MarketItem
	var rawItems []store.UpsertItemInput
	providerErrors := make(map[string]string)

	for _, p := range all {
		if targetProvider != "" && targetProvider != "all" && !strings.EqualFold(p.ID(), targetProvider) {
			continue
		}
		if ml, ok := p.(providers.MarketLister); ok {
			pCtx, pCancel := context.WithTimeout(ctx, 15*time.Second)
			items, err := ml.ListMarketItems(pCtx)
			pCancel()

			// 遇到 401 / Token 失效时触发自动免密重登自愈
			if err != nil && isAuthError(err) {
				log.Printf("[Market] provider %s encountered auth error: %v, triggering auto-relogin...", p.ID(), err)
				if store.TryAutoRecoverProvider(ctx, p.ID()) {
					pCtx2, pCancel2 := context.WithTimeout(ctx, 15*time.Second)
					items, err = ml.ListMarketItems(pCtx2)
					pCancel2()
				}
			}

			if err != nil {
				log.Printf("[Market] provider %s failed to list live market: %v (falling back to cached store)", p.ID(), err)
				providerErrors[p.ID()] = err.Error()
				cached := store.GetActiveMarketRecords(p.ID())
				for _, rec := range cached {
					diff, tag := store.CalculateBargainIndex(rec.Name, rec.Price)
					results = append(results, providers.MarketItem{
						ProviderID:      rec.Provider,
						ID:              rec.ID,
						Name:            rec.Name,
						Price:           rec.Price,
						RenewalPrice:    rec.RenewalPrice,
						DiscountPercent: rec.DiscountPercent,
						ResetPrice:      rec.ResetPrice,
						PlatformFee:     rec.PlatformFee,
						FlowUsed:        rec.FlowUsed,
						Cycle:           rec.Cycle,
						Location:        rec.Location,
						Bandwidth:       rec.Bandwidth,
						ConfigDesc:      rec.ConfigDesc,
						CPU:             rec.CPU,
						Memory:          rec.Memory,
						Disk:            rec.Disk,
						TrafficTotal:    rec.TrafficTotal,
						Detail:          rec.Detail,
						IPCheckDetail:   rec.IPCheckDetail,
						DueTime:         rec.DueTime,
						SLA:             rec.SLA,
						CanBuy:          rec.CanBuy,
						PortLimit:       rec.PortLimit,
						SellerUsername:  rec.SellerUsername,
						SellerUserID:    rec.SellerUserID,
						MarketType:      rec.MarketType,
						CreatedAt:       rec.LastSeenAt,
						DiffPercent:     diff,
						BargainTag:      tag,
					})
				}
			} else {
				store.RecordRefreshTime("market:" + p.ID())
				for _, it := range items {
					diff, tag := store.CalculateBargainIndex(it.Name, it.Price)
					it.DiffPercent = diff
					it.BargainTag = tag
					results = append(results, it)
					rawItems = append(rawItems, store.UpsertItemInput{
						ID:              it.ID,
						Provider:        it.ProviderID,
						Name:            it.Name,
						Price:           it.Price,
						RenewalPrice:    it.RenewalPrice,
						DiscountPercent: it.DiscountPercent,
						ResetPrice:      it.ResetPrice,
						PlatformFee:     it.PlatformFee,
						FlowUsed:        it.FlowUsed,
						Cycle:           it.Cycle,
						Location:        it.Location,
						Bandwidth:       it.Bandwidth,
						ConfigDesc:      it.ConfigDesc,
						CPU:             it.CPU,
						Memory:          it.Memory,
						Disk:            it.Disk,
						TrafficTotal:    it.TrafficTotal,
						Detail:          it.Detail,
						IPCheckDetail:   it.IPCheckDetail,
						DueTime:         it.DueTime,
						SLA:             it.SLA,
						CanBuy:          it.CanBuy,
						PortLimit:       it.PortLimit,
						SellerUsername:  it.SellerUsername,
						SellerUserID:    it.SellerUserID,
						MarketType:      it.MarketType,
						BargainTag:      it.BargainTag,
						DiffPercent:     it.DiffPercent,
					})
				}
			}
		}
	}

	if len(rawItems) > 0 {
		store.UpsertMarketBatch(rawItems)
	}
	if targetProvider == "" || targetProvider == "all" {
		store.RecordRefreshTime("market:all")
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"total":                  len(results),
		"items":                  results,
		"provider_errors":        providerErrors,
		"refreshed_at":           time.Now().Format(time.RFC3339),
		"provider_refresh_times": store.GetAllRefreshTimes(),
	})
}

type StoreProductItem = providers.StoreProduct

func handleUnifiedStore(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	var products []providers.StoreProduct

	// 动态遍历实现了 StoreLister 的所有服务商 (懒猫云 42 款、Akile 179 款全量真实动态获取)
	all := providers.GetAll()
	for _, p := range all {
		if provider != "" && provider != "all" && !strings.EqualFold(p.ID(), provider) {
			continue
		}
		if sl, ok := p.(providers.StoreLister); ok {
			pCtx, pCancel := context.WithTimeout(r.Context(), 8*time.Second)
			items, err := sl.ListStoreProducts(pCtx)
			pCancel()

			if err != nil && isAuthError(err) {
				log.Printf("[Store] provider %s encountered auth error: %v, triggering auto-relogin...", p.ID(), err)
				if store.TryAutoRecoverProvider(r.Context(), p.ID()) {
					pCtx2, pCancel2 := context.WithTimeout(r.Context(), 8*time.Second)
					items, err = sl.ListStoreProducts(pCtx2)
					pCancel2()
				}
			}

			if err == nil {
				products = append(products, items...)
				store.RecordRefreshTime("store:" + p.ID())
			} else {
				log.Printf("[Store] provider %s failed to list store products: %v", p.ID(), err)
			}
		}
	}

	if provider == "" || provider == "all" {
		store.RecordRefreshTime("store:all")
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"total":                  len(products),
		"products":               products,
		"refreshed_at":           time.Now().Format(time.RFC3339),
		"provider_refresh_times": store.GetAllRefreshTimes(),
	})
}

func handleGetRefreshTimes(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, store.GetAllRefreshTimes())
}

func handleUnifiedCheckin(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	all := providers.GetAll()
	results := make(map[string]*providers.CheckinResult)

	for id, p := range all {
		if ac, ok := p.(providers.AutoCheckin); ok {
			start := time.Now()
			res, err := ac.DoCheckin(ctx)
			dur := time.Since(start).Milliseconds()
			if err != nil {
				results[id] = &providers.CheckinResult{
					Success: false,
					Message: err.Error(),
				}
				scheduler.GetCheckinManager().RecordResult(id, nil, err)
				audit.Record("rest_api", "checkin", id, nil, "FAILED", err.Error(), dur)
			} else {
				results[id] = res
				scheduler.GetCheckinManager().RecordResult(id, res, nil)
				audit.Record("rest_api", "checkin", id, map[string]interface{}{"account": res.Account, "reward": res.Reward, "website": res.Website}, "SUCCESS", res.Message, dur)
			}
		}
	}

	respondJSON(w, http.StatusOK, results)
}

func handleSingleCheckin(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "provider")
	p, ok := providers.Get(id)
	if !ok {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "provider not found: " + id})
		return
	}
	ac, ok := p.(providers.AutoCheckin)
	if !ok {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "provider does not support checkin"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	start := time.Now()
	res, err := ac.DoCheckin(ctx)
	dur := time.Since(start).Milliseconds()
	if err != nil {
		scheduler.GetCheckinManager().RecordResult(id, nil, err)
		audit.Record("rest_api", "checkin", id, nil, "FAILED", err.Error(), dur)
		respondJSON(w, http.StatusOK, &providers.CheckinResult{
			Success: false,
			Message: err.Error(),
		})
		return
	}
	scheduler.GetCheckinManager().RecordResult(id, res, nil)
	audit.Record("rest_api", "checkin", id, map[string]interface{}{"account": res.Account, "reward": res.Reward, "website": res.Website}, "SUCCESS", res.Message, dur)
	respondJSON(w, http.StatusOK, res)
}

func handleGetAudit(w http.ResponseWriter, r *http.Request) {
	days := 7
	if dStr := r.URL.Query().Get("days"); dStr != "" {
		if d, err := strconv.Atoi(dStr); err == nil && d > 0 {
			days = d
		}
	}
	limit := 50
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}
	source := r.URL.Query().Get("source")
	provider := r.URL.Query().Get("provider")
	status := r.URL.Query().Get("status")

	entries, err := audit.Query(audit.QueryOptions{
		Days:     days,
		Limit:    limit,
		Source:   source,
		Provider: provider,
		Status:   status,
	})
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"total":   len(entries),
		"entries": entries,
	})
}

func handlePruneAudit(w http.ResponseWriter, r *http.Request) {
	deleted, err := audit.PruneLogs()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"deleted": deleted,
	})
}

func handleLazyCatInstances(w http.ResponseWriter, r *http.Request) {
	p, ok := providers.Get("lazycat")
	if !ok {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "lazycat provider not found"})
		return
	}
	im, ok := p.(providers.InstanceManager)
	if !ok {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "provider does not support instance listing"})
		return
	}
	insts, err := im.ListInstances(r.Context())
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	ident := GetIdentity(r.Context())
	if ident != nil && ident.Role == "read" {
		for i := range insts {
			insts[i].SSHPassword = ""
			insts[i].RootPassword = ""
		}
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"total":     len(insts),
		"instances": insts,
	})
}

func handleLazyCatSync(w http.ResponseWriter, r *http.Request) {
	p, ok := providers.Get("lazycat")
	if !ok {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "lazycat provider not found"})
		return
	}
	es, ok := p.(providers.EgressSyncer)
	if !ok {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "provider does not support egress syncing"})
		return
	}
	start := time.Now()
	if err := es.SyncEgress(r.Context()); err != nil {
		dur := time.Since(start).Milliseconds()
		audit.Record("rest_api", "lazycat.egress_sync", "lazycat", nil, "FAILED", err.Error(), dur)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	dur := time.Since(start).Milliseconds()
	audit.Record("rest_api", "lazycat.egress_sync", "lazycat", nil, "SUCCESS", "12家宽出口轮询与sing-box热重载完成", dur)
	respondJSON(w, http.StatusOK, map[string]string{"status": "success", "message": "12家宽出口轮询与sing-box热重载完成"})
}

func handleLazyCatCheckin(w http.ResponseWriter, r *http.Request) {
	p, ok := providers.Get("lazycat")
	if !ok {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "lazycat provider not found"})
		return
	}
	ac, ok := p.(providers.AutoCheckin)
	if !ok {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "provider does not support checkin"})
		return
	}
	start := time.Now()
	res, err := ac.DoCheckin(r.Context())
	dur := time.Since(start).Milliseconds()
	if err != nil {
		audit.Record("rest_api", "lazycat.checkin", "lazycat", nil, "FAILED", err.Error(), dur)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	audit.Record("rest_api", "lazycat.checkin", "lazycat", map[string]interface{}{"account": res.Account, "reward": res.Reward, "website": res.Website}, "SUCCESS", res.Message, dur)
	respondJSON(w, http.StatusOK, res)
}

func maskToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) <= 6 {
		return "******"
	}
	return s[:3] + "******" + s[len(s)-3:]
}

func handleGetTokens(w http.ResponseWriter, r *http.Request) {
	st := config.LoadSavedTokens()
	creds := config.LoadSavedCredentials()

	akile := st.AkileToken
	if akile == "" {
		akile = config.Global.AkileToken
	}
	acck := st.ACCKToken
	if acck == "" {
		acck = config.Global.ACCKToken
	}
	lazycat := st.LazyCatToken
	if lazycat == "" {
		lazycat = config.Global.LazyCatToken
	}
	proxy := st.ProxyURL
	if proxy == "" {
		proxy = config.Global.ProxyURL
	}

	// 解析 Akile Token 状态与到期时间
	_, akileExp, akileHasExp := ParseJWTExp(akile)
	akileExpired := akileHasExp && time.Now().After(akileExp)
	akileSecLeft := int64(0)
	akileExpStr := ""
	if akileHasExp {
		akileExpStr = akileExp.Format("2006-01-02 15:04:05")
		if !akileExpired {
			akileSecLeft = int64(time.Until(akileExp).Seconds())
		}
	}

	// 解析 ACCK Token 状态与到期时间
	_, acckExp, acckHasExp := ParseJWTExp(acck)
	acckExpired := acckHasExp && time.Now().After(acckExp)
	acckSecLeft := int64(0)
	acckExpStr := ""
	if acckHasExp {
		acckExpStr = acckExp.Format("2006-01-02 15:04:05")
		if !acckExpired {
			acckSecLeft = int64(time.Until(acckExp).Seconds())
		}
	}

	// 解析 LazyCat Token 状态与到期时间
	_, lzExp, lzHasExp := ParseJWTExp(lazycat)
	lzExpired := lzHasExp && time.Now().After(lzExp)
	lzSecLeft := int64(0)
	lzExpStr := ""
	if lzHasExp {
		lzExpStr = lzExp.Format("2006-01-02 15:04:05")
		if !lzExpired {
			lzSecLeft = int64(time.Until(lzExp).Seconds())
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"akile_configured":        akile != "",
		"akile_token_masked":      maskToken(akile),
		"akile_expires_at":        akileExpStr,
		"akile_is_expired":        akileExpired,
		"akile_seconds_left":      akileSecLeft,
		"akile_has_credentials":   creds.Akile != nil && creds.Akile.Username != "",
		"akile_auto_refresh":      creds.Akile != nil && creds.Akile.AutoRefresh,

		"acck_configured":         acck != "",
		"acck_token_masked":       maskToken(acck),
		"acck_expires_at":         acckExpStr,
		"acck_is_expired":         acckExpired,
		"acck_seconds_left":       acckSecLeft,
		"acck_has_credentials":    creds.ACCK != nil && creds.ACCK.Username != "",

		"lazycat_configured":      lazycat != "",
		"lazycat_token_masked":    maskToken(lazycat),
		"lazycat_expires_at":      lzExpStr,
		"lazycat_is_expired":      lzExpired,
		"lazycat_seconds_left":    lzSecLeft,
		"lazycat_has_credentials": creds.LazyCat != nil && creds.LazyCat.Username != "",
		"lazycat_auto_refresh":    creds.LazyCat != nil && creds.LazyCat.AutoRefresh,

		"proxy_url":               proxy,
	})
}

type SaveTokensRequest struct {
	AkileToken   string `json:"akile_token"`
	ACCKToken    string `json:"acck_token"`
	LazyCatToken string `json:"lazycat_token"`
	ProxyURL     string `json:"proxy_url"`
}

func handleSaveTokens(w http.ResponseWriter, r *http.Request) {
	var req SaveTokensRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的 JSON 请求体"})
		return
	}

	start := time.Now()
	if err := config.SaveTokens(req.AkileToken, req.ACCKToken, req.LazyCatToken, req.ProxyURL); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "保存 Token 失败: " + err.Error()})
		return
	}

	// 动态重载所有 Provider 驱动
	_ = providers.InitAll(r.Context(), config.Global)

	dur := time.Since(start).Milliseconds()
	audit.Record("rest_api", "settings.update_tokens", "system", map[string]interface{}{
		"akile_updated":   req.AkileToken != "",
		"acck_updated":    req.ACCKToken != "",
		"lazycat_updated": req.LazyCatToken != "",
	}, "SUCCESS", "更新并热加载云厂商 API 凭证", dur)

	respondJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "Token 已安全持久化并即时热重载生效",
	})
}

// ==================== 购买计划与智能狙击 Handlers ====================

func handleListPlans(w http.ResponseWriter, r *http.Request) {
	plans := store.ListPlans()
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"total": len(plans),
		"plans": plans,
	})
}

func handleCreatePlan(w http.ResponseWriter, r *http.Request) {
	var req store.PurchasePlan
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的购买计划请求体"})
		return
	}
	created, err := store.CreatePlan(&req)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	audit.Record("rest_api", "plan.create", "system", map[string]interface{}{
		"plan_id":   created.ID,
		"name":      created.Name,
		"provider":  created.Provider,
		"max_price": created.MaxPrice,
	}, "SUCCESS", "创建 VPS 购买计划: "+created.Name, 0)
	respondJSON(w, http.StatusCreated, created)
}

func handleUpdatePlan(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var rawMap map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&rawMap); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的请求体"})
		return
	}
	updated, err := store.UpdatePlanMap(id, rawMap)
	if err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	audit.Record("rest_api", "plan.update", "system", map[string]interface{}{
		"plan_id": updated.ID,
		"name":    updated.Name,
	}, "SUCCESS", "更新购买计划: "+updated.Name, 0)
	respondJSON(w, http.StatusOK, updated)
}

func handleDeletePlan(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := store.DeletePlan(id); err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	audit.Record("rest_api", "plan.delete", "system", map[string]interface{}{"plan_id": id}, "SUCCESS", "删除购买计划", 0)
	respondJSON(w, http.StatusOK, map[string]string{"status": "success", "message": "计划已删除"})
}

func handleTogglePlan(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	plan, err := store.TogglePlan(id)
	if err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	stateStr := "开启"
	if !plan.IsActive {
		stateStr = "停用"
	}
	audit.Record("rest_api", "plan.toggle", "system", map[string]interface{}{"plan_id": id, "active": plan.IsActive}, "SUCCESS", stateStr+"购买计划: "+plan.Name, 0)
	respondJSON(w, http.StatusOK, plan)
}

func handleBuyMarketItemDirect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProviderID string `json:"provider_id"`
		ItemID     string `json:"item_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ProviderID == "" || req.ItemID == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "请提供有效的 provider_id 与 item_id"})
		return
	}

	p, ok := providers.Get(req.ProviderID)
	if !ok {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "未知的服务商: " + req.ProviderID})
		return
	}

	buyer, ok := p.(providers.MarketBuyer)
	if !ok {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "服务商不支持一键购买: " + req.ProviderID})
		return
	}

	if err := buyer.BuyMarketItem(r.Context(), req.ItemID); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "下单购买失败: " + err.Error()})
		return
	}

	audit.Record("rest_api", "market.buy_direct", req.ProviderID, map[string]interface{}{"item_id": req.ItemID}, "SUCCESS", "手动一键下单秒杀集市商品: "+req.ItemID, 0)
	respondJSON(w, http.StatusOK, map[string]string{"status": "success", "message": "购买指令已成功执行"})
}

func handleGetMarketStats(w http.ResponseWriter, r *http.Request) {
	stats := store.GetMarketStats()
	respondJSON(w, http.StatusOK, stats)
}

func handleGetSchedulerStatus(w http.ResponseWriter, r *http.Request) {
	if scheduler.GlobalScheduler == nil {
		scheduler.Init()
	}
	report := scheduler.GlobalScheduler.GetStatus()
	respondJSON(w, http.StatusOK, report)
}

func handleTriggerBurst(w http.ResponseWriter, r *http.Request) {
	if scheduler.GlobalScheduler == nil {
		scheduler.Init()
	}
	scheduler.GlobalScheduler.TriggerManualBurst(5 * time.Minute)
	audit.Record("rest_api", "scheduler.burst", "system", nil, "SUCCESS", "用户手动触发 5 分钟极速突发监控", 0)
	respondJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "已激活 5 分钟极速突发巡检模式 (5秒轮询)",
	})
}

func respondJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
