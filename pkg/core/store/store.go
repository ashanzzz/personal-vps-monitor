package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type MatchedItemPreview struct {
	ID              string  `json:"id"`
	Provider        string  `json:"provider"`
	Title           string  `json:"title"`
	Price           float64 `json:"price"`
	RenewalPrice    float64 `json:"renewal_price,omitempty"`
	DiscountPercent float64 `json:"discount_percent,omitempty"`
	SellerUsername  string  `json:"seller_username,omitempty"`
	SellerUserID    int     `json:"seller_user_id,omitempty"`
	Location        string  `json:"location,omitempty"`
	NodeName        string  `json:"node_name,omitempty"`
	Bandwidth       string  `json:"bandwidth,omitempty"`
	PortLimit       int     `json:"port_limit,omitempty"`
	ConfigDesc      string  `json:"config_desc,omitempty"`
	Detail          string  `json:"detail,omitempty"`
	IPCheckDetail   string  `json:"ip_check_detail,omitempty"`
	DueTime         string  `json:"due_time,omitempty"`
	SLA             string  `json:"sla,omitempty"`
	CanBuy          bool    `json:"can_buy"`
	MatchedAt       string  `json:"matched_at"`
}

type PurchasePlan struct {
	ID                 string               `json:"id"`
	Name               string               `json:"name"`
	Provider           string               `json:"provider"`             // "akile", "acck", "lazycat", "all"
	MarketType         string               `json:"market_type"`          // "secondhand", "new_stock", "all"
	RegionKeyword      string               `json:"region_keyword"`       // 如 "HK", "日本", "tw", "us"
	MaxPrice           float64              `json:"max_price"`            // 售价最高限价
	MaxRenewalPrice    float64              `json:"max_renewal_price"`    // 续费价最高限价 (0 表示不限)
	CycleFilter        string               `json:"cycle_filter,omitempty"` // 周期过滤 ("all", "monthly", "yearly", "other")
	MinDiscountPercent float64              `json:"min_discount_percent"` // 最低折扣 (如 30 代表 3折)
	MinPortCount       int                  `json:"min_port_count"`       // 最少端口数 (针对懒猫云)
	MinBandwidth       int                  `json:"min_bandwidth"`        // 最低带宽 Mbps
	KeywordFilter      string               `json:"keyword_filter"`       // 标题或备注包含的关键词
	SafeMode           bool                 `json:"safe_mode"`            // 默认 true: 模拟模式 (仅供筛选查看，不扣费)
	AutoBuy            bool                 `json:"auto_buy"`             // 是否开启真实下单秒杀
	MaxBuyCount        int                  `json:"max_buy_count"`        // 连续抢购最大购买台数 (默认 1)
	CurrentBoughtCount int                  `json:"current_bought_count"` // 已成功秒杀台数
	IsActive           bool                 `json:"is_active"`            // 规则启用开关
	CreatedAt          string               `json:"created_at"`
	LastTriggeredAt    string               `json:"last_triggered_at,omitempty"`
	MatchCount         int                  `json:"match_count"`
	LastMatchItem      string               `json:"last_match_item,omitempty"`
	MatchedItems       []MatchedItemPreview `json:"matched_items,omitempty"` // 模拟模式下实时保留的最新命中列表
}

