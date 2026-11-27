package config

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type UnraidContainerStatus struct {
	Name      string `json:"name"`
	Target    string `json:"target"`
	Online    bool   `json:"online"`
	Detail    string `json:"detail,omitempty"`
	LatencyMs int64  `json:"latency_ms"`
}

type AutoDiscoveryReport struct {
	Timestamp  string                  `json:"timestamp"`
	Containers []UnraidContainerStatus `json:"containers"`
	CredentialsFound []string          `json:"credentials_found"`
}

// ProbeUnraidContainers 探测本地凭据来源与核心服务的连通状态
func ProbeUnraidContainers(ctx context.Context) AutoDiscoveryReport {
	report := AutoDiscoveryReport{
		Timestamp:        time.Now().Format(time.RFC3339),
		Containers:       make([]UnraidContainerStatus, 0),
		CredentialsFound: make([]string, 0),
	}

	targets := []struct {
		Name   string
		URL    string
		Method string
		Note   string
	}{
		{"Vaultwarden 凭据保险箱", "http://192.168.8.11:4743/alive", "GET", "用户主密码与 VPS 凭据主存储 (容器 7b36826ed52a)"},
		{"Unraid 本地网关 (Sing-box)", "http://192.168.8.11:9090", "GET", "宿主网络代理核心 (Clash/Mihomo 端口 7890 / API 9090)"},
	}

	client := &http.Client{Timeout: 1500 * time.Millisecond}

	for _, t := range targets {
		start := time.Now()
		req, err := http.NewRequestWithContext(ctx, t.Method, t.URL, nil)
		online := false
		detail := ""

		if err == nil {
			if strings.Contains(t.URL, ":9090") {
				req.Header.Set("Authorization", "Bearer Woo@@@204317")
			}
			resp, err := client.Do(req)
			if err == nil {
				online = (resp.StatusCode < 500)
				if strings.Contains(t.URL, ":4743") {
					detail = fmt.Sprintf("在线 (HTTP %d - Vaultwarden 正常就绪)", resp.StatusCode)
				} else {
					detail = fmt.Sprintf("在线 (HTTP %d)", resp.StatusCode)
				}
				_ = resp.Body.Close()
			} else {
				if strings.Contains(t.URL, ":4743") {
					detail = "未连接/离线 (请检查 192.168.8.11:4743)"
				} else {
					detail = "未连接/离线"
				}
			}
		}

		dur := time.Since(start).Milliseconds()
		report.Containers = append(report.Containers, UnraidContainerStatus{
			Name:      t.Name,
			Target:    t.URL,
			Online:    online,
			Detail:    detail,
			LatencyMs: dur,
		})
	}

	// 自动扫描本地持久化目录中的凭据
	tokenFiles := []string{
		"/app/data/credentials.json",
		"./data/credentials.json",
		"/app/data/lazycat_token.txt",
		"/mnt/user/appdata/personal-vps-monitor/lazycat_token.txt",
		"/app/data/.env",
		"/mnt/user/appdata/personal-vps-monitor/.env",
		"./data/admin.json",
	}

	for _, p := range tokenFiles {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Size() > 0 {
			report.CredentialsFound = append(report.CredentialsFound, filepath.Base(p))
		}
	}

	return report
}

// AutoSyncCredentialsFromVault 尝试从本地已挂载卷与 bw serve 读取凭据自动填充
func AutoSyncCredentialsFromVault(ctx context.Context) {
	// 如果全局未配置懒猫云 Token，尝试从 token 文本自动读
	if Global.LazyCatToken == "" {
		for _, tf := range []string{"/app/data/lazycat_token.txt", "/mnt/user/appdata/personal-vps-monitor/lazycat_token.txt"} {
			if b, err := os.ReadFile(tf); err == nil {
				val := strings.Trim(strings.TrimSpace(string(b)), "\"'")
				if val != "" {
					Global.LazyCatToken = val
					break
				}
			}
		}
	}
}
