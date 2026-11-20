package lazycat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// FleetNode 懒猫云动态母鸡与住宅出口遥测指标
type FleetNode struct {
	UUID          string  `json:"uuid"`
	Name          string  `json:"name"`
	Region        string  `json:"region"`
	IsMother      bool    `json:"is_mother"`
	IsResidential bool    `json:"is_residential"`
	Status        string  `json:"status"` // "UP" | "DOWN"
	CPUPercent    float64 `json:"cpu_percent"`
	LoadAvg       float64 `json:"load_avg"`
	RAMUsedGB     float64 `json:"ram_used_gb"`
	RAMTotalGB    float64 `json:"ram_total_gb"`
	RAMPercent    float64 `json:"ram_percent"`
	SpeedDownKB   float64 `json:"speed_down_kb"`
	SpeedUpKB     float64 `json:"speed_up_kb"`
	UptimeDays    float64 `json:"uptime_days"`
}

type fleetCacheEntry struct {
	nodes     []FleetNode
	fetchedAt time.Time
}

var (
	fleetMu    sync.RWMutex
	fleetCache *fleetCacheEntry
)

// FetchFleetTelemetry 按需拉取 tz.lazycat.my Komari JSON-RPC 2.0 节点遥测数据 (15s 内存缓存)
func (c *Client) FetchFleetTelemetry(ctx context.Context, forceRefresh bool) ([]FleetNode, error) {
	fleetMu.RLock()
	if !forceRefresh && fleetCache != nil && time.Since(fleetCache.fetchedAt) < 15*time.Second {
		nodes := fleetCache.nodes
		fleetMu.RUnlock()
		return nodes, nil
	}
	fleetMu.RUnlock()

	// 纯 Go 原生 HTTP Transport
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 8 * time.Second,
		IdleConnTimeout:       10 * time.Second,
		DisableKeepAlives:     true,
	}
	if c.proxyURL != "" {
		if u, err := url.Parse(c.proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	callRPC := func(method string) ([]byte, error) {
		payload := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  method,
			"params":  []interface{}{},
		}
		data, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, "POST", "https://tz.lazycat.my/api/rpc2", bytes.NewBuffer(data))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) LazyCatProbe/1.0")

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		return io.ReadAll(resp.Body)
	}

	nodesRaw, err := callRPC("common:getNodes")
	if err != nil {
		return nil, fmt.Errorf("call getNodes error: %w", err)
	}
	statusRaw, err := callRPC("common:getNodesLatestStatus")
	if err != nil {
		return nil, fmt.Errorf("call getNodesLatestStatus error: %w", err)
	}

	var nodesRes struct {
		Result map[string]struct {
			Name      string `json:"name"`
			Region    string `json:"region"`
			MemTotal  int64  `json:"mem_total"`
			DiskTotal int64  `json:"disk_total"`
		} `json:"result"`
	}
	var statusRes struct {
		Result map[string]struct {
			CPU       float64 `json:"cpu"`
			Load      float64 `json:"load"`
			RAM       int64   `json:"ram"`
			NetIn     float64 `json:"net_in"`
			NetOut    float64 `json:"net_out"`
			UptimeSec int64   `json:"uptime"`
			UpdatedAt string  `json:"updated_at"`
		} `json:"result"`
	}

	_ = json.Unmarshal(nodesRaw, &nodesRes)
	_ = json.Unmarshal(statusRaw, &statusRes)

	var results []FleetNode
	for nodeUUID, n := range nodesRes.Result {
		st := statusRes.Result[nodeUUID]
		name := n.Name
		isMother := strings.Contains(name, "母鸡") || strings.Contains(name, "Global-2") || strings.Contains(name, "HostHatch") || strings.Contains(name, "Computebox")
		isRes := strings.Contains(name, "家宽") || strings.Contains(name, "住宅") || strings.Contains(name, "PAVLOV") ||
			strings.Contains(name, "Frontier") || strings.Contains(name, "Biglobe") || strings.Contains(name, "GLBB") ||
			strings.Contains(name, "KDDI") || strings.Contains(name, "HKT") || strings.Contains(name, "StarHub") ||
			strings.Contains(name, "DITO") || strings.Contains(name, "Korea-Telecom") || strings.Contains(name, "SoftBank")

		statusStr := "UP"
		if st.UptimeSec <= 0 && st.CPU == 0 && st.RAM == 0 {
			statusStr = "DOWN"
		}

		ramUsedGB := float64(st.RAM) / (1024 * 1024 * 1024)
		ramTotalGB := float64(n.MemTotal) / (1024 * 1024 * 1024)
		ramPct := 0.0
		if ramTotalGB > 0 {
			ramPct = (ramUsedGB / ramTotalGB) * 100.0
		}

		results = append(results, FleetNode{
			UUID:          nodeUUID,
			Name:          name,
			Region:        n.Region,
			IsMother:      isMother,
			IsResidential: isRes,
			Status:        statusStr,
			CPUPercent:    st.CPU,
			LoadAvg:       st.Load,
			RAMUsedGB:     ramUsedGB,
			RAMTotalGB:    ramTotalGB,
			RAMPercent:    ramPct,
			SpeedDownKB:   st.NetIn / 1024.0,
			SpeedUpKB:     st.NetOut / 1024.0,
			UptimeDays:    float64(st.UptimeSec) / 86400.0,
		})
	}

	fleetMu.Lock()
	fleetCache = &fleetCacheEntry{
		nodes:     results,
		fetchedAt: time.Now(),
	}
	fleetMu.Unlock()

	return results, nil
}
