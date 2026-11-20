package providers

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/config"
	"github.com/mark3labs/mcp-go/server"
)

var (
	ErrNotSupported = errors.New("capability not supported by provider")
)

// Instance 统一实例数据结构
type Instance struct {
	ProviderID   string  `json:"provider_id"`
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	IP           string  `json:"ip"`
	IPv4         string  `json:"ipv4,omitempty"`
	IPv6         string  `json:"ipv6,omitempty"`
	SSHUser      string  `json:"ssh_user,omitempty"`
	SSHPassword  string  `json:"ssh_password,omitempty"`
	SSHPort      int     `json:"ssh_port,omitempty"`
	Status       string  `json:"status"`
	Region       string  `json:"region"`
	Category     string  `json:"category,omitempty"` // 例如: "全球家宽" / "固定出口"
	CPU          string  `json:"cpu,omitempty"`      // CPU 配置 (如 "1核")
	Memory       string  `json:"memory,omitempty"`   // 内存配置 (如 "1024 MB")
	TrafficUsed      string  `json:"traffic_used,omitempty"`       // 已用流量 (如 "363.20 GB")
	TrafficTotal     string  `json:"traffic_total,omitempty"`      // 总流量配额 (如 "1000 GB")
	TrafficPercent   float64 `json:"traffic_percent,omitempty"`    // 使用百分比 (0.0 ~ 100.0)
	TrafficResetDate  string  `json:"traffic_reset_date,omitempty"`  // 流量重置日
	TrafficResetPrice float64 `json:"traffic_reset_price,omitempty"` // 流量重置费用 (¥)
	AutoReset         bool    `json:"auto_reset,omitempty"`          // 是否开启自动重置流量
	UserBalance       float64 `json:"user_balance,omitempty"`        // 用户平台可用余额 (¥)
	RootPassword      string  `json:"root_password,omitempty"`       // root 初始/当前密码
	RenewPrice        float64 `json:"renew_price,omitempty"`         // 续费金额 (¥)
	AutoRenew        bool    `json:"auto_renew,omitempty"`         // 是否开启自动续费
	PortCount        int     `json:"port_count,omitempty"`
	PortLimit        int     `json:"port_limit,omitempty"`
	ExpireDate       string  `json:"expire_date,omitempty"`
	IsStale          bool    `json:"is_stale,omitempty"`           // 是否为缓存陈旧数据 (登录失效时为 true)
	StaleReason      string  `json:"stale_reason,omitempty"`       // 陈旧原因 (如: "Token 失效 / 密码错误")
	LastSyncedAt     string  `json:"last_synced_at,omitempty"`     // 最近同步时间戳
}

// PortMapping 统一端口映射数据结构
type PortMapping struct {
	ID          int    `json:"id"`
	Protocol    string `json:"protocol"`
	PrivatePort int    `json:"private_port"`
	PublicPort  int    `json:"public_port"`
	Remark      string `json:"remark"`
}

// CheckinResult 统一每日签到结果
type CheckinResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Reward  string `json:"reward,omitempty"`
	Website string `json:"website,omitempty"` // 所属网站/平台
	Account string `json:"account,omitempty"` // 所属账号/用户标识
}

// MarketItem 统一二手与一手集市流转商品数据结构
type MarketItem struct {
	ProviderID      string  `json:"provider_id"`
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Price           float64 `json:"price"`
	RenewalPrice    float64 `json:"renewal_price,omitempty"`
	DiscountPercent float64 `json:"discount_percent,omitempty"`
	ResetPrice      float64 `json:"reset_price,omitempty"`       // 流量重置费用 (¥)
	PlatformFee     float64 `json:"platform_fee,omitempty"`      // 平台交易/过户手续费 (¥)
	FlowUsed        string  `json:"flow_used,omitempty"`         // 已使用流量配额 (如 "7.7 GB / 1500 GB")
	Cycle           int     `json:"cycle,omitempty"`             // 计费周期 (月数，例如 1:月付, 12:年付)
	CycleName       string  `json:"cycle_name,omitempty"`        // 周期人类可读标签: "月付", "年付", "季付", "半年付"
	MonthlyPrice    float64 `json:"monthly_price,omitempty"`     // 归一化等效月租 (¥/月)
	SellerUsername  string  `json:"seller_username,omitempty"`
	SellerUserID    int     `json:"seller_user_id,omitempty"`
	Location        string  `json:"location"`
	NodeName        string  `json:"node_name,omitempty"`
	Bandwidth       string  `json:"bandwidth,omitempty"`
	PortLimit       int     `json:"port_limit,omitempty"`
	ConfigDesc      string  `json:"config_desc,omitempty"`
	CPU             string  `json:"cpu,omitempty"`               // CPU 核心 (如 "1C" 或 "1核")
	Memory          string  `json:"memory,omitempty"`            // 内存大小 (如 "1024M" 或 "1024 MB")
	Disk            string  `json:"disk,omitempty"`              // 磁盘空间 (如 "10G" 或 "5 GB")
	TrafficTotal    string  `json:"traffic_total,omitempty"`     // 月流量总配额 (如 "1000G" 或 "1500 GB")
	Detail          string  `json:"detail,omitempty"`           // IP 状态 / 挂单备注 (如 "[IP正常]")
	IPCheckDetail   string  `json:"ip_check_detail,omitempty"`   // 三网延迟测试详情
	DueTime         string  `json:"due_time,omitempty"`          // 到期时间
	SLA             string  `json:"sla,omitempty"`               // 在线保证率 (如 "99.9%")
	CreatedAt       string  `json:"created_at,omitempty"`
	CanBuy          bool    `json:"can_buy"`
	MarketType      string  `json:"market_type,omitempty"`  // "secondhand" or "new_stock"
	BargainTag      string  `json:"bargain_tag,omitempty"`  // 捡漏评估标签 (如 "🔥 神机秒杀" / "🏷️ 超值捡漏")
	DiffPercent     float64 `json:"diff_percent,omitempty"` // 相比历史同款均价波动百分比
}

