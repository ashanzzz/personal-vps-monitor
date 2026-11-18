package acck

import (
	"context"
	"fmt"
	"strconv"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/config"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"github.com/mark3labs/mcp-go/server"
)

type ACCKProvider struct {
	client *Client
}

func init() {
	providers.Register(&ACCKProvider{})
}

func (p *ACCKProvider) ID() string {
	return "acck"
}

func (p *ACCKProvider) Name() string {
	return "ACCKCloud (acck.io)"
}

func (p *ACCKProvider) Init(ctx context.Context, cfg *config.Config) error {
	p.client = NewClient(cfg.ACCKToken)
	return nil
}

// ListInstances 实例查询
func (p *ACCKProvider) ListInstances(ctx context.Context) ([]providers.Instance, error) {
	if p.client == nil {
		return nil, fmt.Errorf("acck client not initialized")
	}

	servers, err := p.client.GetMyServers(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]providers.Instance, 0, len(servers))
	for _, s := range servers {
		out = append(out, providers.Instance{
			ProviderID: p.ID(),
			ID:         strconv.Itoa(s.ID),
			Name:       s.Name,
			IP:         s.IP,
			Status:     s.Status,
			Region:     s.Region,
			ExpireDate: s.ExpireDate,
		})
	}
	return out, nil
}

// DoCheckin 每日签到
func (p *ACCKProvider) DoCheckin(ctx context.Context) (*providers.CheckinResult, error) {
	if p.client == nil {
		return nil, fmt.Errorf("acck client not initialized")
	}

	res, err := p.client.Checkin(ctx)
	if err != nil {
		return nil, err
	}

	success := (res.Code == 200 || res.Code == 0)
	return &providers.CheckinResult{
		Success: success,
		Message: res.Msg,
		Reward:  res.Data,
	}, nil
}

// ListMarketItems 二手集市商品浏览
func (p *ACCKProvider) ListMarketItems(ctx context.Context) ([]providers.MarketItem, error) {
	if p.client == nil {
		return nil, fmt.Errorf("acck client not initialized")
	}

	listings, err := p.client.GetExchangeListings(ctx, 50)
	if err != nil {
		return nil, err
	}

	out := make([]providers.MarketItem, 0, len(listings))
	for _, l := range listings {
		name := l.PlanName
		if name == "" {
			name = l.IntroText
		}
		loc := l.AreaName
		bw := ""
		if l.BandwidthMbps > 0 {
			bw = fmt.Sprintf("%d Mbps", l.BandwidthMbps)
		}
		out = append(out, providers.MarketItem{
			ProviderID: p.ID(),
			ID:         strconv.Itoa(l.ID),
			Name:       name,
			Price:      l.Price,
			Location:   loc,
			Bandwidth:  bw,
			ConfigDesc: fmt.Sprintf("%dC / %dM", l.CPU, l.MemoryMB),
			CreatedAt:  l.CreatedAt,
		})
	}
	return out, nil
}

func (p *ACCKProvider) RegisterMCPTools(s *server.MCPServer) error {
	return registerTools(s, p)
}
