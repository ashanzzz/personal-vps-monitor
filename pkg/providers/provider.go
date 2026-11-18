package providers

import (
	"context"
	"errors"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/config"
	"github.com/mark3labs/mcp-go/server"
)

var (
	ErrNotSupported = errors.New("capability not supported by provider")
)

// Instance 统一实例数据结构
type Instance struct {
	ProviderID string `json:"provider_id"`
	ID         string `json:"id"`
	Name       string `json:"name"`
	IP         string `json:"ip"`
	Status     string `json:"status"`
	Region     string `json:"region"`
	ExpireDate string `json:"expire_date,omitempty"`
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

// EgressSyncer 家宽出口轮询与同步能力特化接口
type EgressSyncer interface {
	SyncEgress(ctx context.Context) error
}
