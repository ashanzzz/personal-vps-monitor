package sshexec

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/audit"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
)

// ProxyInfo 结构化代理节点信息
type ProxyInfo struct {
	Port      int    `json:"port"`
	Protocol  string `json:"protocol"`
	Security  string `json:"security,omitempty"` // reality, tls, none
	Network   string `json:"network,omitempty"`  // tcp, ws, grpc
	SNI       string `json:"sni,omitempty"`
	UUID      string `json:"uuid,omitempty"`
	PublicKey string `json:"public_key,omitempty"`
	ShortID   string `json:"short_id,omitempty"`
	Path      string `json:"path,omitempty"`
	Flow      string `json:"flow,omitempty"`
	Remark    string `json:"remark,omitempty"`
	Status    string `json:"status"` // "listening" or "stopped"
	Method    string `json:"method,omitempty"`
	Password  string `json:"password,omitempty"`
	Link      string `json:"link,omitempty"`
}

// InstanceProxiesResult 主机代理服务探查综合结果
type InstanceProxiesResult struct {
	InstanceID     string      `json:"instance_id"`
	IP             string      `json:"ip"`
	ManagementType string      `json:"management_type"` // "3x-ui", "sing-box", "xray", "docker", "unknown"
	Proxies        []ProxyInfo `json:"proxies"`
}

// InspectProxies 探查目标主机运行的代理入站服务
func InspectProxies(ctx context.Context, inst *providers.Instance) (*InstanceProxiesResult, error) {
	fillDefaults(inst)

	// 探查脚本：探测 3x-ui SQLite 路径、提取 inbounds、探测 sing-box / xray 配置文件、以及抓取 ss -tulpn 监听状态
	inspectScript := `
echo "=== SS_TULPN ==="
ss -tulpn 2>/dev/null || netstat -tulpn 2>/dev/null

echo "=== 3X_UI_DB ==="
DB_PATH=""
for p in /opt/1panel/apps/3x-ui/db/x-ui.db /etc/x-ui/x-ui.db /usr/local/x-ui/db/x-ui.db; do
  if [ -f "$p" ]; then
    DB_PATH="$p"
    break
  fi
done

if [ -n "$DB_PATH" ]; then
  echo "DB_FOUND:$DB_PATH"
  sqlite3 -json "$DB_PATH" "SELECT id, port, protocol, settings, stream_settings, enable, remark FROM inbounds;" 2>/dev/null || \
  docker exec 3x-ui sqlite3 -json /etc/x-ui/x-ui.db "SELECT id, port, protocol, settings, stream_settings, enable, remark FROM inbounds;" 2>/dev/null || \
  sqlite3 "$DB_PATH" "SELECT id, port, protocol, replace(replace(settings, char(10), ''), char(13), ''), replace(replace(stream_settings, char(10), ''), char(13), ''), enable, remark FROM inbounds;" 2>/dev/null || true
fi

echo "=== SING_BOX_CFG ==="
if [ -f /etc/sing-box/config.json ]; then
  echo ""
  echo "FILE:/etc/sing-box/config.json"
  cat /etc/sing-box/config.json
  echo ""
fi
for c in $(docker ps --format '{{.Names}}' 2>/dev/null); do
  if [ "$c" = "sb-ss" ] || [ "$c" = "sing-box" ] || [ "$c" = "ss1" ]; then
    echo ""
    echo "FILE:docker-$c"
    docker exec "$c" cat /etc/sing-box/config.json 2>/dev/null || docker exec "$c" cat /cfg.json 2>/dev/null || true
    echo ""
  fi
done

echo "=== XRAY_CFG ==="
for xp in /etc/v2ray-agent/docker/config.json /usr/local/etc/xray/config.json /etc/xray/config.json /etc/xray/sb-relay.json; do
  if [ -f "$xp" ]; then
    echo ""
    echo "FILE:$xp"
    cat "$xp"
    echo ""
  fi
done
`

	res, err := ExecuteCommand(ctx, inst, inspectScript, 12*time.Second)
	if err != nil {
		return nil, fmt.Errorf("inspect command execution failed: %w", err)
	}

	result := parseInspectOutput(inst, res.Stdout)
	return result, nil
}

