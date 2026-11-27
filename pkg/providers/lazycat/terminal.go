package lazycat

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许本地与内网反向代理 WebSocket 升级
	},
}

// TerminalExecResult 终端命令执行结构化结果
type TerminalExecResult struct {
	Output     string `json:"output"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
}

var ansiEscapeRegex = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]`)

func stripANSI(s string) string {
	return ansiEscapeRegex.ReplaceAllString(s, "")
}

func randomMarkerID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err == nil {
		return hex.EncodeToString(b)
	}
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

func (c *Client) proxyCandidates() []string {
	var list []string
	seen := make(map[string]bool)
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p != "" && !seen[p] {
			seen[p] = true
			list = append(list, p)
		}
	}
	add(c.proxyURL)
	if extra := os.Getenv("PROXY_FALLBACK_URLS"); extra != "" {
		for _, p := range strings.Split(extra, ",") {
			add(p)
		}
	}
	if len(list) == 0 {
		list = append(list, "")
	}
	return list
}

func formatWSDialError(err error, resp *http.Response, proxyURL string) error {
	if resp == nil {
		return fmt.Errorf("failed to dial upstream lazycat terminal (proxy=%s): %w", proxyURL, err)
	}
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	bodyStr := string(bodyBytes)
	if resp.StatusCode == http.StatusForbidden &&
		(strings.Contains(bodyStr, "Access Restricted by Region") ||
			strings.Contains(bodyStr, "Blocked by WAF") ||
			strings.Contains(bodyStr, "暂不支持当前区域访问")) {
		return fmt.Errorf(
			"Cloudflare WAF 区域拦截 (HTTP 403)：当前代理 (%s) 出站命中直连或受限地区节点，请检查网关 (192.168.8.11:9090) 是否切为海外节点",
			proxyURL,
		)
	}
	snippet := strings.TrimSpace(bodyStr)
	if len(snippet) > 300 {
		snippet = snippet[:300] + "..."
	}
	return fmt.Errorf("failed to dial upstream lazycat terminal (HTTP %d, proxy=%s): %v %s", resp.StatusCode, proxyURL, err, snippet)
}

func (c *Client) dialTerminalWS(ctx context.Context, instanceID string, handshakeTimeout time.Duration) (*websocket.Conn, error) {
	upstreamURL := fmt.Sprintf("wss://dash.lazycat.wiki/api/ws/instances/%s/terminal?token=%s", instanceID, url.QueryEscape(c.token))
	headers := http.Header{}
	headers.Set("Origin", "https://dash.lazycat.wiki")
	headers.Set("Referer", "https://dash.lazycat.wiki/")
	headers.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/126.0 Safari/537.36")

	var lastErr error
	for _, pURL := range c.proxyCandidates() {
		dialer := websocket.Dialer{
			HandshakeTimeout: handshakeTimeout,
			TLSClientConfig:  &tls.Config{InsecureSkipVerify: false},
		}
		if pURL != "" {
			if u, parseErr := url.Parse(pURL); parseErr == nil {
				dialer.Proxy = http.ProxyURL(u)
			}
		}

		conn, resp, err := dialer.DialContext(ctx, upstreamURL, headers)
		if err == nil {
			return conn, nil
		}
		lastErr = formatWSDialError(err, resp, pURL)
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}
	return nil, lastErr
}

// ProxyTerminalWebSocket 实现浏览器与 dash.lazycat.wiki 终端 WebSocket 的原生双向流式隧道
func (c *Client) ProxyTerminalWebSocket(w http.ResponseWriter, r *http.Request, instanceID string) error {
	if c.token == "" {
		http.Error(w, "lazycat token not configured", http.StatusUnauthorized)
		return fmt.Errorf("lazycat token not configured")
	}

	// 1. Upgrade client HTTP request to WebSocket
	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return fmt.Errorf("upgrade client ws error: %w", err)
	}
	defer clientConn.Close()

	// 2. Setup Upstream Dialer (脱离短请求 timeout，使用独立拨号超时)
	dialCtx, dialCancel := context.WithTimeout(context.Background(), 15*time.Second)
	upstreamConn, err := c.dialTerminalWS(dialCtx, instanceID, 15*time.Second)
	dialCancel()
	if err != nil {
		errMsg := err.Error()
		_ = clientConn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31m[VPS Hub] "+errMsg+"\x1b[0m\r\n"))
		time.Sleep(300 * time.Millisecond)
		return err
	}
	defer upstreamConn.Close()

	var clientWriteMu sync.Mutex
	var upstreamWriteMu sync.Mutex
	writeClient := func(msgType int, data []byte) error {
		clientWriteMu.Lock()
		defer clientWriteMu.Unlock()
		return clientConn.WriteMessage(msgType, data)
	}
	writeUpstream := func(msgType int, data []byte) error {
		upstreamWriteMu.Lock()
		defer upstreamWriteMu.Unlock()
		return upstreamConn.WriteMessage(msgType, data)
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(120 * time.Second))
	clientConn.SetPongHandler(func(string) error {
		_ = clientConn.SetReadDeadline(time.Now().Add(120 * time.Second))
		return nil
	})
	_ = upstreamConn.SetReadDeadline(time.Now().Add(120 * time.Second))
	upstreamConn.SetPongHandler(func(string) error {
		_ = upstreamConn.SetReadDeadline(time.Now().Add(120 * time.Second))
		return nil
	})

	done := make(chan struct{})
	errChan := make(chan error, 3)

	// Keepalive Ping 防止 Nginx / 中间代理空闲静默断连
	go func() {
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				clientWriteMu.Lock()
				err1 := clientConn.WriteControl(websocket.PingMessage, []byte("keepalive"), time.Now().Add(5*time.Second))
				clientWriteMu.Unlock()
				if err1 != nil {
					errChan <- err1
					return
				}
				upstreamWriteMu.Lock()
				err2 := upstreamConn.WriteControl(websocket.PingMessage, []byte("keepalive"), time.Now().Add(5*time.Second))
				upstreamWriteMu.Unlock()
				if err2 != nil {
					errChan <- err2
					return
				}
			}
		}
	}()

	// Pump Client -> Upstream
	go func() {
		for {
			msgType, msg, err := clientConn.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			_ = clientConn.SetReadDeadline(time.Now().Add(120 * time.Second))
			if err := writeUpstream(msgType, msg); err != nil {
				errChan <- err
				return
			}
		}
	}()

	// Pump Upstream -> Client
	go func() {
		for {
			msgType, msg, err := upstreamConn.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			_ = upstreamConn.SetReadDeadline(time.Now().Add(120 * time.Second))
			if err := writeClient(msgType, msg); err != nil {
				errChan <- err
				return
			}
		}
	}()

	<-errChan
	close(done)
	return nil
}

