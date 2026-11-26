package sshexec

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
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
	URILinks    []string      `json:"uri_links,omitempty"`
	ClashConfig string        `json:"clash_config,omitempty"`
	SingBoxJSON string        `json:"sing_box_json,omitempty"`
}

func buildRelayForInstance(ctx context.Context, inst *providers.Instance) *RelayNode {
	relayProxies, _ := InspectProxies(ctx, inst)
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

	// 兜底默认值
	if activeRelayProxy == nil {
		if strings.Contains(strings.ToUpper(inst.Name), "JPSB") || inst.IP == "45.192.242.72" {
			activeRelayProxy = &ProxyInfo{
				Port:      23488,
				Protocol:  "vless",
				Security:  "reality",
				SNI:       "www.oracle.com",
				UUID:      "6c82d382-abe8-4193-8d15-36122f4569f1",
				PublicKey: "8HDGlFr5HeRx0csoPyjuhF7jbvsPOzIIFniCpUi6xxE",
				ShortID:   "6ba85179e30d4fc2",
			}
		} else if strings.Contains(strings.ToUpper(inst.Name), "IIJ") || inst.IP == "156.231.113.91" {
			activeRelayProxy = &ProxyInfo{
				Port:      33338,
				Protocol:  "vless",
				Security:  "reality",
				SNI:       "aws.amazon.com",
				UUID:      "625d013c-28c5-4295-a5e7-459cf52459b9",
				PublicKey: "Djl4lJnk4RIOWfywQJxvCRwhGyZdUdSk9Xquv-35TzI",
				ShortID:   "3c4ffcc0",
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
	if strings.Contains(strings.ToUpper(inst.Name), "JP") || strings.Contains(inst.Name, "日本") || inst.Region == "日本" {
		relayFlag = "🇯🇵"
	}

	relayNodeName := fmt.Sprintf("%s %s-直连", relayFlag, inst.Name)
	relayTag := "relay-" + inst.ID
	return &RelayNode{
		Name:      relayNodeName,
		Tag:       relayTag,
		Server:    inst.IP,
		Port:      activeRelayProxy.Port,
		Protocol:  activeRelayProxy.Protocol,
		UUID:      activeRelayProxy.UUID,
		SNI:       activeRelayProxy.SNI,
		PublicKey: activeRelayProxy.PublicKey,
		ShortID:   activeRelayProxy.ShortID,
		Flow:      "xtls-rprx-vision",
	}
}

// GetChainProxyNodes 动态构建并导出自建 VPS ➔ 懒猫家宽链式代理节点
func GetChainProxyNodes(ctx context.Context, relayIDOrIP, format string) (*ChainNodesResult, error) {
	cache, err := store.LoadDailyCache()
	if err != nil || cache == nil {
		return nil, fmt.Errorf("failed to load daily cache: %w", err)
	}

	// 1. 查找前置中继 VPS 候选列表
	var candidateRelays []providers.Instance
	for _, inst := range cache.Instances {
		if inst.ProviderID == "lazycat" {
			continue
		}
		candidateRelays = append(candidateRelays, inst)
	}

	if len(candidateRelays) == 0 {
		return nil, fmt.Errorf("no eligible relay VPS instance found")
	}

	// 构建所有可用中继节点池
	var relays []RelayNode
	var relayJPSB, relayIIJ, relay4837 *RelayNode
	relayMap := make(map[string]*RelayNode)

	for _, inst := range candidateRelays {
		cpy := inst
		r := buildRelayForInstance(ctx, &cpy)
		relays = append(relays, *r)
		relayMap[inst.ID] = r
		relayMap[inst.IP] = r

		if strings.Contains(strings.ToUpper(inst.Name), "JPSB") || inst.IP == "45.192.242.72" {
			relayJPSB = r
		} else if strings.Contains(strings.ToUpper(inst.Name), "IIJ") || inst.IP == "156.231.113.91" {
			relayIIJ = r
		} else if strings.Contains(inst.Name, "4837") || inst.IP == "38.175.222.44" {
			relay4837 = r
		}
	}

	// 检查是否指定了单特定中继
	var fixedRelay *RelayNode
	if relayIDOrIP != "" && relayIDOrIP != "smart" {
		for _, inst := range candidateRelays {
			if inst.ID == relayIDOrIP || inst.IP == relayIDOrIP || strings.Contains(strings.ToLower(inst.Name), strings.ToLower(relayIDOrIP)) {
				fixedRelay = relayMap[inst.ID]
				break
			}
		}
		if fixedRelay == nil && len(relays) > 0 {
			fixedRelay = &relays[0]
		}
	}

	// 2. 提取懒猫云家宽出口节点与端口映射
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

	// 3. 组装落地节点与链式组合 (复用容器真实 VLESS Reality 凭证，不再硬编码假账号)
	var landings []LandingNode
	var chains []ChainNode
	var uriLinks []string

	for _, en := range egressNodes {
		pubPort := privToPubMap[en.PrivatePort]
		if pubPort <= 0 {
			pubPort = en.PrivatePort + 30000
		}

		landingProto := "vless"
		if en.Protocol != "" {
			landingProto = en.Protocol
		}
		nodeUUID := en.UUID
		if nodeUUID == "" {
			nodeUUID = fmt.Sprintf("lazycat-out-%d", en.ID)
		}
		nodeSNI := en.SNI
		if nodeSNI == "" {
			nodeSNI = "www.apple.com"
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
			SNI:         nodeSNI,
			UUID:        nodeUUID,
			PublicKey:   en.PublicKey,
			ShortID:     en.ShortID,
		}
		landings = append(landings, lNode)

		// 智能地理就近配对中继
		var chosenRelay *RelayNode
		if fixedRelay != nil {
			chosenRelay = fixedRelay
		} else {
			// 亚太/日本 -> 首选 JPSB4 (次选 IIJ，再选 4837)
			cc := strings.ToUpper(en.CountryCode)
			if cc == "JP" || cc == "KR" || cc == "TW" || cc == "SG" || cc == "HK" {
				if relayJPSB != nil {
					chosenRelay = relayJPSB
				} else if relayIIJ != nil {
					chosenRelay = relayIIJ
				} else if relay4837 != nil {
					chosenRelay = relay4837
				}
			} else {
				// 美洲/其他 -> 首选 4837 (次选 JPSB)
				if relay4837 != nil {
					chosenRelay = relay4837
				} else if relayJPSB != nil {
					chosenRelay = relayJPSB
				} else if relayIIJ != nil {
					chosenRelay = relayIIJ
				}
			}
		}

		if chosenRelay == nil && len(relays) > 0 {
			chosenRelay = &relays[0]
		}

		if chosenRelay == nil {
			continue
		}

		chainNodeName := fmt.Sprintf("🔗 链式|%s ➔ %s", chosenRelay.Name, lNode.Name)
		chainTag := fmt.Sprintf("chain-%s-to-out%d", chosenRelay.Tag, en.ID)

		cNode := ChainNode{
			Name:        chainNodeName,
			Tag:         chainTag,
			RelayName:   chosenRelay.Name,
			RelayTag:    chosenRelay.Tag,
			DialerProxy: chosenRelay.Name,
			Detour:      chosenRelay.Tag,
			Server:      lazycatHostIP,
			Port:        pubPort,
			Protocol:    landingProto,
			SNI:         nodeSNI,
			UUID:        nodeUUID,
			PublicKey:   en.PublicKey,
			ShortID:     en.ShortID,
		}
		chains = append(chains, cNode)

		// Sub-Store 兼容 VLESS Reality URI 格式 (携带 ?dialerProxy=)
		params := url.Values{}
		params.Set("encryption", "none")
		params.Set("flow", "xtls-rprx-vision")
		params.Set("security", "reality")
		params.Set("sni", nodeSNI)
		params.Set("fp", "chrome")
		params.Set("type", "tcp")
		if en.PublicKey != "" {
			params.Set("pbk", en.PublicKey)
		}
		if en.ShortID != "" {
			params.Set("sid", en.ShortID)
		}
		params.Set("dialerProxy", chosenRelay.Name)
		uri := fmt.Sprintf("vless://%s@%s:%d?%s#%s",
			nodeUUID,
			lazycatHostIP,
			pubPort,
			params.Encode(),
			url.PathEscape(chainNodeName),
		)
		uriLinks = append(uriLinks, uri)
	}

	res := &ChainNodesResult{
		GeneratedAt: time.Now().Format(time.RFC3339),
		Relays:      relays,
		Landings:    landings,
		Chains:      chains,
		URILinks:    uriLinks,
	}

	// 4. 格式化输出
	formatLower := strings.ToLower(format)
	if formatLower == "clash" || formatLower == "mihomo" || formatLower == "yaml" || formatLower == "all" {
		res.ClashConfig = generateClashConfig(relays, chains)
	}
	if formatLower == "sing-box" || formatLower == "singbox" || formatLower == "json-box" || formatLower == "all" {
		res.SingBoxJSON = generateSingBoxConfig(relays, chains)
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

func generateClashConfig(relays []RelayNode, chains []ChainNode) string {
	var sb strings.Builder
	sb.WriteString("proxies:\n")

	// 1. 中继节点列表 (去重)
	seenRelays := make(map[string]bool)
	for _, relay := range relays {
		if seenRelays[relay.Name] {
			continue
		}
		seenRelays[relay.Name] = true

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
	}

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
		if proto == "vless" {
			sb.WriteString(fmt.Sprintf("    uuid: %s\n", c.UUID))
			sb.WriteString("    network: tcp\n")
			sb.WriteString("    tls: true\n")
			sb.WriteString("    udp: true\n")
			sb.WriteString("    flow: xtls-rprx-vision\n")
			if c.SNI != "" {
				sb.WriteString(fmt.Sprintf("    servername: %s\n", c.SNI))
			}
			if c.PublicKey != "" {
				sb.WriteString("    reality-opts:\n")
				sb.WriteString(fmt.Sprintf("      public-key: %s\n", c.PublicKey))
				if c.ShortID != "" {
					sb.WriteString(fmt.Sprintf("      short-id: %s\n", c.ShortID))
				}
			}
			sb.WriteString("    client-fingerprint: chrome\n")
		} else {
			if c.Username != "" {
				sb.WriteString(fmt.Sprintf("    username: %q\n", c.Username))
				sb.WriteString(fmt.Sprintf("    password: %q\n", c.Password))
			}
			sb.WriteString("    udp: true\n")
		}
		sb.WriteString(fmt.Sprintf("    dialer-proxy: %q\n\n", c.DialerProxy))
	}

	return sb.String()
}

func generateSingBoxConfig(relays []RelayNode, chains []ChainNode) string {
	outbounds := make([]map[string]interface{}, 0)

	// 中继 Outbounds (去重)
	seenRelays := make(map[string]bool)
	for _, relay := range relays {
		if seenRelays[relay.Tag] {
			continue
		}
		seenRelays[relay.Tag] = true

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
	}

	// 链式落地 Outbound
	for _, c := range chains {
		landingOutbound := map[string]interface{}{
			"type":        c.Protocol,
			"tag":         c.Tag,
			"server":      c.Server,
			"server_port": c.Port,
			"detour":      c.Detour, // 注入中继 Detour
		}
		if c.Protocol == "vless" {
			landingOutbound["uuid"] = c.UUID
			landingOutbound["flow"] = "xtls-rprx-vision"
			realityCfg := map[string]interface{}{
				"enabled":    true,
				"public_key": c.PublicKey,
			}
			if c.ShortID != "" {
				realityCfg["short_id"] = c.ShortID
			}
			landingOutbound["tls"] = map[string]interface{}{
				"enabled":     true,
				"server_name": c.SNI,
				"reality":     realityCfg,
				"utls": map[string]interface{}{
					"enabled":     true,
					"fingerprint": "chrome",
				},
			}
		} else if c.Username != "" {
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
