package sshexec

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
)

// 原生 sing-box 生命周期（面向 Alpine / OpenRC / 无 Docker / 无 python3 的 NAT 实例）
//
// 设计约束：
//  1. 远程脚本只允许 BusyBox sh 内建 + wget/tar/rc-service，禁止 python3/docker/jq/sed -i；
//  2. 配置由 Go 侧整份渲染后 base64 传输，避免 heredoc 转义与空格敏感的 sed 替换；
//  3. 换端口不改宿机配置，只做 NAT 公网端口重发布（见 RepublishNATPort）。

// SingBoxVersion 默认部署版本；升级时改此常量即可。
const SingBoxVersion = "1.11.15"

// singBoxConfigPath 宿机配置路径。
const singBoxConfigPath = "/etc/sing-box/config.json"

// SingBoxInbound 单个 VLESS+Reality 入站参数。
type SingBoxInbound struct {
	Tag        string `json:"-"`
	ListenPort int    `json:"-"`
	UUID       string `json:"-"`
	Flow       string `json:"-"`
	SNI        string `json:"-"`
	PrivateKey string `json:"-"`
	ShortID    string `json:"-"`
}

// SingBoxNativeHealth 原生 sing-box 运行态。
type SingBoxNativeHealth struct {
	Installed      bool
	Running        bool
	Version        string
	ServiceStatus  string
	ListeningPorts []int
}

type renderedInbound struct {
	Type       string            `json:"type"`
	Tag        string            `json:"tag"`
	Listen     string            `json:"listen"`
	ListenPort int               `json:"listen_port"`
	Users      []renderedUser    `json:"users"`
	TLS        renderedInboundTL `json:"tls"`
}

type renderedUser struct {
	UUID string `json:"uuid"`
	Flow string `json:"flow"`
}

type renderedInboundTL struct {
	Enabled    bool            `json:"enabled"`
	ServerName string          `json:"server_name"`
	Reality    renderedReality `json:"reality"`
}

type renderedReality struct {
	Enabled    bool              `json:"enabled"`
	Handshake  renderedHandshake `json:"handshake"`
	PrivateKey string            `json:"private_key"`
	ShortID    []string          `json:"short_id"`
}

type renderedHandshake struct {
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
}

type renderedSingBoxConfig struct {
	Log       renderedLog         `json:"log"`
	Inbounds  []renderedInbound   `json:"inbounds"`
	Outbounds []map[string]string `json:"outbounds"`
}

type renderedLog struct {
	Level     string `json:"level"`
	Timestamp bool   `json:"timestamp"`
	Output    string `json:"output"`
}

// RenderSingBoxConfig 渲染完整 sing-box 配置（单入站 VLESS+Reality）。
func RenderSingBoxConfig(in SingBoxInbound) ([]byte, error) {
	if err := validateInbound(in); err != nil {
		return nil, err
	}

	cfg := renderedSingBoxConfig{
		Log: renderedLog{Level: "warn", Timestamp: true, Output: "/var/log/sing-box.log"},
		Inbounds: []renderedInbound{{
			Type:       "vless",
			Tag:        in.Tag,
			Listen:     "0.0.0.0",
			ListenPort: in.ListenPort,
			Users:      []renderedUser{{UUID: in.UUID, Flow: in.Flow}},
			TLS: renderedInboundTL{
				Enabled:    true,
				ServerName: in.SNI,
				Reality: renderedReality{
					Enabled:    true,
					Handshake:  renderedHandshake{Server: in.SNI, ServerPort: 443},
					PrivateKey: in.PrivateKey,
					ShortID:    []string{in.ShortID},
				},
			},
		}},
		Outbounds: []map[string]string{{"type": "direct", "tag": "direct"}},
	}

	return json.MarshalIndent(cfg, "", "  ")
}

func validateInbound(in SingBoxInbound) error {
	if strings.TrimSpace(in.Tag) == "" {
		return errors.New("inbound tag is required")
	}
	if strings.ContainsAny(in.Tag, "\"'` \t\n/\\$") {
		return fmt.Errorf("inbound tag %q contains illegal characters", in.Tag)
	}
	if in.ListenPort <= 0 || in.ListenPort > 65535 {
		return fmt.Errorf("listen port %d out of range 1-65535", in.ListenPort)
	}
	if strings.TrimSpace(in.UUID) == "" {
		return errors.New("uuid is required")
	}
	if strings.TrimSpace(in.SNI) == "" {
		return errors.New("sni is required")
	}
	if strings.TrimSpace(in.PrivateKey) == "" {
		return errors.New("reality private key is required")
	}
	if strings.TrimSpace(in.ShortID) == "" {
		return errors.New("reality short id is required")
	}
	if in.Flow == "" {
		// sing-box 默认 vision 流控；空值会导致部分客户端握手失败。
		return errors.New("flow is required (e.g. xtls-rprx-vision)")
	}
	return nil
}

