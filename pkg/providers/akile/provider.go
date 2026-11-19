package akile

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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
	p.client = NewClient(cfg.AkileToken, cfg.ProxyURL)
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

	accountStr := "ashan"
	if u, err := p.client.GetUserInfo(ctx); err == nil && u.Data.Username != "" {
		if u.Data.Email != "" {
			accountStr = fmt.Sprintf("%s (%s)", u.Data.Username, u.Data.Email)
		} else {
			accountStr = u.Data.Username
		}
	}

	res, err := p.client.Checkin(ctx)
	if err != nil {
		return nil, err
	}

	success := (res.StatusCode == 0 || res.StatusCode == 200 || strings.Contains(res.StatusMsg, "已签到"))
	reward := ""
	if res.Data.AkCoinEarned > 0 {
		reward = fmt.Sprintf("+%.1f AK币", res.Data.AkCoinEarned)
	}

	msg := res.StatusMsg
	if strings.Contains(msg, "今日已签到") {
		msg = "今日已签到 (请勿重复打卡)"
	}

	return &providers.CheckinResult{
		Success: success,
		Message: msg,
		Reward:  reward,
		Website: "https://akile.ai",
		Account: accountStr,
	}, nil
}

// ListMarketItems 二手集市 (PushShop)
func (p *AkileProvider) ListMarketItems(ctx context.Context) ([]providers.MarketItem, error) {
	if p.client == nil {
		return nil, fmt.Errorf("akile client not initialized")
	}

	items, err := p.client.GetSecondhandList(ctx, 1, 30)
	if err != nil {
		return nil, err
	}

	out := make([]providers.MarketItem, 0, len(items))
	for _, item := range items {
		out = append(out, providers.MarketItem{
			ProviderID: p.ID(),
			ID:         strconv.Itoa(item.ID),
			Name:       item.Name,
			Price:      item.Price,
			Location:   item.AreaName,
			Bandwidth:  item.Bandwidth,
			ConfigDesc: item.Configuration,
			CreatedAt:  item.CreateTime,
		})
	}
	return out, nil
}

func (p *AkileProvider) RefreshToken(ctx context.Context) (string, error) {
	if p.client == nil {
		return "", fmt.Errorf("akile client not initialized")
	}
	return p.client.RefreshToken(ctx)
}

func (p *AkileProvider) RegisterMCPTools(s *server.MCPServer) error {
	return registerTools(s, p)
}
