package sshexec

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/audit"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"github.com/google/uuid"
)

// AddProxyRequest 新增代理请求参数
type AddProxyRequest struct {
	Port       int    `json:"port"`
	Protocol   string `json:"protocol,omitempty"`    // 默认 "vless"
	Security   string `json:"security,omitempty"`    // 默认 "reality"
	SNI        string `json:"sni,omitempty"`         // 默认 "aws.amazon.com"
	UUID       string `json:"uuid,omitempty"`        // 可选，未传则自动生成
	PublicKey  string `json:"public_key,omitempty"`  // 可选，未传且 reality 则自动生成
	PrivateKey string `json:"private_key,omitempty"` // 可选，未传且 reality 则自动生成
	ShortID    string `json:"short_id,omitempty"`    // 可选，未传且 reality 则自动生成
	Remark     string `json:"remark,omitempty"`      // 可选，备注说明
}

// ChangePortResult 换端口自愈响应
type ChangePortResult struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	OldPort   int    `json:"old_port"`
	NewPort   int    `json:"new_port"`
	Listening bool   `json:"listening"`
}

// GenerateRealityKeyPair 纯 Go 原生生成 Xray Reality 兼容的 x25519 密钥对与 ShortID
func GenerateRealityKeyPair() (privateKeyBase64, publicKeyBase64, shortID string, err error) {
	curve := ecdh.X25519()
	priv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", "", fmt.Errorf("generate x25519 private key failed: %w", err)
	}

	privateKeyBase64 = base64.RawURLEncoding.EncodeToString(priv.Bytes())
	publicKeyBase64 = base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes())

	sidBytes := make([]byte, 4)
	if _, err := rand.Read(sidBytes); err == nil {
		shortID = hex.EncodeToString(sidBytes)
	} else {
		shortID = "3c4ffcc0"
	}

	return privateKeyBase64, publicKeyBase64, shortID, nil
}

// ChangeProxyPort 修改主机代理入站端口（自愈专用）
func ChangeProxyPort(ctx context.Context, inst *providers.Instance, oldPort, newPort int) (*ChangePortResult, error) {
	if oldPort <= 0 || newPort <= 0 {
		return nil, errors.New("old_port and new_port must be positive port numbers")
	}
	if oldPort == newPort {
		return &ChangePortResult{
			Success:   true,
			Message:   fmt.Sprintf("Old port and new port are identical (%d), no action needed", oldPort),
			OldPort:   oldPort,
			NewPort:   newPort,
			Listening: true,
		}, nil
	}

	fillDefaults(inst)

	// 先探查管理类型与 3x-ui 数据库路径
	checkScript := `
for p in /opt/1panel/apps/3x-ui/db/x-ui.db /etc/x-ui/x-ui.db /usr/local/x-ui/db/x-ui.db; do
  if [ -f "$p" ]; then
    echo "DB_PATH:$p"
    exit 0
  fi
done
`
	res, err := ExecuteCommand(ctx, inst, checkScript, 8*time.Second)
	if err != nil {
		return nil, fmt.Errorf("probe database path failed: %w", err)
	}

	dbPath := ""
	for _, line := range strings.Split(res.Stdout, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "DB_PATH:") {
			dbPath = strings.TrimPrefix(strings.TrimSpace(line), "DB_PATH:")
			break
		}
	}

	if dbPath != "" {
		// 3x-ui 管理类型
		updateScript := fmt.Sprintf(`
sqlite3 "%s" "UPDATE inbounds SET port = %d, tag = REPLACE(tag, '%d', '%d') WHERE port = %d;"
docker restart 3x-ui 2>/dev/null || systemctl restart x-ui 2>/dev/null || true
ufw allow %d/tcp 2>/dev/null || iptables -I INPUT -p tcp --dport %d -j ACCEPT 2>/dev/null || true
sleep 2
ss -tulpn | grep -E ':%d\b'
`, dbPath, newPort, oldPort, newPort, oldPort, newPort, newPort, newPort)

		out, err := ExecuteCommand(ctx, inst, updateScript, 15*time.Second)
		if err != nil {
			return nil, fmt.Errorf("execute port change update script failed: %w", err)
		}

		listening := strings.Contains(out.Stdout, fmt.Sprintf(":%d", newPort))
		msg := fmt.Sprintf("3x-ui 代理端口已从 %d 切换为 %d (监听状态: %v)", oldPort, newPort, listening)

		return &ChangePortResult{
			Success:   listening,
			Message:   msg,
			OldPort:   oldPort,
			NewPort:   newPort,
			Listening: listening,
		}, nil
	}

	// 原生 sing-box 或 xray 配置文件场景
	replacer := strings.NewReplacer(
		"{OLD_PORT}", strconv.Itoa(oldPort),
		"{NEW_PORT}", strconv.Itoa(newPort),
	)
	cfgChangeScript := replacer.Replace(`
MODIFIED=0
if [ -f /etc/sing-box/config.json ]; then
  sed -i 's/"listen_port": {OLD_PORT}/"listen_port": {NEW_PORT}/g' /etc/sing-box/config.json
  systemctl reload sing-box 2>/dev/null || systemctl restart sing-box 2>/dev/null || true
  MODIFIED=1
fi
if [ -f /etc/v2ray-agent/docker/config.json ]; then
  sed -i 's/"port": {OLD_PORT}/"port": {NEW_PORT}/g' /etc/v2ray-agent/docker/config.json
  docker restart v2ray-agent-docker 2>/dev/null || true
  MODIFIED=1
fi
if [ -f /usr/local/etc/xray/config.json ]; then
  sed -i 's/"port": {OLD_PORT}/"port": {NEW_PORT}/g' /usr/local/etc/xray/config.json
  systemctl reload xray 2>/dev/null || systemctl restart xray 2>/dev/null || true
  MODIFIED=1
fi
if [ -f /etc/xray/config.json ]; then
  sed -i 's/"port": {OLD_PORT}/"port": {NEW_PORT}/g' /etc/xray/config.json
  systemctl reload xray 2>/dev/null || systemctl restart xray 2>/dev/null || true
  MODIFIED=1
fi
ufw allow {NEW_PORT}/tcp 2>/dev/null || iptables -I INPUT -p tcp --dport {NEW_PORT} -j ACCEPT 2>/dev/null || true
sleep 2
ss -tulpn | grep -E ':{NEW_PORT}\b'
`)

	out, err := ExecuteCommand(ctx, inst, cfgChangeScript, 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("execute config port change script failed: %w", err)
	}

	listening := strings.Contains(out.Stdout, fmt.Sprintf(":%d", newPort))
	msg := fmt.Sprintf("原生配置代理端口已从 %d 切换为 %d (监听状态: %v)", oldPort, newPort, listening)

	return &ChangePortResult{
		Success:   listening,
		Message:   msg,
		OldPort:   oldPort,
		NewPort:   newPort,
		Listening: listening,
	}, nil
}