// SingBoxDownloadURL 返回官方静态二进制下载地址（linux 静态构建，musl 可运行）。
func SingBoxDownloadURL(version, arch string) (string, error) {
	if version == "" {
		version = SingBoxVersion
	}
	switch arch {
	case "x86_64", "amd64":
		return fmt.Sprintf("https://github.com/SagerNet/sing-box/releases/download/v%s/sing-box-%s-linux-amd64.tar.gz", version, version), nil
	case "aarch64", "arm64":
		return fmt.Sprintf("https://github.com/SagerNet/sing-box/releases/download/v%s/sing-box-%s-linux-arm64.tar.gz", version, version), nil
	default:
		return "", fmt.Errorf("unsupported architecture %q (only x86_64/amd64/aarch64 supported)", arch)
	}
}

// BuildInstallScript 生成宿机安装脚本：下发二进制 + 写配置 + 注册 OpenRC 服务 + 启动。
// 脚本不得依赖 python3 / docker / jq / sed -i。
func BuildInstallScript(cfg []byte, arch, version string) (string, error) {
	if len(cfg) == 0 {
		return "", errors.New("sing-box config is empty")
	}
	var probe map[string]any
	if err := json.Unmarshal(cfg, &probe); err != nil {
		return "", fmt.Errorf("sing-box config is not valid json: %w", err)
	}
	url, err := SingBoxDownloadURL(version, arch)
	if err != nil {
		return "", err
	}
	if version == "" {
		version = SingBoxVersion
	}
	b64 := base64.StdEncoding.EncodeToString(cfg)

	var sb strings.Builder
	sb.WriteString("#!/bin/sh\nset -eu\n")
	sb.WriteString(fmt.Sprintf("ARCH=\"$(uname -m)\"\nif [ \"$ARCH\" != \"x86_64\" ] && [ \"$ARCH\" != \"aarch64\" ]; then echo \"UNSUPPORTED_ARCH:$ARCH\"; exit 2; fi\n"))
	sb.WriteString("command -v rc-service >/dev/null 2>&1 || { echo \"OPENRC_MISSING\"; exit 3; }\n")
	sb.WriteString(fmt.Sprintf("SB_URL='%s'\n", url))
	sb.WriteString(fmt.Sprintf("SB_VER='%s'\n", version))
	sb.WriteString("mkdir -p /etc/sing-box /var/log\n")

	// 1) 二进制：已存在且版本一致则跳过下载，节省流量。
	sb.WriteString("NEED_DL=1\n")
	sb.WriteString("if [ -x /usr/local/bin/sing-box ]; then\n")
	sb.WriteString("  if /usr/local/bin/sing-box version 2>/dev/null | head -n1 | grep -q \"$SB_VER\"; then NEED_DL=0; fi\n")
	sb.WriteString("fi\n")
	sb.WriteString("if [ \"$NEED_DL\" = \"1\" ]; then\n")
	sb.WriteString("  rm -rf /tmp/sb-dl && mkdir -p /tmp/sb-dl\n")
	sb.WriteString("  wget -q -O /tmp/sb-dl/sb.tar.gz \"$SB_URL\" || { echo \"DOWNLOAD_FAILED\"; exit 4; }\n")
	sb.WriteString("  tar -xzf /tmp/sb-dl/sb.tar.gz -C /tmp/sb-dl || { echo \"EXTRACT_FAILED\"; exit 5; }\n")
	sb.WriteString("  SB_BIN=\"$(find /tmp/sb-dl -type f -name sing-box | head -n1)\"\n")
	sb.WriteString("  [ -n \"$SB_BIN\" ] || { echo \"BINARY_NOT_FOUND\"; exit 6; }\n")
	sb.WriteString("  cp \"$SB_BIN\" /usr/local/bin/sing-box\n")
	sb.WriteString("  chmod +x /usr/local/bin/sing-box\n")
	sb.WriteString("  rm -rf /tmp/sb-dl\n")
	sb.WriteString("fi\n")

	// 2) 配置：原子替换 + 预检，失败回滚。
	sb.WriteString(fmt.Sprintf("CFG='%s'\n", singBoxConfigPath))
	sb.WriteString("if [ -f \"$CFG\" ]; then cp \"$CFG\" \"$CFG.bak\"; fi\n")
	sb.WriteString(fmt.Sprintf("printf '%%s' '%s' | base64 -d > \"$CFG.new\"\n", b64))
	sb.WriteString("if ! /usr/local/bin/sing-box check -c \"$CFG.new\" >/dev/null 2>&1; then\n")
	sb.WriteString("  echo \"CONFIG_CHECK_FAILED\"\n")
	sb.WriteString("  rm -f \"$CFG.new\"\n")
	sb.WriteString("  [ -f \"$CFG.bak\" ] && cp \"$CFG.bak\" \"$CFG\"\n")
	sb.WriteString("  exit 7\n")
	sb.WriteString("fi\n")
	sb.WriteString("mv \"$CFG.new\" \"$CFG\"\n")

	// 3) OpenRC 服务脚本。
	sb.WriteString("cat > /etc/init.d/sing-box <<'INITEOF'\n")
	sb.WriteString("#!/sbin/openrc-run\n")
	sb.WriteString("name=\"sing-box\"\n")
	sb.WriteString("description=\"sing-box proxy server\"\n")
	sb.WriteString("command=\"/usr/local/bin/sing-box\"\n")
	sb.WriteString("command_args=\"run -c /etc/sing-box/config.json\"\n")
	sb.WriteString("command_background=\"yes\"\n")
	sb.WriteString("pidfile=\"/run/sing-box.pid\"\n")
	sb.WriteString("depend() {\n  need net\n  after firewall\n}\n")
	sb.WriteString("INITEOF\n")
	sb.WriteString("chmod +x /etc/init.d/sing-box\n")
	sb.WriteString("rc-update add sing-box default >/dev/null 2>&1 || true\n")
	sb.WriteString("rc-service sing-box restart >/dev/null 2>&1 || rc-service sing-box start >/dev/null 2>&1 || true\n")

	// 4) 回执。
	sb.WriteString("sleep 2\n")
	sb.WriteString("echo \"VER:$(/usr/local/bin/sing-box version 2>/dev/null | head -n1)\"\n")
	sb.WriteString("echo \"SVC:$(rc-service sing-box status 2>&1 | head -n1)\"\n")
	sb.WriteString("echo \"PORTS:\"\n")
	sb.WriteString("ss -tlnp 2>/dev/null || netstat -tlnp 2>/dev/null || true\n")
	sb.WriteString("echo \"DONE\"\n")

	return sb.String(), nil
}

