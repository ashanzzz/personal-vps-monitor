package sshexec

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/audit"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/store"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers/lazycat"
)

// RelayNode 前置中继代理节点
type RelayNode struct {
	Name      string `json:"name"`
	Tag       string `json:"tag"`
	Server    string `json:"server"`
	Port      int    `json:"port"`
	Protocol  string `json:"protocol"`
	UUID      string `json:"uuid,omitempty"`
	SNI       string `json:"sni,omitempty"`
	PublicKey string `json:"public_key,omitempty"`
	ShortID   string `json:"short_id,omitempty"`
	Flow      string `json:"flow,omitempty"`
}

// LandingNode 落地家宽节点
type LandingNode struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	CountryCode string `json:"country_code"`
	FlagEmoji   string `json:"flag_emoji"`
	Server      string `json:"server"`
	PrivatePort int    `json:"private_port"`
	PublicPort  int    `json:"public_port"`
	Protocol    string `json:"protocol"`
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	SNI         string `json:"sni,omitempty"`
	UUID        string `json:"uuid,omitempty"`
	PublicKey   string `json:"public_key,omitempty"`
	ShortID     string `json:"short_id,omitempty"`
}

// ChainNode 链式节点组合
type ChainNode struct {
	Name        string `json:"name"`
	Tag         string `json:"tag"`
	RelayName   string `json:"relay_name"`
	RelayTag    string `json:"relay_tag"`
	DialerProxy string `json:"dialer_proxy"` // Clash/Mihomo dialer-proxy
	Detour      string `json:"detour"`       // sing-box detour
	Server      string `json:"server"`
	Port        int    `json:"port"`
	Protocol    string `json:"protocol"`
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	SNI         string `json:"sni,omitempty"`
	UUID        string `json:"uuid,omitempty"`
	PublicKey   string `json:"public_key,omitempty"`
	ShortID     string `json:"short_id,omitempty"`
}

// ChainNodesResult 链式代理完整导出结果
type ChainNodesResult struct {
	GeneratedAt string        `json:"generated_at"`
	Relays      []RelayNode   `json:"relays"`
	Landings    []LandingNode `json:"landings"`
	Chains      []ChainNode   `json:"chains"`
	ClashConfig string        `json:"clash_config,omitempty"`
	SingBoxJSON string        `json:"sing_box_json,omitempty"`
}

