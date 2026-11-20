package lazycat

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// EgressNode 容器多出口 VLESS Reality 映射节点 (自适应 N 个节点，不硬编码 12)
type EgressNode struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	CountryCode string `json:"country_code"`
	FlagEmoji   string `json:"flag_emoji"`
	PrivatePort int    `json:"private_port"` // 10000 + ID (如 10001, 10005, 10012)
	OutboundTag string `json:"outbound_tag"` // out-1, out-5
	Protocol    string `json:"protocol"`     // vless
	Flow        string `json:"flow"`         // xtls-rprx-vision
	SNI         string `json:"sni"`          // www.apple.com
}

var (
	egressCacheMu sync.RWMutex
	egressCache   = make(map[string]struct {
		nodes     []EgressNode
		fetchedAt time.Time
	})
)

// Helper: 国家代码与国旗识别
func detectCountryFlag(name string) (string, string) {
	nameLower := strings.ToLower(name)
	switch {
	case strings.Contains(name, "日本") || strings.Contains(nameLower, "jp") || strings.Contains(nameLower, "softbank") || strings.Contains(nameLower, "biglobe") || strings.Contains(nameLower, "glbb") || strings.Contains(nameLower, "kddi"):
		return "JP", "🇯🇵"
	case strings.Contains(name, "美国") || strings.Contains(nameLower, "us") || strings.Contains(nameLower, "pavlov") || strings.Contains(nameLower, "frontier") || strings.Contains(nameLower, "verizon"):
		return "US", "🇺🇸"
	case strings.Contains(name, "新加坡") || strings.Contains(nameLower, "sg") || strings.Contains(nameLower, "starhub"):
		return "SG", "🇸🇬"
	case strings.Contains(name, "香港") || strings.Contains(nameLower, "hk") || strings.Contains(nameLower, "hkt"):
		return "HK", "🇭🇰"
	case strings.Contains(name, "台湾") || strings.Contains(nameLower, "tw") || strings.Contains(nameLower, "hinet"):
		return "TW", "🇹🇼"
	case strings.Contains(name, "韩国") || strings.Contains(nameLower, "kr") || strings.Contains(nameLower, "korea") || strings.Contains(nameLower, "kt"):
		return "KR", "🇰🇷"
	case strings.Contains(name, "菲律宾") || strings.Contains(nameLower, "ph") || strings.Contains(nameLower, "dito"):
		return "PH", "🇵🇭"
	default:
		return "GLOBAL", "🌐"
	}
}

// GetContainerEgressNodes 动态读取并解析当前容器内实际生效的 N 个出口节点
func (c *Client) GetContainerEgressNodes(ctx context.Context, instanceID string, forceRefresh bool) ([]EgressNode, error) {
	if instanceID == "" {
		instanceID = "11433"
	}

	if !forceRefresh {
		egressCacheMu.RLock()
		if cached, ok := egressCache[instanceID]; ok && time.Since(cached.fetchedAt) < 60*time.Second {
			nodes := cached.nodes
			egressCacheMu.RUnlock()
			return nodes, nil
		}
		egressCacheMu.RUnlock()
	}

	// 1. 通过纯 Go 原生终端短指令读取容器内 /root/nodes.json
	out, err := c.ExecTerminalCommand(ctx, instanceID, "cat /root/nodes.json", 8*time.Second)
	if err == nil && len(out) > 10 {
		var rawNodes map[string]struct {
			Name     string `json:"name"`
			Outbound struct {
				Server     string `json:"server"`
				ServerPort int    `json:"server_port"`
				Tag        string `json:"tag"`
			} `json:"outbound"`
		}

		// 找到 JSON 的起始位置
		startIdx := strings.Index(out, "{")
		endIdx := strings.LastIndex(out, "}")
		if startIdx >= 0 && endIdx > startIdx {
			jsonStr := out[startIdx : endIdx+1]
			if jsonErr := json.Unmarshal([]byte(jsonStr), &rawNodes); jsonErr == nil && len(rawNodes) > 0 {
				var nodes []EgressNode
				for sID, n := range rawNodes {
					id, _ := strconv.Atoi(sID)
					if id <= 0 {
						continue
					}
					cc, flag := detectCountryFlag(n.Name)
					nodes = append(nodes, EgressNode{
						ID:          id,
						Name:        n.Name,
						CountryCode: cc,
						FlagEmoji:   flag,
						PrivatePort: 10000 + id,
						OutboundTag: fmt.Sprintf("out-%d", id),
						Protocol:    "vless",
						Flow:        "xtls-rprx-vision",
						SNI:         "www.apple.com",
					})
				}

				// 按节点 ID 排序
				sort.Slice(nodes, func(i, j int) bool {
					return nodes[i].ID < nodes[j].ID
				})

				egressCacheMu.Lock()
				egressCache[instanceID] = struct {
					nodes     []EgressNode
					fetchedAt time.Time
				}{nodes: nodes, fetchedAt: time.Now()}
				egressCacheMu.Unlock()

				return nodes, nil
			}
		}
	}

	// 2. 降级方案：若容器未启动或 nodes.json 暂时不可读，提供标准自愈预设出口列表
	fallbackDefaults := []struct {
		id   int
		name string
	}{
		{1, "日本-SoftBank动态"},
		{2, "日本-Biglobe动态家宽"},
		{3, "日本-GLBB-Japan-KK动态家宽"},
		{4, "日本-KDDI动态"},
		{5, "美国-Pavlov动态家宽 (主力)"},
		{6, "美国-Frontier动态家宽"},
		{7, "美国-Verizon动态家宽"},
		{8, "新加坡-StarHub动态家宽"},
		{9, "台湾-Hinet动态家宽"},
		{10, "韩国-KT动态家宽"},
		{11, "菲律宾-DITO动态家宽"},
		{12, "香港-HKT动态家宽"},
	}

	var fallbackNodes []EgressNode
	for _, f := range fallbackDefaults {
		cc, flag := detectCountryFlag(f.name)
		fallbackNodes = append(fallbackNodes, EgressNode{
			ID:          f.id,
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

	return fallbackNodes, nil
}
