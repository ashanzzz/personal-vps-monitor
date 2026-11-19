package main

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/audit"
	"github.com/spf13/cobra"
)

var (
	auditDays     int
	auditLimit    int
	auditSource   string
	auditProvider string
	auditStatus   string
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "查看 AI 与后台自动化的行为审计日志 (过去 30 天可溯源)",
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := audit.Query(audit.QueryOptions{
			Days:     auditDays,
			Limit:    auditLimit,
			Source:   auditSource,
			Provider: auditProvider,
			Status:   auditStatus,
		})
		if err != nil {
			return err
		}

		if len(entries) == 0 {
			fmt.Println("未找到符合条件的审计记录。")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TIME\tSOURCE\tACTION\tPROVIDER\tSTATUS\tDURATION\tMESSAGE")
		for _, e := range entries {
			t, _ := time.Parse(time.RFC3339, e.Timestamp)
			timeStr := t.Format("2006-01-02 15:04:05")
			durStr := fmt.Sprintf("%dms", e.DurationMs)
			msg := e.Message
			if len(msg) > 60 {
				msg = msg[:57] + "..."
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", timeStr, e.Source, e.Action, e.Provider, e.Status, durStr, msg)
		}
		_ = w.Flush()
		fmt.Printf("\n显示条数: %d (查询范围: 最近 %d 天)\n", len(entries), auditDays)
		return nil
	},
}

var auditPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "清理过期日志 (系统运行日志 >7天, 行为审计日志 >30天)",
	RunE: func(cmd *cobra.Command, args []string) error {
		deleted, err := audit.PruneLogs()
		if err != nil {
			return err
		}
		fmt.Printf("成功清理 %d 个已过期日志文件。\n", deleted)
		return nil
	},
}

func init() {
	auditCmd.Flags().IntVarP(&auditDays, "days", "d", 7, "查询过去天数 (1-30)")
	auditCmd.Flags().IntVarP(&auditLimit, "limit", "n", 30, "返回最大记录数")
	auditCmd.Flags().StringVarP(&auditSource, "source", "s", "", "按调用源过滤 (ai_mcp / daemon / cli / rest_api)")
	auditCmd.Flags().StringVarP(&auditProvider, "provider", "p", "", "按厂商过滤 (lazycat / akile / acck / system)")
	auditCmd.Flags().StringVar(&auditStatus, "status", "", "按状态过滤 (SUCCESS / FAILED)")

	auditCmd.AddCommand(auditPruneCmd)
	rootCmd.AddCommand(auditCmd)
}
