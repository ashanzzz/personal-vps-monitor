package acck

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerTools(s *server.MCPServer, p *ACCKProvider) error {
	// 1. acck_list_instances
	s.AddTool(mcp.NewTool("acck_list_instances",
		mcp.WithDescription("获取 ACCKCloud (acck.io) 账户下全部 VPS 服务器列表"),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		insts, err := p.ListInstances(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(insts, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	})

	// 2. acck_checkin
	s.AddTool(mcp.NewTool("acck_checkin",
		mcp.WithDescription("执行 ACCKCloud 每日自动打卡签到获取积分"),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		res, err := p.DoCheckin(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("ACCK 签到结果: %s (奖励: %s)", res.Message, res.Reward)), nil
	})

	// 3. acck_market_listings
	s.AddTool(mcp.NewTool("acck_market_listings",
		mcp.WithDescription("获取 ACCKCloud 二手转让集市 (Exchange) 最新挂牌机器列表"),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		items, err := p.ListMarketItems(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(items, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	})

	return nil
}
