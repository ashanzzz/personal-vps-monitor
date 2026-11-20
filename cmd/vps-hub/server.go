package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/api"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/config"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/vault"
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

		// 启动时在后台静默尝试从 Vaultwarden 自动解密同步一次最新凭据
		go func() {
			time.Sleep(2 * time.Second)
			if items, err := vault.PullAndSyncFromVaultwarden(context.Background(), "", "", ""); err == nil && len(items) > 0 {
				log.Printf("[Startup] 成功从 Vaultwarden 自动解密提取并更新了 %d 个厂商凭据！", len(items))
			}
		}()

		return http.ListenAndServe(addr, handler)
	},
}

func init() {
	serverCmd.Flags().StringVarP(&serverAddr, "addr", "a", "", "HTTP 服务监听地址 (例如 :3338)")
	rootCmd.AddCommand(serverCmd)
}
