package lazycat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewClient(token string) *Client {
	return &Client{
		baseURL: "https://dash.lazycat.wiki/api",
		token:   token,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
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

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "personal-vps-monitor/1.0 (Go)")

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
		return nil, fmt.Errorf("lazycat api error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

type PortMappingItem struct {
	ID          int    `json:"id"`
	Protocol    string `json:"protocol"`
	PrivatePort int    `json:"private_port"`
	PublicPort  int    `json:"public_port"`
	Remark      string `json:"remark"`
}

type InstanceDetailResponse struct {
	ID           int               `json:"id"`
	Name         string            `json:"name"`
	Status       string            `json:"status"`
	HostPublicIP string            `json:"host_public_ip"`
	NatPublicIP  string            `json:"nat_public_ip"`
	PortLimit    int               `json:"port_limit"`
	PortMappings []PortMappingItem `json:"port_mappings"`

	// 兼容某些返回包装在 instance 内部的情况
	Instance *struct {
		ID           int               `json:"id"`
		Name         string            `json:"name"`
		Status       string            `json:"status"`
		HostPublicIP string            `json:"host_public_ip"`
		PortMappings []PortMappingItem `json:"port_mappings"`
	} `json:"instance,omitempty"`
}

func (c *Client) GetInstanceDetail(ctx context.Context, instanceID string) (*InstanceDetailResponse, error) {
	path := fmt.Sprintf("/instances/%s", instanceID)
	data, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var res InstanceDetailResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("json unmarshal failed: %w", err)
	}

	// 如果外层没有，提取内层
	if res.ID == 0 && res.Instance != nil {
		res.ID = res.Instance.ID
		res.Name = res.Instance.Name
		res.Status = res.Instance.Status
		res.HostPublicIP = res.Instance.HostPublicIP
		res.PortMappings = res.Instance.PortMappings
	}

	return &res, nil
}

func (c *Client) AddPortMapping(ctx context.Context, instanceID string, privatePort int, protocol, remark string) (int, error) {
	path := fmt.Sprintf("/instances/%s/port-mappings", instanceID)
	payload := map[string]interface{}{
		"privatePort": privatePort,
		"protocol":    protocol,
		"remark":      remark,
	}

	data, err := c.doRequest(ctx, "POST", path, payload)
	if err != nil {
		return 0, err
	}

	var res struct {
		Mapping struct {
			PublicPort int `json:"publicPort"`
		} `json:"mapping"`
	}
	_ = json.Unmarshal(data, &res)
	return res.Mapping.PublicPort, nil
}

func (c *Client) DeletePortMapping(ctx context.Context, instanceID string, mappingID int) error {
	path := fmt.Sprintf("/instances/%s/port-mappings/%d", instanceID, mappingID)
	_, err := c.doRequest(ctx, "DELETE", path, nil)
	return err
}
