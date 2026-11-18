package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"github.com/spf13/cobra"
)

var instancesCmd = &cobra.Command{
	Use:   "instances",
	Short: "聚合查看全网名下所有 VPS 实例资产与状态 (LazyCat / Akile / ACCK)",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		all := providers.GetAll()

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "PROVIDER\tID\tNAME\tIP\tSTATUS\tREGION\tEXPIRE_DATE")

		total := 0
		for _, p := range all {
			if im, ok := p.(providers.InstanceManager); ok {
				insts, err := im.ListInstances(ctx)
				if err == nil {
					for _, i := range insts {
						total++
						fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", i.ProviderID, i.ID, i.Name, i.IP, i.Status, i.Region, i.ExpireDate)
					}
				}
			}
		}
		_ = w.Flush()
		fmt.Printf("\n全网资产总计: %d 台机器\n", total)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(instancesCmd)
}
