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

	return nil
}
