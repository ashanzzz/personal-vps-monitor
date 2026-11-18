package template

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerTools(s *server.MCPServer, p *TemplateProvider) error {
	s.AddTool(mcp.NewTool("template_list_instances",
		mcp.WithDescription("示例模板: 查询该厂商名下所有 VPS 实例列表"),
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