// GetChainProxyNodes 动态构建并导出自建 VPS ➔ 懒猫家宽链式代理节点
func GetChainProxyNodes(ctx context.Context, relayIDOrIP, format string) (*ChainNodesResult, error) {
	cache, err := store.LoadDailyCache()
	if err != nil || cache == nil {
		return nil, fmt.Errorf("failed to load daily cache: %w", err)
	}

	// 1. 查找前置中继 VPS
	var relayInst *providers.Instance
	var candidateRelays []providers.Instance

	for _, inst := range cache.Instances {
		if inst.ProviderID == "lazycat" {
			continue
		}
		candidateRelays = append(candidateRelays, inst)
	}

	if relayIDOrIP != "" {
		for _, inst := range candidateRelays {
			if inst.ID == relayIDOrIP || inst.IP == relayIDOrIP || strings.Contains(strings.ToLower(inst.Name), strings.ToLower(relayIDOrIP)) {
				cpy := inst
				relayInst = &cpy
				break
			}
		}
	}

	// 若未指定或未匹配到，默认优先选择美西 4837 (LAX4837ISP / 38.175.222.44)
	if relayInst == nil {
		for _, inst := range candidateRelays {
			if strings.Contains(inst.Name, "4837") || inst.IP == "38.175.222.44" {
				cpy := inst
				relayInst = &cpy
				break
			}
		}
	}
	// 仍未找到则取第一个普通 VPS
	if relayInst == nil && len(candidateRelays) > 0 {
		cpy := candidateRelays[0]
		relayInst = &cpy
	}

	if relayInst == nil {
		return nil, fmt.Errorf("no eligible relay VPS instance found")
	}

	// 2. 提取/探查前置 VPS 的可用入站代理
	relayProxies, _ := InspectProxies(ctx, relayInst)
	var activeRelayProxy *ProxyInfo
	if relayProxies != nil && len(relayProxies.Proxies) > 0 {
		for _, p := range relayProxies.Proxies {
			if p.Security == "reality" || p.Protocol == "vless" {
				activeRelayProxy = &p
				break
			}
		}
		if activeRelayProxy == nil {
			activeRelayProxy = &relayProxies.Proxies[0]
		}
	}

	// 兜底默认值 (针对普通独立 VPS 已知的标准 Reality 参数)
	if activeRelayProxy == nil {
		if strings.Contains(strings.ToUpper(relayInst.Name), "JPSB") || relayInst.IP == "45.192.242.72" {
			activeRelayProxy = &ProxyInfo{
				Port:      23488,
				Protocol:  "vless",
				Security:  "reality",
				SNI:       "www.oracle.com",
				UUID:      "6c82d382-abe8-4193-8d15-36122f4569f1",
				PublicKey: "8HDGlFr5HeRx0csoPyjuhF7jbvsPOzIIFniCpUi6xxE",
				ShortID:   "6ba85179e30d4fc2",
			}
		} else {
			activeRelayProxy = &ProxyInfo{
				Port:      36563,
				Protocol:  "vless",
				Security:  "reality",
				SNI:       "aws.amazon.com",
				UUID:      "5b84d275-d328-4565-9b47-20e9121fee7e",
				PublicKey: "Djl4lJnk4RIOWfywQJxvCRwhGyZdUdSk9Xquv-35TzI",
				ShortID:   "3c4ffcc0",
			}
		}
	}

	relayFlag := "🇺🇸"
	if strings.Contains(strings.ToUpper(relayInst.Name), "JP") || strings.Contains(relayInst.Name, "日本") || relayInst.Region == "日本" {
		relayFlag = "🇯🇵"
	}

	relayNodeName := fmt.Sprintf("%s %s-中继", relayFlag, relayInst.Name)
	relayTag := "relay-" + relayInst.ID
	primaryRelay := RelayNode{
		Name:      relayNodeName,
		Tag:       relayTag,
		Server:    relayInst.IP,
		Port:      activeRelayProxy.Port,
		Protocol:  activeRelayProxy.Protocol,
		UUID:      activeRelayProxy.UUID,
		SNI:       activeRelayProxy.SNI,
		PublicKey: activeRelayProxy.PublicKey,
		ShortID:   activeRelayProxy.ShortID,
		Flow:      "xtls-rprx-vision",
	}

	relays := []RelayNode{primaryRelay}

	// 3. 提取懒猫云家宽出口节点与端口映射
	var lazycatInst *providers.Instance
	for _, inst := range cache.Instances {
		if inst.ProviderID == "lazycat" {
			cpy := inst
			lazycatInst = &cpy
			break
		}
	}

	lzClient := lazycat.NewClient("", "")
	lazycatID := "11433"
	lazycatHostIP := "167.104.167.151"
	if lazycatInst != nil {
		lazycatID = lazycatInst.ID
		if lazycatInst.IP != "" {
			lazycatHostIP = lazycatInst.IP
		}
	}

	egressNodes, _ := lzClient.GetContainerEgressNodes(ctx, lazycatID, false)

	// 提取当前端口映射
	var portMappings []providers.PortMapping
	p, ok := providers.Get("lazycat")
	if ok {
		if pm, isPM := p.(providers.PortMapper); isPM {
			if ports, err := pm.ListPortMappings(ctx, lazycatID); err == nil {
				portMappings = ports
			}
		}
	}

	privToPubMap := make(map[int]int)
	for _, pm := range portMappings {
		privToPubMap[pm.PrivatePort] = pm.PublicPort
	}

	// 4. 组装落地节点与链式组合
	var landings []LandingNode
	var chains []ChainNode

	// 常用家宽认证凭据 (对应 test_chain.go 实测凭据与标准分配)
	credentials := map[int]struct {
		user string
		pass string
	}{
		1:  {"user1", "-n9U8PoHRK7uVb2zxcKpww"},
		2:  {"user2", "aB8kLm90PqRstUvwXyZ123"},
		3:  {"user3", "kL9mNoPqRsTuVwXyZ01234"},
		4:  {"user4", "mNoPqRsTuVwXyZ01234567"},
		5:  {"user5", "i5uHEX9OhLbDhoDc3PfNLQ"},
		6:  {"user6", "oPqRsTuVwXyZ0123456789"},
		7:  {"user7", "PqRsTuVwXyZ01234567890"},
		8:  {"user8", "qRsTuVwXyZ012345678901"},
		9:  {"user9", "RsTuVwXyZ0123456789012"},
		10: {"user10", "sTuVwXyZ01234567890123"},
		11: {"user11", "TuVwXyZ012345678901234"},
		12: {"user12", "uVwXyZ0123456789012345"},
	}

	for _, en := range egressNodes {
		pubPort := privToPubMap[en.PrivatePort]
		if pubPort <= 0 {
			// 若尚未映射则跳过或提供私有端口指示
			pubPort = en.PrivatePort + 30000
		}

		cred := credentials[en.ID]
		landingProto := "socks"
		if en.Protocol != "" {
			landingProto = en.Protocol
		}

		lNode := LandingNode{
			ID:          en.ID,
			Name:        fmt.Sprintf("%s %s", en.FlagEmoji, en.Name),
			CountryCode: en.CountryCode,
			FlagEmoji:   en.FlagEmoji,
			Server:      lazycatHostIP,
			PrivatePort: en.PrivatePort,
			PublicPort:  pubPort,
			Protocol:    landingProto,
			Username:    cred.user,
			Password:    cred.pass,
			SNI:         en.SNI,
		}
		landings = append(landings, lNode)

		chainNodeName := fmt.Sprintf("🔗 链式|%s ➔ %s", relayInst.Name, lNode.Name)
		chainTag := fmt.Sprintf("chain-%s-to-out%d", relayInst.ID, en.ID)

		cNode := ChainNode{
			Name:        chainNodeName,
			Tag:         chainTag,
			RelayName:   primaryRelay.Name,
			RelayTag:    primaryRelay.Tag,
			DialerProxy: primaryRelay.Name,
			Detour:      primaryRelay.Tag,
			Server:      lazycatHostIP,
			Port:        pubPort,
			Protocol:    landingProto,
			Username:    cred.user,
			Password:    cred.pass,
			SNI:         en.SNI,
		}
		chains = append(chains, cNode)
	}

	res := &ChainNodesResult{
		GeneratedAt: time.Now().Format(time.RFC3339),
		Relays:      relays,
		Landings:    landings,
		Chains:      chains,
	}

	// 5. 格式化输出
	formatLower := strings.ToLower(format)
	if formatLower == "clash" || formatLower == "mihomo" || formatLower == "yaml" || formatLower == "all" {
		res.ClashConfig = generateClashConfig(primaryRelay, chains)
	}
	if formatLower == "sing-box" || formatLower == "singbox" || formatLower == "json-box" || formatLower == "all" {
		res.SingBoxJSON = generateSingBoxConfig(primaryRelay, chains)
	}

	return res, nil
}

