package lazycat

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// EgressNode 容器多出口 VLESS Reality 映射节点 (按地区优先级排列，自适应 N 个节点)
type EgressNode struct {
	ID          int    `json:"id"`
	Priority    int    `json:"priority"` // 地区优先级排名: 1=US, 2=JP, 3=SG, 4=KR, 5=TW, 6=HK, 7=PH, 8=GLOBAL
	Name        string `json:"name"`
	CountryCode string `json:"country_code"`
	FlagEmoji   string `json:"flag_emoji"`
	PrivatePort int    `json:"private_port"` // 10000 + ID (如 10001, 10005, 10012)
	PublicPort  int    `json:"public_port,omitempty"`
	Mapped      bool   `json:"mapped"`
	HostIP      string `json:"host_ip,omitempty"`
	OutboundTag string `json:"outbound_tag"` // out-1, out-5
	Protocol    string `json:"protocol"`     // vless
	Flow        string `json:"flow"`         // xtls-rprx-vision
	SNI         string `json:"sni"`          // www.apple.com
	UUID        string `json:"uuid,omitempty"`
	PublicKey   string `json:"public_key,omitempty"`
	ShortID     string `json:"short_id,omitempty"`
	VLESSUrl    string `json:"vless_url,omitempty"`
}

// EgressSubscriptionBundle 带鉴权订阅导出的结构化集合
type EgressSubscriptionBundle struct {
	InstanceID       string       `json:"instance_id"`
	HostPublicIP     string       `json:"host_public_ip"`
	PriorityOrder    []string     `json:"priority_order"`
	TotalNodes       int          `json:"total_nodes"`
	MappedNodes      int          `json:"mapped_nodes"`
	ExportedNodes    int          `json:"exported_nodes"`
	RealityPublicKey string       `json:"reality_public_key,omitempty"`
	RealityShortID   string       `json:"reality_short_id,omitempty"`
	RealitySNI       string       `json:"reality_sni,omitempty"`
	RefreshedAt      string       `json:"refreshed_at"`
	Nodes            []EgressNode `json:"nodes"`
	URILinks         []string     `json:"uri_links"`
	Base64           string       `json:"base64,omitempty"`
	ClashYAML        string       `json:"clash_yaml,omitempty"`
	SingBoxJSON      string       `json:"singbox_json,omitempty"`
}

type egressCredsFile struct {
	Reality struct {
		PublicKey  string `json:"public_key"`
		ShortID    string `json:"short_id"`
		ServerName string `json:"server_name"`
	} `json:"_reality"`
	Nodes map[string]struct {
		UUID string `json:"uuid"`
	} `json:"-"`
}

var (
	egressCacheMu sync.RWMutex
	egressCache   = make(map[string]struct {
		nodes     []EgressNode
		fetchedAt time.Time
	})

	// countryPriorityOrder 地区优先级顺序: US > JP > SG > KR > TW > HK > PH > GLOBAL
	countryPriorityOrder = []string{"US", "JP", "SG", "KR", "TW", "HK", "PH", "GLOBAL"}
)

// ClearEgressCache 清除指定实例或全部实例的出口缓存
func ClearEgressCache(instanceID string) {
	egressCacheMu.Lock()
	defer egressCacheMu.Unlock()
	if instanceID == "" {
		egressCache = make(map[string]struct {
			nodes     []EgressNode
			fetchedAt time.Time
		})
		return
	}
	delete(egressCache, instanceID)
}

func countryPriorityRank(cc string) int {
	upper := strings.ToUpper(strings.TrimSpace(cc))
	for idx, code := range countryPriorityOrder {
		if code == upper {
			return idx + 1
		}
	}
	return len(countryPriorityOrder) + 1
}

