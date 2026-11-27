package lazycat

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/audit"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerTools(s *server.MCPServer, p *LazyCatProvider) error {
	// 1. lazycat_list_ports
	s.AddTool(mcp.NewTool("lazycat_list_ports",
		mcp.WithDescription("获取懒猫云当前小鸡所有已映射的公网 NAT 端口列表与配额"),
		mcp.WithString("instance_id", mcp.Description("可选实例ID，留空使用默认实例")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()
		instID := ""
		if v, ok := args["instance_id"].(string); ok {
			instID = v
		}
		ports, err := p.ListPortMappings(ctx, instID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(ports, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	})

	// 2. lazycat_add_port
	s.AddTool(mcp.NewTool("lazycat_add_port",
		mcp.WithDescription("为懒猫云实例添加一条新的公网 NAT 端口映射"),
		mcp.WithNumber("private_port", mcp.Required(), mcp.Description("容器内部私有监听端口，例如 10001")),
		mcp.WithString("protocol", mcp.Description("协议类型: tcp 或 udp，默认 tcp")),
		mcp.WithString("remark", mcp.Description("端口备注说明")),
		mcp.WithString("instance_id", mcp.Description("可选实例ID")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()
		privPort := 0
		if v, ok := args["private_port"].(float64); ok {
			privPort = int(v)
		}
		proto := "tcp"
		if v, ok := args["protocol"].(string); ok && v != "" {
			proto = v
		}
		remark := ""
		if v, ok := args["remark"].(string); ok {
			remark = v
		}
		instID := ""
		if v, ok := args["instance_id"].(string); ok {
			instID = v
		}

		start := time.Now()
		res, err := p.AddPortMapping(ctx, instID, privPort, proto, remark)
		dur := time.Since(start).Milliseconds()
		if err != nil {
			audit.Record("ai_mcp", "lazycat.port_add", "lazycat", map[string]interface{}{"private_port": privPort, "protocol": proto, "remark": remark}, "FAILED", err.Error(), dur)
			return mcp.NewToolResultError(err.Error()), nil
		}
		msg := fmt.Sprintf("端口映射成功: 私有 %d -> 公网 %d (%s)", res.PrivatePort, res.PublicPort, res.Remark)
		audit.Record("ai_mcp", "lazycat.port_add", "lazycat", map[string]interface{}{"private_port": privPort, "public_port": res.PublicPort, "protocol": proto, "remark": remark}, "SUCCESS", msg, dur)
		return mcp.NewToolResultText(msg), nil
	})

	// 3. lazycat_delete_port
	s.AddTool(mcp.NewTool("lazycat_delete_port",
		mcp.WithDescription("删除懒猫云实例的一条公网 NAT 端口映射"),
		mcp.WithNumber("mapping_id", mcp.Required(), mcp.Description("要删除的映射记录 ID")),
		mcp.WithString("instance_id", mcp.Description("可选实例ID")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()
		mapID := 0
		if v, ok := args["mapping_id"].(float64); ok {
			mapID = int(v)
		}
		instID := ""
		if v, ok := args["instance_id"].(string); ok {
			instID = v
		}

		start := time.Now()
		err := p.DeletePortMapping(ctx, instID, mapID)
		dur := time.Since(start).Milliseconds()
		if err != nil {
			audit.Record("ai_mcp", "lazycat.port_delete", "lazycat", map[string]interface{}{"mapping_id": mapID}, "FAILED", err.Error(), dur)
			return mcp.NewToolResultError(err.Error()), nil
		}
		msg := fmt.Sprintf("已成功删除端口映射记录 ID %d", mapID)
		audit.Record("ai_mcp", "lazycat.port_delete", "lazycat", map[string]interface{}{"mapping_id": mapID}, "SUCCESS", msg, dur)
		return mcp.NewToolResultText(msg), nil
	})

	// 4. lazycat_sync_egress
	s.AddTool(mcp.NewTool("lazycat_sync_egress",
		mcp.WithDescription("触发懒猫云 12 国家宽原生出口凭据自动轮询与 sing-box 聚合配置热重载"),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		err := p.SyncEgress(ctx)
		dur := time.Since(start).Milliseconds()
		if err != nil {
			audit.Record("ai_mcp", "lazycat.egress_sync", "lazycat", nil, "FAILED", err.Error(), dur)
			return mcp.NewToolResultError(err.Error()), nil
		}
		msg := "成功触发 12 家宽出口凭据轮询与热重载"
		audit.Record("ai_mcp", "lazycat.egress_sync", "lazycat", nil, "SUCCESS", msg, dur)
		return mcp.NewToolResultText(msg), nil
	})

	// 5. lazycat_checkin
	s.AddTool(mcp.NewTool("lazycat_checkin",
		mcp.WithDescription("执行懒猫云每日签到打卡并领取积分 (支持重复打卡自动识别)"),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		res, err := p.DoCheckin(ctx)
		dur := time.Since(start).Milliseconds()
		if err != nil {
			audit.Record("ai_mcp", "lazycat.checkin", "lazycat", nil, "FAILED", err.Error(), dur)
			return mcp.NewToolResultError(err.Error()), nil
		}
		msg := fmt.Sprintf("[%s 签到结果] 账号: %s | 结果: %s (奖励: %s) | 网站: %s", p.Name(), res.Account, res.Message, res.Reward, res.Website)
		audit.Record("ai_mcp", "lazycat.checkin", "lazycat", map[string]interface{}{"account": res.Account, "reward": res.Reward}, "SUCCESS", msg, dur)
		return mcp.NewToolResultText(msg), nil
	})

	// 6. lazycat_exec
	s.AddTool(mcp.NewTool("lazycat_exec",
		mcp.WithDescription("通过懒猫云官方 WebSocket PTY 通道在指定 NAT 容器内免端口映射执行远程 Shell 命令（支持家宽与香港线路机等所有懒猫实例）"),
		mcp.WithString("instance_id", mcp.Description("懒猫云实例 ID（例如 10674 或 11433），留空使用默认实例")),
		mcp.WithString("command", mcp.Required(), mcp.Description("要在容器内执行的 Shell 命令")),
		mcp.WithNumber("timeout_sec", mcp.Description("执行超时秒数（默认 15，最大 120）")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()
		instID := ""
		if v, ok := args["instance_id"].(string); ok {
			instID = v
		}
		if instID == "" {
			instID = p.defaultInst
		}
		cmd := ""
		if v, ok := args["command"].(string); ok {
			cmd = v
		}
		timeout := 15 * time.Second
		if v, ok := args["timeout_sec"].(float64); ok && v > 0 {
			if v > 120 {
				v = 120
			}
			timeout = time.Duration(v) * time.Second
		}

		start := time.Now()
		res, err := p.ExecTerminalCommandWithResult(ctx, instID, cmd, timeout)
		dur := time.Since(start).Milliseconds()
		if err != nil {
			audit.Record("ai_mcp", "lazycat.terminal_exec", "lazycat", map[string]interface{}{"instance_id": instID, "command": cmd}, "FAILED", err.Error(), dur)
			return mcp.NewToolResultError(err.Error()), nil
		}
		audit.Record("ai_mcp", "lazycat.terminal_exec", "lazycat", map[string]interface{}{"instance_id": instID, "command": cmd, "exit_code": res.ExitCode}, "SUCCESS", fmt.Sprintf("exit=%d", res.ExitCode), dur)
		data, _ := json.MarshalIndent(res, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	})

	return nil
}
