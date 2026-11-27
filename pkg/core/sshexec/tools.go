package sshexec

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterTools 注册普通主机 SSH 运维、代理探查、生命周期管理与链式导出 MCP 工具
func RegisterTools(s *server.MCPServer) {
	// ==========================================
	// 模块 1: vps_ssh_exec
	// ==========================================
	s.AddTool(mcp.NewTool("vps_ssh_exec",
		mcp.WithDescription("在指定的 VPS 或懒猫云 NAT 容器（自动优先走 WebSocket PTY，无须占用 22 端口）上免交互远程执行诊断与管理命令，自动记录平台审计。"),
		mcp.WithString("instance_id", mcp.Description("实例 ID (例如 13483，或与 ip 二选一)")),
		mcp.WithString("ip", mcp.Description("目标主机 IP 地址 (与 instance_id 二选一)")),
		mcp.WithString("command", mcp.Required(), mcp.Description("需要远程执行的 Shell 命令 (例如: 'ss -tulpn | grep 3x-ui')")),
		mcp.WithNumber("timeout_sec", mcp.Description("执行超时秒数 (默认 10，最大 60)")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()
		target := ""
		if v, ok := args["instance_id"].(string); ok && strings.TrimSpace(v) != "" {
			target = strings.TrimSpace(v)
		} else if v, ok := args["ip"].(string); ok && strings.TrimSpace(v) != "" {
			target = strings.TrimSpace(v)
		}
		if target == "" {
			return mcp.NewToolResultError("must provide either instance_id or ip"), nil
		}

		command := ""
		if v, ok := args["command"].(string); ok {
			command = strings.TrimSpace(v)
		}
		if command == "" {
			return mcp.NewToolResultError("command cannot be empty"), nil
		}

		timeoutSec := 10
		if v, ok := args["timeout_sec"].(float64); ok && v > 0 {
			timeoutSec = int(v)
		}

		res, _, err := RunCommandWithAudit(ctx, "ai_mcp", target, command, timeoutSec)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("SSH execute failed: %v", err)), nil
		}

		data, _ := json.MarshalIndent(res, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	})

	// ==========================================
	// 模块 2: vps_inspect_proxies
	// ==========================================
	s.AddTool(mcp.NewTool("vps_inspect_proxies",
		mcp.WithDescription("结构化探查主机实际运行的入站代理列表（自动识别 3x-ui / sing-box / xray 协议与监听状态，懒猫 NAT 容器自动将私有端口转换为映射公网端口）"),
		mcp.WithString("instance_id", mcp.Description("实例 ID (例如 13483，或与 ip 二选一)")),
		mcp.WithString("ip", mcp.Description("目标主机 IP 地址 (与 instance_id 二选一)")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()
		target := ""
		if v, ok := args["instance_id"].(string); ok && strings.TrimSpace(v) != "" {
			target = strings.TrimSpace(v)
		} else if v, ok := args["ip"].(string); ok && strings.TrimSpace(v) != "" {
			target = strings.TrimSpace(v)
		}
		if target == "" {
			return mcp.NewToolResultError("must provide either instance_id or ip"), nil
		}

		res, err := InspectProxiesWithAudit(ctx, "ai_mcp", target)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("inspect proxies failed: %v", err)), nil
		}

		data, _ := json.MarshalIndent(res, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	})

	// ==========================================
	// 模块 3: vps_change_proxy_port (自愈专用)
	// ==========================================
	s.AddTool(mcp.NewTool("vps_change_proxy_port",
		mcp.WithDescription("普通主机代理换端口自愈：将指定被封锁端口修改为新端口，自动改写数据库/配置、重启服务容器并放行防火墙。"),
		mcp.WithString("instance_id", mcp.Description("实例 ID (例如 13483，或与 ip 二选一)")),
		mcp.WithString("ip", mcp.Description("目标主机 IP 地址 (与 instance_id 二选一)")),
		mcp.WithNumber("old_port", mcp.Required(), mcp.Description("原代理监听端口号")),
		mcp.WithNumber("new_port", mcp.Required(), mcp.Description("新代理监听端口号")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()
		target := ""
		if v, ok := args["instance_id"].(string); ok && strings.TrimSpace(v) != "" {
			target = strings.TrimSpace(v)
		} else if v, ok := args["ip"].(string); ok && strings.TrimSpace(v) != "" {
			target = strings.TrimSpace(v)
		}
		if target == "" {
			return mcp.NewToolResultError("must provide either instance_id or ip"), nil
		}

		oldPort := 0
		if v, ok := args["old_port"].(float64); ok {
			oldPort = int(v)
		}
		newPort := 0
		if v, ok := args["new_port"].(float64); ok {
			newPort = int(v)
		}

		if oldPort <= 0 || newPort <= 0 {
			return mcp.NewToolResultError("old_port and new_port must be valid positive ports"), nil
		}

		res, err := ChangeProxyPortWithAudit(ctx, "ai_mcp", target, oldPort, newPort)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("change proxy port failed: %v", err)), nil
		}

		data, _ := json.MarshalIndent(res, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	})

	// ==========================================
	// 模块 3: vps_add_proxy
	// ==========================================
	s.AddTool(mcp.NewTool("vps_add_proxy",
		mcp.WithDescription("在普通主机上新增一条入站代理记录，自动生成 Reality x25519 密钥对、UUID 并生效。"),
		mcp.WithString("instance_id", mcp.Description("实例 ID (例如 13483，或与 ip 二选一)")),
		mcp.WithString("ip", mcp.Description("目标主机 IP 地址 (与 instance_id 二选一)")),
		mcp.WithNumber("port", mcp.Required(), mcp.Description("新代理入站监听端口")),
		mcp.WithString("protocol", mcp.Description("协议类型 (默认 vless)")),
		mcp.WithString("security", mcp.Description("安全传输层 (默认 reality)")),
		mcp.WithString("sni", mcp.Description("SNI 伪装域名 (默认 aws.amazon.com)")),
		mcp.WithString("uuid", mcp.Description("指定客户端 UUID (留空自动生成)")),
		mcp.WithString("remark", mcp.Description("代理节点备注说明")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()
		target := ""
		if v, ok := args["instance_id"].(string); ok && strings.TrimSpace(v) != "" {
			target = strings.TrimSpace(v)
		} else if v, ok := args["ip"].(string); ok && strings.TrimSpace(v) != "" {
			target = strings.TrimSpace(v)
		}
		if target == "" {
			return mcp.NewToolResultError("must provide either instance_id or ip"), nil
		}

		port := 0
		if v, ok := args["port"].(float64); ok {
			port = int(v)
		}
		if port <= 0 {
			return mcp.NewToolResultError("port must be positive"), nil
		}

		req := AddProxyRequest{
			Port: port,
		}
		if v, ok := args["protocol"].(string); ok && v != "" {
			req.Protocol = v
		}
		if v, ok := args["security"].(string); ok && v != "" {
			req.Security = v
		}
		if v, ok := args["sni"].(string); ok && v != "" {
			req.SNI = v
		}
		if v, ok := args["uuid"].(string); ok && v != "" {
			req.UUID = v
		}
		if v, ok := args["remark"].(string); ok && v != "" {
			req.Remark = v
		}

		res, err := AddProxyWithAudit(ctx, "ai_mcp", target, req)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("add proxy failed: %v", err)), nil
		}

		data, _ := json.MarshalIndent(res, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	})

	// ==========================================
	// 模块 3: vps_delete_proxy
	// ==========================================
	s.AddTool(mcp.NewTool("vps_delete_proxy",
		mcp.WithDescription("下线并注销普通主机上指定端口的入站代理，释放端口资源。"),
		mcp.WithString("instance_id", mcp.Description("实例 ID (例如 13483，或与 ip 二选一)")),
		mcp.WithString("ip", mcp.Description("目标主机 IP 地址 (与 instance_id 二选一)")),
		mcp.WithNumber("port", mcp.Required(), mcp.Description("要下线的端口号")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()
		target := ""
		if v, ok := args["instance_id"].(string); ok && strings.TrimSpace(v) != "" {
			target = strings.TrimSpace(v)
		} else if v, ok := args["ip"].(string); ok && strings.TrimSpace(v) != "" {
			target = strings.TrimSpace(v)
		}
		if target == "" {
			return mcp.NewToolResultError("must provide either instance_id or ip"), nil
		}

		port := 0
		if v, ok := args["port"].(float64); ok {
			port = int(v)
		}
		if port <= 0 {
			return mcp.NewToolResultError("port must be positive"), nil
		}

		err := DeleteProxyWithAudit(ctx, "ai_mcp", target, port)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("delete proxy failed: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Proxy on port %d successfully deleted and released", port)), nil
	})

	// ==========================================
	// 模块 4: vps_get_chain_proxy_nodes
	// ==========================================
	s.AddTool(mcp.NewTool("vps_get_chain_proxy_nodes",
		mcp.WithDescription("生成「优质普通 VPS 中继 ➔ 懒猫家宽落地出口」链式组合节点清单，自动注入 dialer-proxy / detour，支持 Clash / Sing-box / JSON 格式。"),
		mcp.WithString("relay_id", mcp.Description("可选前置中继 VPS 实例 ID 或 IP (默认自动选用优质美西 4837 / 东京 VPS)")),
		mcp.WithString("format", mcp.Description("输出格式: 'json' (默认), 'clash' (YAML), 'sing-box' (JSON), 或 'all'")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()
		relayID := ""
		if v, ok := args["relay_id"].(string); ok {
			relayID = strings.TrimSpace(v)
		}
		format := "all"
		if v, ok := args["format"].(string); ok && v != "" {
			format = strings.TrimSpace(v)
		}

		res, err := GetChainProxyNodesWithAudit(ctx, "ai_mcp", relayID, format)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get chain proxy nodes failed: %v", err)), nil
		}

		if format == "clash" || format == "mihomo" || format == "yaml" {
			return mcp.NewToolResultText(res.ClashConfig), nil
		}
		if format == "sing-box" || format == "singbox" {
			return mcp.NewToolResultText(res.SingBoxJSON), nil
		}

		data, _ := json.MarshalIndent(res, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	})
}
