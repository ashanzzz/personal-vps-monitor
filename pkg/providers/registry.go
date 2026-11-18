package providers

import (
	"context"
	"fmt"
	"sync"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/config"
	"github.com/mark3labs/mcp-go/server"
)

var (
	mu        sync.RWMutex
	providers = make(map[string]Provider)
)

// Register 注册 Provider
func Register(p Provider) {
	mu.Lock()
	defer mu.Unlock()
	if p == nil {
		panic("cannot register nil provider")
	}
	id := p.ID()
	if _, exists := providers[id]; exists {
		panic(fmt.Sprintf("provider already registered: %s", id))
	}
	providers[id] = p
}

// Get 获取指定 Provider
func Get(id string) (Provider, bool) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := providers[id]
	return p, ok
}

// GetAll 获取全部已注册 Provider
func GetAll() map[string]Provider {
	mu.RLock()
	defer mu.RUnlock()
	copied := make(map[string]Provider, len(providers))
	for k, v := range providers {
		copied[k] = v
	}
	return copied
}

// InitAll 初始化所有 Provider
func InitAll(ctx context.Context, cfg *config.Config) error {
	mu.RLock()
	defer mu.RUnlock()
	for _, p := range providers {
		if err := p.Init(ctx, cfg); err != nil {
			return fmt.Errorf("failed to init provider %s: %w", p.ID(), err)
		}
	}
	return nil
}

// RegisterAllMCPTools 自动收集并将所有 Provider 的工具注册到 MCP Server
func RegisterAllMCPTools(s *server.MCPServer) error {
	mu.RLock()
	defer mu.RUnlock()
	for _, p := range providers {
		if err := p.RegisterMCPTools(s); err != nil {
			return fmt.Errorf("failed to register MCP tools for %s: %w", p.ID(), err)
		}
	}
	return nil
}
