package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/config"
	"github.com/go-chi/chi/v5"
)

func TestExtractToken(t *testing.T) {
	tests := []struct {
		name     string
		req      *http.Request
		expected string
	}{
		{
			name: "Bearer header",
			req: func() *http.Request {
				r := httptest.NewRequest("GET", "/api/instances", nil)
				r.Header.Set("Authorization", "Bearer token-abc-123")
				return r
			}(),
			expected: "token-abc-123",
		},
		{
			name: "Direct Authorization header",
			req: func() *http.Request {
				r := httptest.NewRequest("GET", "/api/instances", nil)
				r.Header.Set("Authorization", "token-direct-456")
				return r
			}(),
			expected: "token-direct-456",
		},
		{
			name: "X-Api-Key header",
			req: func() *http.Request {
				r := httptest.NewRequest("GET", "/api/instances", nil)
				r.Header.Set("X-Api-Key", "api-key-789")
				return r
			}(),
			expected: "api-key-789",
		},
		{
			name: "Query param token",
			req: func() *http.Request {
				return httptest.NewRequest("GET", "/sse?token=query-tok-111", nil)
			}(),
			expected: "query-tok-111",
		},
		{
			name: "Query param api_key",
			req: func() *http.Request {
				return httptest.NewRequest("GET", "/api/instances?api_key=query-key-222", nil)
			}(),
			expected: "query-key-222",
		},
		{
			name: "No token",
			req: func() *http.Request {
				return httptest.NewRequest("GET", "/api/instances", nil)
			}(),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractToken(tt.req)
			if got != tt.expected {
				t.Errorf("ExtractToken() = %q, expected %q", got, tt.expected)
			}
		})
	}
}

func TestAuthenticateToken(t *testing.T) {
	// 临时设置全局配置
	config.Global.PublicControlToken = "test-control-secret-999"
	config.Global.PublicReadToken = "test-read-secret-888"
	_ = os.Setenv("ADMIN_PASSWORD", "SecretMasterPassword123!")

	// 1. 测试空 Token
	if _, err := AuthenticateToken(""); err != ErrNoToken {
		t.Errorf("expected ErrNoToken, got %v", err)
	}

	// 2. 测试无效 Token
	if _, err := AuthenticateToken("completely-invalid-token"); err != ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}

	// 3. 测试活跃 Session Token
	sessTok := generateToken()
	ident, err := AuthenticateToken(sessTok)
	if err != nil {
		t.Fatalf("AuthenticateToken(session) failed: %v", err)
	}
	if ident.Role != "admin" || ident.Source != "session" {
		t.Errorf("unexpected session ident: %+v", ident)
	}

	// 4. 测试 Public Control Token
	ctrlIdent, err := AuthenticateToken("test-control-secret-999")
	if err != nil {
		t.Fatalf("AuthenticateToken(control) failed: %v", err)
	}
	if ctrlIdent.Role != "admin" || ctrlIdent.Source != "control_token" {
		t.Errorf("unexpected control ident: %+v", ctrlIdent)
	}

	// 5. 测试管理员密码作为 API Key
	passIdent, err := AuthenticateToken("SecretMasterPassword123!")
	if err != nil {
		t.Fatalf("AuthenticateToken(password) failed: %v", err)
	}
	if passIdent.Role != "admin" {
		t.Errorf("unexpected password ident: %+v", passIdent)
	}

	// 6. 测试 Public Read Token
	readIdent, err := AuthenticateToken("test-read-secret-888")
	if err != nil {
		t.Fatalf("AuthenticateToken(read) failed: %v", err)
	}
	if readIdent.Role != "read" || readIdent.Source != "read_token" {
		t.Errorf("unexpected read ident: %+v", readIdent)
	}
}

func TestRequireAuthMiddleware(t *testing.T) {
	r := chi.NewRouter()

	r.Get("/public/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Group(func(r chi.Router) {
		r.Use(RequireAuthMiddleware)

		r.Get("/protected/data", func(w http.ResponseWriter, r *http.Request) {
			ident := GetIdentity(r.Context())
			respondJSON(w, http.StatusOK, map[string]interface{}{
				"status": "authorized",
				"role":   ident.Role,
			})
		})

		r.Post("/protected/write", func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, http.StatusOK, map[string]string{"status": "written"})
		})
	})

	// 1. 公开接口允许无认证访问
	req1 := httptest.NewRequest("GET", "/public/health", nil)
	rec1 := httptest.NewRecorder()
	r.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Errorf("public endpoint expected 200, got %d", rec1.Code)
	}

	// 2. 受保护接口无 Token 返回 401
	req2 := httptest.NewRequest("GET", "/protected/data", nil)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated expected 401, got %d", rec2.Code)
	}
	var errResp map[string]interface{}
	_ = json.Unmarshal(rec2.Body.Bytes(), &errResp)
	if errResp["error"] != "unauthorized" {
		t.Errorf("expected error=unauthorized, got %v", errResp["error"])
	}

	// 3. 携带无效 Token 返回 401
	req3 := httptest.NewRequest("GET", "/protected/data", nil)
	req3.Header.Set("Authorization", "Bearer invalid-12345")
	rec3 := httptest.NewRecorder()
	r.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusUnauthorized {
		t.Errorf("invalid token expected 401, got %d", rec3.Code)
	}

	// 4. 携带有效管理员 Session Token 成功返回 200
	validTok := generateToken()
	req4 := httptest.NewRequest("GET", "/protected/data", nil)
	req4.Header.Set("Authorization", "Bearer "+validTok)
	rec4 := httptest.NewRecorder()
	r.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusOK {
		t.Errorf("valid token expected 200, got %d", rec4.Code)
	}

	// 5. 只读令牌 PUBLIC_READ_TOKEN 测试
	config.Global.PublicReadToken = "read-token-xyz"
	req5 := httptest.NewRequest("GET", "/protected/data", nil)
	req5.Header.Set("X-Api-Key", "read-token-xyz")
	rec5 := httptest.NewRecorder()
	r.ServeHTTP(rec5, req5)
	if rec5.Code != http.StatusOK {
		t.Errorf("read token GET expected 200, got %d", rec5.Code)
	}

	// 6. 只读令牌尝试 POST 写操作应被拦截返回 403 Forbidden
	req6 := httptest.NewRequest("POST", "/protected/write", strings.NewReader("{}"))
	req6.Header.Set("X-Api-Key", "read-token-xyz")
	rec6 := httptest.NewRecorder()
	r.ServeHTTP(rec6, req6)
	if rec6.Code != http.StatusForbidden {
		t.Errorf("read token POST expected 403, got %d", rec6.Code)
	}
}

func TestSessionPersistence(t *testing.T) {
	tok := generateToken()

	authMu.RLock()
	exp, exists := tokens[tok]
	authMu.RUnlock()
	if !exists || exp <= time.Now().Unix() {
		t.Fatalf("token should exist and not be expired")
	}

	// 模拟进程重启，重新执行 initSessions
	authMu.Lock()
	tokens = make(map[string]int64)
	authMu.Unlock()

	initSessions()

	authMu.RLock()
	expAfter, existsAfter := tokens[tok]
	authMu.RUnlock()

	if !existsAfter || expAfter != exp {
		t.Errorf("expected session token to survive restart from sessions.json")
	}
}