var (
	realityPrivRe = regexp.MustCompile(`(?mi)^\s*PrivateKey:\s*(\S+)\s*$`)
	realityPubRe  = regexp.MustCompile(`(?mi)^\s*PublicKey:\s*(\S+)\s*$`)
	versionRe     = regexp.MustCompile(`(?i)sing-box version\s+(\S+)`)
	portColonRe   = regexp.MustCompile(`[:](\d{2,5})\s`)
	svcStartedRe  = regexp.MustCompile(`(?i)\b(started|running)\b`)
)

// ParseRealityKeyPair 解析 `sing-box generate reality-keypair` 输出。
func ParseRealityKeyPair(out string) (privateKey, publicKey string, err error) {
	priv := realityPrivRe.FindStringSubmatch(out)
	pub := realityPubRe.FindStringSubmatch(out)
	if len(priv) < 2 || len(pub) < 2 {
		return "", "", fmt.Errorf("cannot parse reality keypair from output: %q", truncate(out, 200))
	}
	return priv[1], pub[1], nil
}

// GenerateRealityKeyPairRemote 在宿机就地生成 Reality 密钥对（避免跨实现编码差异）。
func GenerateRealityKeyPairRemote(ctx context.Context, inst *providers.Instance) (privateKey, publicKey string, err error) {
	res, err := ExecuteCommand(ctx, inst, "/usr/local/bin/sing-box generate reality-keypair 2>&1", 15*time.Second)
	if err != nil {
		return "", "", fmt.Errorf("generate reality keypair failed: %w", err)
	}
	return ParseRealityKeyPair(res.Stdout)
}

