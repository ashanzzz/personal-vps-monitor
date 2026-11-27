package store

import (
	"testing"
)

// resetMarket 清空内存中的集市历史，避免用例之间互相污染
func resetMarket(t *testing.T) {
	t.Helper()
	mu.Lock()
	marketHistory = make(map[string]*MarketRecord)
	mu.Unlock()
}

func seedRecord(provider, id string) {
	marketHistory[provider+":"+id] = &MarketRecord{
		ID:          id,
		Provider:    provider,
		Name:        provider + "-" + id,
		Price:       10,
		FirstSeenAt: "2026-01-01T00:00:00Z",
		LastSeenAt:  "2026-01-01T00:00:00Z",
		IsActive:    true,
	}
}

// 单厂商刷新只允许下架该厂商自己的离场条目，绝不能误伤其它厂商的在售记录
func TestUpsertMarketBatchScopedToActiveProviders(t *testing.T) {
	resetMarket(t)
	defer resetMarket(t)

	seedRecord("akile", "A-keep")
	seedRecord("akile", "A-gone")
	seedRecord("acck", "C-1")
	seedRecord("lazycat", "L-1")

	UpsertMarketBatch([]UpsertItemInput{
		{ID: "A-keep", Provider: "akile", Name: "akile-A-keep", Price: 12},
	}, []string{"akile"})

	mu.RLock()
	defer mu.RUnlock()

	if !marketHistory["akile:A-keep"].IsActive {
		t.Fatalf("本次刷新命中的条目必须保持在售")
	}
	if marketHistory["akile:A-gone"].IsActive {
		t.Fatalf("同一厂商缺席的条目应被判定为已离场")
	}
	if !marketHistory["acck:C-1"].IsActive {
		t.Fatalf("未参与本次刷新的 acck 条目被错误下架")
	}
	if !marketHistory["lazycat:L-1"].IsActive {
		t.Fatalf("未参与本次刷新的 lazycat 条目被错误下架")
	}
}

// 未成功返回的厂商（例如上游超时）不得清空其历史在售数据
func TestUpsertMarketBatchEmptyResultKeepsOtherProviders(t *testing.T) {
	resetMarket(t)
	defer resetMarket(t)

	seedRecord("acck", "C-1")
	seedRecord("lazycat", "L-1")

	// akile 本次返回空列表：允许把 akile 名下条目下架，但其它厂商不受影响
	UpsertMarketBatch([]UpsertItemInput{}, []string{"akile"})

	mu.RLock()
	defer mu.RUnlock()

	if !marketHistory["acck:C-1"].IsActive || !marketHistory["lazycat:L-1"].IsActive {
		t.Fatalf("空结果批次不应影响其它厂商")
	}
}

// 统计必须按厂商完整拆分，供前端核对总数与分项是否自洽
func TestGetMarketStatsByProviderActive(t *testing.T) {
	resetMarket(t)
	defer resetMarket(t)

	seedRecord("akile", "A-1")
	seedRecord("akile", "A-2")
	seedRecord("acck", "C-1")
	seedRecord("lazycat", "L-1")
	seedRecord("lazycat", "L-gone")

	mu.Lock()
	marketHistory["lazycat:L-gone"].IsActive = false
	mu.Unlock()

	stats := GetMarketStats()
	if stats.ActiveItems != 4 {
		t.Fatalf("active_items = %d, want 4", stats.ActiveItems)
	}
	if stats.ByProviderActive["akile"] != 2 || stats.ByProviderActive["acck"] != 1 || stats.ByProviderActive["lazycat"] != 1 {
		t.Fatalf("by_provider_active = %#v", stats.ByProviderActive)
	}
	sum := 0
	for _, v := range stats.ByProviderActive {
		sum += v
	}
	if sum != stats.ActiveItems {
		t.Fatalf("分项之和 %d != active_items %d", sum, stats.ActiveItems)
	}
}
