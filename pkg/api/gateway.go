package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/audit"
	"github.com/go-chi/chi/v5"
)

func getGatewayURL() string {
	if u := os.Getenv("GATEWAY_URL"); u != "" {
		return u
	}
	return "http://192.168.8.11:9090"
}

func getGatewayToken() string {
	if t := os.Getenv("GATEWAY_TOKEN"); t != "" {
		return t
	}
	if p := os.Getenv("ADMIN_PASSWORD"); p != "" {
		return p
	}
	return "Woo@@@204317"
}

// registerGatewayRoutes 注册 Sing-box / OpenClash 网关控制接口
func registerGatewayRoutes(r chi.Router) {
	r.Route("/gateway", func(r chi.Router) {
		// 获取代理组与节点
		r.Get("/proxies", func(w http.ResponseWriter, r *http.Request) {
			gwURL := getGatewayURL()
			gwToken := getGatewayToken()
			client := &http.Client{Timeout: 5 * time.Second}
			req, err := http.NewRequestWithContext(r.Context(), "GET", gwURL+"/proxies", nil)
			if err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			req.Header.Set("Authorization", "Bearer "+gwToken)

			resp, err := client.Do(req)
			if err != nil {
				respondJSON(w, http.StatusBadGateway, map[string]string{"error": "网关无响应: " + err.Error()})
				return
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(body)
		})

		// 切换代理节点
		r.Put("/select", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				GroupName  string `json:"group_name"`
				TargetNode string `json:"target_node"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				respondJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的参数"})
				return
			}

			gwURL := getGatewayURL()
			gwToken := getGatewayToken()
			payload, _ := json.Marshal(map[string]string{"name": body.TargetNode})
			url := fmt.Sprintf("%s/proxies/%s", gwURL, body.GroupName)

			client := &http.Client{Timeout: 5 * time.Second}
			req, err := http.NewRequestWithContext(r.Context(), "PUT", url, bytes.NewBuffer(payload))
			if err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			req.Header.Set("Authorization", "Bearer "+gwToken)
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				respondJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
				return
			}
			defer resp.Body.Close()

			audit.Record(
				"gateway",
				"gateway.switch_node",
				"singbox",
				map[string]interface{}{"group": body.GroupName, "target": body.TargetNode},
				"SUCCESS",
				fmt.Sprintf("已成功将代理组 [%s] 切换至节点 [%s]", body.GroupName, body.TargetNode),
				0,
			)

			respondJSON(w, http.StatusOK, map[string]interface{}{
				"status":   "success",
				"group":    body.GroupName,
				"selected": body.TargetNode,
			})
		})

		// 节点测速
		r.Post("/delay", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				NodeName  string `json:"node_name"`
				TestURL   string `json:"test_url"`
				TimeoutMs int    `json:"timeout_ms"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				respondJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的参数"})
				return
			}
			if body.TestURL == "" {
				body.TestURL = "http://www.gstatic.com/generate_204"
			}
			if body.TimeoutMs <= 0 {
				body.TimeoutMs = 5000
			}

			gwURL := getGatewayURL()
			gwToken := getGatewayToken()
			url := fmt.Sprintf("%s/proxies/%s/delay?url=%s&timeout=%d", gwURL, body.NodeName, body.TestURL, body.TimeoutMs)
			client := &http.Client{Timeout: time.Duration(body.TimeoutMs+1000) * time.Millisecond}
			req, err := http.NewRequestWithContext(r.Context(), "GET", url, nil)
			if err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			req.Header.Set("Authorization", "Bearer "+gwToken)

			resp, err := client.Do(req)
			if err != nil {
				respondJSON(w, http.StatusOK, map[string]interface{}{
					"name":  body.NodeName,
					"delay": -1,
					"error": err.Error(),
				})
				return
			}
			defer resp.Body.Close()

			var res struct {
				Delay int    `json:"delay"`
				Error string `json:"message"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&res)

			respondJSON(w, http.StatusOK, map[string]interface{}{
				"name":  body.NodeName,
				"delay": res.Delay,
				"error": res.Error,
			})
		})
	})
}
