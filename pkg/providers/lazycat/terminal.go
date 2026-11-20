package lazycat

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
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

	// 2. Setup Upstream Dialer
	upstreamURL := fmt.Sprintf("wss://dash.lazycat.wiki/api/ws/instances/%s/terminal?token=%s", instanceID, url.QueryEscape(c.token))
	dialer := websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: false},
	}
	if c.proxyURL != "" {
		if u, parseErr := url.Parse(c.proxyURL); parseErr == nil {
			dialer.Proxy = http.ProxyURL(u)
		}
	}

	headers := http.Header{}
	headers.Set("Origin", "https://dash.lazycat.wiki")
	headers.Set("Referer", "https://dash.lazycat.wiki/")
	headers.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/126.0 Safari/537.36")

	upstreamConn, resp, err := dialer.Dial(upstreamURL, headers)
	if err != nil {
		errMsg := fmt.Sprintf("failed to dial upstream lazycat terminal: %v", err)
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			errMsg += " " + string(body)
		}
		_ = clientConn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31m[VPS Hub] "+errMsg+"\x1b[0m\r\n"))
		time.Sleep(300 * time.Millisecond)
		return fmt.Errorf("%s", errMsg)
	}
	defer upstreamConn.Close()

	errChan := make(chan error, 2)

	// Pump Client -> Upstream
	go func() {
		for {
			msgType, msg, err := clientConn.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			if err := upstreamConn.WriteMessage(msgType, msg); err != nil {
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
			if err := clientConn.WriteMessage(msgType, msg); err != nil {
				errChan <- err
				return
			}
		}
	}()

	<-errChan
	return nil
}

// ExecTerminalCommand 纯 Go 原生执行单条终端指令 (Marker 协议，自动剥离 ANSI 文本)
func (c *Client) ExecTerminalCommand(ctx context.Context, instanceID, cmd string, timeout time.Duration) (string, error) {
	if c.token == "" {
		return "", fmt.Errorf("lazycat token not configured")
	}

	upstreamURL := fmt.Sprintf("wss://dash.lazycat.wiki/api/ws/instances/%s/terminal?token=%s", instanceID, url.QueryEscape(c.token))
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: false},
	}
	if c.proxyURL != "" {
		if u, parseErr := url.Parse(c.proxyURL); parseErr == nil {
			dialer.Proxy = http.ProxyURL(u)
		}
	}

	headers := http.Header{}
	headers.Set("Origin", "https://dash.lazycat.wiki")
	headers.Set("Referer", "https://dash.lazycat.wiki/")
	headers.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/126.0 Safari/537.36")

	conn, _, err := dialer.DialContext(ctx, upstreamURL, headers)
	if err != nil {
		return "", fmt.Errorf("dial terminal ws error: %w", err)
	}
	defer conn.Close()

	// 1. Send resize
	resizeMsg, _ := json.Marshal(map[string]interface{}{"type": "resize", "cols": 200, "rows": 60})
	_ = conn.WriteMessage(websocket.TextMessage, resizeMsg)
	time.Sleep(300 * time.Millisecond)

	// 等待 PTY Shell 建立并输出 motd
	time.Sleep(800 * time.Millisecond)

	marker := fmt.Sprintf("__LZ_DONE_%d__", time.Now().UnixNano()%1000000)
	execCmd := fmt.Sprintf("%s && echo %s\r", strings.TrimSpace(cmd), marker)

	if err := conn.WriteMessage(websocket.TextMessage, []byte(execCmd)); err != nil {
		return "", fmt.Errorf("send cmd error: %w", err)
	}

	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	deadline := time.Now().Add(timeout)
	_ = conn.SetReadDeadline(deadline)

	var buf strings.Builder
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		buf.Write(msg)
		if strings.Contains(buf.String(), marker) {
			time.Sleep(200 * time.Millisecond)
			break
		}
	}

	raw := buf.String()
	// 剥除 ANSI 控制字符与 marker 行
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b\][^\x07]*\x07`)
	cleaned := ansiRegex.ReplaceAllString(raw, "")

	lines := strings.Split(cleaned, "\n")
	var resultLines []string
	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		if !strings.Contains(trimmed, marker) && !strings.Contains(trimmed, `{"type":"connected"`) {
			resultLines = append(resultLines, trimmed)
		}
	}

	return strings.TrimSpace(strings.Join(resultLines, "\n")), nil
}
