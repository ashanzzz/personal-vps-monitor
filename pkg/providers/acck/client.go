package acck

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Client struct {
	baseURL    string
	mu         sync.RWMutex
	token      string
	proxyURL   string
	httpClient *http.Client
}

func NewClient(token, proxyURL string) *Client {
	return NewClientWithBaseURL("https://acck.io/api", token, proxyURL)
}

func NewClientWithBaseURL(baseURL, token, proxyURL string) *Client {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		IdleConnTimeout:       15 * time.Second,
		DisableKeepAlives:     true,
		ForceAttemptHTTP2:     false,
		TLSNextProto:          make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
	}
	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}

	cleanedBase := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if cleanedBase == "" {
		cleanedBase = "https://acck.io/api"
	}

	return &Client{
		baseURL:  cleanedBase,
		token:    cleanToken(token),
		proxyURL: strings.TrimSpace(proxyURL),
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   20 * time.Second,
		},
	}
}

func cleanToken(t string) string {
	t = strings.Trim(strings.TrimSpace(t), "\"'\x27")
	if strings.HasPrefix(strings.ToLower(t), "bearer ") {
		t = strings.TrimSpace(t[7:])
	}
	return t
}

func (c *Client) SetToken(token string) {
	c.mu.Lock()
	c.token = cleanToken(token)
	c.mu.Unlock()
}

func (c *Client) GetToken() string {
	c.mu.RLock()
	tok := c.token
	c.mu.RUnlock()
	return tok
}