// InspectProxiesWithAudit 探查代理并记录审计
func InspectProxiesWithAudit(ctx context.Context, source, idOrIP string) (*InstanceProxiesResult, error) {
	inst, err := FindInstance(idOrIP)
	if err != nil {
		audit.Record(source, "vps.inspect_proxies", "system", map[string]interface{}{
			"id_or_ip": idOrIP,
		}, "FAILED", fmt.Sprintf("Instance not found: %v", err), 0)
		return nil, err
	}

	start := time.Now()
	res, err := InspectProxies(ctx, inst)
	dur := time.Since(start).Milliseconds()

	if err != nil {
		audit.Record(source, "vps.inspect_proxies", inst.ProviderID, map[string]interface{}{
			"instance_id": inst.ID,
			"ip":          inst.IP,
		}, "FAILED", err.Error(), dur)
		return nil, err
	}

	audit.Record(source, "vps.inspect_proxies", inst.ProviderID, map[string]interface{}{
		"instance_id":     inst.ID,
		"ip":              inst.IP,
		"management_type": res.ManagementType,
		"proxy_count":     len(res.Proxies),
	}, "SUCCESS", fmt.Sprintf("Found %d proxies (%s)", len(res.Proxies), res.ManagementType), dur)

	return res, nil
}

func parseInspectOutput(inst *providers.Instance, output string) *InstanceProxiesResult {
	sections := splitSections(output)

	ssOutput := sections["SS_TULPN"]
	dbOutput := sections["3X_UI_DB"]
	sbOutput := sections["SING_BOX_CFG"]
	xrayOutput := sections["XRAY_CFG"]

	listeningPorts := parseListeningPorts(ssOutput)

	result := &InstanceProxiesResult{
		InstanceID:     inst.ID,
		IP:             inst.IP,
		ManagementType: "unknown",
		Proxies:        make([]ProxyInfo, 0),
	}

	// 1. 优先解析 3x-ui
	if strings.Contains(dbOutput, "DB_FOUND:") {
		result.ManagementType = "3x-ui"
		proxies := parse3xUIInbounds(dbOutput, listeningPorts)
		if len(proxies) > 0 {
			result.Proxies = append(result.Proxies, proxies...)
		}
	}

	// 2. 检查 sing-box
	if len(strings.TrimSpace(sbOutput)) > 10 {
		if result.ManagementType == "unknown" {
			result.ManagementType = "sing-box"
		} else {
			result.ManagementType += "+sing-box"
		}
		sbProxies := parseSingBoxInbounds(sbOutput, listeningPorts)
		for _, p := range sbProxies {
			if !hasPort(result.Proxies, p.Port) {
				result.Proxies = append(result.Proxies, p)
			}
		}
	}

	// 3. 检查 xray
	if len(strings.TrimSpace(xrayOutput)) > 10 {
		if result.ManagementType == "unknown" {
			result.ManagementType = "xray"
		} else if !strings.Contains(result.ManagementType, "xray") {
			result.ManagementType += "+xray"
		}
		xrayProxies := parseXrayInbounds(xrayOutput, listeningPorts)
		for _, p := range xrayProxies {
			if !hasPort(result.Proxies, p.Port) {
				result.Proxies = append(result.Proxies, p)
			}
		}
	}

	// 4. 如果没有找到配置文件，但有特定代理端口在监听 (如 23488, 24443)
	if len(result.Proxies) == 0 {
		for port := range listeningPorts {
			if port > 1024 && port != 22 && port != 80 && port != 443 && port != 3478 && port != 40184 {
				result.Proxies = append(result.Proxies, ProxyInfo{
					Port:     port,
					Protocol: "unknown",
					Status:   "listening",
				})
			}
		}
	}

	// 5. 自动为每一个已解析代理生成标准分享链接
	for i := range result.Proxies {
		result.Proxies[i].Link = GenerateProxyLink(&result.Proxies[i], inst.IP, inst.Name)
	}

	return result
}