// GenerateUUIDRemote 在宿机生成 v4 UUID。
func GenerateUUIDRemote(ctx context.Context, inst *providers.Instance) (string, error) {
	res, err := ExecuteCommand(ctx, inst, "/usr/local/bin/sing-box generate uuid 2>&1", 10*time.Second)
	if err != nil {
		return "", fmt.Errorf("generate uuid failed: %w", err)
	}
	uuid := strings.TrimSpace(res.Stdout)
	if len(uuid) < 32 {
		return "", fmt.Errorf("unexpected uuid output: %q", truncate(uuid, 80))
	}
	return uuid, nil
}

// ParseNativeHealth 解析原生 sing-box 探活脚本输出（见 BuildNativeHealthScript）。
func ParseNativeHealth(out string) SingBoxNativeHealth {
	h := SingBoxNativeHealth{}
	if m := versionRe.FindStringSubmatch(out); len(m) == 2 {
		h.Version = m[1]
		h.Installed = true
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "SVC:") {
			h.ServiceStatus = strings.TrimSpace(strings.TrimPrefix(line, "SVC:"))
			if svcStartedRe.MatchString(h.ServiceStatus) {
				h.Running = true
			}
		}
	}
	seen := map[int]bool{}
	for _, m := range portColonRe.FindAllStringSubmatch(out, -1) {
		if p, err := strconv.Atoi(m[1]); err == nil && !seen[p] {
			seen[p] = true
			h.ListeningPorts = append(h.ListeningPorts, p)
		}
	}
	return h
}

// BuildNativeHealthScript 构造探活脚本（ss / netstat 双兜底）。
func BuildNativeHealthScript() string {
	return strings.Join([]string{
		"echo \"VER:$(/usr/local/bin/sing-box version 2>/dev/null | head -n1)\"",
		"echo \"SVC:$(rc-service sing-box status 2>&1 | head -n1)\"",
		"ss -tlnp 2>/dev/null || netstat -tlnp 2>/dev/null || true",
	}, "\n")
}

// NativeSingBoxHealth 真机探活：版本 + OpenRC 状态 + 监听端口。
func NativeSingBoxHealth(ctx context.Context, inst *providers.Instance) (*SingBoxNativeHealth, error) {
	res, err := ExecuteCommand(ctx, inst, BuildNativeHealthScript(), 20*time.Second)
	if err != nil {
		return nil, err
	}
	h := ParseNativeHealth(res.Stdout)
	return &h, nil
}

// RepublishNATPort 重发布 NAT 公网端口：先新增映射，成功后再删旧映射。
// 私网端口保持不变，因此宿机 sing-box 配置与进程完全不动。
func RepublishNATPort(ctx context.Context, inst *providers.Instance, privatePort, oldMappingID int, remark string) (newPublicPort, newMappingID int, err error) {
	p, ok := providers.Get(inst.ProviderID)
	if !ok {
		return 0, 0, fmt.Errorf("provider %q not registered", inst.ProviderID)
	}
	pm, isMapper := p.(providers.PortMapper)
	if !isMapper {
		return 0, 0, fmt.Errorf("provider %q does not support port mapping", inst.ProviderID)
	}
	return republishNATPortWith(ctx, pm, inst.ID, privatePort, oldMappingID, remark)
}

// republishNATPortWith 是可注入核心，便于单元测试回滚路径。
func republishNATPortWith(ctx context.Context, pm providers.PortMapper, instanceID string, privatePort, oldMappingID int, remark string) (newPublicPort, newMappingID int, err error) {
	if privatePort <= 0 {
		return 0, 0, errors.New("private port must be positive")
	}
	mapping, err := pm.AddPortMapping(ctx, instanceID, privatePort, "tcp", remark)
	if err != nil {
		return 0, 0, fmt.Errorf("add port mapping for private port %d failed: %w", privatePort, err)
	}
	if mapping == nil || mapping.PublicPort <= 0 {
		return 0, 0, errors.New("provider returned empty public port")
	}
	if oldMappingID > 0 {
		if delErr := pm.DeletePortMapping(ctx, instanceID, oldMappingID); delErr != nil {
			_ = pm.DeletePortMapping(ctx, instanceID, mapping.ID)
			return 0, 0, fmt.Errorf("delete old mapping %d failed, rolled back: %w", oldMappingID, delErr)
		}
	}
	return mapping.PublicPort, mapping.ID, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
