package scheduler

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/audit"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/notify"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/store"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
)

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "401") || strings.Contains(s, "token") || strings.Contains(s, "unauthorized") || strings.Contains(s, "token无效")
}

type StatusReport struct {
	Mode                  string `json:"mode"`                    // "idle", "steady", "burst"
	ActivePlansCount      int    `json:"active_plans_count"`
	CurrentIntervalSec    int    `json:"current_interval_sec"`
	SlowIntervalSec       int    `json:"slow_interval_sec"`
	BurstIntervalSec      int    `json:"burst_interval_sec"`
	BurstRemainingSeconds int    `json:"burst_remaining_seconds"`
	LastPollAt            string `json:"last_poll_at"`
	TotalMatched          int    `json:"total_matched"`
}

type Scheduler struct {
	mu            sync.RWMutex
	mode          string
	slowInterval  time.Duration
	burstInterval time.Duration
	burstUntil    time.Time
	lastPollAt    time.Time
	stopCh        chan struct{}
	matchedAlerts map[string]time.Time
}

var GlobalScheduler *Scheduler
var once sync.Once

func Init() *Scheduler {
	once.Do(func() {
		GlobalScheduler = &Scheduler{
			mode:          "idle",
			slowInterval:  60 * time.Second, // 严格 1 分钟轮询一次抢购市场
			burstInterval: 15 * time.Second, // 突发抢购模式加速至 15 秒
			matchedAlerts: make(map[string]time.Time),
			stopCh:        make(chan struct{}),
		}
		go GlobalScheduler.startLoop()
		go StartTokenLifecycleRefresher(context.Background())
		go store.StartDailyCacheCron(context.Background())
		go StartCheckinScheduler(context.Background())
	})
	return GlobalScheduler
}

func (s *Scheduler) startLoop() {
	log.Printf("[Scheduler] Intelligent Market Sniper Scheduler initialized")

	for {
		select {
		case <-s.stopCh:
			log.Printf("[Scheduler] Scheduler loop stopped")
			return
		default:
			interval := s.tick()
			time.Sleep(interval)
		}
	}
}

func (s *Scheduler) tick() time.Duration {
	s.mu.Lock()
	now := time.Now()
	hasPlans := store.HasActivePlans()

	if !hasPlans {
		s.mode = "idle"
		s.lastPollAt = now
		s.mu.Unlock()
		return 10 * time.Second // 休眠状态，每 10 秒轻量检查一次是否有新开启的计划
	}

	// 存在活跃购买计划，进入巡检
	if now.Before(s.burstUntil) {
		s.mode = "burst"
	} else {
		s.mode = "steady"
	}
	s.lastPollAt = now
	currMode := s.mode
	s.mu.Unlock()

	// 执行一次全量市场扫描与规则比对
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	s.pollAndMatch(ctx)

	if currMode == "burst" {
		return s.burstInterval
	}
	return s.slowInterval
}

