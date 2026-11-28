package sshexec

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
)

// 原生 sing-box（无 Docker）部署编排：面向 Alpine/OpenRC + NAT 端口映射的小内存实例。

// NativeSingBoxDeployRequest 部署参数。
type NativeSingBoxDeployRequest struct {
	ListenPort int    // 宿机监听端口（NAT 实例的私网端口）
	SNI        string // Reality 目标域名，默认 www.microsoft.com
	Remark     string // 节点备注
}

// NativeSingBoxDeployResult 部署结果。
type NativeSingBoxDeployResult struct {
	UUID       string              `json:"uuid"`
	PublicKey  string              `json:"public_key"`
	ShortID    string              `json:"short_id"`
	ListenPort int                 `json:"listen_port"`
	PublicPort int                 `json:"public_port"`
	MappingID  int                 `json:"mapping_id"`
	ServerIP   string              `json:"server_ip"`
	VLESSURL   string              `json:"vless_url"`
	Health     SingBoxNativeHealth `json:"health"`
	Steps      []string            `json:"steps"`
}

const bootstrapConfig = `{"log":{"level":"warn","timestamp":true},"inbounds":[]}`

// DeployNativeSingBox 在目标实例上部署（或重建）单入站 Reality VLESS 服务。
//
// 顺序：引导安装二进制 → 宿机生成密钥/UUID → 渲染真实配置 → 重装并启动 → 健康校验 → NAT 映射。
// 任一步失败立即返回，不做静默降级。
func DeployNativeSingBox(ctx context.Context, inst *providers.Instance, req NativeSingBoxDeployRequest) (*NativeSingBoxDeployResult, error) {
	if inst == nil {
		return nil, fmt.Errorf("instance is nil")
	}
	if req.ListenPort <= 0 || req.ListenPort > 65535 {
		return nil, fmt.Errorf("listen_port must be within 1-65535")
	}
	if req.SNI == "" {
		req.SNI = "www.microsoft.com"
	}
	if req.Remark == "" {
		req.Remark = "sing-box"
	}
	res := &NativeSingBoxDeployResult{ListenPort: req.ListenPort, ServerIP: inst.IP}

	archOut, err := execOut(ctx, inst, "uname -m", 20*time.Second)
	if err != nil {
		return nil, fmt.Errorf("detect arch failed: %w", err)
	}
	arch := strings.TrimSpace(archOut)
	res.Steps = append(res.Steps, "arch="+arch)

	// 1) 引导：确保二进制与 OpenRC 服务存在（配置为空入站，不影响宿主）。
	health, _ := NativeSingBoxHealth(ctx, inst)
	if health == nil || !health.Installed {
		if err := runInstall(ctx, inst, []byte(bootstrapConfig), arch); err != nil {
			return nil, fmt.Errorf("bootstrap install failed: %w", err)
		}
		res.Steps = append(res.Steps, "bootstrap_installed")
	} else {
		res.Steps = append(res.Steps, "binary_present:"+health.Version)
	}

	// 2) 宿机就地生成 Reality 密钥对与 UUID（避免跨实现编码差异）。
	privateKey, publicKey, err := GenerateRealityKeyPairRemote(ctx, inst)
	if err != nil {
		return nil, err
	}
	uuid, err := GenerateUUIDRemote(ctx, inst)
	if err != nil {
		return nil, err
	}
	sid, err := randomHex(8)
	if err != nil {
		return nil, err
	}
	res.UUID, res.PublicKey, res.ShortID = uuid, publicKey, sid
	res.Steps = append(res.Steps, "keys_generated")

	// 3) 渲染真实配置并原子安装。
	cfg, err := RenderSingBoxConfig(SingBoxInbound{
		Tag:        "vless-in",
		ListenPort: req.ListenPort,
		UUID:       uuid,
		Flow:       "xtls-rprx-vision",
		SNI:        req.SNI,
		PrivateKey: privateKey,
		ShortID:    sid,
	})
	if err != nil {
		return nil, fmt.Errorf("render config failed: %w", err)
	}
	if err := runInstall(ctx, inst, cfg, arch); err != nil {
		return nil, fmt.Errorf("install failed: %w", err)
	}
	res.Steps = append(res.Steps, "config_applied")

	// 4) 健康校验：必须 installed + running。
	health, err = NativeSingBoxHealth(ctx, inst)
	if err != nil {
		return nil, fmt.Errorf("health check failed: %w", err)
	}
	res.Health = *health
	if !health.Installed || !health.Running {
		return nil, fmt.Errorf("sing-box not running after deploy (installed=%v running=%v ports=%v)", health.Installed, health.Running, health.ListeningPorts)
	}
	res.Steps = append(res.Steps, "health_ok")

	// 5) NAT 端口映射：私网端口固定，公网端口由服务商分配。
	if req.ListenPort > 0 && inst.ProviderID != "" {
		if p, ok := providers.Get(inst.ProviderID); ok {
			if pm, isMapper := p.(providers.PortMapper); isMapper {
				pubPort, mapID, err := ensurePortMapping(ctx, pm, inst.ID, req.ListenPort, req.Remark)
				if err != nil {
					return nil, fmt.Errorf("ensure port mapping failed: %w", err)
				}
				res.PublicPort, res.MappingID = pubPort, mapID
				res.Steps = append(res.Steps, fmt.Sprintf("mapping:%d->%d", pubPort, req.ListenPort))
			}
		}
	}
	if res.PublicPort <= 0 {
		res.PublicPort = req.ListenPort
	}

	res.VLESSURL = buildVLESSURL(res, req.SNI, req.Remark)
	res.Steps = append(res.Steps, "done")
	return res, nil
}

