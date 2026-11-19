package acck

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewClient(token, proxyURL string) *Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}
	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}

	return &Client{
		baseURL: "https://acck.io/api",
		token:   cleanToken(token),
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   15 * time.Second,
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

func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	fullURL := c.baseURL + path
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

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Origin", "https://acck.io")
	req.Header.Set("Referer", "https://acck.io/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/126.0 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
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
	ID         int    `json:"id"`
	Name       string `json:"name"`
	IP         string `json:"ip"`
	Status     string `json:"status"`
	Region     string `json:"region"`
	ExpireDate string `json:"expire_date"`
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

type ListingItem struct {
	ID            int    `json:"id"`
	AskPriceCents int    `json:"ask_price_cents"`
	IntroText     string `json:"intro_text"`
	SnapshotJSON  string `json:"snapshot_json"`
	CreatedAt     string `json:"created_at"`

	// 动态解析字段
	PlanName      string  `json:"plan_name"`
	AreaName      string  `json:"area_name"`
	CPU           int     `json:"cpu"`
	MemoryMB      int     `json:"memory_mb"`
	BandwidthMbps int     `json:"bandwidth_mbps"`
	Price         float64 `json:"price"`
}

type snapshotData struct {
	PlanName      string `json:"plan_name"`
	NodeName      string `json:"node_name"`
	AreaName      string `json:"area_name"`
	CPU           int    `json:"cpu"`
	MemoryMB      int    `json:"memory_mb"`
	BandwidthMbps int    `json:"bandwidth_mbps"`
}

func (c *Client) GetExchangeListings(ctx context.Context, limit int) ([]ListingItem, error) {
	path := fmt.Sprintf("/exchange/listings?offset=0&limit=%d", limit)
	data, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Listings []ListingItem `json:"listings"`
		Data     []ListingItem `json:"data"`
	}
	_ = json.Unmarshal(data, &raw)
	items := raw.Listings
	if len(items) == 0 {
		items = raw.Data
	}

	for i := range items {
		items[i].Price = float64(items[i].AskPriceCents) / 100.0
		if items[i].SnapshotJSON != "" {
			var snap snapshotData
			if err := json.Unmarshal([]byte(items[i].SnapshotJSON), &snap); err == nil {
				items[i].PlanName = snap.PlanName
				if items[i].PlanName == "" {
					items[i].PlanName = snap.NodeName
				}
				items[i].AreaName = snap.AreaName
				items[i].CPU = snap.CPU
				items[i].MemoryMB = snap.MemoryMB
				items[i].BandwidthMbps = snap.BandwidthMbps
			}
		}
	}

	return items, nil
}