type MarketRecord struct {
	ID              string  `json:"id"`
	Provider        string  `json:"provider"`
	Name            string  `json:"name"`
	Price           float64 `json:"price"`
	RenewalPrice    float64 `json:"renewal_price,omitempty"`
	DiscountPercent float64 `json:"discount_percent,omitempty"`
	ResetPrice      float64 `json:"reset_price,omitempty"`
	PlatformFee     float64 `json:"platform_fee,omitempty"`
	FlowUsed        string  `json:"flow_used,omitempty"`
	Cycle           int     `json:"cycle,omitempty"`
	CycleName       string  `json:"cycle_name,omitempty"`
	MonthlyPrice    float64 `json:"monthly_price,omitempty"`
	Location        string  `json:"location"`
	Bandwidth       string  `json:"bandwidth"`
	ConfigDesc      string  `json:"config_desc"`
	CPU             string  `json:"cpu,omitempty"`
	Memory          string  `json:"memory,omitempty"`
	Disk            string  `json:"disk,omitempty"`
	TrafficTotal    string  `json:"traffic_total,omitempty"`
	Detail          string  `json:"detail,omitempty"`
	IPCheckDetail   string  `json:"ip_check_detail,omitempty"`
	DueTime         string  `json:"due_time,omitempty"`
	SLA             string  `json:"sla,omitempty"`
	CanBuy          bool    `json:"can_buy"`
	PortLimit       int     `json:"port_limit,omitempty"`
	SellerUsername  string  `json:"seller_username,omitempty"`
	SellerUserID    int     `json:"seller_user_id,omitempty"`
	MarketType      string  `json:"market_type,omitempty"`
	BargainTag      string  `json:"bargain_tag,omitempty"`
	DiffPercent     float64 `json:"diff_percent,omitempty"`
	FirstSeenAt     string  `json:"first_seen_at"`
	LastSeenAt      string  `json:"last_seen_at"`
	IsActive        bool    `json:"is_active"`
	DurationSec     int     `json:"duration_sec"` // 在市场停留秒数 (被抢速度)
}

type MarketStats struct {
	TotalTracked     int            `json:"total_tracked"`
	ActiveItems      int            `json:"active_items"`
	ByProviderActive map[string]int `json:"by_provider_active"`
	AvgPrice         float64        `json:"avg_price"`
	AvgDuration      int            `json:"avg_duration_sec"`
	ByLocation       map[string]int `json:"by_location"`
	HotModels        []string       `json:"hot_models"`
}

var (
	mu            sync.RWMutex
	plans         = make(map[string]*PurchasePlan)
	marketHistory = make(map[string]*MarketRecord)
)

func getDataDir() string {
	if runtime.GOOS == "linux" {
		for _, p := range []string{"/app/data", "/mnt/user/appdata/personal-vps-monitor"} {
			if fi, err := os.Stat(p); err == nil && fi.IsDir() {
				return p
			}
		}
	}
	_ = os.MkdirAll("./data", 0755)
	return "./data"
}

func getPlansFile() string {
	return filepath.Join(getDataDir(), "plans.json")
}

func getMarketFile() string {
	return filepath.Join(getDataDir(), "market_history.json")
}

func init() {
	Load()
}

// Load 从磁盘载入计划与历史缓存
func Load() {
	mu.Lock()
	defer mu.Unlock()

	// 载入购买计划
	if b, err := os.ReadFile(getPlansFile()); err == nil && len(b) > 0 {
		var list []*PurchasePlan
		if err := json.Unmarshal(b, &list); err == nil {
			plans = make(map[string]*PurchasePlan, len(list))
			for _, p := range list {
				plans[p.ID] = p
			}
		}
	}

	// 载入市场历史
	if b, err := os.ReadFile(getMarketFile()); err == nil && len(b) > 0 {
		var list []*MarketRecord
		if err := json.Unmarshal(b, &list); err == nil {
			marketHistory = make(map[string]*MarketRecord, len(list))
			for _, m := range list {
				marketHistory[m.Provider+":"+m.ID] = m
			}
		}
	}
}

// savePlansLocked 内部保存 plans
func savePlansLocked() error {
	list := make([]*PurchasePlan, 0, len(plans))
	for _, p := range plans {
		list = append(list, p)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := getPlansFile() + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, getPlansFile())
}

