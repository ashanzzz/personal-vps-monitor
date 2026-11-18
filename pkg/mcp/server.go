package mcp

import (
	"fmt"
	"log"
	"net/http"

	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"github.com/mark3labs/mcp-go/server"
)

type Server struct {
	mcpServer *server.MCPServer
}

func New() (*Server, error) {
	s := server.NewMCPServer(
		"personal-vps-monitor",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	// 自动收集并注册所有 Provider 的 MCP 工具
	if err := providers.RegisterAllMCPTools(s); err != nil {
		return nil, fmt.Errorf("failed to register provider mcp tools: %w", err)
	}

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