// ChangeProxyPortWithAudit 修改代理端口并自动审计
func ChangeProxyPortWithAudit(ctx context.Context, source, idOrIP string, oldPort, newPort int) (*ChangePortResult, error) {
	inst, err := FindInstance(idOrIP)
	if err != nil {
		audit.Record(source, "vps.change_proxy_port", "system", map[string]interface{}{
			"id_or_ip": idOrIP,
			"old_port": oldPort,
			"new_port": newPort,
		}, "FAILED", fmt.Sprintf("Instance not found: %v", err), 0)
		return nil, err
	}

	start := time.Now()
	res, err := ChangeProxyPort(ctx, inst, oldPort, newPort)
	dur := time.Since(start).Milliseconds()

	if err != nil {
		audit.Record(source, "vps.change_proxy_port", inst.ProviderID, map[string]interface{}{
			"instance_id": inst.ID,
			"ip":          inst.IP,
			"old_port":    oldPort,
			"new_port":    newPort,
		}, "FAILED", err.Error(), dur)
		return nil, err
	}

	audit.Record(source, "vps.change_proxy_port", inst.ProviderID, map[string]interface{}{
		"instance_id": inst.ID,
		"ip":          inst.IP,
		"old_port":    oldPort,
		"new_port":    newPort,
		"listening":   res.Listening,
	}, "SUCCESS", res.Message, dur)

	return res, nil
}

