package main

import (
	"log"
	"net/http"

	"github.com/ashanzzz/personal-vps-monitor/pkg/api"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/config"
	"github.com/spf13/cobra"
)

var serverAddr string

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "启动轻量级 HTTP REST API 服务器",
	RunE: func(cmd *cobra.Command, args []string) error {
		addr := serverAddr
		if addr == "" {
			addr = ":" + config.Global.HTTPPort
		}

		handler := api.NewRouter()
		log.Printf("[HTTP] Personal VPS Hub API listening on http://127.0.0.1%s", addr)
		return http.ListenAndServe(addr, handler)
	},
}

func init() {
	serverCmd.Flags().StringVarP(&serverAddr, "addr", "a", "", "HTTP 服务监听地址 (例如 :3338)")
	rootCmd.AddCommand(serverCmd)
}
