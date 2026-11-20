package lazycat

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

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
	p.client = NewClient(cfg.LazyCatToken, cfg.ProxyURL)
	p.defaultInst = cfg.LazyCatInstance
	return nil
}

// ListInstances 实例查询（动态发现名下所有懒猫云实例）
func (p *LazyCatProvider) ListInstances(ctx context.Context) ([]providers.Instance, error) {
	if p.client == nil || p.client.token == "" {
		return []providers.Instance{}, nil
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

			trafficUsed := ""
			trafficTotal := ""
			trafficPercent := 0.0
			trafficResetDate := ""

			if traffic, trErr := p.client.GetInstanceTraffic(ctx, item.ID); trErr == nil && traffic != nil {
				trafficUsed = traffic.MonthlyUsedFormatted
				trafficTotal = traffic.MonthlyLimitFormatted
				trafficPercent = traffic.Percentage
				trafficResetDate = traffic.PeriodEnd
			}

			trafficResetPrice := 0.0
			userBalance := 0.0
			if quote, qErr := p.client.GetTrafficResetQuote(ctx, item.ID); qErr == nil && quote != nil {
				trafficResetPrice = quote.Price
				userBalance = quote.Balance
			}

			rootPassword := ""
			if pwd, pErr := p.client.GetInstancePassword(ctx, item.ID); pErr == nil && pwd != "" {
				rootPassword = pwd
			}

			renewPrice := 0.0
			autoRenew := false
			if billing, bErr := p.client.GetInstanceBilling(ctx, item.ID); bErr == nil && billing != nil {
				renewPrice = billing.Billing.Price
				autoRenew = billing.Billing.AutoRenew
			}

			out = append(out, providers.Instance{
				ProviderID:        p.ID(),
				ID:                strconv.Itoa(item.ID),
				Name:              item.Name,
				IP:                ip,
				IPv4:              ip,
				SSHUser:           "root",
				SSHPassword:       rootPassword,
				RootPassword:      rootPassword,
				SSHPort:           22,
				Status:            item.Status,
				Region:            "Tokyo",
				Category:          category,
				TrafficUsed:       trafficUsed,
				TrafficTotal:      trafficTotal,
				TrafficPercent:    trafficPercent,
				TrafficResetDate:  trafficResetDate,
				TrafficResetPrice: trafficResetPrice,
				UserBalance:       userBalance,
				RenewPrice:        renewPrice,
				Cycle:             1,
				CycleName:         "月付",
				MonthlyPrice:      renewPrice,
				AutoRenew:         autoRenew,
				PortLimit:         item.PortLimit,
				ExpireDate:        item.ExpiresAt,
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

	trafficUsed := ""
	trafficTotal := ""
	trafficPercent := 0.0
	trafficResetDate := ""
	if traffic, trErr := p.client.GetInstanceTraffic(ctx, detail.ID); trErr == nil && traffic != nil {
		trafficUsed = traffic.MonthlyUsedFormatted
		trafficTotal = traffic.MonthlyLimitFormatted
		trafficPercent = traffic.Percentage
		trafficResetDate = traffic.PeriodEnd
	}

	trafficResetPrice := 0.0
	userBalance := 0.0
	if quote, qErr := p.client.GetTrafficResetQuote(ctx, detail.ID); qErr == nil && quote != nil {
		trafficResetPrice = quote.Price
		userBalance = quote.Balance
	}

	rootPassword := ""
	if pwd, pErr := p.client.GetInstancePassword(ctx, detail.ID); pErr == nil && pwd != "" {
		rootPassword = pwd
	}

	renewPrice := 0.0
	autoRenew := false
	if billing, bErr := p.client.GetInstanceBilling(ctx, detail.ID); bErr == nil && billing != nil {
		renewPrice = billing.Billing.Price
		autoRenew = billing.Billing.AutoRenew
	}

	return []providers.Instance{
		{
			ProviderID:        p.ID(),
			ID:                strconv.Itoa(detail.ID),
			Name:              detail.Name,
			IP:                ip,
			IPv4:              ip,
			SSHUser:           "root",
			SSHPassword:       rootPassword,
			RootPassword:      rootPassword,
			SSHPort:           22,
			Status:            detail.Status,
			Region:            "Tokyo",
			Category:          category,
			TrafficUsed:       trafficUsed,
			TrafficTotal:      trafficTotal,
			TrafficPercent:    trafficPercent,
			TrafficResetDate:  trafficResetDate,
			TrafficResetPrice: trafficResetPrice,
			UserBalance:       userBalance,
			RenewPrice:        renewPrice,
			Cycle:             1,
			CycleName:         "月付",
			MonthlyPrice:      renewPrice,
			AutoRenew:         autoRenew,
			PortLimit:         detail.PortLimit,
			PortCount:         len(detail.PortMappings),
			ExpireDate:        detail.ExpiresAt,
		},
	}, nil
}

func isResidential(pkg, plan, name string) bool {
	combined := strings.ToLower(pkg + " " + plan + " " + name)
	return strings.Contains(combined, "residential") || strings.Contains(combined, "家宽") || strings.Contains(combined, "global")
}

// ListPortMappings 端口映射查询
func (p *LazyCatProvider) ListPortMappings(ctx context.Context, instanceID string) ([]providers.PortMapping, error) {
	if p.client == nil || p.client.token == "" {
		return []providers.PortMapping{}, nil
	}
	if instanceID == "" {
		instanceID = p.defaultInst
	}
	detail, err := p.client.GetInstanceDetail(ctx, instanceID)
	if err != nil {
		return []providers.PortMapping{}, nil
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

// DoCheckin 执行懒猫云每日签到打卡并获取积分
func (p *LazyCatProvider) DoCheckin(ctx context.Context) (*providers.CheckinResult, error) {
	if p.client == nil {
		return nil, fmt.Errorf("lazycat client not initialized")
	}

	accountStr := "ashanzzz"
	if u, err := p.client.GetUserInfo(ctx); err == nil && u.User.Username != "" {
		if u.User.Email != "" {
			accountStr = fmt.Sprintf("%s (%s)", u.User.Username, u.User.Email)
		} else {
			accountStr = u.User.Username
		}
	}

	res, err := p.client.Checkin(ctx)
	if err != nil {
		return nil, err
	}

	reward := ""
	if res.Points > 0 {
		reward = fmt.Sprintf("+%d 积分", res.Points)
	}

	msg := res.Message
	if res.Outcome == "duplicate" || strings.Contains(msg, "已签到") {
		msg = "当天已签到 (请勿重复打卡)"
	}

	return &providers.CheckinResult{
		Success: res.Success,
		Message: msg,
		Reward:  reward,
		Website: "https://dash.lazycat.wiki",
		Account: accountStr,
	}, nil
}

// ListMarketItems 实现 providers.MarketLister 接口，动态全量分页获取懒猫云二手市场在售挂单
func (p *LazyCatProvider) ListMarketItems(ctx context.Context) ([]providers.MarketItem, error) {
	if p.client == nil || p.client.token == "" {
		return []providers.MarketItem{}, nil
	}

	rawList, err := p.client.GetAllMarketListings(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]providers.MarketItem, 0, len(rawList))
	for _, it := range rawList {
		price := it.SalePrice
		name := it.Title
		if name == "" {
			name = it.InstanceName
		}
		loc := it.Snapshot.RegionName
		if loc == "" {
			loc = it.Snapshot.RegionCode
		}
		if loc == "" {
			loc = "全球节点"
		}
		bw := ""
		if it.Snapshot.BandwidthMbps > 0 {
			bw = fmt.Sprintf("%d Mbps", it.Snapshot.BandwidthMbps)
		}
		cfgDesc := ""
		if it.Snapshot.Memory > 0 {
			cfgDesc = fmt.Sprintf("%d MB 内存", it.Snapshot.Memory)
		}
		if it.Snapshot.CPUCores > 0 {
			if cfgDesc != "" {
				cfgDesc = fmt.Sprintf("%d核 / %s", it.Snapshot.CPUCores, cfgDesc)
			} else {
				cfgDesc = fmt.Sprintf("%d核", it.Snapshot.CPUCores)
			}
		}
		if it.Remark != "" {
			if cfgDesc != "" {
				cfgDesc = fmt.Sprintf("%s (%s)", cfgDesc, it.Remark)
			} else {
				cfgDesc = it.Remark
			}
		}

		sla := it.Snapshot.SLA
		if sla == "" {
			sla = "99.9%"
		}
		ipDetail := ""
		if it.Snapshot.IPStatus != "" {
			ipDetail = fmt.Sprintf("IP: %s", it.Snapshot.IPStatus)
		}
		portLimit := it.Snapshot.PortLimit
		if portLimit == 0 {
			portLimit = 10
		}

		cpuStr := ""
		if it.Snapshot.CPUCores > 0 {
			cpuStr = fmt.Sprintf("%dC", it.Snapshot.CPUCores)
		} else if it.Snapshot.CPU > 0 {
			cpuStr = fmt.Sprintf("%dC", it.Snapshot.CPU)
		}
		memStr := ""
		if it.Snapshot.Memory > 0 {
			memStr = fmt.Sprintf("%dM", it.Snapshot.Memory)
		}
		diskStr := ""
		if it.Snapshot.Disk > 0 {
			diskStr = fmt.Sprintf("%dG", it.Snapshot.Disk)
		}
		flowTotalStr := it.Snapshot.MonthlyTrafficLimit
		cycleMonths, cycleName, monthlyPrice := providers.FormatCycle(it.Snapshot.BillingCycle, it.Snapshot.RenewalPrice)

		items = append(items, providers.MarketItem{
			ProviderID:      p.ID(),
			ID:              strconv.Itoa(it.ID),
			Name:            name,
			Price:           price,
			RenewalPrice:    it.Snapshot.RenewalPrice,
			MonthlyPrice:    monthlyPrice,
			Cycle:           cycleMonths,
			CycleName:       cycleName,
			DiscountPercent: it.Snapshot.RenewalDiscountPercent,
			ResetPrice:      it.Snapshot.RenewalPrice,
			FlowUsed:        it.Snapshot.MonthlyTrafficUsed,
			SellerUsername:  it.SellerUsername,
			SellerUserID:    it.SellerUserID,
			Location:        loc,
			NodeName:        it.Snapshot.NodeName,
			Bandwidth:       bw,
			PortLimit:       portLimit,
			ConfigDesc:      cfgDesc,
			CPU:             cpuStr,
			Memory:          memStr,
			Disk:            diskStr,
			TrafficTotal:    flowTotalStr,
			Detail:          ipDetail,
			SLA:             sla,
			CreatedAt:       it.CreatedAt,
			CanBuy:          it.CanBuy,
			MarketType:      "secondhand",
			BargainTag:      "🏷️ 懒猫二手",
		})
	}
	return items, nil
}

// BuyMarketItem 购买二手集市商品
func (p *LazyCatProvider) BuyMarketItem(ctx context.Context, itemID string) error {
	if p.client == nil {
		return fmt.Errorf("lazycat client not initialized")
	}
	id, err := strconv.Atoi(itemID)
	if err != nil {
		return fmt.Errorf("invalid lazycat item id: %s", itemID)
	}
	return p.client.BuyMarketListing(ctx, id)
}

// ListStoreProducts 实现 providers.StoreLister 接口，动态获取懒猫云官方在售套餐 (全量 42+ 款真实规格)
func (p *LazyCatProvider) ListStoreProducts(ctx context.Context) ([]providers.StoreProduct, error) {
	if p.client == nil {
		return []providers.StoreProduct{}, nil
	}

	pkgs, err := p.client.GetPublicPackages(ctx)
	if err != nil {
		return nil, err
	}

	res := make([]providers.StoreProduct, 0, len(pkgs))
	for _, pkg := range pkgs {
		priceStr := "¥--/月"
		cfgDesc := pkg.Description
		bandwidth := pkg.LimitsEgress
		if bandwidth == "" {
			bandwidth = "共享带宽"
		}
		cpuStr := ""
		memStr := ""
		diskStr := ""
		if len(pkg.Plans) > 0 {
			minPrice := pkg.Plans[0].PriceCents
			if minPrice == 0 && pkg.Plans[0].MonthlyPrice > 0 {
				minPrice = pkg.Plans[0].MonthlyPrice
			}
			p0 := pkg.Plans[0]
			priceStr = fmt.Sprintf("¥%.2f/月起", float64(minPrice)/100.0)
			cfgDesc = fmt.Sprintf("%d核 / %dMB 内存 / %dMB 磁盘 (%s)", p0.CPUCores, p0.MemoryMB, p0.DiskMB, pkg.Description)
			if p0.CPUCores > 0 {
				cpuStr = fmt.Sprintf("%dC", p0.CPUCores)
			}
			if p0.MemoryMB > 0 {
				memStr = fmt.Sprintf("%dM", p0.MemoryMB)
			}
			if p0.DiskMB > 0 {
				if p0.DiskMB >= 1024 {
					diskStr = fmt.Sprintf("%dG", p0.DiskMB/1024)
				} else {
					diskStr = fmt.Sprintf("%dM", p0.DiskMB)
				}
			}
		}
		loc := "日本/香港/海外"
		nameUpper := strings.ToUpper(pkg.Name)
		if strings.Contains(nameUpper, "HK") || strings.Contains(pkg.Name, "香港") {
			loc = "中国香港"
		} else if strings.Contains(nameUpper, "JP") || strings.Contains(pkg.Name, "日本") || strings.Contains(nameUpper, "TOKYO") {
			loc = "日本东京"
		} else if strings.Contains(nameUpper, "SG") || strings.Contains(pkg.Name, "狮城") || strings.Contains(pkg.Name, "新加坡") {
			loc = "新加坡"
		} else if strings.Contains(nameUpper, "US") || strings.Contains(pkg.Name, "美") || strings.Contains(pkg.Name, "Frontier") {
			loc = "美国"
		} else if strings.Contains(nameUpper, "TW") || strings.Contains(pkg.Name, "台湾") {
			loc = "中国台湾"
		} else if strings.Contains(nameUpper, "PH") || strings.Contains(pkg.Name, "菲律宾") {
			loc = "菲律宾"
		}

		trafficTotal := pkg.LimitsIngress
		if trafficTotal == "" {
			trafficTotal = "不限"
		}

		res = append(res, providers.StoreProduct{
			ID:           fmt.Sprintf("lz-pkg-%d", pkg.ID),
			ProviderID:   p.ID(),
			Name:         pkg.Name,
			Location:     loc,
			Bandwidth:    bandwidth,
			ConfigDesc:   cfgDesc,
			CPU:          cpuStr,
			Memory:       memStr,
			Disk:         diskStr,
			TrafficTotal: trafficTotal,
			Price:        priceStr,
			InStock:      !pkg.SoldOut,
		})
	}
	return res, nil
}

// GetPublicPackages 获取懒猫云官方在售套餐真实列表 (无需鉴权，全量 42+ 套餐)
func (p *LazyCatProvider) GetPublicPackages(ctx context.Context) ([]PublicPackage, error) {
	if p.client == nil {
		return []PublicPackage{}, nil
	}
	return p.client.GetPublicPackages(ctx)
}

func (p *LazyCatProvider) RegisterMCPTools(s *server.MCPServer) error {
	return registerTools(s, p)
}

func (p *LazyCatProvider) Client() *Client {
	return p.client
}

func (p *LazyCatProvider) ProxyTerminalWebSocket(w http.ResponseWriter, r *http.Request, instanceID string) error {
	if p.client == nil {
		return fmt.Errorf("lazycat client not initialized")
	}
	return p.client.ProxyTerminalWebSocket(w, r, instanceID)
}

func (p *LazyCatProvider) ExecTerminalCommand(ctx context.Context, instanceID, cmd string, timeout time.Duration) (string, error) {
	if p.client == nil {
		return "", fmt.Errorf("lazycat client not initialized")
	}
	return p.client.ExecTerminalCommand(ctx, instanceID, cmd, timeout)
}

func (p *LazyCatProvider) FetchFleetTelemetry(ctx context.Context, forceRefresh bool) ([]FleetNode, error) {
	if p.client == nil {
		return nil, fmt.Errorf("lazycat client not initialized")
	}
	return p.client.FetchFleetTelemetry(ctx, forceRefresh)
}

func (p *LazyCatProvider) GetContainerEgressNodes(ctx context.Context, instanceID string, forceRefresh bool) ([]EgressNode, error) {
	if p.client == nil {
		return nil, fmt.Errorf("lazycat client not initialized")
	}
	return p.client.GetContainerEgressNodes(ctx, instanceID, forceRefresh)
}

func (p *LazyCatProvider) GetInstanceDetail(ctx context.Context, instanceID string) (*InstanceDetailResponse, error) {
	if p.client == nil {
		return nil, fmt.Errorf("lazycat client not initialized")
	}
	return p.client.GetInstanceDetail(ctx, instanceID)
}

func (p *LazyCatProvider) Login(ctx context.Context, username, password, totpCode string) (token string, need2FA bool, msg string, err error) {
	if p.client == nil {
		p.client = NewClient("", config.Global.ProxyURL)
	}
	return p.client.Login(ctx, username, password, totpCode)
}

func (p *LazyCatProvider) SetToken(token string) {
	if p.client == nil {
		p.client = NewClient(token, config.Global.ProxyURL)
	} else {
		p.client.token = token
	}
}
