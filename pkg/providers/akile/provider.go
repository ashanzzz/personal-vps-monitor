package akile

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

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
	if p.client == nil || p.client.token == "" {
		return []providers.Instance{}, nil
	}

	res, err := p.client.ListInstances(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]providers.Instance, 0, len(res))
	for _, v := range res {
		status := "running"
		if v.Status != 1 {
			status = "stopped"
		}
		region := v.AreaName
		if region == "" {
			region = "Akile"
		}
		name := v.ServerReName
		if name == "" {
			name = v.ServerName
		}
		if name == "" {
			name = v.Name
		}

		ipv4 := v.IP
		var ipv6 string
		// 动态调取官方 GetIpAddress 补全 IPv4 与独立分配的 IPv6
		if addrv4, addrv6, ipErr := p.client.GetServerIpAddress(ctx, v.ID); ipErr == nil {
			if addrv4 != "" {
				ipv4 = addrv4
			}
			ipv6 = addrv6
		}

		expireDate := v.ExpireDate
		if expireDate == "" && v.DueTime > 0 {
			expireDate = time.Unix(v.DueTime, 0).Format("2006-01-02 15:04:05")
		}

		renewPrice := v.RenewPrice
		if renewPrice == 0 && v.Price > 0 {
			renewPrice = v.Price / 100.0
		}

		sshUser := v.InitUsername
		if sshUser == "" {
			sshUser = "root"
		}

		cpuStr := ""
		if v.CPU > 0 {
			cpuStr = fmt.Sprintf("%d核", v.CPU)
		}
		memStr := ""
		if v.Memory > 0 {
			memStr = fmt.Sprintf("%d MB", v.Memory)
		}
		flowStr := ""
		if v.Flow > 0 {
			flowStr = fmt.Sprintf("%d GB", v.Flow)
		}

		usedGB := float64(v.UsedFlow) / (1024.0 * 1024.0 * 1024.0)
		usedStr := fmt.Sprintf("%.2f GB", usedGB)
		trafficPercent := 0.0
		if v.Flow > 0 {
			trafficPercent = (usedGB / float64(v.Flow)) * 100.0
		}

		out = append(out, providers.Instance{
			ProviderID:     p.ID(),
			ID:             strconv.Itoa(v.ID),
			Name:           name,
			IP:             ipv4,
			IPv4:           ipv4,
			IPv6:           ipv6,
			SSHUser:        sshUser,
			SSHPassword:    v.InitPassword,
			SSHPort:        22,
			Status:         status,
			Region:         region,
			CPU:            cpuStr,
			Memory:         memStr,
			TrafficUsed:    usedStr,
			TrafficTotal:   flowStr,
			TrafficPercent: trafficPercent,
			RenewPrice:     renewPrice,
			AutoRenew:      v.AutoRenew,
			ExpireDate:     expireDate,
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
	if p.client == nil || p.client.token == "" {
		return []providers.MarketItem{}, nil
	}

	items, err := p.client.GetSecondhandList(ctx, 1, 30)
	if err != nil {
		return nil, err
	}

	out := make([]providers.MarketItem, 0, len(items))
	for _, item := range items {
		priceYuan := item.GetPriceYuan()
		renewYuan := item.GetRenewalPriceYuan()
		var discount float64
		if renewYuan > 0 {
			discount = math.Round((priceYuan/renewYuan)*1000) / 10.0
		}
		name := item.Name
		if name == "" {
			name = item.PlanName
		}
		bw := ""
		if item.Bandwidth > 0 {
			bw = fmt.Sprintf("%d Mbps", item.Bandwidth)
		}
		cfg := fmt.Sprintf("%dC / %dM / %dG", item.CPU, item.Memory, item.Disk)
		flowTotalStr := ""
		if item.Flow > 0 {
			cfg = fmt.Sprintf("%s · %dG", cfg, item.Flow)
			flowTotalStr = fmt.Sprintf("%dG", item.Flow)
		}
		dueStr := ""
		if item.DueTime > 0 {
			dueStr = time.Unix(item.DueTime, 0).Format("2006-01-02 15:04")
		}

		cpuStr := ""
		if item.CPU > 0 {
			cpuStr = fmt.Sprintf("%dC", item.CPU)
		}
		memStr := ""
		if item.Memory > 0 {
			memStr = fmt.Sprintf("%dM", item.Memory)
		}
		diskStr := ""
		if item.Disk > 0 {
			diskStr = fmt.Sprintf("%dG", item.Disk)
		}

		out = append(out, providers.MarketItem{
			ProviderID:      p.ID(),
			ID:              item.GetIDStr(),
			Name:            name,
			NodeName:        item.NodeName,
			Price:           priceYuan,
			RenewalPrice:    renewYuan,
			DiscountPercent: discount,
			Location:        item.AreaName,
			Bandwidth:       bw,
			ConfigDesc:      cfg,
			CPU:             cpuStr,
			Memory:          memStr,
			Disk:            diskStr,
			TrafficTotal:    flowTotalStr,
			Detail:          item.Detail,
			IPCheckDetail:   item.IPCheckDetail,
			DueTime:         dueStr,
			ResetPrice:      item.GetResetPriceYuan(),
			FlowUsed:        item.GetFlowUsedStr(),
			Cycle:           item.ServerCycle,
			CreatedAt:       item.CreateTime,
			CanBuy:          true,
			MarketType:      "secondhand",
		})
	}
	return out, nil
}

// BuyMarketItem 购买二手集市商品 (PushShop)
func (p *AkileProvider) BuyMarketItem(ctx context.Context, itemID string) error {
	if p.client == nil {
		return fmt.Errorf("akile client not initialized")
	}
	id, err := strconv.Atoi(itemID)
	if err != nil {
		return fmt.Errorf("invalid akile item id: %s", itemID)
	}
	return p.client.BuyPushProduct(ctx, id)
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

// ListStoreProducts 动态获取 Akile 官方商城真实全量产品规格 (跨 12 大区域全部真实动态拉取)
func (p *AkileProvider) ListStoreProducts(ctx context.Context) ([]providers.StoreProduct, error) {
	if p.client == nil {
		return []providers.StoreProduct{}, nil
	}

	areas, err := p.client.GetVpsStoreV3(ctx)
	if err != nil {
		return nil, err
	}

	var products []providers.StoreProduct
	for _, area := range areas {
		for _, node := range area.Nodes {
			for _, plan := range node.Plans {
				priceStr := "售罄/按需"
				if len(plan.PriceDatas) > 0 {
					priceYuan := plan.PriceDatas[0].Price / 100.0
					priceStr = fmt.Sprintf("¥%.2f/月", priceYuan)
				}
				bw := ""
				if plan.Bandwidth > 0 {
					bw = fmt.Sprintf("%d Mbps", plan.Bandwidth)
				}
				cfg := fmt.Sprintf("%dC / %dM / %dG SSD", plan.CPU, plan.Memory, plan.Disk)
				cpuStr := ""
				if plan.CPU > 0 {
					cpuStr = fmt.Sprintf("%dC", plan.CPU)
				}
				memStr := ""
				if plan.Memory > 0 {
					memStr = fmt.Sprintf("%dM", plan.Memory)
				}
				diskStr := ""
				if plan.Disk > 0 {
					diskStr = fmt.Sprintf("%dG", plan.Disk)
				}
				flowStr := ""
				if plan.Flow > 0 {
					flowStr = fmt.Sprintf("%dG", plan.Flow)
				}
				products = append(products, providers.StoreProduct{
					ID:           fmt.Sprintf("akile-%d", plan.ID),
					ProviderID:   p.ID(),
					Name:         plan.PlanName,
					Location:     area.AreaName,
					Bandwidth:    bw,
					ConfigDesc:   cfg,
					CPU:          cpuStr,
					Memory:       memStr,
					Disk:         diskStr,
					TrafficTotal: flowStr,
					Price:        priceStr,
					InStock:      true,
				})
			}
		}
	}
	return products, nil
}

func (p *AkileProvider) Login(ctx context.Context, email, password, gaCode string) (token string, need2FA bool, msg string, err error) {
	if p.client == nil {
		p.client = NewClient("", config.Global.ProxyURL)
	}
	return p.client.Login(ctx, email, password, gaCode)
}

func (p *AkileProvider) SetToken(token string) {
	if p.client == nil {
		p.client = NewClient(token, config.Global.ProxyURL)
	} else {
		p.client.SetToken(token)
	}
}
