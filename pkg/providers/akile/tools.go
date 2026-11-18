package akile

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerTools(s *server.MCPServer, p *AkileProvider) error {
	// 1. akile_checkin
	s.AddTool(mcp.NewTool("akile_checkin",
		mcp.WithDescription("执行 Akile Cloud 每日自动签到并领取积分"),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		res, err := p.DoCheckin(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Akile 签到结果: %s (奖励: %s)", res.Message, res.Reward)), nil
	})

	// 2. akile_list_instances
	s.AddTool(mcp.NewTool("akile_list_instances",
		mcp.WithDescription("获取 Akile Cloud 账户下的全部 VPS 虚拟机列表"),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		insts, err := p.ListInstances(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(insts, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	})

	// 3. akile_market_listings
	s.AddTool(mcp.NewTool("akile_market_listings",
		mcp.WithDescription("获取 Akile Cloud 二手集市 (PushShop) 最新挂牌机器列表"),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		items, err := p.ListMarketItems(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(items, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	})

	// 4. akile_refresh_token
	s.AddTool(mcp.NewTool("akile_refresh_token",
		mcp.WithDescription("自动续期保活 Akile JWT 认证 Token (延长24小时有效期)"),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tok, err := p.RefreshToken(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Akile Token 自动续期成功 (新 Token 长度: %d 字符)", len(tok))), nil
	})

	return nil
}
