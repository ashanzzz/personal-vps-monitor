package main

import (
	"fmt"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/config"
	"github.com/ashanzzz/personal-vps-monitor/pkg/mcp"
	"github.com/spf13/cobra"
)

var (
	mcpSSEMode bool
	mcpAddr    string
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "启动 Model Context Protocol (MCP) 服务供 AI 智能体调用",
	Long:  `以 stdio 或 SSE 模式运行 MCP 服务器，自动暴露所有已注册 VPS Provider 的结构化工具供 Claude / Antigravity 等 AI 调度。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		srv, err := mcp.New()
		if err != nil {
			return fmt.Errorf("failed to init mcp server: %w", err)
		}

		if mcpSSEMode {
			addr := mcpAddr
			if addr == "" {
				addr = ":" + config.Global.MCPPort
			}
			return srv.RunSSE(addr)
		}

		return srv.RunStdio()
	},
}

func init() {
	mcpCmd.Flags().BoolVar(&mcpSSEMode, "sse", false, "以 SSE HTTP 模式运行 (默认通过 stdio 交互)")
	mcpCmd.Flags().StringVar(&mcpAddr, "addr", "", "SSE 监听地址 (默认 :3337)")
	rootCmd.AddCommand(mcpCmd)
}
