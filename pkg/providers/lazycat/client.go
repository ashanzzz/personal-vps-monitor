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
	proxyURL   string
	httpClient *http.Client
}

func NewClient(token, proxyURL string) *Client {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 6 * time.Second,
		IdleConnTimeout:       10 * time.Second,
		DisableKeepAlives:     true,
	}
	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}

	return &Client{
		baseURL:  "https://dash.lazycat.wiki/api",
		token:    token,
		proxyURL: proxyURL,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   8 * time.Second,
		},
	}
}

func (c *Client) SetProxyURL(proxyURL string) {
	c.proxyURL = proxyURL
	if transport, ok := c.httpClient.Transport.(*http.Transport); ok {
		if proxyURL != "" {
			if u, err := url.Parse(proxyURL); err == nil {
				transport.Proxy = http.ProxyURL(u)
				return
			}
		}
		transport.Proxy = http.ProxyFromEnvironment
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
		bodyStr := string(respBody)
		if resp.StatusCode == http.StatusForbidden &&
			(strings.Contains(bodyStr, "Access Restricted by Region") ||
				strings.Contains(bodyStr, "Blocked by WAF") ||
				strings.Contains(bodyStr, "暂不支持当前区域访问")) {
			return nil, fmt.Errorf("lazycat api error 403: Cloudflare WAF 区域拦截 (proxy=%s 出站可能走了直连或受限地区)", c.proxyURL)
		}
		return nil, fmt.Errorf("lazycat api error %d: %s", resp.StatusCode, bodyStr)
	}

	return respBody, nil
}

type LazyCatLoginResponse struct {
	Token       string `json:"token"`
	AccessToken string `json:"access_token"`
	Message     string `json:"message"`
	Data        *struct {
		Token       string `json:"token"`
		AccessToken string `json:"accessToken"`
	} `json:"data,omitempty"`
}