// Provider 核心驱动接口
type Provider interface {
	ID() string
	Name() string
	Init(ctx context.Context, cfg *config.Config) error
	RegisterMCPTools(s *server.MCPServer) error
}

// InstanceManager 实例管理能力特化接口
type InstanceManager interface {
	ListInstances(ctx context.Context) ([]Instance, error)
}

// PortMapper 端口映射能力特化接口
type PortMapper interface {
	ListPortMappings(ctx context.Context, instanceID string) ([]PortMapping, error)
	AddPortMapping(ctx context.Context, instanceID string, privatePort int, protocol, remark string) (*PortMapping, error)
	DeletePortMapping(ctx context.Context, instanceID string, mappingID int) error
}

// AutoCheckin 定时签到能力特化接口
type AutoCheckin interface {
	DoCheckin(ctx context.Context) (*CheckinResult, error)
}

// MarketLister 二手集市商品浏览特化接口
type MarketLister interface {
	ListMarketItems(ctx context.Context) ([]MarketItem, error)
}

// MarketBuyer 二手与一手集市购买特化接口
type MarketBuyer interface {
	BuyMarketItem(ctx context.Context, itemID string) error
}

// EgressSyncer 家宽出口轮询与同步能力特化接口
type EgressSyncer interface {
	SyncEgress(ctx context.Context) error
}

// StoreProduct 统一官方新机商城商品数据结构
type StoreProduct struct {
	ID           string `json:"id"`
	ProviderID   string `json:"provider_id"`
	Name         string `json:"name"`
	Location     string `json:"location"`
	Bandwidth    string `json:"bandwidth"`
	ConfigDesc   string `json:"config_desc"`
	CPU          string `json:"cpu,omitempty"`
	Memory       string `json:"memory,omitempty"`
	Disk         string `json:"disk,omitempty"`
	TrafficTotal string `json:"traffic_total,omitempty"`
	Price        string `json:"price"`
	InStock      bool   `json:"in_stock"`
}

// StoreLister 官方商城商品浏览特化接口
type StoreLister interface {
	ListStoreProducts(ctx context.Context) ([]StoreProduct, error)
}

// FormatCycle 格式化计费周期月数与标签名，并计算等效月供 (保留2位小数)
func FormatCycle(cycle int, renewalPrice float64) (cycleMonths int, cycleName string, monthlyPrice float64) {
	if cycle <= 0 {
		cycle = 1
	}
	cycleMonths = cycle
	switch cycle {
	case 1:
		cycleName = "月付"
		monthlyPrice = renewalPrice
	case 3:
		cycleName = "季付"
		if renewalPrice > 0 {
			monthlyPrice = math.Round((renewalPrice/3.0)*100) / 100
		}
	case 6:
		cycleName = "半年付"
		if renewalPrice > 0 {
			monthlyPrice = math.Round((renewalPrice/6.0)*100) / 100
		}
	case 12:
		cycleName = "年付"
		if renewalPrice > 0 {
			monthlyPrice = math.Round((renewalPrice/12.0)*100) / 100
		}
	case 24:
		cycleName = "两年付"
		if renewalPrice > 0 {
			monthlyPrice = math.Round((renewalPrice/24.0)*100) / 100
		}
	case 36:
		cycleName = "三年付"
		if renewalPrice > 0 {
			monthlyPrice = math.Round((renewalPrice/36.0)*100) / 100
		}
	default:
		if cycle%12 == 0 {
			cycleName = fmt.Sprintf("%d年付", cycle/12)
		} else {
			cycleName = fmt.Sprintf("%d个月付", cycle)
		}
		if renewalPrice > 0 {
			monthlyPrice = math.Round((renewalPrice/float64(cycle))*100) / 100
		}
	}
	return
}