func hasPort(list []ProxyInfo, port int) bool {
	for _, p := range list {
		if p.Port == port {
			return true
		}
	}
	return false
}

func splitSections(output string) map[string]string {
	sections := make(map[string]string)
	lines := strings.Split(output, "\n")
	currentSection := ""
	var currentLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "=== ") && strings.HasSuffix(trimmed, " ===") {
			if currentSection != "" {
				sections[currentSection] = strings.Join(currentLines, "\n")
			}
			currentSection = strings.Trim(trimmed, "= ")
			currentLines = nil
		} else {
			currentLines = append(currentLines, line)
		}
	}
	if currentSection != "" {
		sections[currentSection] = strings.Join(currentLines, "\n")
	}
	return sections
}

var portRegex = regexp.MustCompile(`[:](\d+)\s+`)

func parseListeningPorts(ssOutput string) map[int]bool {
	ports := make(map[int]bool)
	lines := strings.Split(ssOutput, "\n")
	for _, line := range lines {
		if !strings.Contains(line, "LISTEN") && !strings.Contains(line, "tcp") && !strings.Contains(line, "udp") {
			continue
		}
		matches := portRegex.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			if len(m) >= 2 {
				if p, err := strconv.Atoi(m[1]); err == nil && p > 0 {
					ports[p] = true
				}
			}
		}
	}
	return ports
}

// parse3xUIInbounds 解析 SQLite inbounds 行记录或 sqlite3 -json 格式
func parse3xUIInbounds(dbOutput string, listeningPorts map[int]bool) []ProxyInfo {
	var proxies []ProxyInfo

	// 1. 尝试使用 JSON 数组反序列化 (sqlite3 -json)
	startIdx := strings.Index(dbOutput, "[")
	endIdx := strings.LastIndex(dbOutput, "]")
	if startIdx >= 0 && endIdx > startIdx {
		jsonStr := dbOutput[startIdx : endIdx+1]
		var rawList []struct {
			ID             int         `json:"id"`
			Port           int         `json:"port"`
			Protocol       string      `json:"protocol"`
			Settings       string      `json:"settings"`
			StreamSettings string      `json:"stream_settings"`
			Enable         int         `json:"enable"`
			Remark         string      `json:"remark"`
		}
		if err := json.Unmarshal([]byte(jsonStr), &rawList); err == nil && len(rawList) > 0 {
			for _, item := range rawList {
				if item.Port <= 0 {
					continue
				}
				info := buildProxyInfoFrom3xUI(item.Port, item.Protocol, item.Settings, item.StreamSettings, item.Remark, listeningPorts)
				proxies = append(proxies, info)
			}
			return proxies
		}
	}

	// 2. 降级：按行解析以管道符分隔的文本
	lines := strings.Split(dbOutput, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "DB_FOUND:") {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 7 {
			continue
		}

		port, err := strconv.Atoi(parts[1])
		if err != nil || port <= 0 {
			continue
		}

		protocol := parts[2]
		rawSettings := parts[3]
		rawStream := parts[4]
		remark := parts[6]

		info := buildProxyInfoFrom3xUI(port, protocol, rawSettings, rawStream, remark, listeningPorts)
		proxies = append(proxies, info)
	}

	return proxies
}

