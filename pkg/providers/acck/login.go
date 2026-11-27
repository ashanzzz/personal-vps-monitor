package acck

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

const acckDefaultUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36"

var (
	// ErrInvalidCredentials 标记明确的上游账号或密码错误，便于调用方避免无效重试撞库
	ErrInvalidCredentials = errors.New("acck invalid credentials")
	// ErrCaptchaRequired 标记需要浏览器人机校验 (如 Cloudflare Turnstile / WAF 拦截)
	ErrCaptchaRequired = errors.New("acck browser captcha challenge required")
)

// AltchaChallengeParams 对应 ACCK /api/security/challenge 返回的 Altcha v1/v2 参数
type AltchaChallengeParams struct {
	Algorithm string          `json:"algorithm,omitempty"`
	Nonce     string          `json:"nonce,omitempty"`
	Salt      string          `json:"salt,omitempty"`
	Cost      int             `json:"cost,omitempty"`
	KeyLength int             `json:"keyLength,omitempty"`
	KeyPrefix string          `json:"keyPrefix,omitempty"`
	ExpiresAt int64           `json:"expiresAt,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`

	// 兼容 Altcha v1 扁平字段
	Challenge string `json:"challenge,omitempty"`
	MaxNumber uint32 `json:"maxnumber,omitempty"`
	Version   int    `json:"_version,omitempty"`
}

// AltchaChallenge 对应 ACCK /api/security/challenge 响应结构
type AltchaChallenge struct {
	Parameters *AltchaChallengeParams `json:"parameters,omitempty"`
	Signature  string                 `json:"signature,omitempty"`

	// 兼容 Altcha v1 顶层结构
	Algorithm string `json:"algorithm,omitempty"`
	Challenge string `json:"challenge,omitempty"`
	Salt      string `json:"salt,omitempty"`
	MaxNumber uint32 `json:"maxnumber,omitempty"`
}

// SolveAltchaChallenge 原生 Go 多核求解 ACCK Altcha PoW 人机挑战，返回 turnstile_token (Base64 JSON)
func SolveAltchaChallenge(ctx context.Context, ch *AltchaChallenge) (string, error) {
	if ch == nil {
		return "", errors.New("empty altcha challenge")
	}

	// Altcha v2 (ACCK 当前采用 PBKDF2/SHA-256 + parameters 对象)
	if ch.Parameters != nil && ch.Parameters.Nonce != "" {
		return solveAltchaV2(ctx, ch)
	}

	// Altcha v1 兜底兼容
	alg := ch.Algorithm
	challengeHex := ch.Challenge
	salt := ch.Salt
	maxNum := ch.MaxNumber
	if ch.Parameters != nil {
		if alg == "" {
			alg = ch.Parameters.Algorithm
		}
		if challengeHex == "" {
			challengeHex = ch.Parameters.Challenge
		}
		if salt == "" {
			salt = ch.Parameters.Salt
		}
		if maxNum == 0 {
			maxNum = ch.Parameters.MaxNumber
		}
	}
	if challengeHex != "" && salt != "" {
		return solveAltchaV1(ctx, alg, challengeHex, salt, ch.Signature, maxNum)
	}

	return "", errors.New("unrecognized altcha challenge format")
}

