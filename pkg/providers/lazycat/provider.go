package lazycat

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/config"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"github.com/mark3labs/mcp-go/server"
)

type LazyCatProvider struct {
	client     *Client
	defaultInst string
}

func init() {
	providers.Register(&LazyCatProvider{})
}

func (p *LazyCatProvider) ID() string {
	return "lazycat"
}

func (p *LazyCatProvider) Name() string {
	return "懒猫云 (LazyCat Cloud)"
}

func (p *LazyCatProvider) Init(ctx context.Context, cfg *config.Config) error {
	p.client = NewClient(cfg.LazyCatToken)
	p.defaultInst = cfg.LazyCatInstance
	return nil
}

// ListInstances 实例查询（动态发现名下所有懒猫云实例）
func (p *LazyCatProvider) ListInstances(ctx context.Context) ([]providers.Instance, error) {
	if p.client == nil {
		return nil, fmt.Errorf("lazycat client not initialized")
	}

	rawList, err := p.client.GetInstances(ctx)
	if err == nil && len(rawList) > 0 {
		out := make([]providers.Instance, 0, len(rawList))
		for _, item := range rawList {
			ip := item.HostPublicIP
			if ip == "" {
				ip = item.NatPublicIP
			}
			category := "固定出口"
			if isResidential(item.PackageName, item.PlanName, item.Name) {
				category = "全球家宽"
			}
			out = append(out, providers.Instance{
				ProviderID: p.ID(),
				ID:         strconv.Itoa(item.ID),
				Name:       item.Name,
				IP:         ip,
				Status:     item.Status,
				Region:     "Tokyo",
				Category:   category,
				PortLimit:  item.PortLimit,
				ExpireDate: item.ExpiresAt,
			})
		}
		return out, nil
	}

	if p.defaultInst == "" {
		return nil, fmt.Errorf("lazycat token missing or no instances found")
	}

	detail, err := p.client.GetInstanceDetail(ctx, p.defaultInst)
	if err != nil {
		return nil, err
	}

	ip := detail.HostPublicIP
	if ip == "" {
		ip = detail.NatPublicIP
	}

	category := "固定出口"
	if isResidential(detail.PackageName, detail.PlanName, detail.Name) {
		category = "全球家宽"
	}

	return []providers.Instance{
		{
			ProviderID: p.ID(),
			ID:         strconv.Itoa(detail.ID),
			Name:       detail.Name,
			IP:         ip,
			Status:     detail.Status,
			Region:     "Tokyo",
			Category:   category,
			PortLimit:  detail.PortLimit,
			PortCount:  len(detail.PortMappings),
			ExpireDate: detail.ExpiresAt,
		},
	}, nil
}

func isResidential(pkg, plan, name string) bool {
	combined := strings.ToLower(pkg + " " + plan + " " + name)
	return strings.Contains(combined, "residential") || strings.Contains(combined, "家宽") || strings.Contains(combined, "global")
}

// ListPortMappings 端口映射查询
func (p *LazyCatProvider) ListPortMappings(ctx context.Context, instanceID string) ([]providers.PortMapping, error) {
	if instanceID == "" {
		instanceID = p.defaultInst
	}
	detail, err := p.client.GetInstanceDetail(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	res := make([]providers.PortMapping, 0, len(detail.PortMappings))
	for _, m := range detail.PortMappings {
		res = append(res, providers.PortMapping{
			ID:          m.ID,
			Protocol:    m.Protocol,
			PrivatePort: m.PrivatePort,
			PublicPort:  m.PublicPort,
			Remark:      m.Remark,
		})
	}
	return res, nil
}

// AddPortMapping 增加端口映射
func (p *LazyCatProvider) AddPortMapping(ctx context.Context, instanceID string, privatePort int, protocol, remark string) (*providers.PortMapping, error) {
	if instanceID == "" {
		instanceID = p.defaultInst
	}
	pub, err := p.client.AddPortMapping(ctx, instanceID, privatePort, protocol, remark)
	if err != nil {
		return nil, err
	}

	return &providers.PortMapping{
		Protocol:    protocol,
		PrivatePort: privatePort,
		PublicPort:  pub,
		Remark:      remark,
	}, nil
}

// DeletePortMapping 删除端口映射
func (p *LazyCatProvider) DeletePortMapping(ctx context.Context, instanceID string, mappingID int) error {
	if instanceID == "" {
		instanceID = p.defaultInst
	}
	return p.client.DeletePortMapping(ctx, instanceID, mappingID)
}

// SyncEgress 触发小鸡 12 家宽出口轮询凭据自愈与 sing-box 热重载
func (p *LazyCatProvider) SyncEgress(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "python3", "/opt/data/scripts/_ssh_lazycat.py", "python3 /opt/lazycat-multi-egress/sync_egress.py")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sync egress failed: %v, out: %s", err, string(out))
	}
	return nil
}

func (p *LazyCatProvider) RegisterMCPTools(s *server.MCPServer) error {
	return registerTools(s, p)
}