// Helper: 国家代码与国旗识别
func detectCountryFlag(name string) (string, string) {
	nameLower := strings.ToLower(name)
	switch {
	case strings.Contains(name, "美国") || strings.Contains(nameLower, "pavlov") || strings.Contains(nameLower, "frontier") || strings.Contains(nameLower, "verizon") || strings.Contains(nameLower, "-us") || strings.Contains(nameLower, "us-"):
		return "US", "🇺🇸"
	case strings.Contains(name, "日本") || strings.Contains(nameLower, "softbank") || strings.Contains(nameLower, "biglobe") || strings.Contains(nameLower, "glbb") || strings.Contains(nameLower, "kddi") || strings.Contains(nameLower, "japan") || strings.Contains(nameLower, "-jp") || strings.Contains(nameLower, "jp-"):
		return "JP", "🇯🇵"
	case strings.Contains(name, "新加坡") || strings.Contains(nameLower, "starhub") || strings.Contains(nameLower, "-sg") || strings.Contains(nameLower, "sg-"):
		return "SG", "🇸🇬"
	case strings.Contains(name, "香港") || strings.Contains(nameLower, "hkt") || strings.Contains(nameLower, "-hk") || strings.Contains(nameLower, "hk-"):
		return "HK", "🇭🇰"
	case strings.Contains(name, "台湾") || strings.Contains(nameLower, "hinet") || strings.Contains(nameLower, "-tw") || strings.Contains(nameLower, "tw-"):
		return "TW", "🇹🇼"
	case strings.Contains(name, "韩国") || strings.Contains(nameLower, "korea") || strings.Contains(nameLower, "-kt") || strings.Contains(nameLower, "-kr") || strings.Contains(nameLower, "kr-"):
		return "KR", "🇰🇷"
	case strings.Contains(name, "菲律宾") || strings.Contains(nameLower, "dito") || strings.Contains(nameLower, "-ph") || strings.Contains(nameLower, "ph-"):
		return "PH", "🇵🇭"
	default:
		return "GLOBAL", "🌐"
	}
}

func sortEgressNodesByPriority(nodes []EgressNode) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Priority != nodes[j].Priority {
			return nodes[i].Priority < nodes[j].Priority
		}
		return nodes[i].ID < nodes[j].ID
	})
}

func extractJSONObject(raw string) string {
	startIdx := strings.Index(raw, "{")
	endIdx := strings.LastIndex(raw, "}")
	if startIdx >= 0 && endIdx > startIdx {
		return raw[startIdx : endIdx+1]
	}
	return ""
}

func parseCredentialsJSON(raw string) *egressCredsFile {
	jsonStr := extractJSONObject(raw)
	if jsonStr == "" {
		return nil
	}
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonStr), &rawMap); err != nil {
		return nil
	}
	res := &egressCredsFile{
		Nodes: make(map[string]struct {
			UUID string `json:"uuid"`
		}),
	}
	if rBytes, ok := rawMap["_reality"]; ok {
		_ = json.Unmarshal(rBytes, &res.Reality)
	}
	for k, v := range rawMap {
		if k == "_reality" {
			continue
		}
		var item struct {
			UUID string `json:"uuid"`
		}
		if err := json.Unmarshal(v, &item); err == nil && item.UUID != "" {
			res.Nodes[k] = item
		}
	}
	return res
}

// BuildVLESSURL 生成标准 VLESS Reality 客户端导入链接
func (n EgressNode) BuildVLESSURL(hostIP string, publicPort int) string {
	if hostIP == "" || publicPort <= 0 || n.UUID == "" {
		return ""
	}
	flow := n.Flow
	if flow == "" {
		flow = "xtls-rprx-vision"
	}
	sni := n.SNI
	if sni == "" {
		sni = "www.apple.com"
	}
	params := url.Values{}
	params.Set("encryption", "none")
	params.Set("flow", flow)
	params.Set("security", "reality")
	params.Set("sni", sni)
	params.Set("fp", "chrome")
	params.Set("type", "tcp")
	if n.PublicKey != "" {
		params.Set("pbk", n.PublicKey)
	}
	if n.ShortID != "" {
		params.Set("sid", n.ShortID)
	}
	title := strings.TrimSpace(fmt.Sprintf("%s %s", n.FlagEmoji, n.Name))
	return fmt.Sprintf("vless://%s@%s:%d?%s#%s", n.UUID, hostIP, publicPort, params.Encode(), url.PathEscape(title))
}

