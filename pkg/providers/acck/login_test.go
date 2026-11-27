package acck

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

func TestSolveAltchaChallengeV2(t *testing.T) {
	nonceHex := "ce6e13ac169d9a0572714a63"
	saltHex := "9d3539bfe11ab233bbb8c993"
	nonceBuf, _ := hex.DecodeString(nonceHex)
	saltBuf, _ := hex.DecodeString(saltHex)

	const targetCounter uint32 = 42
	pw := make([]byte, len(nonceBuf)+4)
	copy(pw, nonceBuf)
	binary.BigEndian.PutUint32(pw[len(nonceBuf):], targetCounter)
	expectedDK := pbkdf2.Key(pw, saltBuf, 100, 32, sha256.New)

	ch := &AltchaChallenge{
		Parameters: &AltchaChallengeParams{
			Algorithm: "PBKDF2/SHA-256",
			Nonce:     nonceHex,
			Salt:      saltHex,
			Cost:      100,
			KeyLength: 32,
			KeyPrefix: hex.EncodeToString(expectedDK[:16]),
			ExpiresAt: time.Now().Add(time.Minute).Unix(),
		},
		Signature: "sig-test",
	}

	b64Token, err := SolveAltchaChallenge(context.Background(), ch)
	if err != nil {
		t.Fatalf("SolveAltchaChallenge failed: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(b64Token)
	if err != nil {
		t.Fatalf("base64 decode failed: %v", err)
	}

	var decoded struct {
		Challenge AltchaChallenge `json:"challenge"`
		Solution  struct {
			Counter    uint32 `json:"counter"`
			DerivedKey string `json:"derivedKey"`
		} `json:"solution"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json unmarshal failed: %v", err)
	}
	if decoded.Solution.Counter != targetCounter {
		t.Fatalf("expected counter %d, got %d", targetCounter, decoded.Solution.Counter)
	}
	if decoded.Solution.DerivedKey != hex.EncodeToString(expectedDK) {
		t.Fatalf("derivedKey mismatch: got %s", decoded.Solution.DerivedKey)
	}
}

func TestSolveAltchaChallengeV1(t *testing.T) {
	salt := "testsalt:"
	const targetNum uint32 = 17
	sum := sha256.Sum256([]byte(salt + "17"))
	ch := &AltchaChallenge{
		Algorithm: "SHA-256",
		Challenge: hex.EncodeToString(sum[:]),
		Salt:      salt,
		Signature: "v1sig",
		MaxNumber: 1000,
	}

	b64Token, err := SolveAltchaChallenge(context.Background(), ch)
	if err != nil {
		t.Fatalf("v1 solve failed: %v", err)
	}
	raw, _ := base64.StdEncoding.DecodeString(b64Token)
	var out struct {
		Number uint32 `json:"number"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal v1 failed: %v", err)
	}
	if out.Number != targetNum {
		t.Fatalf("expected %d, got %d", targetNum, out.Number)
	}
}

func TestClientLoginWithAltchaAnd2FA(t *testing.T) {
	nonceHex := "0102030405060708090a0b0c"
	saltHex := "a1a2a3a4a5a6a7a8a9aaabac"
	nonceBuf, _ := hex.DecodeString(nonceHex)
	saltBuf, _ := hex.DecodeString(saltHex)
	pw := make([]byte, len(nonceBuf)+4)
	copy(pw, nonceBuf)
	binary.BigEndian.PutUint32(pw[len(nonceBuf):], 7)
	dk := pbkdf2.Key(pw, saltBuf, 50, 32, sha256.New)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/security/challenge", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "acck_device", Value: "dev-cookie-123", Path: "/"})
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"parameters": map[string]interface{}{
				"algorithm": "PBKDF2/SHA-256",
				"nonce":     nonceHex,
				"salt":      saltHex,
				"cost":      50,
				"keyLength": 32,
				"keyPrefix": hex.EncodeToString(dk[:12]),
			},
			"signature": "mock-sig",
		})
	})

	mux.HandleFunc("/api/user/login", func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("acck_device")
		if err != nil || cookie.Value != "dev-cookie-123" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing device cookie"})
			return
		}
		var body struct {
			Email          string `json:"email"`
			Password       string `json:"password"`
			TurnstileToken string `json:"turnstile_token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.TurnstileToken == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing turnstile_token"})
			return
		}
		if body.Email != "user@example.com" || body.Password != "secret123" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "邮箱或密码错误"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"requires_2fa": true,
			"mfa_token":    "mfa-temp-token",
			"message":      "请输入二步验证码",
		})
	})

	mux.HandleFunc("/api/user/login/2fa", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			MFAToken string `json:"mfa_token"`
			Code     string `json:"code"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.MFAToken != "mfa-temp-token" || body.Code != "654321" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "验证码错误"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"token":   "eyJhbGciOiJIUzI1NiJ9.eyJleHAiOjE5OTk5OTk5OTl9.sig",
			"message": "登录成功",
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cli := NewClientWithBaseURL(srv.URL+"/api", "", "")

	// 1. 无 2FA 动态码时应返回 need2FA=true
	_, need2FA, _, err := cli.Login(context.Background(), "user@example.com", "secret123", "")
	if err != nil || !need2FA {
		t.Fatalf("expected need2FA=true, got need2FA=%v err=%v", need2FA, err)
	}

	// 2. 错误密码应返回 ErrInvalidCredentials
	_, _, _, err = cli.Login(context.Background(), "user@example.com", "wrong", "")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}

	// 3. 正确账号密码 + 动态码应成功拿到 JWT
	tok, need2FA, _, err := cli.LoginWithOptions(context.Background(), "user@example.com", "secret123", func() (string, error) {
		return "654321", nil
	}, false)
	if err != nil || need2FA {
		t.Fatalf("expected login success, got need2FA=%v err=%v", need2FA, err)
	}
	if tok != "eyJhbGciOiJIUzI1NiJ9.eyJleHAiOjE5OTk5OTk5OTl9.sig" || cli.GetToken() != tok {
		t.Fatalf("unexpected token: %q", tok)
	}
}