// ensurePortMapping 复用已存在的 (私网端口) 映射，否则新建。
func ensurePortMapping(ctx context.Context, pm providers.PortMapper, instanceID string, privatePort int, remark string) (int, int, error) {
	if mappings, err := pm.ListPortMappings(ctx, instanceID); err == nil {
		for _, m := range mappings {
			if m.PrivatePort == privatePort && m.PublicPort > 0 {
				return m.PublicPort, m.ID, nil
			}
		}
	}
	m, err := pm.AddPortMapping(ctx, instanceID, privatePort, "tcp", remark)
	if err != nil {
		return 0, 0, err
	}
	if m == nil || m.PublicPort <= 0 {
		return 0, 0, fmt.Errorf("provider returned empty public port")
	}
	return m.PublicPort, m.ID, nil
}

func runInstall(ctx context.Context, inst *providers.Instance, cfg []byte, arch string) error {
	script, err := BuildInstallScript(cfg, arch, SingBoxVersion)
	if err != nil {
		return err
	}
	out, err := execOut(ctx, inst, script, 58*time.Second)
	if err != nil {
		return err
	}
	if !strings.Contains(out, "DONE") {
		return fmt.Errorf("install script incomplete, tail: %s", tail(out, 300))
	}
	return nil
}

// execOut 执行命令并在非零退出码时返回带输出的错误。
func execOut(ctx context.Context, inst *providers.Instance, cmd string, timeout time.Duration) (string, error) {
	r, err := ExecuteCommand(ctx, inst, cmd, timeout)
	if err != nil {
		return "", err
	}
	if r.ExitCode != 0 {
		return r.Stdout, fmt.Errorf("exit %d: %s", r.ExitCode, tail(r.Stdout+r.Stderr, 300))
	}
	return r.Stdout, nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func buildVLESSURL(res *NativeSingBoxDeployResult, sni, remark string) string {
	q := url.Values{}
	q.Set("encryption", "none")
	q.Set("security", "reality")
	q.Set("sni", sni)
	q.Set("fp", "chrome")
	q.Set("pbk", res.PublicKey)
	q.Set("sid", res.ShortID)
	q.Set("type", "tcp")
	q.Set("flow", "xtls-rprx-vision")
	return fmt.Sprintf("vless://%s@%s:%d?%s#%s",
		res.UUID, res.ServerIP, res.PublicPort, q.Encode(), url.PathEscape(remark))
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