// saveMarketLocked 内部保存 market
func saveMarketLocked() error {
	list := make([]*MarketRecord, 0, len(marketHistory))
	for _, m := range marketHistory {
		list = append(list, m)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := getMarketFile() + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, getMarketFile())
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ==================== 购买计划 CRUD ====================

func ListPlans() []*PurchasePlan {
	mu.RLock()
	defer mu.RUnlock()
	res := make([]*PurchasePlan, 0, len(plans))
	for _, p := range plans {
		cp := *p
		res = append(res, &cp)
	}
	return res
}

func GetActivePlans() []*PurchasePlan {
	mu.RLock()
	defer mu.RUnlock()
	res := make([]*PurchasePlan, 0)
	for _, p := range plans {
		if p.IsActive {
			cp := *p
			res = append(res, &cp)
		}
	}
	return res
}

func HasActivePlans() bool {
	mu.RLock()
	defer mu.RUnlock()
	for _, p := range plans {
		if p.IsActive {
			return true
		}
	}
	return false
}

func CreatePlan(p *PurchasePlan) (*PurchasePlan, error) {
	mu.Lock()
	defer mu.Unlock()

	if p.Name == "" {
		p.Name = fmt.Sprintf("监控计划-%s", time.Now().Format("0102-1504"))
	}
	if p.ID == "" {
		p.ID = "plan-" + newID()
	}
	p.CreatedAt = time.Now().Format(time.RFC3339)
	p.SafeMode = true // 默认保持安全模式

	plans[p.ID] = p
	if err := savePlansLocked(); err != nil {
		return nil, err
	}
	return p, nil
}

func UpdatePlanMap(id string, update map[string]interface{}) (*PurchasePlan, error) {
	mu.Lock()
	defer mu.Unlock()

	existing, ok := plans[id]
	if !ok {
		return nil, fmt.Errorf("plan not found: %s", id)
	}

	if v, exists := update["name"]; exists {
		if s, ok := v.(string); ok && s != "" {
			existing.Name = s
		}
	}
	if v, exists := update["provider"]; exists {
		if s, ok := v.(string); ok && s != "" {
			existing.Provider = s
		}
	}
	if v, exists := update["market_type"]; exists {
		if s, ok := v.(string); ok && s != "" {
			existing.MarketType = s
		}
	}
	if v, exists := update["region_keyword"]; exists {
		if s, ok := v.(string); ok {
			existing.RegionKeyword = s
		}
	}
	if v, exists := update["keyword_filter"]; exists {
		if s, ok := v.(string); ok {
			existing.KeywordFilter = s
		}
	}
	if v, exists := update["max_price"]; exists {
		if num, ok := toFloat(v); ok {
			existing.MaxPrice = num
		}
	}
	if v, exists := update["max_renewal_price"]; exists {
		if num, ok := toFloat(v); ok {
			existing.MaxRenewalPrice = num
		}
	}
	if v, exists := update["cycle_filter"]; exists {
		if s, ok := v.(string); ok {
			existing.CycleFilter = s
		}
	}
	if v, exists := update["min_discount_percent"]; exists {
		if num, ok := toFloat(v); ok {
			existing.MinDiscountPercent = num
		}
	}
	if v, exists := update["min_port_count"]; exists {
		if num, ok := toInt(v); ok {
			existing.MinPortCount = num
		}
	}
	if v, exists := update["min_bandwidth"]; exists {
		if num, ok := toInt(v); ok {
			existing.MinBandwidth = num
		}
	}
	if v, exists := update["max_buy_count"]; exists {
		if num, ok := toInt(v); ok && num > 0 {
			existing.MaxBuyCount = num
		}
	}
	if v, exists := update["safe_mode"]; exists {
		if b, ok := v.(bool); ok {
			existing.SafeMode = b
		}
	}
	if v, exists := update["auto_buy"]; exists {
		if b, ok := v.(bool); ok {
			existing.AutoBuy = b
		}
	}
	if v, exists := update["is_active"]; exists {
		if b, ok := v.(bool); ok {
			existing.IsActive = b
		}
	}

	if err := savePlansLocked(); err != nil {
		return nil, err
	}
	return existing, nil
}

func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func toInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	default:
		return 0, false
	}
}

func UpdatePlan(id string, update *PurchasePlan) (*PurchasePlan, error) {
	mu.Lock()
	defer mu.Unlock()

	existing, ok := plans[id]
	if !ok {
		return nil, fmt.Errorf("plan not found: %s", id)
	}

	if update.Name != "" {
		existing.Name = update.Name
	}
	if update.Provider != "" {
		existing.Provider = update.Provider
	}
	if update.MarketType != "" {
		existing.MarketType = update.MarketType
	}
	if update.RegionKeyword != "" {
		existing.RegionKeyword = update.RegionKeyword
	}
	if update.MaxPrice > 0 {
		existing.MaxPrice = update.MaxPrice
	}
	if update.MaxRenewalPrice >= 0 {
		existing.MaxRenewalPrice = update.MaxRenewalPrice
	}
	if update.CycleFilter != "" {
		existing.CycleFilter = update.CycleFilter
	}
	existing.MinDiscountPercent = update.MinDiscountPercent
	existing.MinPortCount = update.MinPortCount
	existing.KeywordFilter = update.KeywordFilter
	if update.MinBandwidth >= 0 {
		existing.MinBandwidth = update.MinBandwidth
	}
	existing.SafeMode = update.SafeMode
	existing.AutoBuy = update.AutoBuy
	if update.MaxBuyCount > 0 {
		existing.MaxBuyCount = update.MaxBuyCount
	}
	existing.IsActive = update.IsActive

	if err := savePlansLocked(); err != nil {
		return nil, err
	}
	return existing, nil
}