// AddProxy 新增入站代理
func AddProxy(ctx context.Context, inst *providers.Instance, req AddProxyRequest) (*ProxyInfo, error) {
	if req.Port <= 0 || req.Port > 65535 {
		return nil, errors.New("port must be between 1 and 65535")
	}

	fillDefaults(inst)

	if req.Protocol == "" {
		req.Protocol = "vless"
	}
	if req.Security == "" {
		req.Security = "reality"
	}
	if req.SNI == "" {
		req.SNI = "aws.amazon.com"
	}
	if req.UUID == "" {
		req.UUID = uuid.New().String()
	}
	if req.Remark == "" {
		req.Remark = fmt.Sprintf("AI-Proxy-%d", req.Port)
	}

	if req.Security == "reality" && (req.PublicKey == "" || req.PrivateKey == "") {
		priv, pub, sid, err := GenerateRealityKeyPair()
		if err != nil {
			return nil, fmt.Errorf("failed to generate reality key pair: %w", err)
		}
		if req.PrivateKey == "" {
			req.PrivateKey = priv
		}
		if req.PublicKey == "" {
			req.PublicKey = pub
		}
		if req.ShortID == "" {
			req.ShortID = sid
		}
	}
	if req.ShortID == "" {
		req.ShortID = "3c4ffcc0"
	}

	// 探查 3x-ui DB
	dbProbeScript := `
for p in /opt/1panel/apps/3x-ui/db/x-ui.db /etc/x-ui/x-ui.db /usr/local/x-ui/db/x-ui.db; do
  if [ -f "$p" ]; then
    echo "DB_PATH:$p"
    exit 0
  fi
done
`
	res, err := ExecuteCommand(ctx, inst, dbProbeScript, 8*time.Second)
	if err != nil {
		return nil, fmt.Errorf("probe 3x-ui db failed: %w", err)
	}

	dbPath := ""
	for _, line := range strings.Split(res.Stdout, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "DB_PATH:") {
			dbPath = strings.TrimPrefix(strings.TrimSpace(line), "DB_PATH:")
			break
		}
	}

	if dbPath == "" {
		return nil, errors.New("unsupported host environment: 3x-ui database not found on remote host")
	}

	// 组装 3x-ui settings JSON
	settingsObj := map[string]interface{}{
		"clients": []map[string]interface{}{
			{
				"id":         req.UUID,
				"flow":       "xtls-rprx-vision",
				"email":      fmt.Sprintf("user-%d@vps-hub", req.Port),
				"enable":     true,
				"expiryTime": 0,
				"totalGB":    0,
				"reset":      0,
			},
		},
		"decryption": "none",
	}
	settingsJSON, _ := json.Marshal(settingsObj)

	// 组装 3x-ui stream_settings JSON
	streamObj := map[string]interface{}{
		"network": "tcp",
		"tcpSettings": map[string]interface{}{
			"acceptProxyProtocol": false,
			"header": map[string]interface{}{
				"type": "none",
			},
		},
		"security": req.Security,
	}

	if req.Security == "reality" {
		streamObj["realitySettings"] = map[string]interface{}{
			"show":        false,
			"xver":        0,
			"target":      fmt.Sprintf("%s:443", req.SNI),
			"serverNames": []string{req.SNI},
			"privateKey":  req.PrivateKey,
			"shortIds":    []string{req.ShortID},
			"settings": map[string]interface{}{
				"publicKey":   req.PublicKey,
				"fingerprint": "chrome",
				"serverName":  "",
				"spiderX":     "/",
			},
		}
	}
	streamJSON, _ := json.Marshal(streamObj)

	// 安全转义单引号以构造 SQL
	escSettings := strings.ReplaceAll(string(settingsJSON), "'", "''")
	escStream := strings.ReplaceAll(string(streamJSON), "'", "''")
	escRemark := strings.ReplaceAll(req.Remark, "'", "''")

	insertSQL := fmt.Sprintf(`
COUNT=$(sqlite3 "%s" "SELECT count(*) FROM inbounds WHERE port = %d;")
if [ "$COUNT" -gt 0 ]; then
  echo "PORT_EXISTS"
  exit 1
fi

sqlite3 "%s" "INSERT INTO inbounds (user_id, up, down, total, remark, sub_sort_index, enable, expiry_time, traffic_reset, last_traffic_reset_time, listen, port, protocol, settings, stream_settings, tag, sniffing, node_id, share_addr_strategy) VALUES (1, 0, 0, 0, '%s', 1, 1, 0, 'never', 0, '', %d, '%s', '%s', '%s', 'inbound-%d', '{\"enabled\":false}', 0, 'node');"

docker restart 3x-ui 2>/dev/null || systemctl restart x-ui 2>/dev/null || true
ufw allow %d/tcp 2>/dev/null || iptables -I INPUT -p tcp --dport %d -j ACCEPT 2>/dev/null || true
sleep 3
ss -tulpn | grep -E ':%d\b'
`, dbPath, req.Port, dbPath, escRemark, req.Port, req.Protocol, escSettings, escStream, req.Port, req.Port, req.Port, req.Port)

	out, err := ExecuteCommand(ctx, inst, insertSQL, 18*time.Second)
	if err != nil {
		if strings.Contains(out.Stdout, "PORT_EXISTS") {
			return nil, fmt.Errorf("port %d already exists in 3x-ui inbounds", req.Port)
		}
		return nil, fmt.Errorf("insert inbound execution failed: %w (out: %s)", err, out.Stdout)
	}

	listening := strings.Contains(out.Stdout, fmt.Sprintf(":%d", req.Port))
	status := "stopped"
	if listening {
		status = "listening"
	}

	return &ProxyInfo{
		Port:      req.Port,
		Protocol:  req.Protocol,
		Security:  req.Security,
		Network:   "tcp",
		SNI:       req.SNI,
		UUID:      req.UUID,
		PublicKey: req.PublicKey,
		ShortID:   req.ShortID,
		Flow:      "xtls-rprx-vision",
		Remark:    req.Remark,
		Status:    status,
	}, nil
}

