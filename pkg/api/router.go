package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/audit"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
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

	r.Route("/api", func(r chi.Router) {
		// 查询已激活驱动列表
		r.Get("/providers", handleListProviders)

		// 聚合查询全网实例
		r.Get("/instances", handleUnifiedInstances)

		// 聚合查询二手集市转让商品
		r.Get("/market", handleUnifiedMarket)

		// 批量触发每日签到
		r.Post("/checkin", handleUnifiedCheckin)

		// 行为审计日志接口 (查询过去 30 天)
		r.Get("/audit", handleGetAudit)
		r.Post("/audit/prune", handlePruneAudit)

		// 懒猫云特化端点
		r.Route("/lazycat", func(r chi.Router) {
			r.Get("/instances", handleLazyCatInstances)
			r.Get("/ports", handleLazyCatPorts)
			r.Post("/sync", handleLazyCatSync)
			r.Post("/checkin", handleLazyCatCheckin)
		})
	})

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
	all := providers.GetAll()
	var results []providers.Instance

	for _, p := range all {
		if im, ok := p.(providers.InstanceManager); ok {
			insts, err := im.ListInstances(ctx)
			if err == nil {
				results = append(results, insts...)
			}
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"total":     len(results),
		"instances": results,
	})
}

func handleUnifiedMarket(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	all := providers.GetAll()
	var results []providers.MarketItem

	for _, p := range all {
		if ml, ok := p.(providers.MarketLister); ok {
			items, err := ml.ListMarketItems(ctx)
			if err == nil {
				results = append(results, items...)
			}
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"total": len(results),
		"items": results,
	})
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
				audit.Record("rest_api", "checkin", id, nil, "FAILED", err.Error(), dur)
			} else {
				results[id] = res
				audit.Record("rest_api", "checkin", id, map[string]interface{}{"account": res.Account, "reward": res.Reward, "website": res.Website}, "SUCCESS", res.Message, dur)
			}
		}
	}

	respondJSON(w, http.StatusOK, results)
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
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"total":     len(insts),
		"instances": insts,
	})
}

func handleLazyCatPorts(w http.ResponseWriter, r *http.Request) {
	p, ok := providers.Get("lazycat")
	if !ok {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "lazycat provider not found"})
		return
	}
	pm, ok := p.(providers.PortMapper)
	if !ok {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "provider does not support port mapping"})
		return
	}
	instID := r.URL.Query().Get("instance_id")
	ports, err := pm.ListPortMappings(r.Context(), instID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"total": len(ports),
		"ports": ports,
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

func respondJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