func (s *Scheduler) pollAndMatch(ctx context.Context) {
	all := providers.GetAll()
	var rawItems []store.UpsertItemInput
	var fullItems []providers.MarketItem
	var syncedProviders []string

	for _, p := range all {
		if ml, ok := p.(providers.MarketLister); ok {
			items, err := ml.ListMarketItems(ctx)
			if err != nil && isAuthError(err) {
				log.Printf("[Scheduler] provider %s 集市巡检遇到鉴权失效: %v，触发免密重登...", p.ID(), err)
				if store.TryAutoRecoverProvider(ctx, p.ID()) {
					items, err = ml.ListMarketItems(ctx)
				}
			}
			if err == nil {
				syncedProviders = append(syncedProviders, p.ID())
				fullItems = append(fullItems, items...)
				for _, it := range items {
					rawItems = append(rawItems, store.UpsertItemInput{
						ID:              it.ID,
						Provider:        it.ProviderID,
						Name:            it.Name,
						Price:           it.Price,
						RenewalPrice:    it.RenewalPrice,
						DiscountPercent: it.DiscountPercent,
						ResetPrice:      it.ResetPrice,
						PlatformFee:     it.PlatformFee,
						FlowUsed:        it.FlowUsed,
						Cycle:           it.Cycle,
						CycleName:       it.CycleName,
						MonthlyPrice:    it.MonthlyPrice,
						Location:        it.Location,
						Bandwidth:       it.Bandwidth,
						ConfigDesc:      it.ConfigDesc,
						CPU:             it.CPU,
						Memory:          it.Memory,
						Disk:            it.Disk,
						TrafficTotal:    it.TrafficTotal,
						Detail:          it.Detail,
						IPCheckDetail:   it.IPCheckDetail,
						DueTime:         it.DueTime,
						SLA:             it.SLA,
						CanBuy:          it.CanBuy,
						PortLimit:       it.PortLimit,
						SellerUsername:  it.SellerUsername,
						SellerUserID:    it.SellerUserID,
						MarketType:      it.MarketType,
						BargainTag:      it.BargainTag,
						DiffPercent:     it.DiffPercent,
					})
				}
			}
		}
	}

	if len(syncedProviders) > 0 {
		store.UpsertMarketBatch(rawItems, syncedProviders)
	}

	activePlans := store.GetActivePlans()
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, plan := range activePlans {
		var matchedPreviews []store.MatchedItemPreview
		for _, item := range fullItems {
			if matchPlan(plan, item) {
				matchedPreviews = append(matchedPreviews, store.MatchedItemPreview{
					ID:              item.ID,
					Provider:        item.ProviderID,
					Title:           item.Name,
					Price:           item.Price,
					RenewalPrice:    item.RenewalPrice,
					DiscountPercent: item.DiscountPercent,
					SellerUsername:  item.SellerUsername,
					SellerUserID:    item.SellerUserID,
					Location:        item.Location,
					NodeName:        item.NodeName,
					Bandwidth:       item.Bandwidth,
					PortLimit:       item.PortLimit,
					ConfigDesc:      item.ConfigDesc,
					Detail:          item.Detail,
					IPCheckDetail:   item.IPCheckDetail,
					DueTime:         item.DueTime,
					SLA:             item.SLA,
					CanBuy:          item.CanBuy,
					MatchedAt:       time.Now().Format("2006-01-02 15:04:05"),
				})

				alertKey := plan.ID + ":" + item.ProviderID + ":" + item.ID
				if lastTime, seen := s.matchedAlerts[alertKey]; !seen || time.Since(lastTime) >= 15*time.Minute {
					s.matchedAlerts[alertKey] = time.Now()
					store.RecordPlanMatch(plan.ID, item.Name)

					// 记录安全审计日志
					audit.Record("sniper", "plan.match", item.ProviderID, map[string]interface{}{
						"plan_id":   plan.ID,
						"plan_name": plan.Name,
						"item_id":   item.ID,
						"item_name": item.Name,
						"price":     item.Price,
						"seller":    item.SellerUsername,
						"safe_mode": plan.SafeMode,
						"auto_buy":  plan.AutoBuy,
					}, "SUCCESS", fmt.Sprintf("抢购计划命中: %s (¥%.2f - %s)", plan.Name, item.Price, item.Name), 0)

					// 触发异步推送
					go func(p store.PurchasePlan, it providers.MarketItem) {
						sellerStr := ""
						if it.SellerUsername != "" {
							sellerStr = fmt.Sprintf("\n• 挂牌卖家: %s (ID: %d)", it.SellerUsername, it.SellerUserID)
						}
						renewStr := ""
						if it.RenewalPrice > 0 {
							cycleLabel := it.CycleName
							if cycleLabel == "" {
								if it.Cycle >= 12 {
									cycleLabel = "年付"
								} else {
									cycleLabel = "月付"
								}
							}
							if it.Cycle > 1 && it.MonthlyPrice > 0 {
								renewStr = fmt.Sprintf("\n• 续费价格: *¥%.2f/%s* (折合 ¥%.2f/月)", it.RenewalPrice, cycleLabel, it.MonthlyPrice)
							} else {
								renewStr = fmt.Sprintf("\n• 续费价格: *¥%.2f/月*", it.RenewalPrice)
							}
						}
						modeStr := "🛡️ 【模拟筛选模式】仅供筛选查看，未调用扣费"
						if !p.SafeMode && p.AutoBuy {
							modeStr = fmt.Sprintf("⚡ 【实盘秒杀模式】已自动尝试下单 (进度: %d/%d 台)", p.CurrentBoughtCount, p.MaxBuyCount)
						}

						msg := fmt.Sprintf(
							"🎯 *【VPS 抢购规则命中提醒】*\n\n"+
								"• 命中规则: *%s*\n"+
								"• 厂商平台: `%s`\n"+
								"• 套餐名称: *%s*\n"+
								"• 标价金额: *¥%.2f*%s%s\n"+
								"• 机房区域: %s\n"+
								"• 网络带宽: %s\n"+
								"• 硬件配置: %s\n\n"+
								"%s",
							p.Name, strings.ToUpper(it.ProviderID), it.Name, it.Price,
							sellerStr, renewStr, it.Location, it.Bandwidth, it.ConfigDesc, modeStr,
						)
						_ = notify.SendTelegram(msg)
					}(*plan, item)
				}
			}
		}

		// 持续更新该计划在界面上的模拟候选匹配列表
		store.SetPlanMatchedItems(plan.ID, matchedPreviews)

		// 阶段二：实盘自动下单秒杀
		if !plan.SafeMode && plan.AutoBuy && plan.CurrentBoughtCount < plan.MaxBuyCount {
			for _, item := range fullItems {
				if matchPlan(plan, item) && item.CanBuy {
					buyKey := "bought:" + plan.ID + ":" + item.ProviderID + ":" + item.ID
					if _, alreadyBought := s.matchedAlerts[buyKey]; alreadyBought {
						continue
					}
					s.matchedAlerts[buyKey] = time.Now()

					log.Printf("[Sniper] 触发实盘秒杀: 计划 %s -> %s (¥%.2f, 卖家: %s)...", plan.Name, item.Name, item.Price, item.SellerUsername)
					err := executeAutoBuy(ctx, plan, item)
					if err == nil {
						reached := store.IncrementPlanBought(plan.ID)
						log.Printf("[Sniper] 实盘秒杀成功! 已购 %d/%d 台", plan.CurrentBoughtCount+1, plan.MaxBuyCount)
						audit.Record("sniper", "autobuy.success", item.ProviderID, map[string]interface{}{
							"plan_id": plan.ID,
							"item_id": item.ID,
							"price":   item.Price,
						}, "SUCCESS", fmt.Sprintf("实盘自动下单成功: %s", item.Name), 0)

						succMsg := fmt.Sprintf("🎉 *【实盘自动秒杀成功】*\n\n• 规则: *%s*\n• 厂商: `%s`\n• 机器: *%s*\n• 金额: *¥%.2f*\n• 卖家: %s\n• 进度: 已购 %d/%d 台",
							plan.Name, strings.ToUpper(item.ProviderID), item.Name, item.Price, item.SellerUsername, plan.CurrentBoughtCount+1, plan.MaxBuyCount)
						if reached {
							succMsg += "\n\n✅ 已达到该计划最大购买台数上限，任务已自动切回模拟筛选保护模式。"
						}
						_ = notify.SendTelegram(succMsg)

						if reached {
							break
						}
					} else {
						log.Printf("[Sniper] 实盘秒杀下单失败: %v", err)
						audit.Record("sniper", "autobuy.fail", item.ProviderID, map[string]interface{}{
							"plan_id": plan.ID,
							"item_id": item.ID,
							"error":   err.Error(),
						}, "FAIL", fmt.Sprintf("自动下单失败: %v", err), 0)
					}
				}
			}
		}
	}
}

