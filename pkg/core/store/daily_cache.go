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
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/vault"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers/acck"
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
			recovered := TryAutoRecoverProvider(ctx, id)
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

// TryAutoRecoverProvider 尝试为失效的 Provider 自动续期或免密重登 (全流程自动自愈)
func TryAutoRecoverProvider(ctx context.Context, providerID string) bool {
	p, ok := providers.Get(providerID)
	if !ok {
		return false
	}

	creds := config.LoadSavedCredentials()

	switch providerID {
	case "akile":
		if ak, isAk := p.(*akile.AkileProvider); isAk {
			// 1. 优先尝试 Token 快速续期
			if newTok, err := ak.RefreshToken(ctx); err == nil && newTok != "" {
				_ = config.SaveTokens(newTok, "", "", "")
				config.Global.AkileToken = newTok
				ak.SetToken(newTok)
				audit.Record("security", "akile.auto_refresh", "akile", nil, "SUCCESS", "自动续期 Akile Token 成功", 0)
				log.Printf("[AutoRecover] Akile Token 快速续期成功")
				return true
			}
			// 2. 续期失败 (如 401 完全过期)，尝试使用凭据库账号密码重新登录
			if creds.Akile != nil && creds.Akile.Username != "" && creds.Akile.Password != "" {
				gaCode := ""
				if creds.Akile.TOTPSecret != "" {
					if code, err := vault.GenerateTOTP(creds.Akile.TOTPSecret); err == nil && code != "" {
						gaCode = code
					}
				}
				if newTok, need2FA, _, err := ak.Login(ctx, creds.Akile.Username, creds.Akile.Password, gaCode); err == nil && newTok != "" && !need2FA {
					_ = config.SaveTokens(newTok, "", "", "")
					config.Global.AkileToken = newTok
					ak.SetToken(newTok)
					audit.Record("security", "akile.auto_relogin", "akile", map[string]interface{}{"username": creds.Akile.Username}, "SUCCESS", "检测到 Akile Token 掉线，已自动依据凭据库(及实时TOTP)免密重新登录并恢复！", 0)
					log.Printf("[AutoRecover] Akile 自动免密重新登录成功，Token 已更新")
					return true
				}
			}
		}
	case "lazycat":
		if lz, isLz := p.(*lazycat.LazyCatProvider); isLz {
			if creds.LazyCat != nil && creds.LazyCat.Username != "" && creds.LazyCat.Password != "" {
				if tok, need2FA, _, err := lz.Login(ctx, creds.LazyCat.Username, creds.LazyCat.Password, ""); err == nil && tok != "" && !need2FA {
					_ = config.SaveTokens("", "", tok, "")
					config.Global.LazyCatToken = tok
					lz.SetToken(tok)
					audit.Record("security", "lazycat.auto_relogin", "lazycat", map[string]interface{}{"username": creds.LazyCat.Username}, "SUCCESS", "检测到懒猫云 Token 掉线，已自动依据凭据库免密重新登录并恢复！", 0)
					log.Printf("[AutoRecover] 懒猫云自动免密重新登录成功，Token 已更新")
					return true
				}
			}
		}
	case "acck":
		// 如果 ACCK 凭据库配置了备用 Token
		if acckP, isAcck := p.(*acck.ACCKProvider); isAcck {
			if creds.ACCK != nil && creds.ACCK.Password != "" {
				acckP.SetToken(creds.ACCK.Password)
				if err := acckP.VerifyToken(ctx); err == nil {
					_ = config.SaveTokens("", creds.ACCK.Password, "", "")
					config.Global.ACCKToken = creds.ACCK.Password
					audit.Record("security", "acck.auto_relogin", "acck", nil, "SUCCESS", "检测到 ACCK 凭据掉线，已自动从凭据库恢复并校验通过", 0)
					log.Printf("[AutoRecover] ACCK 凭据自动校验并恢复成功")
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
