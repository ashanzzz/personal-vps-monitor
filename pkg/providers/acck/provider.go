package acck

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
	p.client = NewClient(cfg.ACCKToken, cfg.ProxyURL)
	return nil
}

func (p *ACCKProvider) SetToken(token string) {
	if p.client == nil {
		p.client = NewClient(token, config.Global.ProxyURL)
	} else {
		p.client.SetToken(token)
	}
}

func (p *ACCKProvider) GetToken() string {
	if p.client == nil {
		return ""
	}
	return p.client.GetToken()
}

// SetClientForTest 供单元测试注入自定义 mock 客户端
func (p *ACCKProvider) SetClientForTest(cli *Client) {
	p.client = cli
}

func (p *ACCKProvider) VerifyToken(ctx context.Context) error {
	if p.client == nil {
		return fmt.Errorf("acck client not initialized")
	}
	return p.client.VerifyToken(ctx)
}

// Login 使用账号密码（及可选 6 位动态码）登录 ACCK
func (p *ACCKProvider) Login(ctx context.Context, email, password, gaCode string) (token string, need2FA bool, msg string, err error) {
	if p.client == nil {
		p.client = NewClient("", config.Global.ProxyURL)
	}
	return p.client.Login(ctx, email, password, gaCode)
}

// LoginWithTOTPProvider 使用账号密码与实时 TOTP 回调函数登录 ACCK
func (p *ACCKProvider) LoginWithTOTPProvider(ctx context.Context, email, password string, totpFn func() (string, error), isBackgroundAuto bool) (token string, need2FA bool, msg string, err error) {
	if p.client == nil {
		p.client = NewClient("", config.Global.ProxyURL)
	}
	return p.client.LoginWithOptions(ctx, email, password, totpFn, isBackgroundAuto)
}

// ListInstances 实例查询
func (p *ACCKProvider) ListInstances(ctx context.Context) ([]providers.Instance, error) {
	if p.client == nil || p.client.GetToken() == "" {
		if creds := config.LoadSavedCredentials(); creds != nil && creds.ACCK != nil && creds.ACCK.Password != "" {
			return nil, fmt.Errorf("acck token missing (401): awaiting auto relogin")
		}
		return []providers.Instance{}, nil
	}

	servers, err := p.client.GetMyServers(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]providers.Instance, 0, len(servers))
	for _, s := range servers {
		statusStr := "stopped"
		if s.Status == 1 {
			statusStr = "running"
		}
		expireStr := ""
		if s.DueTime > 0 {
			expireStr = time.Unix(s.DueTime, 0).Format("2006-01-02 15:04:05")
		}
		name := s.ServerReName
		if name == "" {
			name = s.ServerName
		}
		sshUser := s.InitUsername
		if sshUser == "" {
			sshUser = "root"
		}

		trafficUsed := ""
		trafficTotal := fmt.Sprintf("%d GB", s.Flow)
		trafficPercent := 0.0
		trafficResetDate := ""

		if s.ServerID != "" {
			if vmInfo, err := p.client.GetVmInfo(ctx, s.ServerID); err == nil && vmInfo != nil {
				usedGB := float64(vmInfo.BandwidthUsage) / (1024.0 * 1024.0 * 1024.0)
				limitGB := float64(vmInfo.BandwidthLimit) / (1024.0 * 1024.0 * 1024.0)
				if limitGB <= 0 && s.Flow > 0 {
					limitGB = float64(s.Flow)
				}
				trafficUsed = fmt.Sprintf("%.2f GB", usedGB)
				if limitGB > 0 {
					trafficTotal = fmt.Sprintf("%.0f GB", limitGB)
					trafficPercent = (usedGB / limitGB) * 100.0
				}
				if vmInfo.NextResetTime > 0 {
					trafficResetDate = time.Unix(vmInfo.NextResetTime, 0).Format("2006-01-02 15:04:05")
				}
			}
		}

		resetPrice := s.ResetFlowPrice
		if resetPrice <= 0 {
			resetPrice = 25.0 // ACCK 标准机型默认流量重置包为 ¥25.00
		}
		var userBalance float64
		renewPrice := s.Price
		if bv, err := p.client.GetBillingView(ctx, s.ID); err == nil && bv != nil {
			userBalance = float64(bv.BalanceCents) / 100.0
			if bv.RenewalPriceCents > 0 {
				renewPrice = float64(bv.RenewalPriceCents)
			}
		}

		cycleMonths, cycleName, monthlyPrice := providers.FormatCycle(s.Cycle, renewPrice)

		out = append(out, providers.Instance{
			ProviderID:        p.ID(),
			ID:                strconv.Itoa(s.ID),
			Name:              name,
			IP:                s.IPv4,
			IPv4:              s.IPv4,
			IPv6:              s.IPv6,
			SSHUser:           sshUser,
			SSHPassword:       s.InitPassword,
			SSHPort:           22,
			Status:            statusStr,
			Region:            s.AreaName,
			CPU:               fmt.Sprintf("%d核", s.CPU),
			Memory:            fmt.Sprintf("%d MB", s.Memory),
			TrafficUsed:       trafficUsed,
			TrafficTotal:      trafficTotal,
			TrafficPercent:    trafficPercent,
			TrafficResetDate:  trafficResetDate,
			TrafficResetPrice: resetPrice,
			AutoReset:         s.AutoReset == 1,
			UserBalance:       userBalance,
			RootPassword:      s.InitPassword,
			RenewPrice:        renewPrice,
			Cycle:             cycleMonths,
			CycleName:         cycleName,
			MonthlyPrice:      monthlyPrice,
			AutoRenew:         s.AutoRenew == 1,
			ExpireDate:        expireStr,
		})
	}
	return out, nil
}