// GetContainerEgressNodes 动态读取并解析当前容器内实际生效的 N 个出口节点及真实 VLESS Reality 凭证
func (c *Client) GetContainerEgressNodes(ctx context.Context, instanceID string, forceRefresh bool) ([]EgressNode, error) {
	if instanceID == "" {
		instanceID = "11433"
	}

	if !forceRefresh {
		egressCacheMu.RLock()
		if cached, ok := egressCache[instanceID]; ok && time.Since(cached.fetchedAt) < 60*time.Second {
			nodes := make([]EgressNode, len(cached.nodes))
			copy(nodes, cached.nodes)
			egressCacheMu.RUnlock()
			return nodes, nil
		}
		egressCacheMu.RUnlock()
	}

	// 1. 单次纯 Go 原生终端短指令同时读取 nodes.json 与 credentials.json（不泄露 private_key 到结构体）
	cmd := "printf '__LZ_NODES__\\n'; (cat /root/nodes.json 2>/dev/null || cat /opt/lazycat-multi-egress/nodes.json 2>/dev/null || echo '{}'); printf '\\n__LZ_CREDS__\\n'; (cat /opt/lazycat-multi-egress/credentials.json 2>/dev/null || echo '{}'); printf '\\n__LZ_DONE__\\n'"
	out, err := c.ExecTerminalCommand(ctx, instanceID, cmd, 12*time.Second)
	if err == nil && len(out) > 10 {
		nodesPart := out
		credsPart := ""
		if idxNodes := strings.Index(out, "__LZ_NODES__"); idxNodes >= 0 {
			afterNodes := out[idxNodes+len("__LZ_NODES__"):]
			if idxCreds := strings.Index(afterNodes, "__LZ_CREDS__"); idxCreds >= 0 {
				nodesPart = afterNodes[:idxCreds]
				rest := afterNodes[idxCreds+len("__LZ_CREDS__"):]
				if idxDone := strings.Index(rest, "__LZ_DONE__"); idxDone >= 0 {
					credsPart = rest[:idxDone]
				} else {
					credsPart = rest
				}
			} else {
				nodesPart = afterNodes
			}
		}

		creds := parseCredentialsJSON(credsPart)
		defaultSNI := "www.apple.com"
		defaultPubKey := ""
		defaultShortID := ""
		if creds != nil {
			if creds.Reality.ServerName != "" {
				defaultSNI = creds.Reality.ServerName
			}
			defaultPubKey = creds.Reality.PublicKey
			defaultShortID = creds.Reality.ShortID
		}

		var rawNodes map[string]struct {
			Name     string `json:"name"`
			Outbound struct {
				Server     string `json:"server"`
				ServerPort int    `json:"server_port"`
				Tag        string `json:"tag"`
			} `json:"outbound"`
		}

		if jsonStr := extractJSONObject(nodesPart); jsonStr != "" {
			if jsonErr := json.Unmarshal([]byte(jsonStr), &rawNodes); jsonErr == nil && len(rawNodes) > 0 {
				var nodes []EgressNode
				for sID, n := range rawNodes {
					id, _ := strconv.Atoi(sID)
					if id <= 0 {
						continue
					}
					cc, flag := detectCountryFlag(n.Name)
					nodeUUID := ""
					if creds != nil {
						if item, ok := creds.Nodes[sID]; ok {
							nodeUUID = item.UUID
						}
					}
					nodes = append(nodes, EgressNode{
						ID:          id,
						Priority:    countryPriorityRank(cc),
						Name:        n.Name,
						CountryCode: cc,
						FlagEmoji:   flag,
						PrivatePort: 10000 + id,
						OutboundTag: fmt.Sprintf("out-%d", id),
						Protocol:    "vless",
						Flow:        "xtls-rprx-vision",
						SNI:         defaultSNI,
						UUID:        nodeUUID,
						PublicKey:   defaultPubKey,
						ShortID:     defaultShortID,
					})
				}

				sortEgressNodesByPriority(nodes)

				egressCacheMu.Lock()
				egressCache[instanceID] = struct {
					nodes     []EgressNode
					fetchedAt time.Time
				}{nodes: nodes, fetchedAt: time.Now()}
				egressCacheMu.Unlock()

				resCopy := make([]EgressNode, len(nodes))
				copy(resCopy, nodes)
				return resCopy, nil
			}
		}
	}

	// 2. 降级方案：若容器未启动或 nodes.json 暂时不可读，提供按优先级排序的标准自愈预设出口列表
	fallbackDefaults := []struct {
		id   int
		name string
	}{
		{1, "日本-SoftBank动态"},
		{2, "日本-Biglobe动态家宽"},
		{3, "日本-GLBB-Japan-KK动态家宽"},
		{4, "日本-KDDI动态"},
		{5, "美国-Pavlov动态家宽"},
		{6, "美国-Frontier动态家宽"},
		{7, "美国-Verizon动态家宽"},
		{8, "中国香港-HKT动态家宽"},
		{9, "新加坡-StarHub动态家宽"},
		{10, "中国台湾-Hinet动态家宽"},
		{11, "韩国-KT动态家宽"},
		{12, "菲律宾-DITO动态家宽"},
	}

	var fallbackNodes []EgressNode
	for _, f := range fallbackDefaults {
		cc, flag := detectCountryFlag(f.name)
		fallbackNodes = append(fallbackNodes, EgressNode{
			ID:          f.id,
			Priority:    countryPriorityRank(cc),
			Name:        f.name,
			CountryCode: cc,
			FlagEmoji:   flag,
			PrivatePort: 10000 + f.id,
			OutboundTag: fmt.Sprintf("out-%d", f.id),
			Protocol:    "vless",
			Flow:        "xtls-rprx-vision",
			SNI:         "www.apple.com",
		})
	}
	sortEgressNodesByPriority(fallbackNodes)

	return fallbackNodes, nil
}

