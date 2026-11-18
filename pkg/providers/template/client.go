package template

import (
	"context"
	"net/http"
	"time"
)

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{
		baseURL: "https://api.example.com",
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) FetchInstances(ctx context.Context) ([]map[string]interface{}, error) {
	// TODO: 实现与上游厂商的 HTTP 请求交互
	return []map[string]interface{}{}, nil
}
