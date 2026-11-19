package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/audit"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type Server struct {
	mcpServer *server.MCPServer
}

func New() (*Server, error) {
	s := server.NewMCPServer(
		"personal-vps-monitor",
		"2.0.0",
		server.WithToolCapabilities(true),
	)

	// 自动收集并注册所有 Provider 的 MCP 工具
	if err := providers.RegisterAllMCPTools(s); err != nil {
		return nil, fmt.Errorf("failed to register provider mcp tools: %w", err)
	}

	// 注册全局审计日志查询工具 (AI 行为回溯与审计)
	s.AddTool(mcp.NewTool("vps_hub_get_audit_logs",
		mcp.WithDescription("查询过去 30 天内 AI 与系统自动化在 VPS Hub 中执行的操作历史记录（按时间倒序）"),
		mcp.WithNumber("days", mcp.Description("查询过去几天 (默认 7, 最大 30)")),
		mcp.WithNumber("limit", mcp.Description("返回最大条数 (默认 30)")),
		mcp.WithString("source", mcp.Description("调用来源过滤: ai_mcp, daemon, cli, rest_api")),
		mcp.WithString("provider", mcp.Description("厂商过滤: lazycat, akile, acck, system")),
		mcp.WithString("status", mcp.Description("状态过滤: SUCCESS, FAILED")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()
		days := 7
		if v, ok := args["days"].(float64); ok && v > 0 {
			days = int(v)
		}
		limit := 30
		if v, ok := args["limit"].(float64); ok && v > 0 {
			limit = int(v)
		}
		source := ""
		if v, ok := args["source"].(string); ok {
			source = v
		}
		provider := ""
		if v, ok := args["provider"].(string); ok {
			provider = v
		}
		status := ""
		if v, ok := args["status"].(string); ok {
			status = v
		}

		entries, err := audit.Query(audit.QueryOptions{
			Days:     days,
			Limit:    limit,
			Source:   source,
			Provider: provider,
			Status:   status,
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(entries, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	})

	return &Server{mcpServer: s}, nil
}

// RunStdio 以标准输入输出运行 (供 AI 编程客户端直接子进程调用)
func (s *Server) RunStdio() error {
	log.Println("[MCP] Starting Model Context Protocol server via stdio...")
	return server.ServeStdio(s.mcpServer)
}

// RunSSE 以 Server-Sent Events (HTTP) 运行
func (s *Server) RunSSE(addr string) error {
	sseServer := server.NewSSEServer(s.mcpServer)
	mux := http.NewServeMux()
	mux.Handle("/sse", sseServer)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	log.Printf("[MCP] Starting MCP SSE server listening on %s/sse ...", addr)
	return http.ListenAndServe(addr, mux)
}
