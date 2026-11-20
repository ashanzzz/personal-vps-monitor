package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/audit"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/config"
	"github.com/go-chi/chi/v5"
)

var (
	authMu sync.RWMutex
	tokens = make(map[string]int64) // token -> expire timestamp

	ErrNoToken      = errors.New("missing access token")
	ErrInvalidToken = errors.New("invalid or expired access token")
)

type AdminCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthIdentity struct {
	Username string `json:"username"`
	Role     string `json:"role"`   // "admin", "control", "read"
	Source   string `json:"source"` // "session", "password_key", "control_token", "read_token"
}

type authContextKeyType struct{}

var authContextKey = authContextKeyType{}

func GetIdentity(ctx context.Context) *AuthIdentity {
	if val, ok := ctx.Value(authContextKey).(*AuthIdentity); ok {
		return val
	}
	return nil
}

func init() {
	initSessions()
}

// 获取持久化密码存储文件路径
func getAuthFilePath() string {
	paths := []string{"./data/admin.json"}
	if runtime.GOOS == "linux" {
		paths = append(paths, "/app/data/admin.json", "/mnt/user/appdata/akile-monitor/admin.json")
	}
	for _, p := range paths {
		dir := filepath.Dir(p)
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			return p
		}
	}
	_ = os.MkdirAll("./data", 0755)
	return "./data/admin.json"
}

// 获取持久化 Session Token 文件路径 (确保服务重启后用户无需重新登录)
func getSessionsFilePath() string {
	paths := []string{"./data/sessions.json"}
	if runtime.GOOS == "linux" {
		paths = append(paths, "/app/data/sessions.json", "/mnt/user/appdata/akile-monitor/sessions.json")
	}
	for _, p := range paths {
		dir := filepath.Dir(p)
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			return p
		}
	}
	_ = os.MkdirAll("./data", 0755)
	return "./data/sessions.json"
}

// 初始化并恢复活跃的 Session 令牌
func initSessions() {
	authMu.Lock()
	defer authMu.Unlock()

	p := getSessionsFilePath()
	b, err := os.ReadFile(p)
	if err != nil {
		return
	}
	var loaded map[string]int64
	if err := json.Unmarshal(b, &loaded); err == nil {
		now := time.Now().Unix()
		for tok, exp := range loaded {
			if exp > now {
				tokens[tok] = exp
			}
		}
	}
}

// 持久化活跃 Session (调用前需持有写锁)
func saveSessionsLocked() {
	p := getSessionsFilePath()
	now := time.Now().Unix()
	active := make(map[string]int64)
	for tok, exp := range tokens {
		if exp > now {
			active[tok] = exp
		}
	}
	tokens = active
	b, err := json.MarshalIndent(active, "", "  ")
	if err == nil {
		_ = os.WriteFile(p, b, 0600)
	}
}

// 加载当前管理员凭证，优先环境变量，并对默认初始值自动升级同步
func loadCredentials() AdminCredentials {
	authMu.RLock()
	defer authMu.RUnlock()

	defaultUser := strings.TrimSpace(os.Getenv("ADMIN_USER"))
	defaultPass := os.Getenv("ADMIN_PASSWORD")

	p := getAuthFilePath()
	if b, err := os.ReadFile(p); err == nil {
		var creds AdminCredentials
		if err := json.Unmarshal(b, &creds); err == nil && creds.Username != "" && creds.Password != "" {
			// 若环境变量配置了非空值，且本地 admin.json 为初厂默认值或与环境变量不一致，自动同步升级
			if defaultUser != "" && defaultPass != "" && (creds.Username == "admin" && creds.Password == "admin") {
				creds.Username = defaultUser
				creds.Password = defaultPass
			}
			return creds
		}
	}

	if defaultUser == "" {
		defaultUser = "admin"
	}
	if defaultPass == "" {
		defaultPass = "admin"
	}

	return AdminCredentials{
		Username: defaultUser,
		Password: defaultPass,
	}
}

// 校验账号密码 (支持文件存储与环境变量双向兜底)
func verifyAdminCredentials(username, password string) bool {
	creds := loadCredentials()
	if strings.TrimSpace(username) == creds.Username && password == creds.Password {
		return true
	}

	envUser := strings.TrimSpace(os.Getenv("ADMIN_USER"))
	envPass := os.Getenv("ADMIN_PASSWORD")
	if envUser != "" && envPass != "" && strings.TrimSpace(username) == envUser && password == envPass {
		return true
	}

	return false
}