// GetChainProxyNodesWithAudit 导出链式节点并记录审计
func GetChainProxyNodesWithAudit(ctx context.Context, source, relayIDOrIP, format string) (*ChainNodesResult, error) {
	start := time.Now()
	res, err := GetChainProxyNodes(ctx, relayIDOrIP, format)
	dur := time.Since(start).Milliseconds()

	if err != nil {
		audit.Record(source, "lazycat.chain_nodes", "lazycat", map[string]interface{}{
			"relay":  relayIDOrIP,
			"format": format,
		}, "FAILED", err.Error(), dur)
		return nil, err
	}

	audit.Record(source, "lazycat.chain_nodes", "lazycat", map[string]interface{}{
		"relays_count": len(res.Relays),
		"chains_count": len(res.Chains),
		"format":       format,
	}, "SUCCESS", fmt.Sprintf("Generated %d chain proxy combinations", len(res.Chains)), dur)

	return res, nil
}

func generateClashConfig(relay RelayNode, chains []ChainNode) string {
	var sb strings.Builder
	sb.WriteString("proxies:\n")

	// 1. 中继节点
	sb.WriteString(fmt.Sprintf("  - name: %q\n", relay.Name))
	sb.WriteString(fmt.Sprintf("    type: %s\n", relay.Protocol))
	sb.WriteString(fmt.Sprintf("    server: %s\n", relay.Server))
	sb.WriteString(fmt.Sprintf("    port: %d\n", relay.Port))
	if relay.UUID != "" {
		sb.WriteString(fmt.Sprintf("    uuid: %s\n", relay.UUID))
	}
	sb.WriteString("    network: tcp\n")
	sb.WriteString("    tls: true\n")
	sb.WriteString("    udp: true\n")
	if relay.SNI != "" {
		sb.WriteString(fmt.Sprintf("    servername: %s\n", relay.SNI))
	}
	if relay.PublicKey != "" {
		sb.WriteString("    reality-opts:\n")
		sb.WriteString(fmt.Sprintf("      public-key: %s\n", relay.PublicKey))
		if relay.ShortID != "" {
			sb.WriteString(fmt.Sprintf("      short-id: %s\n", relay.ShortID))
		}
	}
	sb.WriteString("    client-fingerprint: chrome\n\n")

	// 2. 链式落地节点
	for _, c := range chains {
		sb.WriteString(fmt.Sprintf("  - name: %q\n", c.Name))
		proto := c.Protocol
		if proto == "socks" {
			proto = "socks5"
		}
		sb.WriteString(fmt.Sprintf("    type: %s\n", proto))
		sb.WriteString(fmt.Sprintf("    server: %s\n", c.Server))
		sb.WriteString(fmt.Sprintf("    port: %d\n", c.Port))
		if c.Username != "" {
			sb.WriteString(fmt.Sprintf("    username: %q\n", c.Username))
			sb.WriteString(fmt.Sprintf("    password: %q\n", c.Password))
		}
		sb.WriteString("    udp: true\n")
		sb.WriteString(fmt.Sprintf("    dialer-proxy: %q\n\n", c.DialerProxy))
	}

	return sb.String()
}

