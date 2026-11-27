package acck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type cdpSession struct {
	conn  *websocket.Conn
	msgID int
}

func newCDPSession(ctx context.Context, wsURL string) (*cdpSession, error) {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, err
	}
	return &cdpSession{conn: conn, msgID: 1}, nil
}

func (s *cdpSession) close() {
	if s.conn != nil {
		_ = s.conn.Close()
	}
}

func (s *cdpSession) call(ctx context.Context, method string, params map[string]interface{}) (json.RawMessage, error) {
	id := s.msgID
	s.msgID++

	if deadline, ok := ctx.Deadline(); ok {
		_ = s.conn.SetWriteDeadline(deadline)
		_ = s.conn.SetReadDeadline(deadline)
	} else {
		_ = s.conn.SetWriteDeadline(time.Now().Add(20 * time.Second))
		_ = s.conn.SetReadDeadline(time.Now().Add(20 * time.Second))
	}

	req := map[string]interface{}{
		"id":     id,
		"method": method,
	}
	if params != nil {
		req["params"] = params
	}
	if err := s.conn.WriteJSON(req); err != nil {
		return nil, err
	}

	for {
		_, msgBytes, err := s.conn.ReadMessage()
		if err != nil {
			return nil, err
		}
		var resp struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(msgBytes, &resp); err != nil {
			continue
		}
		if resp.ID != id {
			continue
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("cdp %s error %d: %s", method, resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	}
}

func (s *cdpSession) eval(ctx context.Context, expr string) (json.RawMessage, error) {
	raw, err := s.call(ctx, "Runtime.evaluate", map[string]interface{}{
		"expression":    expr,
		"awaitPromise":  true,
		"returnByValue": true,
	})
	if err != nil {
		return nil, err
	}
	var res struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	if res.ExceptionDetails != nil {
		return nil, fmt.Errorf("js exception: %s", res.ExceptionDetails.Text)
	}
	return res.Result.Value, nil
}

// buildCDPPageWSURL 严格根据配置的 host:port 拼接页面 WebSocket 地址，
// 不直接使用 Steel /json/list 返回的默认无端口 webSocketDebuggerUrl。
func buildCDPPageWSURL(cdpBaseURL, pageID string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(cdpBaseURL))
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid STEEL_CDP_URL: %q", cdpBaseURL)
	}
	wsScheme := "ws"
	if u.Scheme == "https" || u.Scheme == "wss" {
		wsScheme = "wss"
	}
	host := u.Host
	if _, _, splitErr := net.SplitHostPort(host); splitErr != nil {
		if wsScheme == "wss" {
			host = net.JoinHostPort(u.Hostname(), "443")
		} else {
			host = net.JoinHostPort(u.Hostname(), "80")
		}
	}
	return fmt.Sprintf("%s://%s/devtools/page/%s", wsScheme, host, pageID), nil
}