// Login 使用账号密码 (可选 2FA TOTP 动态码) 登录懒猫云
func (c *Client) Login(ctx context.Context, username, password, totpCode string) (token string, need2FA bool, msg string, err error) {
	payload := map[string]interface{}{
		"username": username,
		"password": password,
	}
	if strings.TrimSpace(totpCode) != "" {
		payload["totpCode"] = strings.TrimSpace(totpCode)
	}

	data, err := c.doRequest(ctx, "POST", "/auth/login", payload)
	if err != nil {
		errStr := strings.ToLower(err.Error())
		if strings.TrimSpace(totpCode) == "" && (strings.Contains(errStr, "totp") || strings.Contains(errStr, "2fa") || strings.Contains(errStr, "二次验证") || strings.Contains(errStr, "动态码") || strings.Contains(errStr, "two-factor") || strings.Contains(errStr, "two_fa")) {
			return "", true, "该账号已开启 2FA 二次验证，请输入验证器 6 位动态码", nil
		}
		return "", false, "", err
	}

	var res LazyCatLoginResponse
	_ = json.Unmarshal(data, &res)

	tok := res.Token
	if tok == "" {
		tok = res.AccessToken
	}
	if tok == "" && res.Data != nil {
		if res.Data.Token != "" {
			tok = res.Data.Token
		} else {
			tok = res.Data.AccessToken
		}
	}

	if tok == "" {
		raw := string(data)
		if strings.TrimSpace(totpCode) == "" && (strings.Contains(raw, "totp") || strings.Contains(raw, "2fa") || strings.Contains(raw, "二次验证")) {
			return "", true, "请输入二次验证码", nil
		}
		return "", false, "", fmt.Errorf("登录成功但未提取到 Token: %s", raw)
	}

	c.token = tok
	return tok, false, "登录成功", nil
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

type InstanceDetailInner struct {
	ID             int               `json:"id"`
	Name           string            `json:"name"`
	Status         string            `json:"status"`
	PackageName    string            `json:"packageName"`
	PlanName       string            `json:"planName"`
	HostPublicIP   string            `json:"host_public_ip"`
	HostPublicIPCC string            `json:"hostPublicIp"`
	NatPublicIP    string            `json:"nat_public_ip"`
	NatPublicIPCC  string            `json:"natPublicIp"`
	PortLimit      int               `json:"port_limit"`
	PortLimitCC    int               `json:"portLimit"`
	PortMappings   []PortMappingItem `json:"port_mappings"`
	PortMappingsCC []PortMappingItem `json:"portMappings"`
	ExpiresAt      string            `json:"expires_at"`
	ExpiresAtCC    string            `json:"expiresAt"`
}

type InstanceDetailResponse struct {
	ID             int               `json:"id"`
	Name           string            `json:"name"`
	Status         string            `json:"status"`
	PackageName    string            `json:"packageName"`
	PlanName       string            `json:"planName"`
	HostPublicIP   string            `json:"host_public_ip"`
	HostPublicIPCC string            `json:"hostPublicIp"`
	NatPublicIP    string            `json:"nat_public_ip"`
	NatPublicIPCC  string            `json:"natPublicIp"`
	PortLimit      int               `json:"port_limit"`
	PortLimitCC    int               `json:"portLimit"`
	PortMappings   []PortMappingItem `json:"port_mappings"`
	PortMappingsCC []PortMappingItem `json:"portMappings"`
	ExpiresAt      string            `json:"expires_at"`
	ExpiresAtCC    string            `json:"expiresAt"`

	// 兼容某些返回包装在 instance 内部的情况
	Instance *InstanceDetailInner `json:"instance,omitempty"`
}

func normalizeInstanceDetail(res *InstanceDetailResponse) {
	if res.HostPublicIP == "" {
		res.HostPublicIP = res.HostPublicIPCC
	}
	if res.NatPublicIP == "" {
		res.NatPublicIP = res.NatPublicIPCC
	}
	if res.PortLimit == 0 {
		res.PortLimit = res.PortLimitCC
	}
	if len(res.PortMappings) == 0 {
		res.PortMappings = res.PortMappingsCC
	}
	if res.ExpiresAt == "" {
		res.ExpiresAt = res.ExpiresAtCC
	}

	if res.Instance != nil {
		in := res.Instance
		if res.ID == 0 {
			res.ID = in.ID
		}
		if res.Name == "" {
			res.Name = in.Name
		}
		if res.Status == "" {
			res.Status = in.Status
		}
		if res.PackageName == "" {
			res.PackageName = in.PackageName
		}
		if res.PlanName == "" {
			res.PlanName = in.PlanName
		}
		if res.HostPublicIP == "" {
			if in.HostPublicIP != "" {
				res.HostPublicIP = in.HostPublicIP
			} else {
				res.HostPublicIP = in.HostPublicIPCC
			}
		}
		if res.NatPublicIP == "" {
			if in.NatPublicIP != "" {
				res.NatPublicIP = in.NatPublicIP
			} else {
				res.NatPublicIP = in.NatPublicIPCC
			}
		}
		if res.PortLimit == 0 {
			if in.PortLimit > 0 {
				res.PortLimit = in.PortLimit
			} else {
				res.PortLimit = in.PortLimitCC
			}
		}
		if len(res.PortMappings) == 0 {
			if len(in.PortMappings) > 0 {
				res.PortMappings = in.PortMappings
			} else {
				res.PortMappings = in.PortMappingsCC
			}
		}
		if res.ExpiresAt == "" {
			if in.ExpiresAt != "" {
				res.ExpiresAt = in.ExpiresAt
			} else {
				res.ExpiresAt = in.ExpiresAtCC
			}
		}
	}
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

	normalizeInstanceDetail(&res)
	return &res, nil
}

type InstanceTrafficResponse struct {
	MonthlyUsed           string  `json:"monthlyUsed"`
	MonthlyUsedFormatted  string  `json:"monthlyUsedFormatted"`
	MonthlyLimit          string  `json:"monthlyLimit"`
	MonthlyLimitFormatted string  `json:"monthlyLimitFormatted"`
	TrafficStatus         string  `json:"trafficStatus"`
	Percentage            float64 `json:"percentage"`
	PeriodStart           string  `json:"periodStart"`
	PeriodEnd             string  `json:"periodEnd"`
	PaidResetAvailable    bool    `json:"paidResetAvailable"`
}

func (c *Client) GetInstanceTraffic(ctx context.Context, instanceID int) (*InstanceTrafficResponse, error) {
	path := fmt.Sprintf("/instances/%d/traffic", instanceID)
	data, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var res InstanceTrafficResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

type TrafficResetQuoteResponse struct {
	Available            bool    `json:"available"`
	Price                float64 `json:"price"`
	PriceMode            string  `json:"priceMode"`
	OriginalCyclePrice   float64 `json:"originalCyclePrice"`
	DiscountedCyclePrice float64 `json:"discountedCyclePrice"`
	BillingCycle         int     `json:"billingCycle"`
	DiscountPercent      float64 `json:"discountPercent"`
	Balance              float64 `json:"balance"`
	MonthlyUsedFormatted string  `json:"monthlyUsedFormatted"`
	MonthlyLimitFormatted string `json:"monthlyLimitFormatted"`
	PeriodStart          string  `json:"periodStart"`
	PeriodEnd            string  `json:"periodEnd"`
}

func (c *Client) GetTrafficResetQuote(ctx context.Context, instanceID int) (*TrafficResetQuoteResponse, error) {
	path := fmt.Sprintf("/instances/%d/traffic/reset-quote", instanceID)
	data, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var res TrafficResetQuoteResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

type InstancePasswordResponse struct {
	RootPassword string `json:"rootPassword"`
}

func (c *Client) GetInstancePassword(ctx context.Context, instanceID int) (string, error) {
	path := fmt.Sprintf("/instances/%d/password", instanceID)
	data, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return "", err
	}
	var res InstancePasswordResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return "", err
	}
	return res.RootPassword, nil
}

type InstanceBillingResponse struct {
	Billing struct {
		InstanceID   int     `json:"instanceId"`
		InstanceName string  `json:"instanceName"`
		Price        float64 `json:"price"`
		BillingCycle int     `json:"billingCycle"`
		ExpiresAt    string  `json:"expiresAt"`
		AutoRenew    bool    `json:"autoRenew"`
	} `json:"billing"`
}

func (c *Client) GetInstanceBilling(ctx context.Context, instanceID int) (*InstanceBillingResponse, error) {
	path := fmt.Sprintf("/instances/%d/billing", instanceID)
	data, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var res InstanceBillingResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// BuyMarketListing 购买二手集市指定挂单
func (c *Client) BuyMarketListing(ctx context.Context, listingID int) error {
	path := fmt.Sprintf("/instance-market/listings/%d/buy", listingID)
	_, err := c.doRequest(ctx, "POST", path, map[string]interface{}{})
	return err
}

func (c *Client) AddPortMapping(ctx context.Context, instanceID string, privatePort int, protocol, remark string) (int, error) {
	path := fmt.Sprintf("/instances/%s/ports", instanceID)
	payload := map[string]interface{}{
		"privatePort": privatePort,
		"protocol":    protocol,
	}
	if remark != "" {
		payload["remark"] = remark
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
	path := fmt.Sprintf("/instances/%s/ports/%d", instanceID, mappingID)
	// 懒猫云官方 DELETE /instances/{id}/ports/{portId} 要求带 JSON body {}
	_, err := c.doRequest(ctx, "DELETE", path, map[string]interface{}{})
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

// PackagePlan 懒猫云官方新机套餐子方案规格
type PackagePlan struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	CPUCores     int    `json:"cpuCores"`
	MemoryMB     int    `json:"memory"`
	DiskMB       int    `json:"disk"`
	PortLimit    int    `json:"portLimit"`
	LimitsIngress string `json:"limitsIngress"`
	LimitsEgress  string `json:"limitsEgress"`
	PriceCents   int    `json:"price"`
	MonthlyPrice int    `json:"monthlyPrice"`
}

// PublicPackage 懒猫云官方直营在售套餐定义 (公网公开接口，无需Token)
type PublicPackage struct {
	ID            int           `json:"id"`
	Name          string        `json:"name"`
	Description   string        `json:"description"`
	NetworkMode   string        `json:"network_mode"`
	InstanceType  string        `json:"instance_type"`
	LimitsIngress string        `json:"limits_ingress"`
	LimitsEgress  string        `json:"limits_egress"`
	SoldOut       bool          `json:"soldOut"`
	Plans         []PackagePlan `json:"plans"`
}

// GetPublicPackages 获取懒猫云官方所有真实在售新机规格 (共 42+ 个套餐)
func (c *Client) GetPublicPackages(ctx context.Context) ([]PublicPackage, error) {
	data, err := c.doRequest(ctx, "GET", "/packages/public", nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Packages []PublicPackage `json:"packages"`
		Total    int             `json:"total"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw.Packages, nil
}

// LazyMarketSnapshot 懒猫云二手市场规格快照
type LazyMarketSnapshot struct {
	CPU                    int     `json:"cpu"`
	Memory                 int     `json:"memory"`
	Disk                   int     `json:"disk"`
	CPUCores               int     `json:"cpuCores"`
	NodeName               string  `json:"nodeName"`
	PlanName               string  `json:"planName"`
	RegionName             string  `json:"regionName"`
	RegionCode             string  `json:"regionCode"`
	BandwidthMbps          int     `json:"bandwidthMbps"`
	RenewalPrice           float64 `json:"renewalPrice"`
	RenewalDiscountPercent float64 `json:"renewalDiscountPercent"`
	BillingCycle           int     `json:"billingCycle,omitempty"`
	MonthlyTrafficLimit    string  `json:"monthlyTrafficLimit"`
	MonthlyTrafficUsed     string  `json:"monthlyTrafficUsed"`
	IPStatus               string  `json:"ipStatus"`
	NetworkMode            string  `json:"networkMode"`
	PortLimit              int     `json:"portLimit"`
	SLA                    string  `json:"sla"`
}

// LazyMarketListing 懒猫云二手市场挂单结构
type LazyMarketListing struct {
	ID             int                `json:"id"`
	InstanceID     int                `json:"instanceId"`
	SellerUserID   int                `json:"sellerUserId"`
	SellerUsername string             `json:"sellerUsername"`
	Title          string             `json:"title"`
	InstanceName   string             `json:"instanceName"`
	Remark         string             `json:"remark"`
	SalePrice      float64            `json:"salePrice"`
	Status         string             `json:"status"`
	Snapshot       LazyMarketSnapshot `json:"snapshot"`
	CreatedAt      string             `json:"createdAt"`
	CanBuy         bool               `json:"canBuy"`
}

// GetAllMarketListings 获取懒猫云二手流转集市挂单 (自动分页收集全部)
func (c *Client) GetAllMarketListings(ctx context.Context) ([]LazyMarketListing, error) {
	if c.token == "" {
		return []LazyMarketListing{}, nil
	}

	pageSize := 30
	var allItems []LazyMarketListing

	for page := 1; page <= 15; page++ {
		path := fmt.Sprintf("/instance-market/listings?page=%d&pageSize=%d", page, pageSize)
		data, err := c.doRequest(ctx, "GET", path, nil)
		if err != nil {
			if len(allItems) > 0 {
				break
			}
			return nil, err
		}

		var raw struct {
			Total    int                 `json:"total"`
			Items    []LazyMarketListing `json:"items"`
			Listings []LazyMarketListing `json:"listings"`
			Data     []LazyMarketListing `json:"data"`
		}
		_ = json.Unmarshal(data, &raw)
		batch := raw.Items
		if len(batch) == 0 {
			batch = raw.Listings
		}
		if len(batch) == 0 {
			batch = raw.Data
		}
		if len(batch) == 0 {
			break
		}

		allItems = append(allItems, batch...)

		if raw.Total > 0 && len(allItems) >= raw.Total {
			break
		}
		if len(batch) < pageSize {
			break
		}
	}

	return allItems, nil
}