func buildProxyInfoFrom3xUI(port int, protocol, rawSettings, rawStream, remark string, listeningPorts map[int]bool) ProxyInfo {
	info := ProxyInfo{
		Port:     port,
		Protocol: protocol,
		Remark:   remark,
		Status:   "stopped",
	}
	if listeningPorts[port] {
		info.Status = "listening"
	}

	// 解析 Settings JSON
	var settingsMap struct {
		Clients []struct {
			ID    string `json:"id"`
			Email string `json:"email"`
			Flow  string `json:"flow"`
		} `json:"clients"`
	}
	if err := json.Unmarshal([]byte(rawSettings), &settingsMap); err == nil {
		if len(settingsMap.Clients) > 0 {
			info.UUID = settingsMap.Clients[0].ID
			info.Flow = settingsMap.Clients[0].Flow
		}
	}

	// 解析 StreamSettings JSON
	var streamMap struct {
		Network         string `json:"network"`
		Security        string `json:"security"`
		RealitySettings struct {
			ServerNames []string `json:"serverNames"`
			ShortIDs    []string `json:"shortIds"`
			PublicKey   string   `json:"publicKey"`
			Settings    struct {
				PublicKey   string `json:"publicKey"`
				Fingerprint string `json:"fingerprint"`
			} `json:"settings"`
		} `json:"realitySettings"`
		TLSSettings struct {
			ServerName string `json:"serverName"`
		} `json:"tlsSettings"`
		WSSettings struct {
			Path string `json:"path"`
		} `json:"wsSettings"`
	}

	if err := json.Unmarshal([]byte(rawStream), &streamMap); err == nil {
		info.Network = streamMap.Network
		info.Security = streamMap.Security

		if streamMap.Security == "reality" {
			if len(streamMap.RealitySettings.ServerNames) > 0 {
				info.SNI = streamMap.RealitySettings.ServerNames[0]
			}
			if streamMap.RealitySettings.PublicKey != "" {
				info.PublicKey = streamMap.RealitySettings.PublicKey
			} else if streamMap.RealitySettings.Settings.PublicKey != "" {
				info.PublicKey = streamMap.RealitySettings.Settings.PublicKey
			}
			if len(streamMap.RealitySettings.ShortIDs) > 0 {
				info.ShortID = streamMap.RealitySettings.ShortIDs[0]
			}
		} else if streamMap.Security == "tls" {
			info.SNI = streamMap.TLSSettings.ServerName
		}

		if streamMap.Network == "ws" {
			info.Path = streamMap.WSSettings.Path
			if info.Protocol == "vless" {
				info.Protocol = "vless_ws"
			}
		}
	}

	return info
}

