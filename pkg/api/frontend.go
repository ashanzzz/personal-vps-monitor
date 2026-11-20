package api

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

// mountFrontend 将前端 React SPA 静态资源自动识别并挂载到路由上
// 采用标准的 SPA Fallback 机制：任何非静态文件的前端子路由均返回 index.html
func mountFrontend(r chi.Router) {
	candidates := []string{
		os.Getenv("FRONTEND_DIR"),
		"./frontend/dist",
		"../frontend/dist",
		"/usr/share/nginx/html",
		"/app/frontend/dist",
	}

	var staticDir string
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if fi, err := os.Stat(c); err == nil && fi.IsDir() {
			if _, err := os.Stat(filepath.Join(c, "index.html")); err == nil {
				staticDir = c
				break
			}
		}
	}

	if staticDir == "" {
		log.Println("[Frontend] No frontend dist directory found, running in headless API mode")
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"service": "personal-vps-monitor",
				"version": "v2.0.0-modular",
				"mode":    "headless-api",
				"endpoints": map[string]string{
					"health":    "/health",
					"version":   "/version",
					"api":       "/api/providers",
					"market":    "/api/market",
					"instances": "/api/instances",
					"mcp_sse":   "/sse",
				},
			})
		})
		return
	}

	log.Printf("[Frontend] Serving SPA frontend from: %s", staticDir)
	fileServer := http.FileServer(http.Dir(staticDir))

	// 挂载静态资源及 SPA 回退
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// API 与 MCP 端点不走前端回退
		if strings.HasPrefix(path, "/api") || strings.HasPrefix(path, "/sse") || strings.HasPrefix(path, "/message") || path == "/health" || path == "/version" {
			http.NotFound(w, r)
			return
		}

		cleanPath := strings.TrimPrefix(path, "/")
		if cleanPath != "" {
			targetFile := filepath.Join(staticDir, filepath.FromSlash(cleanPath))
			if fi, err := os.Stat(targetFile); err == nil && !fi.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// SPA Fallback: 始终返回 index.html
		indexPath := filepath.Join(staticDir, "index.html")
		http.ServeFile(w, r, indexPath)
	})
}
