package lazycat

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
		baseURL: "https://dash.lazycat.wiki/api",
		token:   token,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   15 * time.Second,
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

type InstanceListItem struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	PackageName  string `json:"packageName"`
	PlanName     string `json:"planName"`
	PortLimit    int    `json:"portLimit"`
	HostPublicIP string `json:"hostPublicIp"`
	NatPublicIP  string `json:"natPublicIp"`
	ExpiresAt    string `json:"expiresAt"`
}

func (c *Client) GetInstances(ctx context.Context) ([]InstanceListItem, error) {
	data, err := c.doRequest(ctx, "GET", "/instances", nil)
	if err != nil {
		return nil, err
	}

	var res struct {
		Instances []InstanceListItem `json:"instances"`
		Data      []InstanceListItem `json:"data"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("json unmarshal failed: %w", err)
	}

	if len(res.Instances) > 0 {
		return res.Instances, nil
	}
	return res.Data, nil
}

type InstanceDetailResponse struct {
	ID           int               `json:"id"`
	Name         string            `json:"name"`
	Status       string            `json:"status"`
	PackageName  string            `json:"packageName"`
	PlanName     string            `json:"planName"`
	HostPublicIP string            `json:"host_public_ip"`
	NatPublicIP  string            `json:"nat_public_ip"`
	PortLimit    int               `json:"port_limit"`
	PortMappings []PortMappingItem `json:"port_mappings"`
	ExpiresAt    string            `json:"expires_at"`

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

type UserInfoResponse struct {
	User struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
		Email    string `json:"email"`
		VIPLevel int    `json:"vipLevel"`
	} `json:"user"`
}

func (c *Client) GetUserInfo(ctx context.Context) (*UserInfoResponse, error) {
	data, err := c.doRequest(ctx, "GET", "/auth/me", nil)
	if err != nil {
		return nil, err
	}
	var res UserInfoResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

type CheckinResponse struct {
	Success bool   `json:"success"`
	Points  int    `json:"points"`
	Message string `json:"message"`
	Error   string `json:"error"`
	Outcome string `json:"outcome"`
}

func (c *Client) Checkin(ctx context.Context) (*CheckinResponse, error) {
	data, err := c.doRequest(ctx, "POST", "/points-center/checkin/", map[string]interface{}{})
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "409") || strings.Contains(errStr, "已签到") || strings.Contains(errStr, "duplicate") {
			idx := strings.Index(errStr, "{")
			if idx != -1 {
				var dup CheckinResponse
				if json.Unmarshal([]byte(errStr[idx:]), &dup) == nil {
					dup.Success = true
					if dup.Message == "" {
						dup.Message = dup.Error
					}
					return &dup, nil
				}
			}
			return &CheckinResponse{
				Success: true,
				Message: "当天已签到",
				Outcome: "duplicate",
			}, nil
		}
		return nil, err
	}

	var res CheckinResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}
