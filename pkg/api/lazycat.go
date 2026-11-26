package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers/lazycat"
	"github.com/go-chi/chi/v5"
)

func getLazyCatProvider() (*lazycat.LazyCatProvider, error) {
	p, ok := providers.Get("lazycat")
	if !ok {
		return nil, fmt.Errorf("lazycat provider not found")
	}
	lz, ok := p.(*lazycat.LazyCatProvider)
	if !ok {
		return nil, fmt.Errorf("invalid lazycat provider type")
	}
	return lz, nil
}

// GET /api/lazycat/ports?instance_id={id}
func handleLazyCatPorts(w http.ResponseWriter, r *http.Request) {
	lz, err := getLazyCatProvider()
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{"total": 0, "ports": []interface{}{}, "error": err.Error()})
		return
	}

	instID := r.URL.Query().Get("instance_id")
	if instID == "" {
		instID = "11433"
	}

	detail, err := lz.GetInstanceDetail(r.Context(), instID)
	if err != nil {
		// 降级尝试 ListPortMappings
		pm, ok := providers.Get("lazycat")
		if ok {
			if mapper, isMapper := pm.(providers.PortMapper); isMapper {
				if ports, mapErr := mapper.ListPortMappings(r.Context(), instID); mapErr == nil {
					respondJSON(w, http.StatusOK, map[string]interface{}{
						"total":      len(ports),
						"port_limit": 10,
						"ports":      ports,
					})
					return
				}
			}
		}
		respondJSON(w, http.StatusOK, map[string]interface{}{"total": 0, "ports": []interface{}{}, "warning": err.Error()})
		return
	}

	sshPort := 0
	for _, p := range detail.PortMappings {
		if p.PrivatePort == 22 {
			sshPort = p.PublicPort
			break
		}
	}

	portLimit := detail.PortLimit
	if portLimit == 0 {
		if list, listErr := lz.Client().GetInstances(r.Context()); listErr == nil {
			for _, item := range list {
				if strconv.Itoa(item.ID) == instID && item.PortLimit > 0 {
					portLimit = item.PortLimit
					break
				}
			}
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"total":          len(detail.PortMappings),
		"host_public_ip": detail.HostPublicIP,
		"ssh_port":       sshPort,
		"port_limit":     portLimit,
		"ports":          detail.PortMappings,
	})
}

// POST /api/lazycat/ports
func handleLazyCatAddPort(w http.ResponseWriter, r *http.Request) {
	lz, err := getLazyCatProvider()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var req struct {
		InstanceID  string `json:"instance_id"`
		PrivatePort int    `json:"private_port"`
		Protocol    string `json:"protocol"`
		Remark      string `json:"remark"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的端口映射请求体"})
		return
	}
	if req.InstanceID == "" {
		req.InstanceID = "11433"
	}
	if req.PrivatePort <= 0 || req.PrivatePort > 65535 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "私有端口号必须在 1 ~ 65535 范围内"})
		return
	}
	if req.Protocol == "" {
		req.Protocol = "tcp"
	}

	mapping, err := lz.AddPortMapping(r.Context(), req.InstanceID, req.PrivatePort, req.Protocol, req.Remark)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "新增端口映射失败: " + err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":       "success",
		"public_port":  mapping.PublicPort,
		"private_port": req.PrivatePort,
		"protocol":     req.Protocol,
		"remark":       req.Remark,
	})
}

// DELETE /api/lazycat/ports/{id}?instance_id={id}
func handleLazyCatDeletePort(w http.ResponseWriter, r *http.Request) {
	lz, err := getLazyCatProvider()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	idStr := chi.URLParam(r, "id")
	portID, err := strconv.Atoi(idStr)
	if err != nil || portID <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的端口映射 ID"})
		return
	}

	instID := r.URL.Query().Get("instance_id")
	if instID == "" {
		instID = "11433"
	}

	if err := lz.DeletePortMapping(r.Context(), instID, portID); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "删除端口映射失败: " + err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": fmt.Sprintf("已成功删除端口映射 (ID: %d)", portID),
	})
}

// GET /api/lazycat/detail?instance_id={id}
func handleLazyCatInstanceDetail(w http.ResponseWriter, r *http.Request) {
	lz, err := getLazyCatProvider()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	instID := r.URL.Query().Get("instance_id")
	if instID == "" {
		instID = "11433"
	}

	detail, err := lz.GetInstanceDetail(r.Context(), instID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, detail)
}

// GET /api/lazycat/terminal/ws?instance_id={id}
func handleLazyCatTerminalWS(w http.ResponseWriter, r *http.Request) {
	lz, err := getLazyCatProvider()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	instID := r.URL.Query().Get("instance_id")
	if instID == "" {
		instID = "11433"
	}

	_ = lz.ProxyTerminalWebSocket(w, r, instID)
}

// POST /api/lazycat/terminal/exec
func handleLazyCatTerminalExec(w http.ResponseWriter, r *http.Request) {
	lz, err := getLazyCatProvider()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var req struct {
		InstanceID string `json:"instance_id"`
		Command    string `json:"command"`
		TimeoutSec int    `json:"timeout_sec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的指令请求体"})
		return
	}
	if req.InstanceID == "" {
		req.InstanceID = "11433"
	}
	if req.Command == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "执行指令不可为空"})
		return
	}

	timeout := 15 * time.Second
	if req.TimeoutSec > 0 {
		timeout = time.Duration(req.TimeoutSec) * time.Second
	}

	res, err := lz.ExecTerminalCommandWithResult(r.Context(), req.InstanceID, req.Command, timeout)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	status := "success"
	if res.ExitCode != 0 {
		status = "failed"
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":      status,
		"instance_id": req.InstanceID,
		"output":      res.Output,
		"exit_code":   res.ExitCode,
		"duration_ms": res.DurationMs,
	})
}

// GET /api/lazycat/fleet-probe?refresh=true/false
func handleLazyCatFleetProbe(w http.ResponseWriter, r *http.Request) {
	lz, err := getLazyCatProvider()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	forceRefresh := r.URL.Query().Get("refresh") == "true"
	nodes, err := lz.FetchFleetTelemetry(r.Context(), forceRefresh)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"total":        len(nodes),
		"nodes":        nodes,
		"refreshed_at": time.Now().Format(time.RFC3339),
	})
}

// GET /api/lazycat/container/egress-nodes?instance_id={id}&refresh=true/false
func handleLazyCatEgressNodes(w http.ResponseWriter, r *http.Request) {
	lz, err := getLazyCatProvider()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	instID := r.URL.Query().Get("instance_id")
	if instID == "" {
		instID = "11433"
	}

	forceRefresh := r.URL.Query().Get("refresh") == "true"
	nodes, err := lz.GetContainerEgressNodes(r.Context(), instID, forceRefresh)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"total":        len(nodes),
		"instance_id":  instID,
		"nodes":        nodes,
		"refreshed_at": time.Now().Format(time.RFC3339),
	})
}
