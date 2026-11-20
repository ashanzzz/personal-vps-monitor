package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/audit"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/notify"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/store"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
)

type ProviderCheckinState struct {
	ProviderID    string    `json:"provider_id"`
	Status        string    `json:"status"` // "pending", "success", "already_signed", "failed", "retrying"
	LastAttemptAt time.Time `json:"last_attempt_at"`
	SuccessAt     time.Time `json:"success_at,omitempty"`
	RetryCount    int       `json:"retry_count"`
	NextRetryAt   time.Time `json:"next_retry_at,omitempty"`
	Message       string    `json:"message"`
	Reward        string    `json:"reward,omitempty"`
	Account       string    `json:"account,omitempty"`
}

type DailyCheckinManager struct {
	mu     sync.RWMutex
	date   string // YYYY-MM-DD
	states map[string]*ProviderCheckinState
	timers map[string]*time.Timer
}

var (
	GlobalCheckinMgr *DailyCheckinManager
	checkinOnce      sync.Once
)

func getCheckinCachePath() string {
	paths := []string{"/app/data/checkin_cache.json", "./data/checkin_cache.json"}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if _, err := os.Stat("/app/data"); err == nil {
		return "/app/data/checkin_cache.json"
	}
	_ = os.MkdirAll("./data", 0755)
	return "./data/checkin_cache.json"
}

func GetCheckinManager() *DailyCheckinManager {
	checkinOnce.Do(func() {
		GlobalCheckinMgr = &DailyCheckinManager{
			date:   time.Now().Format("2006-01-02"),
			states: make(map[string]*ProviderCheckinState),
			timers: make(map[string]*time.Timer),
		}
		GlobalCheckinMgr.load()
	})
	return GlobalCheckinMgr
}

func (m *DailyCheckinManager) load() {
	p := getCheckinCachePath()
	b, err := os.ReadFile(p)
	if err != nil || len(b) == 0 {
		return
	}
	var saved struct {
		Date   string                           `json:"date"`
		States map[string]*ProviderCheckinState `json:"states"`
	}
	if err := json.Unmarshal(b, &saved); err == nil && saved.Date == m.date {
		m.states = saved.States
	}
}

