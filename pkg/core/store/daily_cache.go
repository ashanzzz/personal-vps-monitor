package store

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/audit"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/config"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers/akile"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers/lazycat"
)

type ProviderSyncStatus struct {
	Status        string `json:"status"` // "ok" or "error"
	Message       string `json:"message,omitempty"`
	LastAttemptAt string `json:"last_attempt_at"`
}

type DailyInstancesCache struct {
	LastSyncedAt      string                        `json:"last_synced_at"`
	NextScheduledSync string                        `json:"next_scheduled_sync"`
	Total             int                           `json:"total"`
	Instances         []providers.Instance          `json:"instances"`
	ProviderStatus    map[string]ProviderSyncStatus `json:"provider_status"`
}

var (
	dailyCacheMu sync.RWMutex
)

func getDailyCacheFile() string {
	return filepath.Join(getDataDir(), "instances_daily_cache.json")
}

// LoadDailyCache 从磁盘加载每日缓存
func LoadDailyCache() (*DailyInstancesCache, error) {
	dailyCacheMu.RLock()
	defer dailyCacheMu.RUnlock()

	b, err := os.ReadFile(getDailyCacheFile())
	if err != nil {
		return nil, err
	}

	var cache DailyInstancesCache
	if err := json.Unmarshal(b, &cache); err != nil {
		return nil, err
	}
	return &cache, nil
}

// SaveDailyCache 持久化每日缓存至磁盘
func SaveDailyCache(cache *DailyInstancesCache) error {
	dailyCacheMu.Lock()
	defer dailyCacheMu.Unlock()

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := getDailyCacheFile() + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpFile, getDailyCacheFile())
}

// SyncAllInstancesDaily 每日全量同步三大服务商资产（月流量、Root密码、IP、规格）
// 核心原则：若某厂商登录失效或接口异常，绝不清空已有缓存，标记 is_stale 并保留历史数据！
func SyncAllInstancesDaily(ctx context.Context, force bool) (*DailyInstancesCache, error) {
	existingCache, _ := LoadDailyCache()

	now := time.Now()
	// 如果不是强制刷新，且距上次同步未满 23 小时，直接返回已有缓存
	if !force && existingCache != nil && existingCache.LastSyncedAt != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", existingCache.LastSyncedAt); err == nil {
			if now.Sub(t) < 23*time.Hour && len(existingCache.Instances) > 0 {
				return existingCache, nil
			}
		}
	}

	allProviders := providers.GetAll()
	mergedInstances := make([]providers.Instance, 0)
	providerStatus := make(map[string]ProviderSyncStatus)

	// 建立旧缓存按 Provider 的索引，用于优雅降级保活
	oldInstancesByProvider := make(map[string][]providers.Instance)
	if existingCache != nil {
		for _, inst := range existingCache.Instances {
			oldInstancesByProvider[inst.ProviderID] = append(oldInstancesByProvider[inst.ProviderID], inst)
		}
	}

	for id, p := range allProviders {
		im, ok := p.(providers.InstanceManager)
		if !ok {
			continue
		}

		status := ProviderSyncStatus{
			Status:        "ok",
			LastAttemptAt: now.Format("2006-01-02 15:04:05"),
		}

		insts, err := im.ListInstances(ctx)
		if err != nil {
			log.Printf("[DailySync] 服务商 %s 抓取失败: %v, 尝试自动续期/重登...", id, err)
			// 尝试自动重登
			recovered := tryAutoRecoverProvider(ctx, id)
			if recovered {
				insts, err = im.ListInstances(ctx)
			}
		}

		if err != nil {
			// 仍然失败，执行绝不清空缓存策略：恢复上次历史数据，并打上陈旧标记
			errReason := fmt.Sprintf("凭据失效或接口失联: %v", err)
			status.Status = "error"
			status.Message = errReason

			if oldList, ok := oldInstancesByProvider[id]; ok && len(oldList) > 0 {
				log.Printf("[DailySync] 服务商 %s 保活降级：保留上次 %d 台实例缓存数据并标注失效原因", id, len(oldList))
				for _, oldInst := range oldList {
					oldInst.IsStale = true
					oldInst.StaleReason = errReason
					mergedInstances = append(mergedInstances, oldInst)
				}
			}
		} else {
			// 成功获取最新数据
			for _, inst := range insts {
				inst.IsStale = false
				inst.StaleReason = ""
				inst.LastSyncedAt = now.Format("2006-01-02 15:04:05")
				mergedInstances = append(mergedInstances, inst)
			}
			log.Printf("[DailySync] 服务商 %s 资产与月流量同步成功 (共 %d 台)", id, len(insts))
		}

		providerStatus[id] = status
	}

	// 计算下一次执行时间（次日凌晨 04:00）
	nextSync := time.Date(now.Year(), now.Month(), now.Day()+1, 4, 0, 0, 0, now.Location())

	newCache := &DailyInstancesCache{
		LastSyncedAt:      now.Format("2006-01-02 15:04:05"),
		NextScheduledSync: nextSync.Format("2006-01-02 15:04:05"),
		Total:             len(mergedInstances),
		Instances:         mergedInstances,
		ProviderStatus:    providerStatus,
	}

	_ = SaveDailyCache(newCache)
	audit.Record("system", "daily_cache.sync", "all", map[string]interface{}{"total": len(mergedInstances), "force": force}, "SUCCESS", "已完成每日资产与流量数据落盘同步", 0)

	return newCache, nil
}

