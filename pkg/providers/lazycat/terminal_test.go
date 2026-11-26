package lazycat

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestBuildAndParseSentinel_IgnoresPTYEchoAndMOTD(t *testing.T) {
	markerID := "abc123def456"
	sent := buildSentinelCommand("echo __ROUND_1__ && uname -r", markerID)

	// Confirm the literal begin/end marker tokens never appear in the typed command
	if strings.Contains(sent, "__LZ_BEGIN_"+markerID+"__") {
		t.Fatalf("sent command must split BEGIN token so PTY echo cannot match prematurely: %s", sent)
	}
	if strings.Contains(sent, "__LZ_END_"+markerID+":") {
		t.Fatalf("sent command must split END token so PTY echo cannot match prematurely: %s", sent)
	}

	// Simulate realistic PTY stream: MOTD banner + keyboard echo of `sent` + real output + real sentinel
	rawStream := "" +
		"\x1b[36mLazyCatCloud Banner\x1b[0m\r\n" +
		"root@global-2:~# " + sent + "\n" +
		"__ROUND_1__\r\n" +
		"7.0.0-14-generic\r\n" +
		"\r\n__LZ_BEGIN_" + markerID + "__\r\n" +
		"__ROUND_1__\r\n" +
		"7.0.0-14-generic\r\n" +
		"\r\n__LZ_END_" + markerID + ":0__\r\n" +
		"root@global-2:~# "

	// Wait: in the simulation above, note that in real execution BEGIN prints BEFORE command output;
	// let's also test the exact real order: MOTD -> echo -> BEGIN -> output -> END.
	realOrderStream := "" +
		"\x1b[36mLazyCatCloud Banner\x1b[0m\r\n" +
		"root@global-2:~# " + sent + "\n" +
		"\r\n__LZ_BEGIN_" + markerID + "__\r\n" +
		"\x1b[32m__ROUND_1__\x1b[0m\r\n" +
		"7.0.0-14-generic\r\n" +
		"\r\n__LZ_END_" + markerID + ":0__\r\n"

	// Before execution finishes (only PTY echo received), parseSentinelOutput must report incomplete!
	onlyEcho := "root@global-2:~# " + sent + "\n"
	if _, _, ok := parseSentinelOutput(onlyEcho, markerID); ok {
		t.Fatalf("parseSentinelOutput matched PTY echo prematurely")
	}

	out, code, ok := parseSentinelOutput(realOrderStream, markerID)
	if !ok {
		t.Fatalf("expected parseSentinelOutput to match complete stream")
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	expected := "__ROUND_1__\n7.0.0-14-generic"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}

	// Also ensure rawStream containing pre-BEGIN noise doesn't fail
	out2, _, ok2 := parseSentinelOutput(rawStream, markerID)
	if !ok2 || out2 != expected {
		t.Fatalf("unexpected rawStream result: ok=%v out=%q", ok2, out2)
	}
}

func TestParseSentinelOutput_NonZeroExitCode(t *testing.T) {
	markerID := "fail99"
	stream := "\r\n__LZ_BEGIN_" + markerID + "__\r\ncat: /missing: No such file\r\n\r\n__LZ_END_" + markerID + ":1__\r\n"
	out, code, ok := parseSentinelOutput(stream, markerID)
	if !ok {
		t.Fatalf("expected complete=true")
	}
	if code != 1 {
		t.Fatalf("expected exit_code=1, got %d", code)
	}
	if out != "cat: /missing: No such file" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestNormalizeInstanceDetail_NestedAndCamelCase(t *testing.T) {
	raw := `{
		"instance": {
			"id": 11433,
			"name": "0.1折 全球家宽 大杯02",
			"status": "running",
			"hostPublicIp": "167.104.167.151",
			"portLimit": 10,
			"expiresAt": "2027-01-14T06:02:40.069Z",
			"port_mappings": [
				{"id": 48528, "protocol": "tcp", "private_port": 10001, "public_port": 40981, "remark": "egress-jp-softbank"}
			]
		}
	}`
	var res InstanceDetailResponse
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	normalizeInstanceDetail(&res)
	if res.PortLimit != 10 {
		t.Fatalf("expected PortLimit=10, got %d", res.PortLimit)
	}
	if res.HostPublicIP != "167.104.167.151" {
		t.Fatalf("expected HostPublicIP=167.104.167.151, got %q", res.HostPublicIP)
	}
	if res.ExpiresAt != "2027-01-14T06:02:40.069Z" {
		t.Fatalf("expected ExpiresAt populated, got %q", res.ExpiresAt)
	}
	if len(res.PortMappings) != 1 || res.PortMappings[0].PublicPort != 40981 {
		t.Fatalf("unexpected PortMappings: %+v", res.PortMappings)
	}
}

func TestFormatWSDialError_Cloudflare403(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Body:       http.NoBody,
	}
	// Replace body with CF region block HTML
	resp.Body = newReadCloser(`<!doctype html><html><title>403 Forbidden</title><h1>抱歉，暂不支持当前区域访问</h1><div class="sub">Access Restricted by Region</div></html>`)
	err := formatWSDialError(errors.New("websocket: bad handshake"), resp, "http://192.168.8.11:7890")
	if !strings.Contains(err.Error(), "Cloudflare WAF 区域拦截") {
		t.Fatalf("expected friendly CF 403 message, got: %v", err)
	}
}