func solveAltchaV2(ctx context.Context, ch *AltchaChallenge) (string, error) {
	t0 := time.Now()
	p := ch.Parameters

	nonceBuf, err := hex.DecodeString(strings.TrimSpace(p.Nonce))
	if err != nil {
		return "", fmt.Errorf("decode altcha nonce: %w", err)
	}
	saltBuf, err := hex.DecodeString(strings.TrimSpace(p.Salt))
	if err != nil {
		return "", fmt.Errorf("decode altcha salt: %w", err)
	}

	cost := p.Cost
	if cost <= 0 {
		cost = 5000
	}
	keyLen := p.KeyLength
	if keyLen <= 0 {
		keyLen = 32
	}

	alg := strings.ToUpper(strings.TrimSpace(p.Algorithm))
	var hashFn func() hash.Hash
	switch alg {
	case "PBKDF2/SHA-256", "SHA-256", "":
		hashFn = sha256.New
	case "PBKDF2/SHA-384", "SHA-384":
		hashFn = sha512.New384
	case "PBKDF2/SHA-512", "SHA-512":
		hashFn = sha512.New
	default:
		return "", fmt.Errorf("unsupported altcha algorithm: %s", p.Algorithm)
	}

	prefixHex := strings.ToLower(strings.TrimSpace(p.KeyPrefix))
	if prefixHex == "" {
		return "", errors.New("missing altcha keyPrefix")
	}
	var prefixBytes []byte
	evenPrefix := len(prefixHex)%2 == 0
	if evenPrefix {
		decoded, decErr := hex.DecodeString(prefixHex)
		if decErr == nil {
			prefixBytes = decoded
		} else {
			evenPrefix = false
		}
	}

	const maxCounter uint32 = 250000
	var nextCounter atomic.Uint32
	var found atomic.Bool
	var winCounter uint32
	var winDK []byte
	var winMu sync.Mutex

	workers := runtime.NumCPU()
	if workers < 4 {
		workers = 4
	}
	if workers > 8 {
		workers = 8
	}

	isPBKDF2 := strings.HasPrefix(alg, "PBKDF2/") || alg == ""
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pw := make([]byte, len(nonceBuf)+4)
			copy(pw, nonceBuf)

			for !found.Load() && ctx.Err() == nil {
				c := nextCounter.Add(1) - 1
				if c >= maxCounter {
					return
				}
				binary.BigEndian.PutUint32(pw[len(nonceBuf):], c)
				var dk []byte
				if isPBKDF2 {
					dk = pbkdf2.Key(pw, saltBuf, cost, keyLen, hashFn)
				} else {
					h := hashFn()
					h.Write(pw)
					dk = h.Sum(nil)
				}

				matched := false
				if evenPrefix {
					matched = bytes.HasPrefix(dk, prefixBytes)
				} else {
					matched = strings.HasPrefix(hex.EncodeToString(dk), prefixHex)
				}
				if matched {
					winMu.Lock()
					if !found.Load() {
						winCounter = c
						winDK = dk
						found.Store(true)
					}
					winMu.Unlock()
					return
				}
			}
		}()
	}
	wg.Wait()

	if !found.Load() {
		if ctx.Err() != nil {
			return "", fmt.Errorf("altcha solve canceled: %w", ctx.Err())
		}
		return "", fmt.Errorf("altcha v2 solution not found within %d attempts", maxCounter)
	}

	payload := struct {
		Challenge *AltchaChallenge `json:"challenge"`
		Solution  struct {
			Counter    uint32 `json:"counter"`
			DerivedKey string `json:"derivedKey"`
			Time       int64  `json:"time"`
		} `json:"solution"`
	}{
		Challenge: ch,
	}
	payload.Solution.Counter = winCounter
	payload.Solution.DerivedKey = hex.EncodeToString(winDK)
	payload.Solution.Time = time.Since(t0).Milliseconds()

	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

