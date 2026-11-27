package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/config"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers/acck"
	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/pbkdf2"
)

func TestHandleAcckLoginAndQuickLogin(t *testing.T) {
	tmpDir := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWD) }()

	// 构建 mock ACCK 上游 (challenge -> login -> 2fa -> user/info)
	nonceHex := "11223344556677889900aabb"
	saltHex := "ccddeeff0011223344556677"
	nonceBuf, _ := hex.DecodeString(nonceHex)
	saltBuf, _ := hex.DecodeString(saltHex)
	pw := make([]byte, len(nonceBuf)+4)
	copy(pw, nonceBuf)
	binary.BigEndian.PutUint32(pw[len(nonceBuf):], 5)
	dk := pbkdf2.Key(pw, saltBuf, 50, 32, sha256.New)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/security/challenge", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "acck_dev", Value: "cookie-ok", Path: "/"})
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"parameters": map[string]interface{}{
				"algorithm": "PBKDF2/SHA-256",
				"nonce":     nonceHex,
				"salt":      saltHex,
				"cost":      50,
				"keyLength": 32,
				"keyPrefix": hex.EncodeToString(dk[:12]),
			},
			"signature": "sig-api-test",
		})
	})
	mux.HandleFunc("/api/user/login", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("acck_dev"); err != nil || c.Value != "cookie-ok" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing cookie"})
			return
		}
		var req struct {
			Email          string `json:"email"`
			Password       string `json:"password"`
			TurnstileToken string `json:"turnstile_token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.TurnstileToken == "" || req.Email != "user@example.com" || req.Password != "pass123" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "邮箱或密码错误"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"requires_2fa": true,
			"mfa_token":    "mfa-api-tok",
		})
	})
	mux.HandleFunc("/api/user/login/2fa", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			MFAToken string `json:"mfa_token"`
			Code     string `json:"code"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.MFAToken != "mfa-api-tok" || req.Code != "123456" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "bad 2fa code"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"token": "hdr.eyJleHAiOjE5MDAwMDAwMDB9.sig",
		})
	})
	mux.HandleFunc("/api/auth/user/info", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":       19440,
			"username": "ashan",
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	p, ok := providers.Get("acck")
	if !ok {
		t.Fatalf("expected acck provider registered via init()")
	}
	acckP, ok := p.(*acck.ACCKProvider)
	if !ok {
		t.Fatalf("unexpected acck provider type %T", p)
	}
	acckP.SetClientForTest(acck.NewClientWithBaseURL(srv.URL+"/api", "", ""))
	defer func() {
		acckP.SetClientForTest(acck.NewClient("", ""))
	}()

	// 清空测试期间凭据缓存文件
	_ = os.MkdirAll(filepath.Join(tmpDir, "data"), 0755)
	config.SetProviderToken("acck", "")

	r := chi.NewRouter()
	r.Post("/api/acck/login", handleAcckLogin)
	r.Post("/api/vendor/quick-login/{provider}", handleVendorQuickLogin)

	// 1. 测试不带动态码登录 -> 返回 need_2fa=true
	body1 := `{"username":"user@example.com","password":"pass123"}`
	req1 := httptest.NewRequest("POST", "/api/acck/login", bytes.NewBufferString(body1))
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200 on need_2fa, got %d: %s", w1.Code, w1.Body.String())
	}
	var res1 map[string]interface{}
	_ = json.Unmarshal(w1.Body.Bytes(), &res1)
	if res1["need_2fa"] != true {
		t.Fatalf("expected need_2fa=true, got %v", res1)
	}

	// 2. 测试带动态码并勾选 save_credentials -> 登录成功且持久化保存凭据与 Token
	body2 := `{"username":"user@example.com","password":"pass123","ga_code":"123456","totp_secret":"JBSWY3DPEHPK3PXP","save_credentials":true}`
	req2 := httptest.NewRequest("POST", "/api/acck/login", bytes.NewBufferString(body2))
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 on login, got %d: %s", w2.Code, w2.Body.String())
	}
	var res2 map[string]interface{}
	_ = json.Unmarshal(w2.Body.Bytes(), &res2)
	if res2["status"] != "success" || res2["token"] != "hdr.eyJleHAiOjE5MDAwMDAwMDB9.sig" {
		t.Fatalf("unexpected login resp: %v", res2)
	}
	if config.GetProviderToken("acck") != "hdr.eyJleHAiOjE5MDAwMDAwMDB9.sig" {
		t.Fatalf("expected Global.ACCKToken updated, got %q", config.GetProviderToken("acck"))
	}
	if st := config.LoadSavedTokens(); st.ACCKToken != "hdr.eyJleHAiOjE5MDAwMDAwMDB9.sig" {
		t.Fatalf("expected saved tokens ACCKToken updated, got %q", st.ACCKToken)
	}
	if sc := config.LoadSavedCredentials(); sc.ACCK == nil || sc.ACCK.TOTPSecret != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("expected saved ACCK credentials with TOTP secret, got %+v", sc.ACCK)
	}

	// 3. 测试 quick-login
	req3 := httptest.NewRequest("POST", "/api/vendor/quick-login/acck", bytes.NewBufferString(`{"ga_code":"123456"}`))
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200 on quick-login, got %d: %s", w3.Code, w3.Body.String())
	}
	_ = time.Second
}