type strReadCloser struct{ *strings.Reader }

func (strReadCloser) Close() error { return nil }

func newReadCloser(s string) *strReadCloser {
	return &strReadCloser{strings.NewReader(s)}
}

func TestExecTerminalMockWS_SlowCommandAndExitCode(t *testing.T) {
	// Verify ExecTerminalCommandWithResult waits for slow output beyond 3.5s rather than truncating early
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.WriteMessage(websocket.TextMessage, []byte(`{"type":"connected","sessionId":"test-1"}`))
		_ = c.WriteMessage(websocket.BinaryMessage, []byte("Alpine Linux v3.23 MOTD\r\n"))

		for {
			_, msg, err := c.ReadMessage()
			if err != nil {
				return
			}
			text := string(msg)
			if strings.Contains(text, `"type":"resize"`) {
				continue
			}
			// Echo the command back immediately (simulating PTY ECHO)
			_ = c.WriteMessage(websocket.BinaryMessage, []byte(text+"\n"))

			// Extract markerID from "...__LZ_BEG''IN_<id>__..."
			idx := strings.Index(text, "__LZ_BEG''IN_")
			if idx == -1 {
				continue
			}
			rest := text[idx+len("__LZ_BEG''IN_"):]
			endIdx := strings.Index(rest, "__")
			markerID := rest[:endIdx]

			// Emit BEGIN, wait 500ms, emit output chunk 1, wait 500ms, emit chunk 2 + END:0
			_ = c.WriteMessage(websocket.BinaryMessage, []byte("\r\n__LZ_BEGIN_"+markerID+"__\r\n"))
			time.Sleep(300 * time.Millisecond)
			_ = c.WriteMessage(websocket.BinaryMessage, []byte("__ROUND_1__\r\n"))
			time.Sleep(300 * time.Millisecond)
			_ = c.WriteMessage(websocket.BinaryMessage, []byte("7.0.0-14-generic\r\n\r\n__LZ_END_"+markerID+":0__\r\n"))
		}
	}))
	defer srv.Close()

	// Exercise the exact MOTD-settle -> send -> read loop via mock server (ensuring settle wait does not poison websocket conn)
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial mock ws: %v", err)
	}
	defer conn.Close()

	type wsMsg struct {
		data []byte
		err  error
	}
	msgCh := make(chan wsMsg, 16)
	_ = conn.SetReadDeadline(time.Now().Add(6 * time.Second))
	go func() {
		defer close(msgCh)
		for {
			_, d, e := conn.ReadMessage()
			msgCh <- wsMsg{data: d, err: e}
			if e != nil {
				return
			}
		}
	}()

	// Drain initial connected + MOTD using timer settle (mirrors ExecTerminalCommandWithResult)
	settle := time.NewTimer(450 * time.Millisecond)
	defer settle.Stop()
drain:
	for {
		select {
		case <-settle.C:
			break drain
		case m, ok := <-msgCh:
			if !ok || m.err != nil {
				t.Fatalf("unexpected close during drain: %v", m.err)
			}
		}
	}

	markerID := "mockid1"
	cmd := buildSentinelCommand("echo __ROUND_1__ && uname -r", markerID)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(cmd)); err != nil {
		t.Fatalf("write cmd: %v", err)
	}

	var buf strings.Builder
	execTimeout := time.After(5 * time.Second)
	for {
		select {
		case <-execTimeout:
			t.Fatalf("timed out waiting for mock output")
		case m, ok := <-msgCh:
			if !ok || m.err != nil {
				t.Fatalf("read failed: %v", m.err)
			}
			buf.Write(m.data)
			if out, code, parsed := parseSentinelOutput(buf.String(), markerID); parsed {
				if code != 0 || out != "__ROUND_1__\n7.0.0-14-generic" {
					t.Fatalf("unexpected parsed result: code=%d out=%q", code, out)
				}
				return
			}
		}
	}
}
