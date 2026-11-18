package akile

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseAI     string
	baseCloud  string
	token      string
	httpClient *http.Client
}

func NewClient(token string) *Client {
	return &Client{
		baseAI:    "https://api.akile.ai/api",
		baseCloud: "https://api.akile.ai/api",
		token:     strings.Trim(strings.TrimSpace(token), "\""),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *Client) SetToken(token string) {
	c.token = strings.Trim(strings.TrimSpace(token), "\"")
}

func (c *Client) doRequest(ctx context.Context, method, baseURL, path string, body interface{}) ([]byte, error) {
	fullURL := baseURL + path
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

	req.Header.Set("Authorization", c.token)
	req.Header.Set("token", c.token)
	req.Header.Set("Origin", "https://akile.ai")
	req.Header.Set("Referer", "https://akile.ai/")
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
		return nil, fmt.Errorf("akile api error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

type CheckinResponse struct {
	StatusCode int    `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
	Data       struct {
		AkCoinEarned float64 `json:"ak_coin_earned"`
	} `json:"data"`
}

func (c *Client) Checkin(ctx context.Context) (*CheckinResponse, error) {
	data, err := c.doRequest(ctx, "GET", c.baseAI, "/v1/user/Checkin", nil)
	if err != nil {
		return nil, err
	}

	var res CheckinResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("json unmarshal failed: %w", err)
	}
	return &res, nil
}

type InstanceListResponse struct {
	Code int `json:"code"`
	Data []struct {
		ID         int    `json:"id"`
		Name       string `json:"name"`
		IP         string `json:"ip"`
		Status     int    `json:"status"`
		ExpireDate string `json:"expire_date"`
	} `json:"data"`
}

func (c *Client) ListInstances(ctx context.Context) (*InstanceListResponse, error) {
	data, err := c.doRequest(ctx, "GET", c.baseCloud, "/v1/vm/list", nil)
	if err != nil {
		return nil, err
	}

	var res InstanceListResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("json unmarshal failed: %w", err)
	}
	return &res, nil
}

type SecondhandItem struct {
	ID            int     `json:"id"`
	Name          string  `json:"name"`
	Price         float64 `json:"price"`
	AreaName      string  `json:"area_name"`
	Bandwidth     string  `json:"bandwidth"`
	Configuration string  `json:"configuration"`
	CreateTime    string  `json:"create_time"`
}

func (c *Client) GetSecondhandList(ctx context.Context, page, pageSize int) ([]SecondhandItem, error) {
	payload := map[string]interface{}{
		"page_num":  page,
		"page_size": pageSize,
	}
	data, err := c.doRequest(ctx, "POST", c.baseCloud, "/v1/pushshop/GetPushProductList", payload)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Code int              `json:"code"`
		List []SecondhandItem `json:"list"`
		Data struct {
			List []SecondhandItem `json:"list"`
		} `json:"data"`
	}
	_ = json.Unmarshal(data, &raw)
	if len(raw.List) > 0 {
		return raw.List, nil
	}
	return raw.Data.List, nil
}

func (c *Client) RefreshToken(ctx context.Context) (string, error) {
	data, err := c.doRequest(ctx, "GET", c.baseAI, "/v1/user/refreshToken", nil)
	if err != nil {
		return "", err
	}

	var raw struct {
		Code int `json:"code"`
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return "", err
	}
	if raw.Data.Token == "" {
		return "", fmt.Errorf("empty token in refresh response: %s", string(data))
	}
	c.token = raw.Data.Token
	return raw.Data.Token, nil
}