// parseSingBoxInbounds 解析 sing-box 配置文件中的 inbounds
func parseSingBoxInbounds(content string, listeningPorts map[int]bool) []ProxyInfo {
	var proxies []ProxyInfo
	lines := strings.Split(content, "\n")
	var jsonBlocks []string
	var currentBlock []string
	inBlock := false

	for _, line := range lines {
		if idx := strings.Index(line, "FILE:"); idx >= 0 {
			before := line[:idx]
			if inBlock && len(strings.TrimSpace(before)) > 0 {
				currentBlock = append(currentBlock, before)
			}
			if inBlock && len(currentBlock) > 0 {
				jsonBlocks = append(jsonBlocks, strings.Join(currentBlock, "\n"))
				currentBlock = nil
			}
			inBlock = true
			continue
		}
		if inBlock {
			currentBlock = append(currentBlock, line)
		}
	}
	if inBlock && len(currentBlock) > 0 {
		jsonBlocks = append(jsonBlocks, strings.Join(currentBlock, "\n"))
	}
	if len(jsonBlocks) == 0 && strings.Contains(content, "{") {
		jsonBlocks = append(jsonBlocks, content)
	}

	for _, block := range jsonBlocks {
		start := strings.Index(block, "{")
		end := strings.LastIndex(block, "}")
		if start < 0 || end <= start {
			continue
		}

		var cfg struct {
			Inbounds []struct {
				Type       string `json:"type"`
				Tag        string `json:"tag"`
				Listen     string `json:"listen"`
				ListenPort int    `json:"listen_port"`
				Port       int    `json:"port"`
				Method     string `json:"method"`
				Password   string `json:"password"`
				Users      []struct {
					UUID string `json:"uuid"`
					Flow string `json:"flow"`
				} `json:"users"`
				TLS struct {
					Enabled    bool   `json:"enabled"`
					ServerName string `json:"server_name"`
					Reality    struct {
						Enabled   bool   `json:"enabled"`
						PublicKey string `json:"public_key"`
						ShortID   string `json:"short_id"`
					} `json:"reality"`
				} `json:"tls"`
				Transport struct {
					Type string `json:"type"`
					Path string `json:"path"`
				} `json:"transport"`
			} `json:"inbounds"`
		}

		if err := json.Unmarshal([]byte(block[start:end+1]), &cfg); err == nil {
			for _, in := range cfg.Inbounds {
				if in.Listen == "127.0.0.1" || in.Listen == "::1" || in.Listen == "localhost" {
					continue
				}
				p := in.ListenPort
				if p <= 0 {
					p = in.Port
				}
				if p <= 0 {
					continue
				}
				info := ProxyInfo{
					Port:     p,
					Protocol: in.Type,
					Remark:   in.Tag,
					Status:   "stopped",
					Method:   in.Method,
					Password: in.Password,
				}
				if listeningPorts[p] {
					info.Status = "listening"
				}
				if len(in.Users) > 0 {
					info.UUID = in.Users[0].UUID
					info.Flow = in.Users[0].Flow
				}
				if in.TLS.Reality.Enabled {
					info.Security = "reality"
					info.SNI = in.TLS.ServerName
					info.PublicKey = in.TLS.Reality.PublicKey
					info.ShortID = in.TLS.Reality.ShortID
				} else if in.TLS.Enabled {
					info.Security = "tls"
					info.SNI = in.TLS.ServerName
				}
				if in.Transport.Type == "ws" {
					info.Network = "ws"
					info.Path = in.Transport.Path
					if info.Protocol == "vless" {
						info.Protocol = "vless_ws"
					}
				}
				proxies = append(proxies, info)
			}
		}
	}
	return proxies
}

// parseXrayInbounds 解析 xray 配置文件中的 inbounds
func parseXrayInbounds(content string, listeningPorts map[int]bool) []ProxyInfo {
	var proxies []ProxyInfo
	lines := strings.Split(content, "\n")
	var jsonBlocks []string
	var currentBlock []string
	inBlock := false

	for _, line := range lines {
		if idx := strings.Index(line, "FILE:"); idx >= 0 {
			before := line[:idx]
			if inBlock && len(strings.TrimSpace(before)) > 0 {
				currentBlock = append(currentBlock, before)
			}
			if inBlock && len(currentBlock) > 0 {
				jsonBlocks = append(jsonBlocks, strings.Join(currentBlock, "\n"))
				currentBlock = nil
			}
			inBlock = true
			continue
		}
		if inBlock {
			currentBlock = append(currentBlock, line)
		}
	}
	if inBlock && len(currentBlock) > 0 {
		jsonBlocks = append(jsonBlocks, strings.Join(currentBlock, "\n"))
	}
	if len(jsonBlocks) == 0 && strings.Contains(content, "{") {
		jsonBlocks = append(jsonBlocks, content)
	}

	for _, block := range jsonBlocks {
		start := strings.Index(block, "{")
		end := strings.LastIndex(block, "}")
		if start < 0 || end <= start {
			continue
		}

		var cfg struct {
			Inbounds []struct {
				Port           int             `json:"port"`
				Protocol       string          `json:"protocol"`
				Tag            string          `json:"tag"`
				Settings       json.RawMessage `json:"settings"`
				StreamSettings struct {
					Network         string `json:"network"`
					Security        string `json:"security"`
					RealitySettings struct {
						ServerName  string   `json:"serverName"`
						ServerNames []string `json:"serverNames"`
						PublicKey   string   `json:"publicKey"`
						ShortId     string   `json:"shortId"`
						ShortIds    []string `json:"shortIds"`
					} `json:"realitySettings"`
					WsSettings struct {
						Path string `json:"path"`
					} `json:"wsSettings"`
				} `json:"streamSettings"`
			} `json:"inbounds"`
		}

		if err := json.Unmarshal([]byte(block[start:end+1]), &cfg); err == nil {
			for _, in := range cfg.Inbounds {
				if in.Port <= 0 {
					continue
				}
				info := ProxyInfo{
					Port:     in.Port,
					Protocol: in.Protocol,
					Remark:   in.Tag,
					Status:   "stopped",
				}
				if listeningPorts[in.Port] {
					info.Status = "listening"
				}

				// 提取 UUID 与 Flow
				var clientsWrap struct {
					Clients []struct {
						ID   string `json:"id"`
						Flow string `json:"flow"`
					} `json:"clients"`
				}
				if err := json.Unmarshal(in.Settings, &clientsWrap); err == nil && len(clientsWrap.Clients) > 0 {
					info.UUID = clientsWrap.Clients[0].ID
					if clientsWrap.Clients[0].Flow != "" {
						info.Flow = clientsWrap.Clients[0].Flow
					}
				}

				info.Network = in.StreamSettings.Network
				info.Security = in.StreamSettings.Security
				if in.StreamSettings.Security == "reality" {
					if in.StreamSettings.RealitySettings.ServerName != "" {
						info.SNI = in.StreamSettings.RealitySettings.ServerName
					} else if len(in.StreamSettings.RealitySettings.ServerNames) > 0 {
						info.SNI = in.StreamSettings.RealitySettings.ServerNames[0]
					}
					info.PublicKey = in.StreamSettings.RealitySettings.PublicKey
					if in.StreamSettings.RealitySettings.ShortId != "" {
						info.ShortID = in.StreamSettings.RealitySettings.ShortId
					} else {
						for _, sid := range in.StreamSettings.RealitySettings.ShortIds {
							if sid != "" {
								info.ShortID = sid
								break
							}
						}
					}
				}
				if in.StreamSettings.Network == "ws" {
					info.Path = in.StreamSettings.WsSettings.Path
					if info.Protocol == "vless" {
						info.Protocol = "vless_ws"
					}
				}

				proxies = append(proxies, info)
			}
		}
	}

	return proxies
}