// DoCheckin 每日签到
func (p *ACCKProvider) DoCheckin(ctx context.Context) (*providers.CheckinResult, error) {
	if p.client == nil {
		return nil, fmt.Errorf("acck client not initialized")
	}

	accountStr := "ashan"
	if u, err := p.client.GetUserInfo(ctx); err == nil && u.Username != "" {
		if u.Email != "" {
			accountStr = fmt.Sprintf("%s (%s)", u.Username, u.Email)
		} else {
			accountStr = u.Username
		}
	}

	res, err := p.client.Checkin(ctx)
	if err != nil {
		return nil, err
	}

	success := (res.Code == 200 || res.Code == 0)
	msg := res.Msg
	if strings.Contains(msg, "今日已签到") {
		msg = "今日已签到 (请勿重复打卡)"
	}

	return &providers.CheckinResult{
		Success: success,
		Message: msg,
		Reward:  res.Data,
		Website: "https://acck.io",
		Account: accountStr,
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
		detail := l.IntroText
		if detail == "" {
			detail = l.NodeComment
		}
		var discount float64
		if l.RenewalPrice > 0 {
			discount = math.Round((l.Price/l.RenewalPrice)*1000) / 10.0
		}
		cycleMonths, cycleName, monthlyPrice := providers.FormatCycle(l.Cycle, l.RenewalPrice)
		cfgDesc := fmt.Sprintf("%dC / %dM", l.CPU, l.MemoryMB)
		if l.DiskGB > 0 {
			cfgDesc = fmt.Sprintf("%s / %dG", cfgDesc, l.DiskGB)
		}
		if l.FlowGB > 0 {
			cfgDesc = fmt.Sprintf("%s · %dG", cfgDesc, l.FlowGB)
		}
		cpuStr := ""
		if l.CPU > 0 {
			cpuStr = fmt.Sprintf("%dC", l.CPU)
		}
		memStr := ""
		if l.MemoryMB > 0 {
			memStr = fmt.Sprintf("%dM", l.MemoryMB)
		}
		diskStr := ""
		if l.DiskGB > 0 {
			diskStr = fmt.Sprintf("%dG", l.DiskGB)
		}
		flowStr := ""
		if l.FlowGB > 0 {
			flowStr = fmt.Sprintf("%dG", l.FlowGB)
		}

		out = append(out, providers.MarketItem{
			ProviderID:      p.ID(),
			ID:              strconv.Itoa(l.ID),
			Name:            name,
			Price:           l.Price,
			RenewalPrice:    l.RenewalPrice,
			MonthlyPrice:    monthlyPrice,
			Cycle:           cycleMonths,
			CycleName:       cycleName,
			DiscountPercent: discount,
			ResetPrice:      l.ResetPrice,
			PlatformFee:     l.PlatformFee,
			FlowUsed:        l.FlowUsed,
			SellerUserID:    l.SellerUserID,
			Location:        loc,
			Bandwidth:       bw,
			ConfigDesc:      cfgDesc,
			CPU:             cpuStr,
			Memory:          memStr,
			Disk:            diskStr,
			TrafficTotal:    flowStr,
			Detail:          detail,
			DueTime:         l.DueTime,
			CreatedAt:       l.CreatedAt,
			CanBuy:          true,
			MarketType:      "secondhand",
		})
	}
	return out, nil
}

// BuyMarketItem 购买二手集市商品 (Exchange)
func (p *ACCKProvider) BuyMarketItem(ctx context.Context, itemID string) error {
	if p.client == nil {
		return fmt.Errorf("acck client not initialized")
	}
	id, err := strconv.Atoi(itemID)
	if err != nil {
		return fmt.Errorf("invalid acck item id: %s", itemID)
	}
	return p.client.BuyExchangeListing(ctx, id)
}

func (p *ACCKProvider) RegisterMCPTools(s *server.MCPServer) error {
	return registerTools(s, p)
}
