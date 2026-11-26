package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/config"
)

func TestLazyCatEgressSubscriptionsAuth(t *testing.T) {
	prevReadToken := config.Global.PublicReadToken
	config.Global.PublicReadToken = "test-readonly-egress-token"
	defer func() {
		config.Global.PublicReadToken = prevReadToken
	}()

	router := NewRouter()

	// 1. 无凭证请求必须返回 401 Unauthorized
	reqUnauth := httptest.NewRequest(http.MethodGet, "/api/lazycat/egress-subscriptions?format=clash", nil)
	wUnauth := httptest.NewRecorder()
	router.ServeHTTP(wUnauth, reqUnauth)
	if wUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized without token, got %d", wUnauth.Code)
	}

	// 2. 携带 PUBLIC_READ_TOKEN 通过 Query 参数 ?token= 访问只读订阅接口应被放行（非 401/403）
	reqRead := httptest.NewRequest(http.MethodGet, "/api/lazycat/egress-subscriptions?format=uri&token=test-readonly-egress-token", nil)
	wRead := httptest.NewRecorder()
	router.ServeHTTP(wRead, reqRead)
	if wRead.Code == http.StatusUnauthorized || wRead.Code == http.StatusForbidden {
		t.Fatalf("expected PUBLIC_READ_TOKEN to be allowed on GET /api/lazycat/egress-subscriptions, got %d (%s)", wRead.Code, wRead.Body.String())
	}
}