// buildSentinelCommand 构造防 PTY 回显误触发的起止双哨兵命令，精确捕获标准输出与退出码
func buildSentinelCommand(cmd, markerID string) string {
	trimmed := strings.TrimSpace(cmd)
	var inner string
	if !strings.ContainsAny(trimmed, "\r\n") && len(trimmed) < 2048 {
		escaped := strings.ReplaceAll(trimmed, "'", "'\"'\"'")
		inner = fmt.Sprintf("sh -c '%s'", escaped)
	} else {
		encoded := base64.StdEncoding.EncodeToString([]byte(trimmed + "\n"))
		inner = fmt.Sprintf("printf '%%s' '%s' | base64 -d | sh", encoded)
	}
	// 用 '' 拆分标记字面量，确保 PTY 键盘回显本身绝不会匹配真实输出的 BEGIN/END 行
	return fmt.Sprintf(
		"stty -echo 2>/dev/null; printf '\\n__LZ_BEG''IN_%s__\\n'; %s; __lz_rc=$?; printf '\\n__LZ_E''ND_%s:%%d__\\n' \"$__lz_rc\"\r",
		markerID, inner, markerID,
	)
}

// parseSentinelOutput 从累积 PTY 流中提取 BEGIN 与 END 哨兵之间的纯净输出与退出码
func parseSentinelOutput(raw, markerID string) (output string, exitCode int, complete bool) {
	cleaned := stripANSI(raw)
	cleaned = strings.ReplaceAll(cleaned, "\r\n", "\n")
	cleaned = strings.ReplaceAll(cleaned, "\r", "\n")

	endRe := regexp.MustCompile(`(?m)^__LZ_END_` + regexp.QuoteMeta(markerID) + `:(-?\d+)__[ \t]*$`)
	endLoc := endRe.FindStringSubmatchIndex(cleaned)
	if endLoc == nil {
		return "", -1, false
	}

	code, err := strconv.Atoi(cleaned[endLoc[2]:endLoc[3]])
	if err != nil {
		code = -1
	}

	beforeEnd := cleaned[:endLoc[0]]
	beginRe := regexp.MustCompile(`(?m)^__LZ_BEGIN_` + regexp.QuoteMeta(markerID) + `__[ \t]*$`)
	beginLoc := beginRe.FindStringIndex(beforeEnd)
	var body string
	if beginLoc != nil {
		body = beforeEnd[beginLoc[1]:]
	} else {
		body = beforeEnd
	}

	body = strings.TrimLeft(body, "\n")
	body = strings.TrimRight(body, "\n")
	return body, code, true
}

