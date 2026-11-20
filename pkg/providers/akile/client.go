package akile

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Client struct {
	baseAI     string
	baseCloud  string
	token      string
	httpClient *http.Client
}

func NewClient(token, proxyURL string) *Client {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 12 * time.Second,
		IdleConnTimeout:       15 * time.Second,
		DisableKeepAlives:     true,
	}
	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}

	return &Client{
		baseAI:    "https://api.akile.ai/api",
		baseCloud: "https://api.akile.ai/api",
		token:     strings.Trim(strings.TrimSpace(token), "\""),
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   15 * time.Second,
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

type AkileServerItem struct {
	ID           int     `json:"id"`
	ServerID     string  `json:"server_id"`
	Name         string  `json:"name"`
	ServerName   string  `json:"server_name"`
	ServerReName string  `json:"server_re_name"`
	NodeName     string  `json:"node_name"`
	IP           string  `json:"ip"`
	Status       int     `json:"status"`
	DueTime      int64   `json:"due_time"`
	ExpireDate   string  `json:"expire_date"`
	AreaName     string  `json:"area_name"`
	Bandwidth    int     `json:"bandwidth"`
	Flow         int     `json:"flow"`
	UsedFlow     int64   `json:"used_flow"`
	Price        float64 `json:"price"`
	RenewPrice   float64 `json:"renew_price"`
	AutoRenew    bool    `json:"auto_renew"`
	InitUsername string  `json:"init_username"`
	InitPassword string  `json:"init_password"`
	SystemName   string  `json:"system_name"`
	CPU          int     `json:"cpu"`
	Memory       int     `json:"memory"`
	Disk         int     `json:"disk"`
}

type AkileIpAddressItem struct {
	Type     string `json:"type"` // "ipv4" or "ipv6"
	Address  string `json:"address"`
	CIDR     int    `json:"cidr"`
	Gateway  string `json:"gateway"`
	PublicIP string `json:"public_ip"`
}

type AkileIpResponse struct {
	StatusCode int    `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
	List       []struct {
		ServerID string               `json:"server_id"`
		Address  []AkileIpAddressItem `json:"address"`
	} `json:"list"`
}

func (c *Client) GetServerIpAddress(ctx context.Context, serverID int) (ipv4, ipv6 string, err error) {
	path := fmt.Sprintf("/v1/server/GetIpAddress?id=%d", serverID)
	data, err := c.doRequest(ctx, "GET", c.baseAI, path, nil)
	if err != nil {
		return "", "", err
	}
	var res AkileIpResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return "", "", err
	}
	for _, item := range res.List {
		for _, addr := range item.Address {
			if strings.EqualFold(addr.Type, "ipv4") && ipv4 == "" {
				ipv4 = addr.Address
			} else if strings.EqualFold(addr.Type, "ipv6") && ipv6 == "" {
				ipv6 = addr.Address
			}
		}
	}
	return ipv4, ipv6, nil
}

type InstanceListResponse struct {
	Code int `json:"code"`
	Data struct {
		List []AkileServerItem `json:"list"`
	} `json:"data"`
	List []AkileServerItem `json:"list"`
}

func (c *Client) ListInstances(ctx context.Context) ([]AkileServerItem, error) {
	payload := map[string]interface{}{
		"page_num":  1,
		"page_size": 50,
	}
	data, err := c.doRequest(ctx, "POST", c.baseCloud, "/v1/server/GetServerV2List", payload)
	if err != nil {
		return nil, err
	}

	var res InstanceListResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("json unmarshal failed: %w", err)
	}
	if len(res.Data.List) > 0 {
		return res.Data.List, nil
	}
	return res.List, nil
}

type SecondhandItem struct {
	ID            json.RawMessage `json:"id"`
	ServerID      int             `json:"server_id"`
	Price         json.RawMessage `json:"price"` // 金额单位：分 (字符串或数字，如 "2300")
	ServerPrice   float64         `json:"server_price"` // 续费原价单位：分 (如 2499)
	Name          string          `json:"name"`
	PlanName      string          `json:"plan_name"`
	NodeName      string          `json:"node_name"`
	AreaName      string          `json:"area_name"`
	CPU           int             `json:"cpu"`
	Memory        int             `json:"memory"`
	Disk          int             `json:"disk"`
	Bandwidth     int             `json:"bandwidth"`
	Flow          int             `json:"flow"`
	FlowUsed      int64           `json:"flow_used"`
	DueTime       int64           `json:"due_time"`
	Detail        string          `json:"detail"`
	IPCheckDetail string          `json:"ip_check_detail"`
	IPv4Num       int             `json:"ipv4_num"`
	IPv6Num       int             `json:"ipv6_num"`
	CreateTime    string          `json:"create_time"`
	ResetPrice    float64         `json:"reset_price"`   // 流量重置费用：分 (如 2000)
	ServerCycle   int             `json:"server_cycle"`  // 计费周期 (如 1)
	IsBid         bool            `json:"is_bid"`        // 是否拍卖竞价模式
}

func (item *SecondhandItem) GetIDStr() string {
	return strings.Trim(string(item.ID), "\"")
}

func (item *SecondhandItem) GetPriceYuan() float64 {
	s := strings.Trim(string(item.Price), "\"")
	val, _ := strconv.ParseFloat(s, 64)
	return val / 100.0
}

func (item *SecondhandItem) GetRenewalPriceYuan() float64 {
	return item.ServerPrice / 100.0
}

func (item *SecondhandItem) GetResetPriceYuan() float64 {
	if item.ResetPrice > 0 {
		return item.ResetPrice / 100.0
	}
	return 0
}

func (item *SecondhandItem) GetFlowUsedStr() string {
	usedGB := float64(item.FlowUsed) / (1024.0 * 1024.0 * 1024.0)
	if item.Flow > 0 {
		return fmt.Sprintf("%.1f GB / %d GB", usedGB, item.Flow)
	}
	if usedGB > 0 {
		return fmt.Sprintf("%.1f GB", usedGB)
	}
	return ""
}

func (c *Client) GetSecondhandList(ctx context.Context, page, pageSize int) ([]SecondhandItem, error) {
	return c.GetAllSecondhandList(ctx)
}

// GetAllSecondhandList 动态依据 total 分页穿透抓取全部在售二手机器
func (c *Client) GetAllSecondhandList(ctx context.Context) ([]SecondhandItem, error) {
	pageSize := 30
	var allItems []SecondhandItem

	payload := map[string]interface{}{
		"page_num":  1,
		"page_size": pageSize,
	}
	data, err := c.doRequest(ctx, "POST", c.baseCloud, "/v1/pushshop/GetPushProductList", payload)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Code  int              `json:"code"`
		Total int              `json:"total"`
		List  []SecondhandItem `json:"list"`
		Data  struct {
			Total int              `json:"total"`
			List  []SecondhandItem `json:"list"`
		} `json:"data"`
	}
	_ = json.Unmarshal(data, &raw)
	items := raw.List
	if len(items) == 0 {
		items = raw.Data.List
	}
	allItems = append(allItems, items...)

	total := raw.Total
	if total == 0 {
		total = raw.Data.Total
	}

	if total > pageSize {
		pages := (total + pageSize - 1) / pageSize
		if pages > 15 {
			pages = 15 // 安全上限，最多 450 台
		}

		var wg sync.WaitGroup
		var mu sync.Mutex

		for p := 2; p <= pages; p++ {
			wg.Add(1)
			go func(pageNum int) {
				defer wg.Done()
				pPayload := map[string]interface{}{
					"page_num":  pageNum,
					"page_size": pageSize,
				}
				pData, pErr := c.doRequest(ctx, "POST", c.baseCloud, "/v1/pushshop/GetPushProductList", pPayload)
				if pErr == nil {
					var pRaw struct {
						List []SecondhandItem `json:"list"`
						Data struct {
							List []SecondhandItem `json:"list"`
						} `json:"data"`
					}
					_ = json.Unmarshal(pData, &pRaw)
					pItems := pRaw.List
					if len(pItems) == 0 {
						pItems = pRaw.Data.List
					}
					if len(pItems) > 0 {
						mu.Lock()
						allItems = append(allItems, pItems...)
						mu.Unlock()
					}
				}
			}(p)
		}
		wg.Wait()
	}

	return allItems, nil
}

// BuyPushProduct 购买二手 PushShop 挂单商品
func (c *Client) BuyPushProduct(ctx context.Context, productID int) error {
	payload := map[string]interface{}{"id": productID}
	_, err := c.doRequest(ctx, "POST", c.baseCloud, "/v1/pushshop/BuyPushProduct", payload)
	return err
}

func (c *Client) RefreshToken(ctx context.Context) (string, error) {
	if c.token == "" {
		return "", fmt.Errorf("当前未配置 Token，无法续期")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.akile.ai/api/v1/user/refreshToken", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", c.token)
	req.Header.Set("token", c.token)
	req.Header.Set("Origin", "https://akile.ai")
	req.Header.Set("Referer", "https://akile.ai/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/126.0 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("akile 续期失败 (HTTP %d): %s", resp.StatusCode, string(respBytes))
	}

	var res struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
		Data       struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return "", fmt.Errorf("解析续期响应失败: %w", err)
	}

	tok := res.Data.Token
	if tok == "" || !strings.Contains(tok, ".") {
		return "", fmt.Errorf("续期响应未包含有效 Token: %s", string(respBytes))
	}

	c.token = tok
	return tok, nil
}

type UserInfoResponse struct {
	StatusCode int `json:"status_code"`
	Data       struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	} `json:"data"`
}

func (c *Client) GetUserInfo(ctx context.Context) (*UserInfoResponse, error) {
	data, err := c.doRequest(ctx, "GET", c.baseAI, "/v1/user/info", nil)
	if err != nil {
		return nil, err
	}
	var res UserInfoResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

type AkileStorePlanPrice struct {
	ID    int     `json:"id"`
	Price float64 `json:"price"` // 金额单位：分
	Cycle int     `json:"cycle"`
}

type AkileStorePlan struct {
	ID         int                   `json:"id"`
	PlanName   string                `json:"plan_name"`
	CPU        int                   `json:"cpu"`
	Memory     int                   `json:"memory"`
	Disk       int                   `json:"disk"`
	IPv4Num    int                   `json:"ipv4_num"`
	IPv6Num    int                   `json:"ipv6_num"`
	Bandwidth  int                   `json:"bandwidth"`
	Flow       int                   `json:"flow"`
	PriceDatas []AkileStorePlanPrice `json:"price_datas"`
}

type AkileStoreNode struct {
	ID        int              `json:"id"`
	GroupName string           `json:"group_name"`
	Plans     []AkileStorePlan `json:"plans"`
}

type AkileStoreArea struct {
	ID       int              `json:"id"`
	AreaName string           `json:"area_name"`
	Nodes    []AkileStoreNode `json:"nodes"`
}

type AkileStoreResponse struct {
	StatusCode int    `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
	Data       struct {
		Areas []AkileStoreArea `json:"areas"`
	} `json:"data"`
}

// GetVpsStoreV3 获取 Akile 官方商城全量真实在售规格 (无需鉴权)
func (c *Client) GetVpsStoreV3(ctx context.Context) ([]AkileStoreArea, error) {
	data, err := c.doRequest(ctx, "GET", c.baseAI, "/v1/store/GetVpsStoreV3?areaId=1", nil)
	if err != nil {
		return nil, err
	}
	var res AkileStoreResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return res.Data.Areas, nil
}

type AkileLoginResponse struct {
	StatusCode int    `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
	Data       struct {
		Token string `json:"token"`
	} `json:"data"`
}

// Login 使用账号密码 (可选 2FA 动态码) 登录 Akile
func (c *Client) Login(ctx context.Context, email, password, gaCode string) (token string, need2FA bool, msg string, err error) {
	payload := map[string]string{
		"email":      email,
		"password":   password,
		"token":      strings.TrimSpace(gaCode),
		"verifyCode": "",
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.akile.ai/api/v1/user/login", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", false, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://akile.ai")
	req.Header.Set("Referer", "https://akile.ai/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/126.0 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", false, "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, "", err
	}

	var res AkileLoginResponse
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return "", false, "", fmt.Errorf("解析 Akile 登录响应失败: %w", err)
	}

	// 判定 2FA 二次验证码需求
	if strings.TrimSpace(gaCode) == "" && (strings.Contains(res.StatusMsg, "2FA") || strings.Contains(res.StatusMsg, "二次验证") || strings.Contains(res.StatusMsg, "验证码") || strings.Contains(res.StatusMsg, "动态码")) {
		return "", true, res.StatusMsg, nil
	}

	if res.StatusCode != 0 {
		return "", false, res.StatusMsg, fmt.Errorf("Akile 登录失败: %s", res.StatusMsg)
	}

	tok := res.Data.Token
	if tok == "" || !strings.Contains(tok, ".") {
		return "", false, res.StatusMsg, fmt.Errorf("登录成功但未返回有效 Token: %s", string(respBytes))
	}

	c.token = tok
	return tok, false, "登录成功", nil
}