// GetEnrichedEgressNodes 获取容器出口节点，并与平台 NAT 公网端口映射绑定生成完整 VLESS Reality 链接
func (c *Client) GetEnrichedEgressNodes(ctx context.Context, instanceID string, forceRefresh bool) ([]EgressNode, string, error) {
	if instanceID == "" {
		instanceID = "11433"
	}

	nodes, err := c.GetContainerEgressNodes(ctx, instanceID, forceRefresh)
	if err != nil {
		return nil, "", err
	}

	hostIP := ""
	privToPub := make(map[int]int)
	if c.token != "" {
		if detail, dErr := c.GetInstanceDetail(ctx, instanceID); dErr == nil && detail != nil {
			hostIP = detail.HostPublicIP
			if hostIP == "" {
				hostIP = detail.NatPublicIP
			}
			for _, pm := range detail.PortMappings {
				if pm.PrivatePort > 0 && pm.PublicPort > 0 && strings.EqualFold(pm.Protocol, "tcp") {
					privToPub[pm.PrivatePort] = pm.PublicPort
				}
			}
		}
	}

	for i := range nodes {
		nodes[i].HostIP = hostIP
		if pubPort, ok := privToPub[nodes[i].PrivatePort]; ok && pubPort > 0 {
			nodes[i].PublicPort = pubPort
			nodes[i].Mapped = true
			nodes[i].VLESSUrl = nodes[i].BuildVLESSURL(hostIP, pubPort)
		}
	}
	sortEgressNodesByPriority(nodes)
	return nodes, hostIP, nil
}

// SubscriptionExportOptions 控制订阅导出格式与过滤选项
type SubscriptionExportOptions struct {
	InstanceID string
	Format     string // json, uri, base64, clash, sing-box
	OnlyMapped bool
	Countries  []string
	GroupName  string
	Refresh    bool
}