// ExecTerminalCommandWithResult 纯 Go 原生执行单条终端指令 (起止双哨兵协议，返回真实输出、退出码与耗时)
func (c *Client) ExecTerminalCommandWithResult(ctx context.Context, instanceID, cmd string, timeout time.Duration) (*TerminalExecResult, error) {
	if c.token == "" {
		return nil, fmt.Errorf("lazycat token not configured")
	}
	if strings.TrimSpace(cmd) == "" {
		return nil, fmt.Errorf("command cannot be empty")
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	startTime := time.Now()
	dialTimeout := 10 * time.Second
	if timeout < dialTimeout {
		dialTimeout = timeout
	}

	conn, err := c.dialTerminalWS(ctx, instanceID, dialTimeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// 1. Send resize
	resizeMsg, _ := json.Marshal(map[string]interface{}{"type": "resize", "cols": 200, "rows": 60})
	_ = conn.WriteMessage(websocket.TextMessage, resizeMsg)

	// 单一持续读协程：gorilla/websocket 一旦因 SetReadDeadline 超时返回错误，连接内部 readErr 会永久锁死，
	// 因此绝不能在 MOTD 静默等待阶段使用 SetReadDeadline，改用 channel + select 计时器。
	type wsReadMsg struct {
		data []byte
		err  error
	}
	msgCh := make(chan wsReadMsg, 64)
	_ = conn.SetReadDeadline(time.Now().Add(timeout + 8*time.Second))
	go func() {
		defer close(msgCh)
		for {
			_, data, err := conn.ReadMessage()
			msgCh <- wsReadMsg{data: data, err: err}
			if err != nil {
				return
			}
		}
	}()

	// 2. 等待 PTY 会话就绪并吸纳初始欢迎横幅 (MOTD)，避免命令与初始化输出竞争
	motdMaxTimer := time.NewTimer(2200 * time.Millisecond)
	defer motdMaxTimer.Stop()
	var settleTimer *time.Timer
	var settleCh <-chan time.Time
	resetSettle := func(d time.Duration) {
		if settleTimer == nil {
			settleTimer = time.NewTimer(d)
			settleCh = settleTimer.C
			return
		}
		if !settleTimer.Stop() {
			select {
			case <-settleTimer.C:
			default:
			}
		}
		settleTimer.Reset(d)
	}
	defer func() {
		if settleTimer != nil {
			settleTimer.Stop()
		}
	}()

	var motdBuf strings.Builder
motdLoop:
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-motdMaxTimer.C:
			break motdLoop
		case <-settleCh:
			break motdLoop
		case m, ok := <-msgCh:
			if !ok {
				return nil, fmt.Errorf("terminal ws closed during handshake/motd")
			}
			if m.err != nil {
				return nil, fmt.Errorf("terminal ws read error during motd: %w", m.err)
			}
			s := string(m.data)
			if strings.Contains(s, `"type":"connected"`) {
				// 收到 connected 后最多再等 1200ms 等候首批 MOTD，如果已有数据则按 350ms 静默判定完成
				resetSettle(1200 * time.Millisecond)
				continue
			}
			if len(m.data) > 0 {
				motdBuf.Write(m.data)
				cleanedMOTD := stripANSI(motdBuf.String())
				if strings.Contains(cleanedMOTD, "请输入 (Yes/No):") || strings.Contains(cleanedMOTD, "服务条款确认") {
					return nil, fmt.Errorf("实例 %s 尚未完成首次登录服务条款确认，请先在 WebShell 中输入 Yes 确认后再执行自动化指令", instanceID)
				}
				resetSettle(350 * time.Millisecond)
			}
		}
	}

	// 3. 发送防回显误触发的双哨兵指令
	markerID := randomMarkerID()
	execCmd := buildSentinelCommand(cmd, markerID)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(execCmd)); err != nil {
		return nil, fmt.Errorf("send cmd error: %w", err)
	}

	// 4. 持续读取直到完整匹配 END 哨兵或超时
	execTimer := time.NewTimer(timeout)
	defer execTimer.Stop()

	var buf strings.Builder
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-execTimer.C:
			out, code, ok := parseSentinelOutput(buf.String(), markerID)
			if ok {
				return &TerminalExecResult{
					Output:     out,
					ExitCode:   code,
					DurationMs: time.Since(startTime).Milliseconds(),
				}, nil
			}
			return nil, fmt.Errorf("terminal exec timeout after %s, partial output: %s", timeout, strings.TrimSpace(stripANSI(buf.String())))
		case m, ok := <-msgCh:
			if !ok || m.err != nil {
				out, code, parsedOK := parseSentinelOutput(buf.String(), markerID)
				if parsedOK {
					return &TerminalExecResult{
						Output:     out,
						ExitCode:   code,
						DurationMs: time.Since(startTime).Milliseconds(),
					}, nil
				}
				if m.err != nil {
					return nil, fmt.Errorf("terminal exec read error (%w), partial output: %s", m.err, strings.TrimSpace(stripANSI(buf.String())))
				}
				return nil, fmt.Errorf("terminal ws closed before completion, partial output: %s", strings.TrimSpace(stripANSI(buf.String())))
			}
			buf.Write(m.data)
			if out, code, parsedOK := parseSentinelOutput(buf.String(), markerID); parsedOK {
				return &TerminalExecResult{
					Output:     out,
					ExitCode:   code,
					DurationMs: time.Since(startTime).Milliseconds(),
				}, nil
			}
		}
	}
}

// ExecTerminalCommand 纯 Go 原生执行单条终端指令 (向后兼容包装)
func (c *Client) ExecTerminalCommand(ctx context.Context, instanceID, cmd string, timeout time.Duration) (string, error) {
	res, err := c.ExecTerminalCommandWithResult(ctx, instanceID, cmd, timeout)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return res.Output, fmt.Errorf("command exited with code %d: %s", res.ExitCode, res.Output)
	}
	return res.Output, nil
}