func TestBuildCDPPageWSURLAndWindowGuard(t *testing.T) {
	wsURL, err := buildCDPPageWSURL("http://192.168.8.11:19223", "PAGE123")
	if err != nil {
		t.Fatalf("buildCDPPageWSURL failed: %v", err)
	}
	if wsURL != "ws://192.168.8.11:19223/devtools/page/PAGE123" {
		t.Fatalf("unexpected wsURL: %s", wsURL)
	}

	if !isSteelAutoWindowAllowed(time.Date(2026, 11, 27, 2, 30, 0, 0, time.Local)) {
		t.Fatal("expected 02:30 to be inside early morning Steel window")
	}
	if isSteelAutoWindowAllowed(time.Date(2026, 11, 27, 14, 0, 0, 0, time.Local)) {
		t.Fatal("expected 14:00 to be outside early morning Steel window")
	}
}

func TestClientLoginRetriesOnTransientEOF(t *testing.T) {
	nonceHex := "0102030405060708090a0b0c"
	saltHex := "a1a2a3a4a5a6a7a8a9aaabac"
	nonceBuf, _ := hex.DecodeString(nonceHex)
	saltBuf, _ := hex.DecodeString(saltHex)
	pw := make([]byte, len(nonceBuf)+4)
	copy(pw, nonceBuf)
	binary.BigEndian.PutUint32(pw[len(nonceBuf):], 3)
	dk := pbkdf2.Key(pw, saltBuf, 50, 32, sha256.New)

	var twofaAttempts atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/api/security/challenge", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"parameters": map[string]interface{}{
				"algorithm": "PBKDF2/SHA-256",
				"nonce":     nonceHex,
				"salt":      saltHex,
				"cost":      50,
				"keyLength": 32,
				"keyPrefix": hex.EncodeToString(dk[:12]),
			},
			"signature": "sig-eof-test",
		})
	})
	mux.HandleFunc("/api/user/login", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"requires_2fa": true,
			"mfa_token":    "mfa-eof-retry",
		})
	})
	mux.HandleFunc("/api/user/login/2fa", func(w http.ResponseWriter, r *http.Request) {
		if twofaAttempts.Add(1) == 1 {
			if hj, ok := w.(http.Hijacker); ok {
				if conn, _, err := hj.Hijack(); err == nil {
					_ = conn.Close()
					return
				}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"token": "eyJhbG.eofrecovered.sig",
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cli := NewClientWithBaseURL(srv.URL+"/api", "", "")
	tok, need2FA, _, err := cli.Login(context.Background(), "user@example.com", "secret123", "112233")
	if err != nil || need2FA {
		t.Fatalf("expected EOF retry success, got need2FA=%v err=%v", need2FA, err)
	}
	if tok != "eyJhbG.eofrecovered.sig" {
		t.Fatalf("unexpected token: %q", tok)
	}
	if twofaAttempts.Load() != 2 {
		t.Fatalf("expected 2 attempts on 2FA endpoint, got %d", twofaAttempts.Load())
	}
}