// 保存凭证
func saveCredentials(creds AdminCredentials) error {
	authMu.Lock()
	defer authMu.Unlock()

	p := getAuthFilePath()
	_ = os.MkdirAll(filepath.Dir(p), 0755)
	b, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0600)
}

// 重置为默认密码 admin / admin
func ResetToDefaultCredentials() {
	_ = saveCredentials(AdminCredentials{
		Username: "admin",
		Password: "admin",
	})
}

// 生成安全 Session Token 并持久化
func generateToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)

	authMu.Lock()
	tokens[token] = time.Now().Add(7 * 24 * time.Hour).Unix()
	saveSessionsLocked()
	authMu.Unlock()

	return token
}

// 撤销 Session Token
func revokeToken(token string) {
	authMu.Lock()
	delete(tokens, token)
	saveSessionsLocked()
	authMu.Unlock()
}

// 从请求中提取 Token (支持 Authorization: Bearer, X-API-Key 以及 Query 参数 ?token=)
func ExtractToken(r *http.Request) string {
	// 1. Authorization: Bearer <token>
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
			return strings.TrimSpace(parts[1])
		}
		if len(parts) == 1 && !strings.Contains(parts[0], " ") {
			return parts[0]
		}
	}

	// 2. X-API-Key: <token>
	if key := strings.TrimSpace(r.Header.Get("X-Api-Key")); key != "" {
		return key
	}
	if key := strings.TrimSpace(r.Header.Get("X-API-KEY")); key != "" {
		return key
	}

	// 3. Query string (?token=... or ?api_key=...)
	if tok := strings.TrimSpace(r.URL.Query().Get("token")); tok != "" {
		return tok
	}
	if tok := strings.TrimSpace(r.URL.Query().Get("api_key")); tok != "" {
		return tok
	}

	return ""
}

// AuthenticateToken 核心鉴权逻辑 (多通道统一身份识别)
func AuthenticateToken(token string) (*AuthIdentity, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrNoToken
	}

	// 通道 1: 活跃的 Web 管理员 Session Token
	authMu.RLock()
	exp, exists := tokens[token]
	authMu.RUnlock()
	if exists && exp > time.Now().Unix() {
		creds := loadCredentials()
		return &AuthIdentity{
			Username: creds.Username,
			Role:     "admin",
			Source:   "session",
		}, nil
	}

	// 通道 2: 第三方可写控制密钥 PUBLIC_CONTROL_TOKEN
	ctrlToken := config.Global.PublicControlToken
	if ctrlToken == "" {
		ctrlToken = os.Getenv("PUBLIC_CONTROL_TOKEN")
	}
	if ctrlToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(ctrlToken)) == 1 {
		return &AuthIdentity{
			Username: "control_agent",
			Role:     "admin",
			Source:   "control_token",
		}, nil
	}

	// 通道 3: 管理员密码直接作为 API Key (便捷运维/CLI/脚本/AI调用，密码长度需>=6)
	creds := loadCredentials()
	if len(creds.Password) >= 6 && subtle.ConstantTimeCompare([]byte(token), []byte(creds.Password)) == 1 {
		return &AuthIdentity{
			Username: creds.Username,
			Role:     "admin",
			Source:   "password_key",
		}, nil
	}
	envPass := os.Getenv("ADMIN_PASSWORD")
	if len(envPass) >= 6 && subtle.ConstantTimeCompare([]byte(token), []byte(envPass)) == 1 {
		return &AuthIdentity{
			Username: creds.Username,
			Role:     "admin",
			Source:   "env_password_key",
		}, nil
	}

	// 通道 4: 第三方只读密钥 PUBLIC_READ_TOKEN (仅允许 GET 查询)
	readToken := config.Global.PublicReadToken
	if readToken == "" {
		readToken = os.Getenv("PUBLIC_READ_TOKEN")
	}
	if readToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(readToken)) == 1 {
		return &AuthIdentity{
			Username: "read_agent",
			Role:     "read",
			Source:   "read_token",
		}, nil
	}

	return nil, ErrInvalidToken
}