// SetPlanMatchedItems 更新计划当前的模拟候选命中列表 (仅供筛选查看)
func SetPlanMatchedItems(planID string, items []MatchedItemPreview) {
	mu.Lock()
	defer mu.Unlock()

	if p, ok := plans[planID]; ok {
		p.MatchedItems = items
		if len(items) > 0 {
			p.LastMatchItem = items[0].Title
			p.LastTriggeredAt = time.Now().Format("2006-01-02 15:04:05")
		}
		_ = savePlansLocked()
	}
}

// IncrementPlanBought 成功秒杀下单后累加购买计数，并检查是否达到上限
func IncrementPlanBought(planID string) (reachedLimit bool) {
	mu.Lock()
	defer mu.Unlock()

	if p, ok := plans[planID]; ok {
		p.CurrentBoughtCount++
		p.MatchCount++
		p.LastTriggeredAt = time.Now().Format("2006-01-02 15:04:05")
		if p.MaxBuyCount <= 0 {
			p.MaxBuyCount = 1
		}
		if p.CurrentBoughtCount >= p.MaxBuyCount {
			// 达到购买上限，自动切回模拟筛选模式并关闭 auto_buy
			p.AutoBuy = false
			p.SafeMode = true
			reachedLimit = true
		}
		_ = savePlansLocked()
		return reachedLimit
	}
	return false
}

func TogglePlan(id string) (*PurchasePlan, error) {
	mu.Lock()
	defer mu.Unlock()

	p, ok := plans[id]
	if !ok {
		return nil, fmt.Errorf("plan not found: %s", id)
	}
	p.IsActive = !p.IsActive
	if err := savePlansLocked(); err != nil {
		return nil, err
	}
	return p, nil
}

func DeletePlan(id string) error {
	mu.Lock()
	defer mu.Unlock()

	if _, ok := plans[id]; !ok {
		return fmt.Errorf("plan not found: %s", id)
	}
	delete(plans, id)
	return savePlansLocked()
}

func RecordPlanMatch(planID, itemName string) {
	mu.Lock()
	defer mu.Unlock()

	if p, ok := plans[planID]; ok {
		p.MatchCount++
		p.LastTriggeredAt = time.Now().Format(time.RFC3339)
		p.LastMatchItem = itemName
		_ = savePlansLocked()
	}
}

// ==================== 二手市场数据与历史走势 ====================

type UpsertItemInput struct {
	ID              string
	Provider        string
	Name            string
	Price           float64
	RenewalPrice    float64
	DiscountPercent float64
	ResetPrice      float64
	PlatformFee     float64
	FlowUsed        string
	Cycle           int
	CycleName       string
	MonthlyPrice    float64
	Location        string
	Bandwidth       string
	ConfigDesc      string
	CPU             string
	Memory          string
	Disk            string
	TrafficTotal    string
	Detail          string
	IPCheckDetail   string
	DueTime         string
	SLA             string
	CanBuy          bool
	PortLimit       int
	SellerUsername  string
	SellerUserID    int
	MarketType      string
	BargainTag      string
	DiffPercent     float64
}

