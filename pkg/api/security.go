package api

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/audit"
)

var (
	secMu          sync.Mutex
	failedAttempts = make(map[string]int)       // ip -> count
	lockouts       = make(map[string]time.Time) // ip -> locked until
)

const (
	MaxFailedAttempts = 5                // 最多允许失败 5 次
	LockoutDuration   = 15 * time.Minute // 封禁 15 分钟
)

// ExtractClientIP 准确提取客户端真实 IP (防反代伪造)
func ExtractClientIP(r *http.Request) string {
	// 优先读取 X-Real-IP
	if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
		return xrip
	}

	// 其次从 X-Forwarded-For 获取第一个非私有/首个 IP
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}

	// 最后回退到 RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

// IsIPLocked 检查 IP 是否处于封禁锁定期
func IsIPLocked(ip string) (bool, time.Duration) {
	secMu.Lock()
	defer secMu.Unlock()

	until, exists := lockouts[ip]
	if !exists {
		return false, 0
	}

	remaining := time.Until(until)
	if remaining <= 0 {
		delete(lockouts, ip)
		delete(failedAttempts, ip)
		return false, 0
	}

	return true, remaining
}

// RecordFailedLogin 记录一次登录失败并触发封禁判定
func RecordFailedLogin(ip, username, userAgent string) {
	secMu.Lock()
	defer secMu.Unlock()

	failedAttempts[ip]++
	count := failedAttempts[ip]

	if count >= MaxFailedAttempts {
		lockouts[ip] = time.Now().Add(LockoutDuration)
		audit.Record(
			"security",
			"auth.ip_lockout",
			"system",
			map[string]interface{}{
				"ip":         ip,
				"username":   username,
				"attempts":   count,
				"user_agent": userAgent,
			},
			"FAILED",
			fmt.Sprintf("检测到暴力破解行为，IP %s 已被系统自动封禁 15 分钟", ip),
			0,
		)
	} else {
		audit.Record(
			"security",
			"auth.failed_attempt",
			"system",
			map[string]interface{}{
				"ip":         ip,
				"username":   username,
				"attempt":    count,
				"remaining":  MaxFailedAttempts - count,
				"user_agent": userAgent,
			},
			"FAILED",
			fmt.Sprintf("用户 %s 登录失败 (密码错误)，当前尝试第 %d/%d 次", username, count, MaxFailedAttempts),
			0,
		)
	}
}

// ResetFailedLogin 登录成功后重置失败计数
func ResetFailedLogin(ip, username string) {
	secMu.Lock()
	defer secMu.Unlock()

	delete(failedAttempts, ip)
	delete(lockouts, ip)

	audit.Record(
		"security",
		"auth.login_success",
		"system",
		map[string]interface{}{
			"ip":       ip,
			"username": username,
		},
		"SUCCESS",
		fmt.Sprintf("管理员 %s 登录成功 (来自 IP: %s)", username, ip),
		0,
	)
}

// SecurityMiddleware 全局防爆破中间件
func SecurityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ExtractClientIP(r)

		// 针对认证接口进行封禁拦截
		if strings.HasPrefix(r.URL.Path, "/api/auth/login") && r.Method == http.MethodPost {
			if locked, remaining := IsIPLocked(ip); locked {
				respondJSON(w, http.StatusTooManyRequests, map[string]interface{}{
					"detail": fmt.Sprintf("由于多次输错密码，您的 IP (%s) 已被安全保护系统锁定，请等待 %d 秒后重试", ip, int(remaining.Seconds())),
				})
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
