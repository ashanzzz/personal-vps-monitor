package akile

import (
	"context"
	"fmt"
	"strconv"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/config"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"github.com/mark3labs/mcp-go/server"
)

type AkileProvider struct {
	client *Client
}

func init() {
	providers.Register(&AkileProvider{})
}

func (p *AkileProvider) ID() string {
	return "akile"
}

func (p *AkileProvider) Name() string {
	return "Akile Cloud"
}

func (p *AkileProvider) Init(ctx context.Context, cfg *config.Config) error {
	p.client = NewClient(cfg.AkileToken)
	return nil
}

func (p *AkileProvider) ListInstances(ctx context.Context) ([]providers.Instance, error) {
	if p.client == nil {
		return nil, fmt.Errorf("akile client not initialized")
	}

	res, err := p.client.ListInstances(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]providers.Instance, 0, len(res.Data))
	for _, v := range res.Data {
		status := "running"
		if v.Status != 1 {
			status = "stopped"
		}
		out = append(out, providers.Instance{
			ProviderID: p.ID(),
			ID:         strconv.Itoa(v.ID),
			Name:       v.Name,
			IP:         v.IP,
			Status:     status,
			Region:     "Akile",
			ExpireDate: v.ExpireDate,
		})
	}
	return out, nil
}

func (p *AkileProvider) DoCheckin(ctx context.Context) (*providers.CheckinResult, error) {
	if p.client == nil {
		return nil, fmt.Errorf("akile client not initialized")
	}

	res, err := p.client.Checkin(ctx)
	if err != nil {
		return nil, err
	}

	success := (res.StatusCode == 0 || res.StatusCode == 200)
	reward := ""
	if res.Data.AkCoinEarned > 0 {
		reward = fmt.Sprintf("+%.1f AK币", res.Data.AkCoinEarned)
	}

	return &providers.CheckinResult{
		Success: success,
		Message: res.StatusMsg,
		Reward:  reward,
	}, nil
}

func (p *AkileProvider) RegisterMCPTools(s *server.MCPServer) error {
	return registerTools(s, p)
}
