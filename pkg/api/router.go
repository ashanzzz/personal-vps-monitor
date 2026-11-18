package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

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

	r.Route("/api", func(r chi.Router) {
		// 查询已激活驱动列表
		r.Get("/providers", handleListProviders)

		// 聚合查询全网实例
		r.Get("/instances", handleUnifiedInstances)

		// 批量触发每日签到
		r.Post("/checkin", handleUnifiedCheckin)
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

func handleUnifiedCheckin(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	all := providers.GetAll()
	results := make(map[string]*providers.CheckinResult)

	for id, p := range all {
		if ac, ok := p.(providers.AutoCheckin); ok {
			res, err := ac.DoCheckin(ctx)
			if err != nil {
				results[id] = &providers.CheckinResult{
					Success: false,
					Message: err.Error(),
				}
			} else {
				results[id] = res
			}
		}
	}

	respondJSON(w, http.StatusOK, results)
}

func respondJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
