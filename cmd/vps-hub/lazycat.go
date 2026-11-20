package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/audit"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"github.com/spf13/cobra"
)

var lazycatCmd = &cobra.Command{
	Use:   "lazycat",
	Short: "懒猫云相关运维指令",
}

var probePorts bool

var lazycatPortsCmd = &cobra.Command{
	Use:   "ports [instance_id]",
	Short: "动态查看懒猫云公网 NAT 端口映射列表及在线连通性",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, ok := providers.Get("lazycat")
		if !ok {
			return fmt.Errorf("lazycat provider not registered")
		}
		pm, ok := p.(providers.PortMapper)
		if !ok {
			return fmt.Errorf("lazycat does not implement PortMapper")
		}

		instID := ""
		if len(args) > 0 {
			instID = args[0]
		}

		ports, err := pm.ListPortMappings(context.Background(), instID)
		if err != nil {
			return err
		}

		// 获取公网 IP 用于探测
		publicIP := "167.104.167.151"
		if im, ok := p.(providers.InstanceManager); ok {
			if insts, err := im.ListInstances(context.Background()); err == nil {
				for _, inst := range insts {
					if instID == "" || inst.ID == instID {
						if inst.IP != "" {
							publicIP = inst.IP
						}
						break
					}
				}
			}
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		if probePorts {
			fmt.Fprintln(w, "ID	PROTOCOL	PRIVATE_PORT	PUBLIC_PORT	STATUS	LATENCY	REMARK")
			for _, m := range ports {
				status := "CLOSED"
				latencyStr := "-"
				if m.Protocol == "tcp" && publicIP != "" {
					addr := net.JoinHostPort(publicIP, strconv.Itoa(m.PublicPort))
					start := time.Now()
					conn, err := net.DialTimeout("tcp", addr, 1500*time.Millisecond)
					if err == nil {
						_ = conn.Close()
						status = "ONLINE"
						latencyStr = fmt.Sprintf("%dms", time.Since(start).Milliseconds())
					}
				}
				fmt.Fprintf(w, "%d	%s	%d	%d	%s	%s	%s\n", m.ID, m.Protocol, m.PrivatePort, m.PublicPort, status, latencyStr, m.Remark)
			}
		} else {
			fmt.Fprintln(w, "ID	PROTOCOL	PRIVATE_PORT	PUBLIC_PORT	REMARK")
			for _, m := range ports {
				fmt.Fprintf(w, "%d	%s	%d	%d	%s\n", m.ID, m.Protocol, m.PrivatePort, m.PublicPort, m.Remark)
			}
		}
		_ = w.Flush()
		fmt.Printf("\n当前映射端口数: %d\n", len(ports))
		return nil
	},
}

var lazycatInstancesCmd = &cobra.Command{
	Use:   "instances",
	Short: "动态查看懒猫云名下所有实例、系列类别（全球家宽 vs 固定出口）、端口配额与状态",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, ok := providers.Get("lazycat")
		if !ok {
			return fmt.Errorf("lazycat provider not registered")
		}
		im, ok := p.(providers.InstanceManager)
		if !ok {
			return fmt.Errorf("lazycat does not implement InstanceManager")
		}

		insts, err := im.ListInstances(context.Background())
		if err != nil {
			return err
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID	NAME	SERIES	STATUS	PUBLIC_IP	PORT_LIMIT	EGRESS_TYPE")
		for _, i := range insts {
			egressType := "固定出口 (Fixed Node)"
			if i.Category == "全球家宽" {
				egressType = "12国家宽轮询出口 (Multi-Egress)"
			}
			fmt.Fprintf(w, "%s	%s	%s	%s	%s	%d	%s\n", i.ID, i.Name, i.Category, i.Status, i.IP, i.PortLimit, egressType)
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
		start := time.Now()
		if err := es.SyncEgress(context.Background()); err != nil {
			dur := time.Since(start).Milliseconds()
			audit.Record("cli", "lazycat.egress_sync", "lazycat", nil, "FAILED", err.Error(), dur)
			return err
		}
		dur := time.Since(start).Milliseconds()
		audit.Record("cli", "lazycat.egress_sync", "lazycat", nil, "SUCCESS", "12家宽出口轮询与sing-box热重载完成", dur)
		fmt.Println("同步与重载成功完成。")
		return nil
	},
}

var lazycatCheckinCmd = &cobra.Command{
	Use:   "checkin",
	Short: "执行懒猫云每日签到打卡并领取积分",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, ok := providers.Get("lazycat")
		if !ok {
			return fmt.Errorf("lazycat provider not registered")
		}
		ac, ok := p.(providers.AutoCheckin)
		if !ok {
			return fmt.Errorf("lazycat does not implement AutoCheckin")
		}

		start := time.Now()
		res, err := ac.DoCheckin(context.Background())
		dur := time.Since(start).Milliseconds()
		if err != nil {
			audit.Record("cli", "lazycat.checkin", "lazycat", nil, "FAILED", err.Error(), dur)
			return err
		}
		audit.Record("cli", "lazycat.checkin", "lazycat", map[string]interface{}{"account": res.Account, "reward": res.Reward}, "SUCCESS", res.Message, dur)
		fmt.Printf("懒猫云签到结果: %s (奖励: %s)\n所属网站: %s | 所属账号: %s\n", res.Message, res.Reward, res.Website, res.Account)
		return nil
	},
}

func init() {
	lazycatPortsCmd.Flags().BoolVarP(&probePorts, "probe", "p", true, "探测端口真实 TCP 连通性与响应延迟")
	lazycatCmd.AddCommand(lazycatPortsCmd)
	lazycatCmd.AddCommand(lazycatInstancesCmd)
	lazycatCmd.AddCommand(lazycatSyncCmd)
	lazycatCmd.AddCommand(lazycatCheckinCmd)
	rootCmd.AddCommand(lazycatCmd)
}
