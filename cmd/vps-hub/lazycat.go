package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"github.com/spf13/cobra"
)

var lazycatCmd = &cobra.Command{
	Use:   "lazycat",
	Short: "懒猫云相关运维指令",
}

var lazycatPortsCmd = &cobra.Command{
	Use:   "ports",
	Short: "查看当前懒猫云公网 NAT 端口映射列表",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, ok := providers.Get("lazycat")
		if !ok {
			return fmt.Errorf("lazycat provider not registered")
		}
		pm, ok := p.(providers.PortMapper)
		if !ok {
			return fmt.Errorf("lazycat does not implement PortMapper")
		}

		ports, err := pm.ListPortMappings(context.Background(), "")
		if err != nil {
			return err
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tPROTOCOL\tPRIVATE_PORT\tPUBLIC_PORT\tREMARK")
		for _, m := range ports {
			fmt.Fprintf(w, "%d\t%s\t%d\t%d\t%s\n", m.ID, m.Protocol, m.PrivatePort, m.PublicPort, m.Remark)
		}
		return w.Flush()
	},
}

var lazycatSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "触发 12 家宽出口凭据轮询与 sing-box 聚合配置热重载",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, ok := providers.Get("lazycat")
		if !ok {
			return fmt.Errorf("lazycat provider not registered")
		}
		es, ok := p.(providers.EgressSyncer)
		if !ok {
			return fmt.Errorf("lazycat does not implement EgressSyncer")
		}

		fmt.Println("正在触发懒猫云 12 家宽出口轮询与 sing-box 热重载...")
		if err := es.SyncEgress(context.Background()); err != nil {
			return err
		}
		fmt.Println("同步与重载成功完成。")
		return nil
	},
}

func init() {
	lazycatCmd.AddCommand(lazycatPortsCmd)
	lazycatCmd.AddCommand(lazycatSyncCmd)
	rootCmd.AddCommand(lazycatCmd)
}