func executeAutoBuy(ctx context.Context, plan *store.PurchasePlan, item providers.MarketItem) error {
	p, ok := providers.Get(item.ProviderID)
	if !ok {
		return fmt.Errorf("provider not found: %s", item.ProviderID)
	}
	buyer, ok := p.(providers.MarketBuyer)
	if !ok {
		return fmt.Errorf("provider %s does not support buying", item.ProviderID)
	}
	return buyer.BuyMarketItem(ctx, item.ID)
}

func matchPlan(p *store.PurchasePlan, it providers.MarketItem) bool {
	// 厂商匹配
	if p.Provider != "all" && p.Provider != "" && !strings.EqualFold(p.Provider, it.ProviderID) {
		return false
	}

	// 市场类型匹配 (一手 / 二手)
	if p.MarketType != "all" && p.MarketType != "" && it.MarketType != "" && !strings.EqualFold(p.MarketType, it.MarketType) {
		return false
	}

	// 价格上限
	if p.MaxPrice > 0 && it.Price > p.MaxPrice {
		return false
	}

	// 计费周期过滤 ("all", "yearly", "monthly", "other")
	if p.CycleFilter != "" && p.CycleFilter != "all" {
		switch strings.ToLower(p.CycleFilter) {
		case "yearly", "year", "年付":
			if it.Cycle < 12 {
				return false
			}
		case "monthly", "month", "月付":
			if it.Cycle > 1 {
				return false
			}
		case "other", "quarterly", "季付/半年付":
			if it.Cycle <= 1 || it.Cycle >= 12 {
				return false
			}
		}
	}

	// 续费价上限 (统一按折算后的月付基准比较)
	if p.MaxRenewalPrice > 0 {
		effMonthlyRenew := it.MonthlyPrice
		if effMonthlyRenew <= 0 {
			if it.Cycle > 1 {
				effMonthlyRenew = it.RenewalPrice / float64(it.Cycle)
			} else {
				effMonthlyRenew = it.RenewalPrice
			}
		}
		if effMonthlyRenew > 0 && effMonthlyRenew > p.MaxRenewalPrice {
			return false
		}
	}

	// 最低折扣 (如 30 代表 3折)
	if p.MinDiscountPercent > 0 && (it.DiscountPercent <= 0 || it.DiscountPercent > p.MinDiscountPercent) {
		return false
	}

	// 最少端口配额 (针对懒猫云等 NAT 主机)
	if p.MinPortCount > 0 && it.PortLimit > 0 && it.PortLimit < p.MinPortCount {
		return false
	}

	// 区域关键字
	if p.RegionKeyword != "" {
		combined := strings.ToLower(it.Location + " " + it.NodeName + " " + it.Name)
		kw := strings.ToLower(p.RegionKeyword)
		if !strings.Contains(combined, kw) {
			return false
		}
	}

	// 自定义关键词过滤 (匹配标题、节点、硬件配置、IP详情、线路特性)
	if p.KeywordFilter != "" {
		combined := strings.ToLower(it.Name + " " + it.NodeName + " " + it.ConfigDesc + " " + it.Detail + " " + it.IPCheckDetail)
		kw := strings.ToLower(p.KeywordFilter)
		if !strings.Contains(combined, kw) {
			return false
		}
	}

	// 最小带宽
	if p.MinBandwidth > 0 && it.Bandwidth != "" {
		bwNum := extractBandwidth(it.Bandwidth)
		if bwNum > 0 && bwNum < p.MinBandwidth {
			return false
		}
	}

	return true
}

