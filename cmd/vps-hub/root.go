package main

import (
	"context"
	"fmt"
	"os"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/config"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "vps-hub",
	Short: "Personal VPS Hub - 极简模块化 VPS 与出口统一控制中枢",
	Long:  `基于 Go 构建的高性能、极低内存占用、无前端 Headless VPS 纳管系统，支持 CLI、REST API 及 AI 原生 MCP 调用。`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Load()
		return providers.InitAll(context.Background(), cfg)
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
