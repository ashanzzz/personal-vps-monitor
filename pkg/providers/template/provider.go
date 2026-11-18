package template

import (
	"context"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/config"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"github.com/mark3labs/mcp-go/server"
)

type TemplateProvider struct {
	client *Client
}

func init() {
	// 模板驱动默认不直接注册，复制本目录并重命名包名后启用:
	// providers.Register(&MyNewProvider{})
}

func (p *TemplateProvider) ID() string {
	return "template_vendor"
}

func (p *TemplateProvider) Name() string {
	return "示例模板厂商"
}

func (p *TemplateProvider) Init(ctx context.Context, cfg *config.Config) error {
	p.client = NewClient("your-api-key")
	return nil
}

// 示例实现 InstanceManager 接口
func (p *TemplateProvider) ListInstances(ctx context.Context) ([]providers.Instance, error) {
	_, err := p.client.FetchInstances(ctx)
	if err != nil {
		return nil, err
	}
	return []providers.Instance{
		{
			ProviderID: p.ID(),
			ID:         "demo-1",
			Name:       "Demo VPS",
			IP:         "1.2.3.4",
			Status:     "running",
			Region:     "US",
		},
	}, nil
}

func (p *TemplateProvider) RegisterMCPTools(s *server.MCPServer) error {
	return registerTools(s, p)
}