func (m *DailyCheckinManager) saveLocked() {
	p := getCheckinCachePath()
	payload := struct {
		Date   string                           `json:"date"`
		States map[string]*ProviderCheckinState `json:"states"`
	}{
		Date:   m.date,
		States: m.states,
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err == nil {
		_ = os.MkdirAll(filepath.Dir(p), 0755)
		_ = os.WriteFile(p, b, 0644)
	}
}

// StartCheckinScheduler 启动每日 08:00 定时打卡协程，失败后每 30 分钟自动重试
func StartCheckinScheduler(ctx context.Context) {
	mgr := GetCheckinManager()

	go func() {
		now := time.Now()
		// 若当前时间已过今日 08:00 且今日尚未完成打卡，启动时补打一次
		targetToday := time.Date(now.Year(), now.Month(), now.Day(), 8, 0, 0, 0, now.Location())
		if now.After(targetToday) {
			mgr.mu.RLock()
			needsInitialCheckin := false
			for _, p := range []string{"akile", "lazycat", "acck"} {
				st, ok := mgr.states[p]
				if !ok || (st.Status != "success" && st.Status != "already_signed") {
					needsInitialCheckin = true
					break
				}
			}
			mgr.mu.RUnlock()

			if needsInitialCheckin {
				log.Printf("[DailyCheckin] 服务启动：检测到今日 08:00 打卡尚未全部完成，立即执行补充打卡...")
				mgr.RunCheckinAll(ctx)
			}
		}

		// 主定时循环：等待每天 08:00
		for {
			now = time.Now()
			next08 := time.Date(now.Year(), now.Month(), now.Day(), 8, 0, 0, 0, now.Location())
			if !now.Before(next08) {
				next08 = next08.Add(24 * time.Hour)
			}
			waitDur := next08.Sub(now)

			select {
			case <-ctx.Done():
				return
			case <-time.After(waitDur):
				// 跨天重置状态
				mgr.mu.Lock()
				mgr.date = time.Now().Format("2006-01-02")
				mgr.states = make(map[string]*ProviderCheckinState)
				for _, t := range mgr.timers {
					t.Stop()
				}
				mgr.timers = make(map[string]*time.Timer)
				mgr.saveLocked()
				mgr.mu.Unlock()

				log.Printf("[DailyCheckin] 触发每天 08:00 定时首发打卡...")
				mgr.RunCheckinAll(ctx)
			}
		}
	}()
}

// RunCheckinAll 执行全部可用服务商打卡
func (m *DailyCheckinManager) RunCheckinAll(ctx context.Context) {
	all := providers.GetAll()
	for _, p := range all {
		if _, ok := p.(providers.AutoCheckin); ok {
			go m.CheckinSingle(ctx, p.ID())
		}
	}
}

// CheckinSingle 执行单个服务商打卡，失败则压入 30 分钟定时重试队列
func (m *DailyCheckinManager) CheckinSingle(ctx context.Context, providerID string) {
	m.mu.Lock()
	today := time.Now().Format("2006-01-02")
	if m.date != today {
		m.date = today
		m.states = make(map[string]*ProviderCheckinState)
	}

	st, exists := m.states[providerID]
	if !exists {
		st = &ProviderCheckinState{
			ProviderID: providerID,
			Status:     "pending",
		}
		m.states[providerID] = st
	}

	if st.Status == "success" || st.Status == "already_signed" {
		m.mu.Unlock()
		return
	}

	st.LastAttemptAt = time.Now()
	m.saveLocked()
	m.mu.Unlock()

	p, ok := providers.Get(providerID)
	if !ok {
		return
	}
	cp, ok := p.(providers.AutoCheckin)
	if !ok {
		return
	}

	// 打卡前先尝试执行一次鉴权探活自愈，确保 Token 有效
	pCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	res, err := cp.DoCheckin(pCtx)
	// 若遇到 401 掉线，尝试自动重登
	if err != nil && (strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "token") || strings.Contains(err.Error(), "unauthorized")) {
		log.Printf("[DailyCheckin] 服务商 %s 打卡检测到 Token 掉线，正在执行免密重登自愈...", providerID)
		if store.TryAutoRecoverProvider(pCtx, providerID) {
			pCtx2, cancel2 := context.WithTimeout(ctx, 25*time.Second)
			res, err = cp.DoCheckin(pCtx2)
			cancel2()
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	if err == nil {
		st.Status = "success"
		st.SuccessAt = now
		st.Message = res.Message
		st.Reward = res.Reward
		st.Account = res.Account
		st.NextRetryAt = time.Time{}

		// 检查文案是否为已签到
		if strings.Contains(res.Message, "已经") || strings.Contains(res.Message, "已签") {
			st.Status = "already_signed"
		}

		if t, ok := m.timers[providerID]; ok {
			t.Stop()
			delete(m.timers, providerID)
		}

		m.saveLocked()
		audit.Record("scheduler", "checkin.success", providerID, map[string]interface{}{
			"reward":  res.Reward,
			"account": res.Account,
			"message": res.Message,
		}, "SUCCESS", fmt.Sprintf("服务商 %s 每日自动打卡成功: %s (%s)", providerID, res.Message, res.Reward), 0)

		_ = notify.SendTelegram(fmt.Sprintf("🎯 每日自动打卡成功\n服务商: %s\n账号: %s\n结果: %s\n奖励: %s", providerID, res.Account, res.Message, res.Reward))
		log.Printf("[DailyCheckin] 服务商 %s 打卡成功: %s (%s)", providerID, res.Message, res.Reward)
	} else {
		// 打卡失败，压入 30 分钟重试队列
		st.Status = "retrying"
		st.RetryCount++
		st.Message = err.Error()
		nextRetry := now.Add(30 * time.Minute)
		st.NextRetryAt = nextRetry

		m.saveLocked()
		audit.Record("scheduler", "checkin.failed_pending_retry", providerID, map[string]interface{}{
			"error":        err.Error(),
			"retry_count":  st.RetryCount,
			"next_retry":   nextRetry.Format("15:04:05"),
		}, "FAILED", fmt.Sprintf("服务商 %s 每日打卡失败 (第 %d 次尝试): %v，系统将在 30 分钟后 (%s) 自动重试", providerID, st.RetryCount, err, nextRetry.Format("15:04:05")), 0)

		log.Printf("[DailyCheckin] 服务商 %s 打卡失败: %v。已调度于 30 分钟后 (%s) 自动重试 (第 %d 次)", providerID, err, nextRetry.Format("15:04:05"), st.RetryCount)

		// 清理旧定时器并启动 30 分钟重试
		if t, ok := m.timers[providerID]; ok {
			t.Stop()
		}
		m.timers[providerID] = time.AfterFunc(30*time.Minute, func() {
			m.CheckinSingle(context.Background(), providerID)
		})
	}
}

// RecordResult 允许外部（如手动单点签到、批量签到）直接回写打卡状态
func (m *DailyCheckinManager) RecordResult(providerID string, res *providers.CheckinResult, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	if m.date != today {
		m.date = today
		m.states = make(map[string]*ProviderCheckinState)
	}

	st, exists := m.states[providerID]
	if !exists {
		st = &ProviderCheckinState{
			ProviderID: providerID,
		}
		m.states[providerID] = st
	}

	now := time.Now()
	st.LastAttemptAt = now

	if err == nil && res != nil && res.Success {
		st.Status = "success"
		st.SuccessAt = now
		st.Message = res.Message
		st.Reward = res.Reward
		st.Account = res.Account
		st.NextRetryAt = time.Time{}

		if strings.Contains(res.Message, "已经") || strings.Contains(res.Message, "已签") {
			st.Status = "already_signed"
		}

		if t, ok := m.timers[providerID]; ok {
			t.Stop()
			delete(m.timers, providerID)
		}
	} else {
		st.Status = "retrying"
		st.RetryCount++
		if err != nil {
			st.Message = err.Error()
		} else if res != nil {
			st.Message = res.Message
		}
		st.NextRetryAt = now.Add(30 * time.Minute)
	}
	m.saveLocked()
}

// GetSummary 获取今日打卡全局健康状态摘要 (0ms 极速响应)
func (m *DailyCheckinManager) GetSummary() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	res := make(map[string]interface{})
	for k, v := range m.states {
		res[k] = map[string]interface{}{
			"status":          v.Status,
			"last_attempt_at": v.LastAttemptAt.Format("2006-01-02 15:04:05"),
			"success_at":      v.SuccessAt.Format("2006-01-02 15:04:05"),
			"retry_count":     v.RetryCount,
			"next_retry_at":   v.NextRetryAt.Format("2006-01-02 15:04:05"),
			"message":         v.Message,
			"reward":          v.Reward,
			"account":         v.Account,
		}
	}
	return res
}