// RequireAuthMiddleware 严格身份拦截中间件 (拦截未授权的局域网或外部请求)
func RequireAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := ExtractToken(r)
		if token == "" {
			ip := ExtractClientIP(r)
			audit.Record(
				"security",
				"auth.unauthorized_access",
				"system",
				map[string]interface{}{
					"ip":         ip,
					"path":       r.URL.Path,
					"method":     r.Method,
					"user_agent": r.UserAgent(),
				},
				"FAILED",
				fmt.Sprintf("局域网身份拦截：未携带令牌拒绝访问 %s %s (来源 IP: %s)", r.Method, r.URL.Path, ip),
				0,
			)
			respondJSON(w, http.StatusUnauthorized, map[string]interface{}{
				"error":  "unauthorized",
				"detail": "身份认证失败：缺少访问令牌 (请登录或携带 Authorization: Bearer <token> 或 X-API-Key)",
			})
			return
		}

		ident, err := AuthenticateToken(token)
		if err != nil {
			ip := ExtractClientIP(r)
			audit.Record(
				"security",
				"auth.invalid_token",
				"system",
				map[string]interface{}{
					"ip":         ip,
					"path":       r.URL.Path,
					"method":     r.Method,
					"user_agent": r.UserAgent(),
				},
				"FAILED",
				fmt.Sprintf("局域网身份拦截：无效令牌拒绝访问 %s %s (来源 IP: %s)", r.Method, r.URL.Path, ip),
				0,
			)
			respondJSON(w, http.StatusUnauthorized, map[string]interface{}{
				"error":  "unauthorized",
				"detail": "身份认证失败：访问令牌无效或已过期，请重新登录",
			})
			return
		}

		// 只读角色权限拦截：若凭证角色为 read，仅允许 GET、HEAD、OPTIONS 等安全幂等方法，且严格禁止进入终端控制台
		if ident.Role == "read" {
			if strings.Contains(r.URL.Path, "/terminal") || (r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions) {
				respondJSON(w, http.StatusForbidden, map[string]interface{}{
					"error":  "forbidden",
					"detail": "权限不足：当前凭据为只读权限 (PUBLIC_READ_TOKEN)，禁止执行写操作或进入终端控制台",
				})
				return
			}
		}

		ctx := context.WithValue(r.Context(), authContextKey, ident)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// 注册公开 Auth 路由
func registerAuthRoutes(r chi.Router) {
	r.Route("/auth", func(r chi.Router) {
		// 检查初始化状态 (公开)
		r.Get("/status", func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, http.StatusOK, map[string]interface{}{
				"initialized": true,
			})
		})

		// 登录 (公开，防暴力破解限流)
		r.Post("/login", func(w http.ResponseWriter, r *http.Request) {
			ip := ExtractClientIP(r)

			// 检查是否已被锁定
			if locked, remaining := IsIPLocked(ip); locked {
				respondJSON(w, http.StatusTooManyRequests, map[string]string{
					"detail": fmt.Sprintf("由于多次输错密码，您的 IP (%s) 已被安全保护系统锁定，请等待 %d 秒后重试", ip, int(remaining.Seconds())),
				})
				return
			}

			var req struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				respondJSON(w, http.StatusBadRequest, map[string]string{"detail": "请求参数无效"})
				return
			}

			if !verifyAdminCredentials(req.Username, req.Password) {
				RecordFailedLogin(ip, req.Username, r.UserAgent())
				respondJSON(w, http.StatusUnauthorized, map[string]string{"detail": "用户名或密码错误"})
				return
			}

			creds := loadCredentials()
			ResetFailedLogin(ip, creds.Username)
			token := generateToken()
			respondJSON(w, http.StatusOK, map[string]interface{}{
				"token":    token,
				"username": creds.Username,
			})
		})

		// 退出登录 (吊销会话)
		r.Post("/logout", func(w http.ResponseWriter, r *http.Request) {
			token := ExtractToken(r)
			if token != "" {
				revokeToken(token)
			}
			respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		})

		// 首次设置 (setup) (公开)
		r.Post("/setup", func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				respondJSON(w, http.StatusBadRequest, map[string]string{"detail": "请求参数无效"})
				return
			}

			_ = saveCredentials(AdminCredentials{
				Username: strings.TrimSpace(req.Username),
				Password: req.Password,
			})

			token := generateToken()
			respondJSON(w, http.StatusOK, map[string]interface{}{
				"token":    token,
				"username": req.Username,
			})
		})
	})
}

// 处理密码修改 (必须由 RequireAuthMiddleware 保护)
func handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"detail": "请求参数无效"})
		return
	}

	creds := loadCredentials()
	if req.OldPassword != creds.Password && req.OldPassword != os.Getenv("ADMIN_PASSWORD") {
		respondJSON(w, http.StatusBadRequest, map[string]string{"detail": "原密码不正确"})
		return
	}

	creds.Password = req.NewPassword
	if err := saveCredentials(creds); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"detail": "保存失败: " + err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
