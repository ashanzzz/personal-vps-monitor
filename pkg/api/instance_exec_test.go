package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/store"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
)

func TestInstanceExecRoutesAuthProtection(t *testing.T) {
	router := NewRouter()

	testCases := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{
			name:       "SSH Exec Unauthorized",
			method:     http.MethodPost,
			path:       "/api/instances/13483/exec",
			body:       `{"command":"whoami"}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Exec By IP Unauthorized",
			method:     http.MethodPost,
			path:       "/api/instances/exec-by-ip",
			body:       `{"ip":"38.175.222.44","command":"whoami"}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Inspect Proxies Unauthorized",
			method:     http.MethodGet,
			path:       "/api/instances/13483/proxies",
			body:       "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Change Proxy Port Unauthorized",
			method:     http.MethodPut,
			path:       "/api/instances/13483/proxies/36563",
			body:       `{"new_port":36564}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Add Proxy Unauthorized",
			method:     http.MethodPost,
			path:       "/api/instances/13483/proxies",
			body:       `{"port":40555}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Delete Proxy Unauthorized",
			method:     http.MethodDelete,
			path:       "/api/instances/13483/proxies/40555",
			body:       "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Get Chain Nodes Unauthorized",
			method:     http.MethodGet,
			path:       "/api/lazycat/chain-nodes",
			body:       "",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("path %s: expected status %d, got %d", tc.path, tc.wantStatus, w.Code)
			}
		})
	}
}

func TestLazyCatChainNodesAuthorized(t *testing.T) {
	// 注入模拟资产缓存
	_ = store.SaveDailyCache(&store.DailyInstancesCache{
		Total: 2,
		Instances: []providers.Instance{
			{
				ProviderID:   "acck",
				ID:           "13483",
				Name:         "LAX4837ISP",
				IP:           "38.175.222.44",
				Status:       "running",
				SSHUser:      "root",
				SSHPassword:  "fake-pass",
				SSHPort:      22,
			},
			{
				ProviderID:   "lazycat",
				ID:           "11433",
				Name:         "LazyCat-Home",
				IP:           "167.104.167.151",
				Status:       "running",
			},
		},
	})

	router := NewRouter()

	// 携带合法 Session Cookie 或 Master Key
	req := httptest.NewRequest(http.MethodGet, "/api/lazycat/chain-nodes?format=json", nil)
	// 注入 Authorization: Bearer
	token := generateToken()
	req.Header.Set("Authorization", "Bearer "+token)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "chains") || !strings.Contains(body, "relays") {
		t.Errorf("response body does not contain expected chain fields: %s", body)
	}

	// 测试 Clash YAML 导出
	reqYAML := httptest.NewRequest(http.MethodGet, "/api/lazycat/chain-nodes?format=clash", nil)
	reqYAML.Header.Set("Authorization", "Bearer "+token)
	wYAML := httptest.NewRecorder()
	router.ServeHTTP(wYAML, reqYAML)

	if wYAML.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for clash format, got %d", wYAML.Code)
	}
	if wYAML.Header().Get("Content-Type") != "text/yaml; charset=utf-8" {
		t.Errorf("expected text/yaml content-type, got %s", wYAML.Header().Get("Content-Type"))
	}
	if !strings.Contains(wYAML.Body.String(), "proxies:") || !strings.Contains(wYAML.Body.String(), "dialer-proxy:") {
		t.Errorf("clash yaml body invalid: %s", wYAML.Body.String())
	}
}
