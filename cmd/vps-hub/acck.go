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

var acckCmd = &cobra.Command{
	Use:   "acck",
	Short: "ACCKCloud (acck.io) 相关运维指令",
}

var acckCheckinCmd = &cobra.Command{
	Use:   "checkin",
	Short: "执行 ACCK 每日自动签到",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, ok := providers.Get("acck")
		if !ok {
			return fmt.Errorf("acck provider not registered")
		}
		ac, ok := p.(providers.AutoCheckin)
		if !ok {
			return fmt.Errorf("acck does not implement AutoCheckin")
		}

		start := time.Now()
		res, err := ac.DoCheckin(context.Background())
		dur := time.Since(start).Milliseconds()
		if err != nil {
			audit.Record("cli", "acck.checkin", "acck", nil, "FAILED", err.Error(), dur)
			return err
		}
		audit.Record("cli", "acck.checkin", "acck", map[string]interface{}{"account": res.Account, "reward": res.Reward}, "SUCCESS", res.Message, dur)
		fmt.Printf("ACCK 签到结果: %s (奖励: %s)\n所属网站: %s | 所属账号: %s\n", res.Message, res.Reward, res.Website, res.Account)
		return nil
	},
}

var acckInstancesCmd = &cobra.Command{
	Use:   "instances",
	Short: "查看 ACCK 实例列表",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, ok := providers.Get("acck")
		if !ok {
			return fmt.Errorf("acck provider not registered")
		}
		im, ok := p.(providers.InstanceManager)
		if !ok {
			return fmt.Errorf("acck does not implement InstanceManager")
		}

		insts, err := im.ListInstances(context.Background())
		if err != nil {
			return err
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tIP\tSTATUS\tREGION\tEXPIRE_DATE")
		for _, i := range insts {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", i.ID, i.Name, i.IP, i.Status, i.Region, i.ExpireDate)
		}
		return w.Flush()
	},
}

var acckMarketCmd = &cobra.Command{
	Use:   "market",
	Short: "查看 ACCK 二手集市 (Exchange) 挂牌列表",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, ok := providers.Get("acck")
		if !ok {
			return fmt.Errorf("acck provider not registered")
		}
		ml, ok := p.(providers.MarketLister)
		if !ok {
			return fmt.Errorf("acck does not implement MarketLister")
		}

		items, err := ml.ListMarketItems(context.Background())
		if err != nil {
			return err
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tPRICE\tLOCATION\tBANDWIDTH\tCONFIG")
		for _, it := range items {
			fmt.Fprintf(w, "%s\t%s\t%.2f\t%s\t%s\t%s\n", it.ID, it.Name, it.Price, it.Location, it.Bandwidth, it.ConfigDesc)
		}
		return w.Flush()
	},
}

func init() {
	acckCmd.AddCommand(acckCheckinCmd)
	acckCmd.AddCommand(acckInstancesCmd)
	acckCmd.AddCommand(acckMarketCmd)
	rootCmd.AddCommand(acckCmd)
}
