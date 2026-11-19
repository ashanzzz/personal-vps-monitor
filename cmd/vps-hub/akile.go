package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/audit"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"github.com/spf13/cobra"
)

var akileCmd = &cobra.Command{
	Use:   "akile",
	Short: "Akile Cloud 相关运维指令",
}

var akileCheckinCmd = &cobra.Command{
	Use:   "checkin",
	Short: "执行每日签到并领取积分",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, ok := providers.Get("akile")
		if !ok {
			return fmt.Errorf("akile provider not registered")
		}
		ac, ok := p.(providers.AutoCheckin)
		if !ok {
			return fmt.Errorf("akile does not implement AutoCheckin")
		}

		start := time.Now()
		res, err := ac.DoCheckin(context.Background())
		dur := time.Since(start).Milliseconds()
		if err != nil {
			audit.Record("cli", "akile.checkin", "akile", nil, "FAILED", err.Error(), dur)
			return err
		}
		audit.Record("cli", "akile.checkin", "akile", map[string]interface{}{"account": res.Account, "reward": res.Reward}, "SUCCESS", res.Message, dur)
		fmt.Printf("Akile 签到结果: %s (奖励: %s)\n所属网站: %s | 所属账号: %s\n", res.Message, res.Reward, res.Website, res.Account)
		return nil
	},
}

var akileInstancesCmd = &cobra.Command{
	Use:   "instances",
	Short: "查看 Akile 实例列表",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, ok := providers.Get("akile")
		if !ok {
			return fmt.Errorf("akile provider not registered")
		}
		im, ok := p.(providers.InstanceManager)
		if !ok {
			return fmt.Errorf("akile does not implement InstanceManager")
		}

		insts, err := im.ListInstances(context.Background())
		if err != nil {
			return err
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tIP\tSTATUS\tEXPIRE_DATE")
		for _, i := range insts {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", i.ID, i.Name, i.IP, i.Status, i.ExpireDate)
		}
		return w.Flush()
	},
}

func init() {
	akileCmd.AddCommand(akileCheckinCmd)
	akileCmd.AddCommand(akileInstancesCmd)
	rootCmd.AddCommand(akileCmd)
}