func extractBandwidth(bwStr string) int {
	clean := strings.ToLower(bwStr)
	clean = strings.ReplaceAll(clean, "mbps", "")
	clean = strings.ReplaceAll(clean, "m", "")
	clean = strings.TrimSpace(clean)
	n, _ := strconv.Atoi(clean)
	return n
}

// TriggerManualBurst 用户手动在界面点击一键加速 5 分钟
func (s *Scheduler) TriggerManualBurst(duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if duration <= 0 {
		duration = 5 * time.Minute
	}
	s.burstUntil = time.Now().Add(duration)
	s.mode = "burst"
}

func (s *Scheduler) GetStatus() StatusReport {
	s.mu.RLock()
	defer s.mu.RUnlock()

	activePlans := store.GetActivePlans()
	now := time.Now()

	burstRem := 0
	if now.Before(s.burstUntil) {
		burstRem = int(s.burstUntil.Sub(now).Seconds())
	}

	currInt := int(s.slowInterval.Seconds())
	if s.mode == "burst" {
		currInt = int(s.burstInterval.Seconds())
	}

	lastStr := ""
	if !s.lastPollAt.IsZero() {
		lastStr = s.lastPollAt.Format(time.RFC3339)
	}

	totalMatched := 0
	for _, p := range activePlans {
		totalMatched += p.MatchCount
	}

	return StatusReport{
		Mode:                  s.mode,
		ActivePlansCount:      len(activePlans),
		CurrentIntervalSec:    currInt,
		SlowIntervalSec:       int(s.slowInterval.Seconds()),
		BurstIntervalSec:      int(s.burstInterval.Seconds()),
		BurstRemainingSeconds: burstRem,
		LastPollAt:            lastStr,
		TotalMatched:          totalMatched,
	}
}