// ExportEgressSubscriptions 生成多格式、带鉴权的 VLESS Reality 家宽订阅包
func (c *Client) ExportEgressSubscriptions(ctx context.Context, opts SubscriptionExportOptions) (*EgressSubscriptionBundle, error) {
	if opts.InstanceID == "" {
		opts.InstanceID = "11433"
	}

	nodes, hostIP, err := c.GetEnrichedEgressNodes(ctx, opts.InstanceID, opts.Refresh)
	if err != nil {
		return nil, err
	}

	countryFilter := make(map[string]bool)
	for _, cc := range opts.Countries {
		if trimmed := strings.ToUpper(strings.TrimSpace(cc)); trimmed != "" {
			countryFilter[trimmed] = true
		}
	}

	mappedCount := 0
	var exported []EgressNode
	var uriLinks []string
	var realityPubKey, realityShortID, realitySNI string

	for _, n := range nodes {
		if n.Mapped {
			mappedCount++
		}
		if realityPubKey == "" && n.PublicKey != "" {
			realityPubKey = n.PublicKey
		}
		if realityShortID == "" && n.ShortID != "" {
			realityShortID = n.ShortID
		}
		if realitySNI == "" && n.SNI != "" {
			realitySNI = n.SNI
		}
		if opts.OnlyMapped && !n.Mapped {
			continue
		}
		if len(countryFilter) > 0 && !countryFilter[strings.ToUpper(n.CountryCode)] {
			continue
		}
		exported = append(exported, n)
		if n.VLESSUrl != "" {
			uriLinks = append(uriLinks, n.VLESSUrl)
		}
	}

	bundle := &EgressSubscriptionBundle{
		InstanceID:       opts.InstanceID,
		HostPublicIP:     hostIP,
		PriorityOrder:    []string{"US", "JP", "SG", "KR", "TW", "HK", "PH"},
		TotalNodes:       len(nodes),
		MappedNodes:      mappedCount,
		ExportedNodes:    len(exported),
		RealityPublicKey: realityPubKey,
		RealityShortID:   realityShortID,
		RealitySNI:       realitySNI,
		RefreshedAt:      time.Now().Format(time.RFC3339),
		Nodes:            exported,
		URILinks:         uriLinks,
	}

	groupName := strings.TrimSpace(opts.GroupName)
	if groupName == "" {
		groupName = "LazyCat-家宽优选"
	}

	fmtLower := strings.ToLower(strings.TrimSpace(opts.Format))
	if fmtLower == "" {
		fmtLower = "json"
	}

	rawURIBlock := strings.Join(uriLinks, "\n")
	if fmtLower == "base64" || fmtLower == "sub" || fmtLower == "json" || fmtLower == "all" {
		bundle.Base64 = base64.StdEncoding.EncodeToString([]byte(rawURIBlock))
	}
	if fmtLower == "clash" || fmtLower == "mihomo" || fmtLower == "yaml" || fmtLower == "json" || fmtLower == "all" {
		bundle.ClashYAML = generateEgressClashYAML(exported, groupName)
	}
	if fmtLower == "sing-box" || fmtLower == "singbox" || fmtLower == "json" || fmtLower == "all" {
		bundle.SingBoxJSON = generateEgressSingBoxJSON(exported, groupName)
	}

	return bundle, nil
}