// UpsertMarketBatch 批量将拉取到的市场数据落库并推导停留时长。
// activeProviders 界定本次请求真实覆盖的服务商范围：仅当某个服务商在本次请求中成功返回时，
// 才允许把该服务商名下缺席的条目判定为已离场，避免单平台筛选或单平台超时误伤其它厂商的历史在售记录。
func UpsertMarketBatch(currentItems []UpsertItemInput, activeProviders []string) {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now()
	nowStr := now.Format(time.RFC3339)
	seenKeys := make(map[string]bool)
	scope := make(map[string]bool, len(activeProviders))
	for _, pid := range activeProviders {
		if pid != "" {
			scope[pid] = true
		}
	}

	for _, item := range currentItems {
		key := item.Provider + ":" + item.ID
		seenKeys[key] = true

		if rec, exists := marketHistory[key]; exists {
			rec.LastSeenAt = nowStr
			rec.IsActive = true
			rec.Price = item.Price
			if item.RenewalPrice > 0 {
				rec.RenewalPrice = item.RenewalPrice
			}
			if item.DiscountPercent > 0 {
				rec.DiscountPercent = item.DiscountPercent
			}
			if item.ResetPrice > 0 {
				rec.ResetPrice = item.ResetPrice
			}
			if item.PlatformFee > 0 {
				rec.PlatformFee = item.PlatformFee
			}
			if item.FlowUsed != "" {
				rec.FlowUsed = item.FlowUsed
			}
			if item.Cycle > 0 {
				rec.Cycle = item.Cycle
			}
			if item.CycleName != "" {
				rec.CycleName = item.CycleName
			}
			if item.MonthlyPrice > 0 {
				rec.MonthlyPrice = item.MonthlyPrice
			}
			if item.Location != "" {
				rec.Location = item.Location
			}
			if item.Bandwidth != "" {
				rec.Bandwidth = item.Bandwidth
			}
			if item.ConfigDesc != "" {
				rec.ConfigDesc = item.ConfigDesc
			}
			if item.CPU != "" {
				rec.CPU = item.CPU
			}
			if item.Memory != "" {
				rec.Memory = item.Memory
			}
			if item.Disk != "" {
				rec.Disk = item.Disk
			}
			if item.TrafficTotal != "" {
				rec.TrafficTotal = item.TrafficTotal
			}
			if item.Detail != "" {
				rec.Detail = item.Detail
			}
			if item.IPCheckDetail != "" {
				rec.IPCheckDetail = item.IPCheckDetail
			}
			if item.DueTime != "" {
				rec.DueTime = item.DueTime
			}
			if item.SLA != "" {
				rec.SLA = item.SLA
			}
			if item.PortLimit > 0 {
				rec.PortLimit = item.PortLimit
			}
			if item.SellerUsername != "" {
				rec.SellerUsername = item.SellerUsername
			}
			if item.SellerUserID > 0 {
				rec.SellerUserID = item.SellerUserID
			}
			if item.MarketType != "" {
				rec.MarketType = item.MarketType
			}
			if item.BargainTag != "" {
				rec.BargainTag = item.BargainTag
			}
			if item.DiffPercent != 0 {
				rec.DiffPercent = item.DiffPercent
			}
			if t, err := time.Parse(time.RFC3339, rec.FirstSeenAt); err == nil {
				rec.DurationSec = int(now.Sub(t).Seconds())
			}
		} else {
			marketHistory[key] = &MarketRecord{
				ID:              item.ID,
				Provider:        item.Provider,
				Name:            item.Name,
				Price:           item.Price,
				RenewalPrice:    item.RenewalPrice,
				DiscountPercent: item.DiscountPercent,
				ResetPrice:      item.ResetPrice,
				PlatformFee:     item.PlatformFee,
				FlowUsed:        item.FlowUsed,
				Cycle:           item.Cycle,
				CycleName:       item.CycleName,
				MonthlyPrice:    item.MonthlyPrice,
				Location:        item.Location,
				Bandwidth:       item.Bandwidth,
				ConfigDesc:      item.ConfigDesc,
				CPU:             item.CPU,
				Memory:          item.Memory,
				Disk:            item.Disk,
				TrafficTotal:    item.TrafficTotal,
				Detail:          item.Detail,
				IPCheckDetail:   item.IPCheckDetail,
				DueTime:         item.DueTime,
				SLA:             item.SLA,
				CanBuy:          item.CanBuy,
				PortLimit:       item.PortLimit,
				SellerUsername:  item.SellerUsername,
				SellerUserID:    item.SellerUserID,
				MarketType:      item.MarketType,
				BargainTag:      item.BargainTag,
				DiffPercent:     item.DiffPercent,
				FirstSeenAt:     nowStr,
				LastSeenAt:      nowStr,
				IsActive:        true,
				DurationSec:     0,
			}
		}
	}

	// 标记未出现在本批次中的商品为已离场 (Sold / Removed)
	// 仅对本次请求确实成功拉取到的服务商生效，绝不跨厂商误判下架
	for key, rec := range marketHistory {
		if !seenKeys[key] && rec.IsActive && scope[strings.ToLower(rec.Provider)] {
			rec.IsActive = false
		}
	}

	_ = saveMarketLocked()
}