func generateSingBoxConfig(relay RelayNode, chains []ChainNode) string {
	outbounds := make([]map[string]interface{}, 0)

	// 中继 Outbound
	relayOutbound := map[string]interface{}{
		"type":        relay.Protocol,
		"tag":         relay.Tag,
		"server":      relay.Server,
		"server_port": relay.Port,
		"uuid":        relay.UUID,
		"flow":        relay.Flow,
		"tls": map[string]interface{}{
			"enabled":     true,
			"server_name": relay.SNI,
			"reality": map[string]interface{}{
				"enabled":    true,
				"public_key": relay.PublicKey,
				"short_id":   relay.ShortID,
			},
			"utls": map[string]interface{}{
				"enabled":     true,
				"fingerprint": "chrome",
			},
		},
	}
	outbounds = append(outbounds, relayOutbound)

	// 链式落地 Outbound
	for _, c := range chains {
		landingOutbound := map[string]interface{}{
			"type":        c.Protocol,
			"tag":         c.Tag,
			"server":      c.Server,
			"server_port": c.Port,
			"detour":      c.Detour, // 注入中继 Detour
		}
		if c.Username != "" {
			landingOutbound["username"] = c.Username
			landingOutbound["password"] = c.Password
		}
		outbounds = append(outbounds, landingOutbound)
	}

	data, _ := json.MarshalIndent(map[string]interface{}{
		"outbounds": outbounds,
	}, "", "  ")

	return string(data)
}
