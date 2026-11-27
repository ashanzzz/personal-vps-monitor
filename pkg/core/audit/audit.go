package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type AuditEntry struct {
	ID         string                 `json:"id"`
	Timestamp  string                 `json:"timestamp"`
	Source     string                 `json:"source"`   // "ai_mcp", "daemon", "cli", "rest_api"
	Action     string                 `json:"action"`   // e.g. "lazycat.port_add", "akile.checkin"
	Provider   string                 `json:"provider"` // "lazycat", "akile", "acck", "system"
	Params     map[string]interface{} `json:"params,omitempty"`
	Status     string                 `json:"status"` // "SUCCESS" / "FAILED"
	Message    string                 `json:"message"`
	DurationMs int64                  `json:"duration_ms,omitempty"`
}

type QueryOptions struct {
	Days     int    // 查询过去几天 (默认 7, 最大 30)
	Limit    int    // 返回条数 (默认 50)
	Source   string // 按调用来源过滤 ("ai_mcp", "daemon", "cli", "rest_api")
	Provider string // 按厂商过滤 ("lazycat", "akile", "acck")
	Status   string // 按状态过滤 ("SUCCESS", "FAILED")
}

var (
	mu sync.Mutex
)

// GetLogDir 动态定位日志持久化目录
func GetLogDir() string {
	candidates := []string{
		"/app/data/logs",
		"/mnt/user/appdata/personal-vps-monitor/logs",
		"./logs",
	}

	for _, p := range candidates {
		parent := filepath.Dir(p)
		if fi, err := os.Stat(parent); err == nil && fi.IsDir() {
			_ = os.MkdirAll(p, 0755)
			return p
		}
	}

	_ = os.MkdirAll("./logs", 0755)
	return "./logs"
}

// Record 记录一次行为审计日志
func Record(source, action, provider string, params map[string]interface{}, status, message string, durationMs int64) *AuditEntry {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now()
	entry := &AuditEntry{
		ID:         fmt.Sprintf("%d-%s", now.UnixNano(), action),
		Timestamp:  now.Format(time.RFC3339),
		Source:     source,
		Action:     action,
		Provider:   provider,
		Params:     params,
		Status:     strings.ToUpper(status),
		Message:    message,
		DurationMs: durationMs,
	}

	logDir := GetLogDir()
	fileName := fmt.Sprintf("audit-%s.jsonl", now.Format("2006-01-02"))
	filePath := filepath.Join(logDir, fileName)

	data, err := json.Marshal(entry)
	if err == nil {
		f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err == nil {
			_, _ = f.Write(append(data, '\n'))
			_ = f.Close()
		}
	}

	// 同时输出到标准日志
	log.Printf("[AUDIT] [%s] %s (%s) -> %s: %s (%dms)", source, action, provider, entry.Status, message, durationMs)
	return entry
}

// Query 查询审计日志
func Query(opts QueryOptions) ([]AuditEntry, error) {
	mu.Lock()
	defer mu.Unlock()

	if opts.Days <= 0 {
		opts.Days = 7
	}
	if opts.Days > 30 {
		opts.Days = 30
	}
	if opts.Limit <= 0 {
		opts.Limit = 50
	}

	logDir := GetLogDir()
	now := time.Now()
	var results []AuditEntry

	// 从今天往前遍历 opts.Days 天
	for i := 0; i < opts.Days; i++ {
		dateStr := now.AddDate(0, 0, -i).Format("2006-01-02")
		filePath := filepath.Join(logDir, fmt.Sprintf("audit-%s.jsonl", dateStr))

		f, err := os.Open(filePath)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		var dayEntries []AuditEntry
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var e AuditEntry
			if err := json.Unmarshal(line, &e); err == nil {
				if opts.Source != "" && !strings.EqualFold(e.Source, opts.Source) {
					continue
				}
				if opts.Provider != "" && !strings.EqualFold(e.Provider, opts.Provider) {
					continue
				}
				if opts.Status != "" && !strings.EqualFold(e.Status, opts.Status) {
					continue
				}
				dayEntries = append(dayEntries, e)
			}
		}
		_ = f.Close()

		// 每天内部倒序
		for j := len(dayEntries) - 1; j >= 0; j-- {
			results = append(results, dayEntries[j])
			if len(results) >= opts.Limit {
				break
			}
		}
		if len(results) >= opts.Limit {
			break
		}
	}

	return results, nil
}

// PruneLogs 清理过期日志 (审计保留 30 天，系统普通运行日志保留 7 天)
func PruneLogs() (int, error) {
	mu.Lock()
	defer mu.Unlock()

	logDir := GetLogDir()
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return 0, err
	}

	now := time.Now()
	auditCutoff := now.AddDate(0, 0, -30)
	systemCutoff := now.AddDate(0, 0, -7)
	deleted := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		fullPath := filepath.Join(logDir, name)

		if strings.HasPrefix(name, "audit-") && strings.HasSuffix(name, ".jsonl") {
			datePart := strings.TrimSuffix(strings.TrimPrefix(name, "audit-"), ".jsonl")
			if t, err := time.Parse("2006-01-02", datePart); err == nil {
				if t.Before(auditCutoff) {
					_ = os.Remove(fullPath)
					deleted++
				}
			}
		} else if strings.HasPrefix(name, "system-") && strings.HasSuffix(name, ".log") {
			datePart := strings.TrimSuffix(strings.TrimPrefix(name, "system-"), ".log")
			if t, err := time.Parse("2006-01-02", datePart); err == nil {
				if t.Before(systemCutoff) {
					_ = os.Remove(fullPath)
					deleted++
				}
			}
		}
	}

	log.Printf("[Audit] Log prune completed: removed %d expired log files", deleted)
	return deleted, nil
}

// LogSystem 同时向文件和标准输出记录系统日志
func LogSystem(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	now := time.Now()
	logDir := GetLogDir()
	filePath := filepath.Join(logDir, fmt.Sprintf("system-%s.log", now.Format("2006-01-02")))

	formatted := fmt.Sprintf("%s %s\n", now.Format("2006-01-02 15:04:05"), msg)
	log.Print(msg)

	mu.Lock()
	defer mu.Unlock()
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		_, _ = f.WriteString(formatted)
		_ = f.Close()
	}
}