func generateEgressClashYAML(nodes []EgressNode, groupName string) string {
	var sb strings.Builder
	sb.WriteString("proxies:\n")

	var proxyNames []string
	for _, n := range nodes {
		if n.HostIP == "" || n.PublicPort <= 0 || n.UUID == "" {
			continue
		}
		displayName := strings.TrimSpace(fmt.Sprintf("%s %s", n.FlagEmoji, n.Name))
		proxyNames = append(proxyNames, displayName)

		sni := n.SNI
		if sni == "" {
			sni = "www.apple.com"
		}
		flow := n.Flow
		if flow == "" {
			flow = "xtls-rprx-vision"
		}

		sb.WriteString(fmt.Sprintf("  - name: %q\n", displayName))
		sb.WriteString("    type: vless\n")
		sb.WriteString(fmt.Sprintf("    server: %s\n", n.HostIP))
		sb.WriteString(fmt.Sprintf("    port: %d\n", n.PublicPort))
		sb.WriteString(fmt.Sprintf("    uuid: %s\n", n.UUID))
		sb.WriteString("    network: tcp\n")
		sb.WriteString("    tls: true\n")
		sb.WriteString("    udp: true\n")
		sb.WriteString(fmt.Sprintf("    flow: %s\n", flow))
		sb.WriteString(fmt.Sprintf("    servername: %s\n", sni))
		if n.PublicKey != "" {
			sb.WriteString("    reality-opts:\n")
			sb.WriteString(fmt.Sprintf("      public-key: %s\n", n.PublicKey))
			if n.ShortID != "" {
				sb.WriteString(fmt.Sprintf("      short-id: %s\n", n.ShortID))
			}
		}
		sb.WriteString("    client-fingerprint: chrome\n\n")
	}

	if len(proxyNames) > 0 {
		autoGroup := groupName + "-自动优选"
		sb.WriteString("proxy-groups:\n")
		sb.WriteString(fmt.Sprintf("  - name: %q\n", groupName))
		sb.WriteString("    type: select\n")
		sb.WriteString("    proxies:\n")
		sb.WriteString(fmt.Sprintf("      - %q\n", autoGroup))
		for _, pn := range proxyNames {
			sb.WriteString(fmt.Sprintf("      - %q\n", pn))
		}
		sb.WriteString(fmt.Sprintf("\n  - name: %q\n", autoGroup))
		sb.WriteString("    type: url-test\n")
		sb.WriteString("    url: http://www.gstatic.com/generate_204\n")
		sb.WriteString("    interval: 180\n")
		sb.WriteString("    tolerance: 50\n")
		sb.WriteString("    proxies:\n")
		for _, pn := range proxyNames {
			sb.WriteString(fmt.Sprintf("      - %q\n", pn))
		}
	}

	return sb.String()
}

func generateEgressSingBoxJSON(nodes []EgressNode, groupName string) string {
	outbounds := make([]map[string]interface{}, 0, len(nodes)+2)
	tags := make([]string, 0, len(nodes))

	for _, n := range nodes {
		if n.HostIP == "" || n.PublicPort <= 0 || n.UUID == "" {
			continue
		}
		tag := fmt.Sprintf("lazycat-%s-%d", strings.ToLower(n.CountryCode), n.ID)
		tags = append(tags, tag)

		sni := n.SNI
		if sni == "" {
			sni = "www.apple.com"
		}
		flow := n.Flow
		if flow == "" {
			flow = "xtls-rprx-vision"
		}

		realityCfg := map[string]interface{}{
			"enabled":    true,
			"public_key": n.PublicKey,
		}
		if n.ShortID != "" {
			realityCfg["short_id"] = n.ShortID
		}

		ob := map[string]interface{}{
			"type":        "vless",
			"tag":         tag,
			"server":      n.HostIP,
			"server_port": n.PublicPort,
			"uuid":        n.UUID,
			"flow":        flow,
			"tls": map[string]interface{}{
				"enabled":     true,
				"server_name": sni,
				"reality":     realityCfg,
				"utls": map[string]interface{}{
					"enabled":     true,
					"fingerprint": "chrome",
				},
			},
		}
		outbounds = append(outbounds, ob)
	}

	if len(tags) > 0 {
		autoTag := groupName + "-auto"
		outbounds = append(outbounds, map[string]interface{}{
			"type":      "urltest",
			"tag":       autoTag,
			"outbounds": tags,
			"url":       "http://www.gstatic.com/generate_204",
			"interval":  "3m",
			"tolerance": 50,
		})
		selectorMembers := append([]string{autoTag}, tags...)
		outbounds = append(outbounds, map[string]interface{}{
			"type":      "selector",
			"tag":       groupName,
			"outbounds": selectorMembers,
			"default":   tags[0],
		})
	}

	data, _ := json.MarshalIndent(map[string]interface{}{
		"outbounds": outbounds,
	}, "", "  ")
	return string(data)
}
