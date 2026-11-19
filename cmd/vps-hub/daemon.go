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
		handler := api.NewRouter()

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

		ports := []string{config.Global.HTTPPort, "80", "3338"}
		seen := make(map[string]bool)
		var servers []*http.Server

		for _, p := range ports {
			if p == "" || seen[p] {
				continue
			}
			seen[p] = true
			a := ":" + p
			srv := &http.Server{
				Addr:    a,
				Handler: handler,
			}
			servers = append(servers, srv)

			go func(addrStr string, s *http.Server) {
				log.Printf("[Daemon] Starting VPS-Hub HTTP Server on %s", addrStr)
				if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					log.Printf("[Daemon] Listener on %s stopped (may be in use or non-root): %v", addrStr, err)
				}
			}(a, srv)
		}

		// 优雅退出监听
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		<-stop

		log.Println("[Daemon] Shutting down gracefully...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		for _, s := range servers {
			_ = s.Shutdown(shutdownCtx)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(daemonCmd)
}
