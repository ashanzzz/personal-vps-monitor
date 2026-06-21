package main

import (
	"math"
	"strings"
)

// ComputeScore 根据可配置规则对机器进行抢购评分
func ComputeScore(spec map[string]interface{}, minScore int) int {
	baseScore := 50

	// 1. 低延迟与网络线路权重判定
	latency, ok := spec["latency"].(float64)
	if ok && latency > 0 {
		if latency < 30 {
			baseScore += 25
		} else if latency < 80 {
			baseScore += 15
		} else if latency > 200 {
			baseScore -= 20
		}
	}

	// 2. 特定优质线路标签加分 (jpIIJ, jppro, hkcmi 等)
	labels, ok := spec["labels"].(string)
	if ok {
		labelsLower := strings.ToLower(labels)
		if strings.Contains(labelsLower, "iij") || strings.Contains(labelsLower, "pro") {
			baseScore += 15
		}
		if strings.Contains(labelsLower, "cn2") || strings.Contains(labelsLower, "9929") {
			baseScore += 20
		}
	}

	// 3. 硬件性价比测算 (CPU/内存/硬盘/带宽拓扑计分)
	price, _ := spec["price"].(float64)
	band, _ := spec["bandwidth"].(float64)
	if price > 0 && band > 0 {
		valueRatio := band / price
		baseScore += int(math.Min(15, valueRatio*5))
	}

	// 4. 被墙(GFW)状态修正边界拦截
	gfwBlocked, _ := spec["gfw_blocked"].(bool)
	if gfwBlocked {
		baseScore -= 40
	}

	// 边界锁死
	if baseScore > 100 { baseScore = 100 }
	if baseScore < 0 { baseScore = 0 }
	return baseScore
}

// ComputeStarRating 将百分制评分压缩映射为 1-5 星级
func ComputeStarRating(score int) int {
	if score >= 90 { return 5 }
	if score >= 75 { return 4 }
	if score >= 60 { return 3 }
	if score >= 40 { return 2 }
	return 1
}