// GenerateProxyLink 生成标准节点分享链接 (VLESS Reality / VLESS WS / SS2022)
func GenerateProxyLink(p *ProxyInfo, ip, hostName string) string {
	name := p.Remark
	if name == "" {
		name = hostName
	}
	if hostName != "" && !strings.Contains(name, hostName) {
		name = fmt.Sprintf("%s-%s", hostName, name)
	}

	proto := strings.ToLower(p.Protocol)
	switch proto {
	case "vless":
		if p.Security == "reality" {
			flow := p.Flow
			if flow == "" {
				flow = "xtls-rprx-vision"
			}
			net := p.Network
			if net == "" {
				net = "tcp"
			}
			return fmt.Sprintf("vless://%s@%s:%d?encryption=none&flow=%s&security=reality&sni=%s&fp=chrome&pbk=%s&sid=%s&type=%s#%s",
				p.UUID, ip, p.Port, flow, p.SNI, p.PublicKey, p.ShortID, net, url.PathEscape(name))
		}
		if p.Network == "ws" {
			return fmt.Sprintf("vless://%s@%s:%d?path=%s&security=%s&sni=%s&type=ws#%s",
				p.UUID, ip, p.Port, url.QueryEscape(p.Path), p.Security, p.SNI, url.PathEscape(name))
		}
	case "vless_ws":
		return fmt.Sprintf("vless://%s@%s:%d?path=%s&security=%s&sni=%s&type=ws#%s",
			p.UUID, ip, p.Port, url.QueryEscape(p.Path), p.Security, p.SNI, url.PathEscape(name))
	case "shadowsocks", "ss":
		if p.Method != "" && p.Password != "" {
			userinfo := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", p.Method, p.Password)))
			return fmt.Sprintf("ss://%s@%s:%d#%s", userinfo, ip, p.Port, url.PathEscape(name))
		}
	}
	return ""
}
