package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/api"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/config"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/notify"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "以守护进程常驻运行 (包含后台定时巡检、自愈及 REST API)",
	RunE: func(cmd *cobra.Command, args []string) error {
		addr := ":" + config.Global.HTTPPort
		handler := api.NewRouter()

		server := &http.Server{
			Addr:    addr,
			Handler: handler,
		}

		// 启动后台定时任务 (例如每 4 小时触发一次各厂商自动签到)
		ticker := time.NewTicker(4 * time.Hour)
		defer ticker.Stop()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func() {
			for {
				select {
				case <-ticker.C:
					log.Println("[Daemon] Running scheduled checkin...")
					all := providers.GetAll()
					for id, p := range all {
						if ac, ok := p.(providers.AutoCheckin); ok {
							res, err := ac.DoCheckin(ctx)
							if err != nil {
								log.Printf("[Daemon] Checkin for %s failed: %v", id, err)
							} else {
								msg := fmt.Sprintf("✅ *[%s 自动签到]*\n结果: %s", p.Name(), res.Message)
								_ = notify.SendTelegram(msg)
							}
						}
					}
				case <-ctx.Done():
					return
				}
			}
		}()

		// 异步启动 HTTP 服务
		go func() {
			log.Printf("[Daemon] Starting VPS-Hub Daemon on %s", addr)
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("[Daemon] HTTP server error: %v", err)
			}
		}()

		// 优雅退出监听
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		<-stop

		log.Println("[Daemon] Shutting down gracefully...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		return server.Shutdown(shutdownCtx)
	},
}

func init() {
	rootCmd.AddCommand(daemonCmd)
}