// VerifyToken 校验当前 Token 是否有效
func (c *Client) VerifyToken(ctx context.Context) error {
	data, err := c.doRequest(ctx, "GET", "/auth/user/info", nil)
	if err != nil {
		return err
	}
	if strings.Contains(string(data), "401") || strings.Contains(string(data), "unauthorized") {
		return fmt.Errorf("token 校验失败 (401 未授权)")
	}
	return nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	fullURL := c.baseURL + path

	var resp *http.Response
	var lastErr error

	for attempt := 0; attempt < 3; attempt++ {
		var bodyReader io.Reader
		if body != nil {
			data, err := json.Marshal(body)
			if err != nil {
				return nil, err
			}
			bodyReader = bytes.NewBuffer(data)
		}

		req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
		if err != nil {
			return nil, err
		}

		if tok := c.GetToken(); tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		req.Header.Set("Origin", "https://acck.io")
		req.Header.Set("Referer", "https://acck.io/")
		req.Header.Set("User-Agent", acckDefaultUA)
		req.Header.Set("Accept", "application/json, text/plain, */*")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err = c.httpClient.Do(req)
		if err == nil {
			lastErr = nil
			break
		}
		lastErr = err
		if attempt < 2 && isTransientNetworkError(ctx, err) {
			backoff := time.Duration(350*(attempt+1)) * time.Millisecond
			if sleepErr := sleepWithContext(ctx, backoff); sleepErr != nil {
				return nil, lastErr
			}
			continue
		}
		return nil, err
	}

	if lastErr != nil {
		return nil, lastErr
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("acck api error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

type CheckinResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data string `json:"data"`
}

type UserInfoResponse struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

func (c *Client) GetUserInfo(ctx context.Context) (*UserInfoResponse, error) {
	data, err := c.doRequest(ctx, "GET", "/auth/user/info", nil)
	if err != nil {
		return nil, err
	}
	var res UserInfoResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) Checkin(ctx context.Context) (*CheckinResponse, error) {
	data, err := c.doRequest(ctx, "POST", "/auth/user/ac-shop/checkin", map[string]interface{}{})
	if err != nil {
		if strings.Contains(err.Error(), "今日已签到") {
			return &CheckinResponse{
				Code: 0,
				Msg:  "今日已签到",
			}, nil
		}
		return nil, err
	}

	var res CheckinResponse
	_ = json.Unmarshal(data, &res)
	if res.Msg == "" {
		res.Msg = string(data)
	}
	return &res, nil
}

type ServerItem struct {
	ID           int     `json:"id"`
	ServerID     string  `json:"server_id"`
	ServerName   string  `json:"server_name"`
	ServerReName string  `json:"server_re_name"`
	NodeName     string  `json:"node_name"`
	AreaName     string  `json:"area_name"`
	IPv4         string  `json:"ipv4"`
	IPv6         string  `json:"ipv6"`
	Status       int     `json:"status"`
	CPU          int     `json:"cpu"`
	Memory       int     `json:"memory"`
	Disk         int     `json:"disk"`
	Flow         int     `json:"flow"`
	Bandwidth    int     `json:"bandwidth"`
	Price        float64 `json:"price"`
	DueTime      int64   `json:"due_time"`
	AutoRenew      int     `json:"auto_renew"`
	AutoReset      int     `json:"auto_reset"`
	Cycle          int     `json:"cycle"`
	ResetFlowPrice float64 `json:"reset_flow_price"`
	InitUsername   string  `json:"init_username"`
	InitPassword   string  `json:"init_password"`
}

type BillingViewResponse struct {
	Currency          string `json:"currency"`
	BalanceCents      int64  `json:"balance_cents"`
	RenewalPriceCents int    `json:"renewal_price_cents"`
	Frozen            bool   `json:"frozen"`
}

func (c *Client) GetBillingView(ctx context.Context, serverID int) (*BillingViewResponse, error) {
	path := fmt.Sprintf("/auth/user/server/%d/billing-view", serverID)
	data, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var res BillingViewResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) GetMyServers(ctx context.Context) ([]ServerItem, error) {
	data, err := c.doRequest(ctx, "GET", "/auth/user/server/list", nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Data    []ServerItem `json:"data"`
		Servers []ServerItem `json:"servers"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	if len(raw.Servers) > 0 {
		return raw.Servers, nil
	}
	return raw.Data, nil
}

type VmInfoResponse struct {
	Bandwidth      int    `json:"bandwidth"`
	BandwidthLimit int64  `json:"bandwidth_limit"`
	BandwidthUsage int64  `json:"bandwidth_usage"`
	CPU            int    `json:"cpu"`
	Disk           int    `json:"disk"`
	Memory         int    `json:"memory"`
	Status         string `json:"status"`
	NextResetTime  int64  `json:"next_reset_time"`
}

func (c *Client) GetVmInfo(ctx context.Context, serverID string) (*VmInfoResponse, error) {
	path := fmt.Sprintf("/auth/user/server/%s/vm/info", serverID)
	data, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var res VmInfoResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

type ListingItem struct {
	ID               int    `json:"id"`
	SellerUserID     int    `json:"seller_user_id"`
	ServerID         int    `json:"server_id"`
	AskPriceCents    int    `json:"ask_price_cents"`
	PlatformFeeCents int    `json:"platform_fee_cents"`
	IPStatus         string `json:"ip_status"`
	IntroText        string `json:"intro_text"`
	SnapshotJSON     string `json:"snapshot_json"`
	CreatedAt        string `json:"created_at"`

	// 动态解析字段
	PlanName      string  `json:"plan_name"`
	NodeComment   string  `json:"node_comment"`
	AreaName      string  `json:"area_name"`
	CPU           int     `json:"cpu"`
	MemoryMB      int     `json:"memory_mb"`
	DiskGB        int     `json:"disk_gb"`
	FlowGB        int     `json:"flow_gb"`
	BandwidthMbps int     `json:"bandwidth_mbps"`
	Price         float64 `json:"price"`
	RenewalPrice  float64 `json:"renewal_price"`
	ResetPrice    float64 `json:"reset_price"`
	PlatformFee   float64 `json:"platform_fee"`
	FlowUsed      string  `json:"flow_used"`
	Cycle         int     `json:"cycle"`
	DueTime       string  `json:"due_time"`
}

type snapshotData struct {
	PlanName            string  `json:"plan_name"`
	NodeName            string  `json:"node_name"`
	NodeComment         string  `json:"node_comment"`
	AreaName            string  `json:"area_name"`
	AreaFlag            string  `json:"area_flag"`
	CPU                 int     `json:"cpu"`
	MemoryMB            int     `json:"memory_mb"`
	DiskGB              int     `json:"disk_gb"`
	BandwidthMbps       int     `json:"bandwidth_mbps"`
	RenewPriceCents     int     `json:"renew_price_cents"`
	ResetFlowPriceCents int     `json:"reset_flow_price_cents"`
	DueTimeUnix         int64   `json:"due_time_unix"`
	FlowGB              int     `json:"flow_gb"`
	FlowUsedGB          float64 `json:"flow_used_gb"`
	Cycle               int     `json:"cycle"`
}

func (c *Client) GetExchangeListings(ctx context.Context, limit int) ([]ListingItem, error) {
	return c.GetAllExchangeListings(ctx)
}

// GetAllExchangeListings 动态穿透游标分页抓取全网所有在售挂单 (打破单页50条限制)
func (c *Client) GetAllExchangeListings(ctx context.Context) ([]ListingItem, error) {
	var allItems []ListingItem
	curOffset := 0
	maxPages := 15 // 安全上限，防止无限死循环，最高 750 台

	for page := 0; page < maxPages; page++ {
		path := fmt.Sprintf("/exchange/listings?offset=%d&limit=50", curOffset)
		data, err := c.doRequest(ctx, "GET", path, nil)
		if err != nil {
			if len(allItems) > 0 {
				break
			}
			return nil, err
		}

		var raw struct {
			Listings   []ListingItem `json:"listings"`
			Data       []ListingItem `json:"data"`
			HasMore    bool          `json:"has_more"`
			NextOffset int           `json:"next_offset"`
		}
		_ = json.Unmarshal(data, &raw)
		batch := raw.Listings
		if len(batch) == 0 {
			batch = raw.Data
		}
		if len(batch) == 0 {
			break
		}

		for i := range batch {
			batch[i].Price = float64(batch[i].AskPriceCents) / 100.0
			if batch[i].PlatformFeeCents > 0 {
				batch[i].PlatformFee = float64(batch[i].PlatformFeeCents) / 100.0
			}
			if batch[i].SnapshotJSON != "" {
				var snap snapshotData
				if err := json.Unmarshal([]byte(batch[i].SnapshotJSON), &snap); err == nil {
					batch[i].PlanName = snap.PlanName
					if batch[i].PlanName == "" {
						batch[i].PlanName = snap.NodeName
					}
					batch[i].AreaName = snap.AreaName
					batch[i].CPU = snap.CPU
					batch[i].MemoryMB = snap.MemoryMB
					batch[i].DiskGB = snap.DiskGB
					batch[i].FlowGB = snap.FlowGB
					batch[i].BandwidthMbps = snap.BandwidthMbps
					batch[i].RenewalPrice = float64(snap.RenewPriceCents) / 100.0
					if snap.ResetFlowPriceCents > 0 {
						batch[i].ResetPrice = float64(snap.ResetFlowPriceCents) / 100.0
					}
					if snap.DueTimeUnix > 0 {
						batch[i].DueTime = time.Unix(snap.DueTimeUnix, 0).Format("2006-01-02 15:04")
					}
					batch[i].NodeComment = snap.NodeComment
					if batch[i].IntroText == "" {
						batch[i].IntroText = snap.NodeComment
					}
					batch[i].Cycle = snap.Cycle
					if snap.FlowGB > 0 {
						batch[i].FlowUsed = fmt.Sprintf("%.2f GB / %d GB", snap.FlowUsedGB, snap.FlowGB)
					}
				}
			}
		}

		allItems = append(allItems, batch...)

		// 检查是否有下一页
		if !raw.HasMore || raw.NextOffset <= curOffset {
			break
		}
		curOffset = raw.NextOffset
	}

	return allItems, nil
}

// BuyExchangeListing 购买 ACCK Exchange 二手挂单
func (c *Client) BuyExchangeListing(ctx context.Context, listingID int) error {
	path := fmt.Sprintf("/exchange/listings/%d/buy", listingID)
	_, err := c.doRequest(ctx, "POST", path, map[string]interface{}{})
	return err
}