func solveAltchaV1(ctx context.Context, alg, challengeHex, salt, signature string, maxNum uint32) (string, error) {
	t0 := time.Now()
	if maxNum == 0 {
		maxNum = 1000000
	}
	algUpper := strings.ToUpper(strings.TrimSpace(alg))
	var hashFn func() hash.Hash
	switch algUpper {
	case "SHA-256", "":
		algUpper = "SHA-256"
		hashFn = sha256.New
	case "SHA-384":
		hashFn = sha512.New384
	case "SHA-512":
		hashFn = sha512.New
	default:
		return "", fmt.Errorf("unsupported altcha v1 algorithm: %s", alg)
	}

	target, err := hex.DecodeString(strings.ToLower(strings.TrimSpace(challengeHex)))
	if err != nil {
		return "", fmt.Errorf("decode v1 challenge: %w", err)
	}

	var nextCounter atomic.Uint32
	var found atomic.Bool
	var winNum uint32
	var winMu sync.Mutex

	workers := runtime.NumCPU()
	if workers < 2 {
		workers = 2
	}
	if workers > 8 {
		workers = 8
	}

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h := hashFn()
			for !found.Load() && ctx.Err() == nil {
				n := nextCounter.Add(1) - 1
				if n > maxNum {
					return
				}
				h.Reset()
				h.Write([]byte(salt))
				h.Write([]byte(strconv.FormatUint(uint64(n), 10)))
				if bytes.Equal(h.Sum(nil), target) {
					winMu.Lock()
					if !found.Load() {
						winNum = n
						found.Store(true)
					}
					winMu.Unlock()
					return
				}
			}
		}()
	}
	wg.Wait()

	if !found.Load() {
		if ctx.Err() != nil {
			return "", fmt.Errorf("altcha v1 solve canceled: %w", ctx.Err())
		}
		return "", fmt.Errorf("altcha v1 solution not found within %d", maxNum)
	}

	payload := map[string]interface{}{
		"algorithm": algUpper,
		"challenge": challengeHex,
		"number":    winNum,
		"salt":      salt,
		"signature": signature,
		"took":      time.Since(t0).Milliseconds(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

func (c *Client) originURL() string {
	u, err := url.Parse(c.baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "https://acck.io"
	}
	return u.Scheme + "://" + u.Host
}

func (c *Client) newLoginHTTPClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSHandshakeTimeout:   8 * time.Second,
		ResponseHeaderTimeout: 12 * time.Second,
		IdleConnTimeout:       15 * time.Second,
		DisableKeepAlives:     true,
	}
	if c.proxyURL != "" {
		if u, err := url.Parse(c.proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	} else if tr, ok := c.httpClient.Transport.(*http.Transport); ok && tr != nil && tr.Proxy != nil {
		transport.Proxy = tr.Proxy
	}
	return &http.Client{
		Jar:       jar,
		Transport: transport,
		Timeout:   25 * time.Second,
	}
}

func setACCKBrowserHeaders(req *http.Request, origin string, isJSON bool) {
	req.Header.Set("Origin", origin)
	req.Header.Set("Referer", origin+"/login")
	req.Header.Set("User-Agent", acckDefaultUA)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	if isJSON {
		req.Header.Set("Content-Type", "application/json")
	}
}

func isCloudflareBlockResponse(statusCode int, body []byte) bool {
	if statusCode != http.StatusForbidden && statusCode != http.StatusServiceUnavailable {
		return false
	}
	lower := strings.ToLower(string(body))
	return strings.Contains(lower, "just a moment") ||
		strings.Contains(lower, "cf-browser-verification") ||
		strings.Contains(lower, "challenge-platform") ||
		strings.Contains(lower, "attention required")
}

// Login 使用账号密码（及可选 6 位动态码）登录 ACCK
func (c *Client) Login(ctx context.Context, email, password, totpCode string) (token string, need2FA bool, msg string, err error) {
	return c.LoginWithOptions(ctx, email, password, func() (string, error) {
		return strings.TrimSpace(totpCode), nil
	}, false)
}

// LoginWithOptions 使用账号密码与动态 2FA 回调执行 ACCK 全自动登录；
// 优先纯 Go 解算 Altcha PoW + CookieJar；若上游临时开启 Turnstile/CF WAF 且允许使用 Steel CDP，自动降级至 Steel。
func (c *Client) LoginWithOptions(ctx context.Context, email, password string, totpProvider func() (string, error), isBackgroundAuto bool) (token string, need2FA bool, msg string, err error) {
	email = strings.TrimSpace(email)
	if email == "" || password == "" {
		return "", false, "", fmt.Errorf("%w: 邮箱与密码不能为空", ErrInvalidCredentials)
	}

	origin := c.originURL()
	httpCli := c.newLoginHTTPClient()

	tryAltchaLogin := func() (string, bool, string, error) {
		// 1. 拉取 Altcha 挑战（携带 CookieJar 自动承接设备 Cookie）
		chReq, reqErr := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/security/challenge?action=login", nil)
		if reqErr != nil {
			return "", false, "", reqErr
		}
		setACCKBrowserHeaders(chReq, origin, false)

		chResp, doErr := httpCli.Do(chReq)
		if doErr != nil {
			return "", false, "", fmt.Errorf("获取 ACCK 人机挑战失败: %w", doErr)
		}
		chBytes, _ := io.ReadAll(chResp.Body)
		chResp.Body.Close()

		if isCloudflareBlockResponse(chResp.StatusCode, chBytes) {
			return "", false, "", fmt.Errorf("%w: Cloudflare WAF intercepted challenge (HTTP %d)", ErrCaptchaRequired, chResp.StatusCode)
		}

		turnstileToken := ""
		if chResp.StatusCode == http.StatusOK {
			var ch AltchaChallenge
			if unmErr := json.Unmarshal(chBytes, &ch); unmErr == nil && (ch.Parameters != nil || ch.Challenge != "") {
				solved, solveErr := SolveAltchaChallenge(ctx, &ch)
				if solveErr != nil {
					return "", false, "", fmt.Errorf("解算 ACCK Altcha 人机挑战失败: %w", solveErr)
				}
				turnstileToken = solved
			}
		} else if chResp.StatusCode != http.StatusNotFound {
			return "", false, "", fmt.Errorf("ACCK 人机挑战接口返回异常 (HTTP %d): %s", chResp.StatusCode, strings.TrimSpace(string(chBytes)))
		}

		// 2. 提交账号密码登录
		loginBody, _ := json.Marshal(map[string]string{
			"email":           email,
			"password":        password,
			"turnstile_token": turnstileToken,
		})
		loginReq, reqErr := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/user/login", bytes.NewReader(loginBody))
		if reqErr != nil {
			return "", false, "", reqErr
		}
		setACCKBrowserHeaders(loginReq, origin, true)

		loginResp, doErr := httpCli.Do(loginReq)
		if doErr != nil {
			return "", false, "", fmt.Errorf("ACCK 登录请求失败: %w", doErr)
		}
		respBytes, _ := io.ReadAll(loginResp.Body)
		loginResp.Body.Close()

		if isCloudflareBlockResponse(loginResp.StatusCode, respBytes) {
			return "", false, "", fmt.Errorf("%w: Cloudflare WAF intercepted login (HTTP %d)", ErrCaptchaRequired, loginResp.StatusCode)
		}

		var res struct {
			Requires2FA             bool   `json:"requires_2fa"`
			Requires2FASetup        bool   `json:"requires_2fa_setup"`
			RequiresPasswordUpgrade bool   `json:"requires_password_upgrade"`
			MFAToken                string `json:"mfa_token"`
			Token                   string `json:"token"`
			Message                 string `json:"message"`
			Error                   string `json:"error"`
		}
		if jsonErr := json.Unmarshal(respBytes, &res); jsonErr != nil {
			return "", false, "", fmt.Errorf("解析 ACCK 登录响应失败 (HTTP %d): %w", loginResp.StatusCode, jsonErr)
		}

		if res.Error != "" {
			errLower := strings.ToLower(res.Error)
			if strings.Contains(res.Error, "邮箱或密码") || strings.Contains(res.Error, "密码错误") || strings.Contains(errLower, "invalid credentials") {
				return "", false, res.Error, fmt.Errorf("%w: %s", ErrInvalidCredentials, res.Error)
			}
			if strings.Contains(errLower, "turnstile") || strings.Contains(errLower, "altcha") || strings.Contains(res.Error, "人机") || strings.Contains(res.Error, "验证失败") {
				return "", false, res.Error, fmt.Errorf("%w: %s", ErrCaptchaRequired, res.Error)
			}
			return "", false, res.Error, fmt.Errorf("ACCK 登录失败 (HTTP %d): %s", loginResp.StatusCode, res.Error)
		}

		if res.RequiresPasswordUpgrade {
			return "", false, res.Message, errors.New("ACCK 要求升级账号密码，请先前往官网完成密码升级")
		}
		if res.Requires2FASetup {
			return "", false, res.Message, errors.New("ACCK 要求首次绑定 2FA 两步验证，请先在官网完成设置")
		}

		// 3. 如需 2FA，在拿到 mfa_token 的瞬间实时生成/读取 6 位动态码并提交
		if res.Requires2FA || res.MFAToken != "" {
			code := ""
			if totpProvider != nil {
				cStr, cErr := totpProvider()
				if cErr != nil {
					return "", false, "", fmt.Errorf("生成 ACCK 2FA 动态码失败: %w", cErr)
				}
				code = strings.TrimSpace(cStr)
			}
			if code == "" {
				hint := res.Message
				if hint == "" {
					hint = "该 ACCK 账号已开启 2FA 二次验证，请输入 6 位动态验证码或绑定 TOTP 密钥"
				}
				return "", true, hint, nil
			}

			mfaBody, _ := json.Marshal(map[string]string{
				"mfa_token": res.MFAToken,
				"code":      code,
			})
			mfaReq, reqErr := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/user/login/2fa", bytes.NewReader(mfaBody))
			if reqErr != nil {
				return "", false, "", reqErr
			}
			setACCKBrowserHeaders(mfaReq, origin, true)

			mfaResp, doErr := httpCli.Do(mfaReq)
			if doErr != nil {
				return "", false, "", fmt.Errorf("ACCK 2FA 验证请求失败: %w", doErr)
			}
			mfaBytes, _ := io.ReadAll(mfaResp.Body)
			mfaResp.Body.Close()

			var mfaRes struct {
				Token   string `json:"token"`
				Message string `json:"message"`
				Error   string `json:"error"`
			}
			if jsonErr := json.Unmarshal(mfaBytes, &mfaRes); jsonErr != nil {
				return "", false, "", fmt.Errorf("解析 ACCK 2FA 响应失败 (HTTP %d): %w", mfaResp.StatusCode, jsonErr)
			}
			if mfaRes.Error != "" {
				return "", false, mfaRes.Error, fmt.Errorf("ACCK 2FA 验证失败: %s", mfaRes.Error)
			}
			res.Token = mfaRes.Token
			if mfaRes.Message != "" {
				res.Message = mfaRes.Message
			}
		}

		tok := cleanToken(res.Token)
		if tok == "" || !strings.Contains(tok, ".") {
			return "", false, res.Message, fmt.Errorf("ACCK 登录响应未包含有效 JWT Token: %s", strings.TrimSpace(string(respBytes)))
		}
		c.SetToken(tok)
		if res.Message == "" {
			res.Message = "登录成功"
		}
		return tok, false, res.Message, nil
	}

	tok, need2FA, msg, err := tryAltchaLogin()
	if err == nil || need2FA || errors.Is(err, ErrInvalidCredentials) {
		return tok, need2FA, msg, err
	}

	// 若纯 Go 仅因人机挑战/Cloudflare 受阻，且显式配置了 STEEL_CDP_URL，按时间窗口规范可选走 Steel 兜底
	if errors.Is(err, ErrCaptchaRequired) {
		if steelURL := strings.TrimSpace(os.Getenv("STEEL_CDP_URL")); steelURL != "" {
			if isBackgroundAuto && !isSteelAutoWindowAllowed(time.Now()) {
				log.Printf("[ACCKLogin] 上游触发浏览器人机挑战，当前非凌晨时段 (00:00-06:00)，按资源约束跳过自动 Steel 调用")
				return "", false, msg, fmt.Errorf("%w (非凌晨时段禁止后台自动调起 Steel Browser)", err)
			}
			log.Printf("[ACCKLogin] 纯 Go 人机挑战受阻 (%v)，尝试通过 Steel Browser CDP (%s) 兜底登录...", err, steelURL)
			sTok, sNeed2FA, sMsg, sErr := c.loginViaSteelCDP(ctx, steelURL, email, password, totpProvider)
			if sErr == nil && sTok != "" {
				c.SetToken(sTok)
			}
			return sTok, sNeed2FA, sMsg, sErr
		}
	}

	return "", false, msg, err
}

func isSteelAutoWindowAllowed(now time.Time) bool {
	h := now.Hour()
	return h >= 0 && h < 6
}
