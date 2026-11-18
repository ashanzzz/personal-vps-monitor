package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"github.com/spf13/cobra"
)

var marketCmd = &cobra.Command{
	Use:   "market",
	Short: "聚合查看全网二手集市转让机器列表 (Akile / ACCK)",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		all := providers.GetAll()

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "PROVIDER\tID\tNAME\tPRICE\tLOCATION\tBANDWIDTH\tCONFIG")

		total := 0
		for pid, p := range all {
			if ml, ok := p.(providers.MarketLister); ok {
				items, err := ml.ListMarketItems(ctx)
				if err == nil {
					for _, it := range items {
						total++
						fmt.Fprintf(w, "%s\t%s\t%s\t%.2f\t%s\t%s\t%s\n", pid, it.ID, it.Name, it.Price, it.Location, it.Bandwidth, it.ConfigDesc)
					}
				}
			}
		}
		_ = w.Flush()
		fmt.Printf("\n全网二手集市共汇总 %d 台在售机器\n", total)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(marketCmd)
}