// AddProxyWithAudit 新增代理并记录审计
func AddProxyWithAudit(ctx context.Context, source, idOrIP string, req AddProxyRequest) (*ProxyInfo, error) {
	inst, err := FindInstance(idOrIP)
	if err != nil {
		audit.Record(source, "vps.add_proxy", "system", map[string]interface{}{
			"id_or_ip": idOrIP,
			"port":     req.Port,
		}, "FAILED", fmt.Sprintf("Instance not found: %v", err), 0)
		return nil, err
	}

	start := time.Now()
	p, err := AddProxy(ctx, inst, req)
	dur := time.Since(start).Milliseconds()

	if err != nil {
		audit.Record(source, "vps.add_proxy", inst.ProviderID, map[string]interface{}{
			"instance_id": inst.ID,
			"ip":          inst.IP,
			"port":        req.Port,
			"protocol":    req.Protocol,
		}, "FAILED", err.Error(), dur)
		return nil, err
	}

	audit.Record(source, "vps.add_proxy", inst.ProviderID, map[string]interface{}{
		"instance_id": inst.ID,
		"ip":          inst.IP,
		"port":        p.Port,
		"protocol":    p.Protocol,
		"security":    p.Security,
		"sni":         p.SNI,
		"uuid":        p.UUID,
		"status":      p.Status,
	}, "SUCCESS", fmt.Sprintf("Added proxy on port %d (%s/%s)", p.Port, p.Protocol, p.Security), dur)

	return p, nil
}

// DeleteProxy 下线并删除指定端口代理
func DeleteProxy(ctx context.Context, inst *providers.Instance, port int) error {
	if port <= 0 {
		return errors.New("invalid port")
	}

	fillDefaults(inst)

	delScript := fmt.Sprintf(`
DB_PATH=""
for p in /opt/1panel/apps/3x-ui/db/x-ui.db /etc/x-ui/x-ui.db /usr/local/x-ui/db/x-ui.db; do
  if [ -f "$p" ]; then
    DB_PATH="$p"
    break
  fi
done

if [ -n "$DB_PATH" ]; then
  sqlite3 "$DB_PATH" "DELETE FROM inbounds WHERE port = %d;"
  docker restart 3x-ui 2>/dev/null || systemctl restart x-ui 2>/dev/null || true
fi

sleep 2
ss -tulpn | grep -E ':%d\b' || true
`, port, port)

	out, err := ExecuteCommand(ctx, inst, delScript, 15*time.Second)
	if err != nil {
		return fmt.Errorf("delete proxy execution failed: %w", err)
	}

	// 检查端口是否仍有监听
	if strings.Contains(out.Stdout, fmt.Sprintf(":%d", port)) {
		return fmt.Errorf("warning: port %d is still listening after delete operation", port)
	}

	return nil
}

// DeleteProxyWithAudit 删除代理并记录审计
func DeleteProxyWithAudit(ctx context.Context, source, idOrIP string, port int) error {
	inst, err := FindInstance(idOrIP)
	if err != nil {
		audit.Record(source, "vps.delete_proxy", "system", map[string]interface{}{
			"id_or_ip": idOrIP,
			"port":     port,
		}, "FAILED", fmt.Sprintf("Instance not found: %v", err), 0)
		return err
	}

	start := time.Now()
	err = DeleteProxy(ctx, inst, port)
	dur := time.Since(start).Milliseconds()

	if err != nil {
		audit.Record(source, "vps.delete_proxy", inst.ProviderID, map[string]interface{}{
			"instance_id": inst.ID,
			"ip":          inst.IP,
			"port":        port,
		}, "FAILED", err.Error(), dur)
		return err
	}

	audit.Record(source, "vps.delete_proxy", inst.ProviderID, map[string]interface{}{
		"instance_id": inst.ID,
		"ip":          inst.IP,
		"port":        port,
	}, "SUCCESS", fmt.Sprintf("Deleted proxy on port %d and released port", port), dur)

	return nil
}
