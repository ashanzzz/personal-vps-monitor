package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/audit"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/sshexec"
	"github.com/go-chi/chi/v5"
)

// registerInstanceExecRoutes 注册普通主机 SSH 执行、代理探查与自愈生命周期管理路由
func registerInstanceExecRoutes(r chi.Router) {
	// 模块 1: 免交互远程 SSH 执行引擎
	r.Post("/instances/{id}/exec", handleInstanceSSHExec)
	r.Post("/instances/exec-by-ip", handleInstanceSSHExecByIP)

	// 模块 2: 普通主机代理服务结构化探查
	r.Get("/instances/{id}/proxies", handleInstanceInspectProxies)

	// 模块 2.1: 单台实例 SSH / Root 凭据按需取用（列表接口已统一脱敏）
	r.Get("/instances/{id}/credential", handleInstanceCredential)

	// 模块 3: 代理换端口自愈与增删生命周期管理
	r.Put("/instances/{id}/proxies/{old_port}", handleInstanceChangeProxyPort)
	r.Post("/instances/{id}/proxies", handleInstanceAddProxy)
	r.Delete("/instances/{id}/proxies/{port}", handleInstanceDeleteProxy)

	// 模块 4: 自建 VPS + 懒猫家宽「链式代理」组合导出
	r.Get("/lazycat/chain-nodes", handleLazyCatChainNodes)
}

// POST /api/instances/{id}/exec
func handleInstanceSSHExec(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "instance id or IP required"})
		return
	}

	var req struct {
		Command    string `json:"command"`
		TimeoutSec int    `json:"timeout_sec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	if strings.TrimSpace(req.Command) == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "command cannot be empty"})
		return
	}

	res, _, err := sshexec.RunCommandWithAudit(r.Context(), "rest_api", id, req.Command, req.TimeoutSec)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, res)
}

// POST /api/instances/exec-by-ip
func handleInstanceSSHExecByIP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IP         string `json:"ip"`
		Command    string `json:"command"`
		TimeoutSec int    `json:"timeout_sec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	ip := strings.TrimSpace(req.IP)
	if ip == "" {
		ip = r.URL.Query().Get("ip")
	}
	if ip == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "ip address required"})
		return
	}

	if strings.TrimSpace(req.Command) == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "command cannot be empty"})
		return
	}

	res, _, err := sshexec.RunCommandWithAudit(r.Context(), "rest_api", ip, req.Command, req.TimeoutSec)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, res)
}

// GET /api/instances/{id}/credential
// 列表接口已统一脱敏，此处按需单独取用单台实例的 SSH / Root 凭据。
func handleInstanceCredential(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "instance id required"})
		return
	}

	inst, err := sshexec.FindInstance(id)
	if err != nil || inst == nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "instance not found: " + id})
		return
	}

	pwd := inst.SSHPassword
	if pwd == "" {
		pwd = inst.RootPassword
	}

	reveal := r.URL.Query().Get("reveal") == "true"
	password := pwd
	if !reveal && pwd != "" {
		password = "••••••••"
	}

	// 凭据查看同样纳入安全审计
	audit.Record("rest_api", "instance.credential_view", inst.ProviderID, map[string]interface{}{
		"instance_id": inst.ID,
		"reveal":      reveal,
	}, "SUCCESS", "按需查看实例 SSH 凭据", 0)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"provider_id": inst.ProviderID,
		"id":          inst.ID,
		"ssh_user":    inst.SSHUser,
		"ssh_port":    inst.SSHPort,
		"has_password": pwd != "",
		"password":    password,
	})
}

// GET /api/instances/{id}/proxies
func handleInstanceInspectProxies(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "instance id or IP required"})
		return
	}

	res, err := sshexec.InspectProxiesWithAudit(r.Context(), "rest_api", id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, res)
}

// PUT /api/instances/{id}/proxies/{old_port}
func handleInstanceChangeProxyPort(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	oldPortStr := chi.URLParam(r, "old_port")
	oldPort, err := strconv.Atoi(oldPortStr)
	if err != nil || oldPort <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid old_port"})
		return
	}

	var req struct {
		NewPort int `json:"new_port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	if req.NewPort <= 0 || req.NewPort > 65535 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "new_port must be between 1 and 65535"})
		return
	}

	res, err := sshexec.ChangeProxyPortWithAudit(r.Context(), "rest_api", id, oldPort, req.NewPort)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, res)
}

// POST /api/instances/{id}/proxies
func handleInstanceAddProxy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "instance id or IP required"})
		return
	}

	var req sshexec.AddProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	if req.Port <= 0 || req.Port > 65535 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "port must be between 1 and 65535"})
		return
	}

	res, err := sshexec.AddProxyWithAudit(r.Context(), "rest_api", id, req)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, res)
}

// DELETE /api/instances/{id}/proxies/{port}
func handleInstanceDeleteProxy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	portStr := chi.URLParam(r, "port")
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid port"})
		return
	}

	err = sshexec.DeleteProxyWithAudit(r.Context(), "rest_api", id, port)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Proxy on port " + portStr + " successfully deleted",
		"port":    port,
	})
}

// GET /api/lazycat/chain-nodes
func handleLazyCatChainNodes(w http.ResponseWriter, r *http.Request) {
	relay := r.URL.Query().Get("relay_id")
	if relay == "" {
		relay = r.URL.Query().Get("relay_ip")
	}
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	res, err := sshexec.GetChainProxyNodesWithAudit(r.Context(), "rest_api", relay, format)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// 针对 Sub-Store 或 Clash / Sing-box 直连拉取规范输出
	formatLower := strings.ToLower(format)
	if formatLower == "clash" || formatLower == "mihomo" || formatLower == "yaml" {
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.Header().Set("Subscription-Userinfo", "upload=0; download=0; total=1073741824000; expire=1893456000")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(res.ClashConfig))
		return
	}

	if formatLower == "sing-box" || formatLower == "singbox" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(res.SingBoxJSON))
		return
	}

	// 默认返回结构化 JSON
	respondJSON(w, http.StatusOK, res)
}