// GetMarketStats 计算市场宏观走势与热门指标
func GetMarketStats() MarketStats {
	mu.RLock()
	defer mu.RUnlock()

	stats := MarketStats{
		TotalTracked:     len(marketHistory),
		ActiveItems:      0,
		ByProviderActive: make(map[string]int),
		ByLocation:       make(map[string]int),
		HotModels:        make([]string, 0),
	}

	totalPrice := 0.0
	priceCount := 0
	totalDuration := 0

	modelCount := make(map[string]int)

	for _, m := range marketHistory {
		if m.IsActive {
			stats.ActiveItems++
			stats.ByProviderActive[strings.ToLower(m.Provider)]++
			totalPrice += m.Price
			priceCount++
		}
		if m.Location != "" {
			stats.ByLocation[m.Location]++
		}
		if m.DurationSec > 0 {
			totalDuration += m.DurationSec
		}
		if m.Name != "" {
			modelCount[m.Name]++
		}
	}

	if priceCount > 0 {
		stats.AvgPrice = totalPrice / float64(priceCount)
	}
	if len(marketHistory) > 0 {
		stats.AvgDuration = totalDuration / len(marketHistory)
	}

	for k := range modelCount {
		stats.HotModels = append(stats.HotModels, k)
		if len(stats.HotModels) >= 5 {
			break
		}
	}

	return stats
}

// CalculateBargainIndex 计算单项商品的捡漏指数 (与历史同配均价对比)
func CalculateBargainIndex(name string, price float64) (diffPercent float64, tag string) {
	mu.RLock()
	defer mu.RUnlock()

	sum := 0.0
	cnt := 0
	for _, m := range marketHistory {
		if strings.EqualFold(m.Name, name) && m.Price > 0 {
			sum += m.Price
			cnt++
		}
	}

	if cnt < 2 {
		return 0, ""
	}

	avg := sum / float64(cnt)
	diff := (price - avg) / avg * 100 // 负数代表低于均价

	if diff <= -25 {
		return diff, "神机秒杀"
	} else if diff <= -10 {
		return diff, "超值好价"
	} else if diff >= 20 {
		return diff, "略有溢价"
	}
	return diff, "平价转让"
}

// 刷新时间全局维护表
var (
	refreshTimesMu sync.RWMutex
	refreshTimes   = make(map[string]string)
)

// RecordRefreshTime 记录某个模块/厂商的最后刷新时间 (RFC3339 格式)
func RecordRefreshTime(key string) string {
	refreshTimesMu.Lock()
	defer refreshTimesMu.Unlock()
	nowStr := time.Now().Format(time.RFC3339)
	refreshTimes[key] = nowStr
	return nowStr
}

// GetRefreshTime 获取指定模块/厂商的最后刷新时间
func GetRefreshTime(key string) string {
	refreshTimesMu.RLock()
	defer refreshTimesMu.RUnlock()
	return refreshTimes[key]
}

// GetAllRefreshTimes 获取所有模块/厂商的刷新时间表
func GetAllRefreshTimes() map[string]string {
	refreshTimesMu.RLock()
	defer refreshTimesMu.RUnlock()
	res := make(map[string]string, len(refreshTimes))
	for k, v := range refreshTimes {
		res[k] = v
	}
	return res
}

// GetActiveMarketRecords 获取在售的二手市场持久化记录 (支持指定 provider 筛选)
func GetActiveMarketRecords(provider string) []*MarketRecord {
	mu.RLock()
	defer mu.RUnlock()
	var out []*MarketRecord
	for _, rec := range marketHistory {
		if rec.IsActive {
			if provider == "" || provider == "all" || strings.EqualFold(rec.Provider, provider) {
				out = append(out, rec)
			}
		}
	}
	return out
}