func (c *Client) loginViaSteelCDP(ctx context.Context, cdpBaseURL, email, password string, totpProvider func() (string, error)) (string, bool, string, error) {
	baseHTTP := strings.TrimRight(strings.TrimSpace(cdpBaseURL), "/")
	if strings.HasPrefix(baseHTTP, "ws://") {
		baseHTTP = "http://" + strings.TrimPrefix(baseHTTP, "ws://")
	} else if strings.HasPrefix(baseHTTP, "wss://") {
		baseHTTP = "https://" + strings.TrimPrefix(baseHTTP, "wss://")
	}

	// 内网 Steel CDP 请求直连，不走代理
	localClient := &http.Client{
		Timeout:   15 * time.Second,
		Transport: &http.Transport{Proxy: nil},
	}

	reqNew, err := http.NewRequestWithContext(ctx, "PUT", baseHTTP+"/json/new", nil)
	if err != nil {
		return "", false, "", err
	}
	respNew, err := localClient.Do(reqNew)
	if err != nil {
		return "", false, "", fmt.Errorf("创建 Steel CDP 页面失败: %w", err)
	}
	bodyNew, _ := io.ReadAll(respNew.Body)
	respNew.Body.Close()

	var pg struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(bodyNew, &pg); err != nil || pg.ID == "" {
		return "", false, "", fmt.Errorf("未能解析 Steel CDP 页面 ID: %s", string(bodyNew))
	}

	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if closeReq, err := http.NewRequestWithContext(closeCtx, "GET", baseHTTP+"/json/close/"+pg.ID, nil); err == nil {
			if closeResp, err := localClient.Do(closeReq); err == nil {
				closeResp.Body.Close()
			}
		}
	}()

	wsURL, err := buildCDPPageWSURL(baseHTTP, pg.ID)
	if err != nil {
		return "", false, "", err
	}
	sess, err := newCDPSession(ctx, wsURL)
	if err != nil {
		return "", false, "", fmt.Errorf("连接 Steel CDP WebSocket 失败: %w", err)
	}
	defer sess.close()

	if _, err := sess.call(ctx, "Page.enable", nil); err != nil {
		return "", false, "", err
	}

	origin := c.originURL()
	if _, err := sess.call(ctx, "Page.navigate", map[string]interface{}{"url": origin + "/login"}); err != nil {
		return "", false, "", err
	}

	// 等待 Turnstile / Altcha 隐藏控件完成计算
	var captchaToken string
	for i := 0; i < 20; i++ {
		if ctx.Err() != nil {
			return "", false, "", ctx.Err()
		}
		select {
		case <-ctx.Done():
			return "", false, "", ctx.Err()
		case <-time.After(1 * time.Second):
		}
		valRaw, evalErr := sess.eval(ctx, `(document.querySelector('input[name="cf-turnstile-response"]')?.value || document.querySelector('input[name="altcha"]')?.value || '')`)
		if evalErr != nil {
			continue
		}
		var val string
		if json.Unmarshal(valRaw, &val) == nil && strings.TrimSpace(val) != "" {
			captchaToken = strings.TrimSpace(val)
			break
		}
	}
	if captchaToken == "" {
		return "", false, "", errors.New("Steel CDP 未能在超时时间内完成 ACCK 页面人机校验")
	}

	emailJSON, _ := json.Marshal(email)
	pwJSON, _ := json.Marshal(password)
	tokJSON, _ := json.Marshal(captchaToken)

	loginExpr := fmt.Sprintf(`(async () => {
		const res = await fetch('/api/user/login', {
			method: 'POST',
			headers: {'Content-Type': 'application/json', 'Accept': 'application/json'},
			body: JSON.stringify({email: %s, password: %s, turnstile_token: %s})
		});
		return await res.json();
	})()`, string(emailJSON), string(pwJSON), string(tokJSON))

	rawLogin, err := sess.eval(ctx, loginExpr)
	if err != nil {
		return "", false, "", fmt.Errorf("Steel CDP 登录脚本执行失败: %w", err)
	}

	var loginRes struct {
		Requires2FA bool   `json:"requires_2fa"`
		MFAToken    string `json:"mfa_token"`
		Token       string `json:"token"`
		Message     string `json:"message"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(rawLogin, &loginRes); err != nil {
		return "", false, "", fmt.Errorf("解析 Steel CDP 登录结果失败: %w", err)
	}
	if loginRes.Error != "" {
		return "", false, loginRes.Error, fmt.Errorf("ACCK 登录失败: %s", loginRes.Error)
	}

	if loginRes.Requires2FA || loginRes.MFAToken != "" {
		code := ""
		if totpProvider != nil {
			cStr, cErr := totpProvider()
			if cErr != nil {
				return "", false, "", fmt.Errorf("生成 ACCK 2FA 动态码失败: %w", cErr)
			}
			code = strings.TrimSpace(cStr)
		}
		if code == "" {
			return "", true, "该 ACCK 账号已开启 2FA 二次验证，请输入 6 位动态验证码或绑定 TOTP 密钥", nil
		}

		mfaTokJSON, _ := json.Marshal(loginRes.MFAToken)
		codeJSON, _ := json.Marshal(code)
		mfaExpr := fmt.Sprintf(`(async () => {
			const res = await fetch('/api/user/login/2fa', {
				method: 'POST',
				headers: {'Content-Type': 'application/json', 'Accept': 'application/json'},
				body: JSON.stringify({mfa_token: %s, code: %s})
			});
			return await res.json();
		})()`, string(mfaTokJSON), string(codeJSON))

		rawMFA, err := sess.eval(ctx, mfaExpr)
		if err != nil {
			return "", false, "", fmt.Errorf("Steel CDP 2FA 执行失败: %w", err)
		}
		var mfaRes struct {
			Token   string `json:"token"`
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		if err := json.Unmarshal(rawMFA, &mfaRes); err != nil {
			return "", false, "", fmt.Errorf("解析 Steel CDP 2FA 响应失败: %w", err)
		}
		if mfaRes.Error != "" {
			return "", false, mfaRes.Error, fmt.Errorf("ACCK 2FA 验证失败: %s", mfaRes.Error)
		}
		loginRes.Token = mfaRes.Token
		if mfaRes.Message != "" {
			loginRes.Message = mfaRes.Message
		}
	}

	tok := cleanToken(loginRes.Token)
	if tok == "" || !strings.Contains(tok, ".") {
		return "", false, loginRes.Message, errors.New("Steel CDP 登录未返回有效 JWT Token")
	}
	if loginRes.Message == "" {
		loginRes.Message = "登录成功 (Steel)"
	}
	return tok, false, loginRes.Message, nil
}