// tryAutoRecoverProvider 尝试为失效的 Provider 自动续期或免密重登
func tryAutoRecoverProvider(ctx context.Context, providerID string) bool {
	p, ok := providers.Get(providerID)
	if !ok {
		return false
	}

	switch providerID {
	case "akile":
		if ak, isAk := p.(*akile.AkileProvider); isAk {
			if newTok, err := ak.RefreshToken(ctx); err == nil && newTok != "" {
				_ = config.SaveTokens(newTok, "", "", "")
				config.Global.AkileToken = newTok
				ak.SetToken(newTok)
				return true
			}
		}
	case "lazycat":
		creds := config.LoadSavedCredentials()
		if creds.LazyCat != nil && creds.LazyCat.Password != "" {
			if lz, isLz := p.(*lazycat.LazyCatProvider); isLz {
				if tok, _, _, err := lz.Login(ctx, creds.LazyCat.Username, creds.LazyCat.Password, ""); err == nil && tok != "" {
					_ = config.SaveTokens("", "", tok, "")
					config.Global.LazyCatToken = tok
					lz.SetToken(tok)
					return true
				}
			}
		}
	}
	return false
}

// StartDailyCacheCron 启动后台每日定时同步守护协程 (每天 04:00 执行)
func StartDailyCacheCron(ctx context.Context) {
	go func() {
		// 启动时检查：如果没有缓存或距上次同步超过 24 小时，立即执行一次
		c, err := LoadDailyCache()
		if err != nil || c == nil || len(c.Instances) == 0 {
			log.Printf("[DailySync] 未检测到每日缓存，正在执行首次初始化同步...")
			_, _ = SyncAllInstancesDaily(ctx, true)
		}

		for {
			now := time.Now()
			// 计算距今天或明天 04:00 的等待时长
			nextSync := time.Date(now.Year(), now.Month(), now.Day(), 4, 0, 0, 0, now.Location())
			if now.After(nextSync) {
				nextSync = nextSync.Add(24 * time.Hour)
			}
			waitDuration := nextSync.Sub(now)

			select {
			case <-ctx.Done():
				return
			case <-time.After(waitDuration):
				log.Printf("[DailySync] 触发凌晨 04:00 每日资产与月度流量自动同步...")
				_, _ = SyncAllInstancesDaily(ctx, true)
			}
		}
	}()
}
