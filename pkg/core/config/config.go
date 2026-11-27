package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/joho/godotenv"
)

type Config struct {
	// 懒猫云凭证
	LazyCatToken    string `json:"lazycat_token"`
	LazyCatInstance string `json:"lazycat_instance"`

	// Akile 凭证
	AkileToken string `json:"akile_token"`

	// ACCK 凭证
	ACCKToken string `json:"acck_token"`

	// 网络代理配置 (避免境内直连 Cloudflare 403)
	ProxyURL        string `json:"proxy_url"`
	LazyCatProxyURL string `json:"lazycat_proxy_url"`

	// 通知凭证
	TelegramBotToken string `json:"telegram_bot_token"`
	TelegramChatID   string `json:"telegram_chat_id"`

	// 服务端口配置
	HTTPPort string `json:"http_port"`
	MCPPort  string `json:"mcp_port"`

	// 统一访问与开放 API 鉴权密钥
	PublicReadToken    string `json:"public_read_token"`
	PublicControlToken string `json:"public_control_token"`
}

type SavedTokens struct {
	AkileToken      string `json:"akile_token"`
	ACCKToken       string `json:"acck_token"`
	LazyCatToken    string `json:"lazycat_token"`
	ProxyURL        string `json:"proxy_url,omitempty"`
	LazyCatProxyURL string `json:"lazycat_proxy_url,omitempty"`
}

func (c *Config) EffectiveLazyCatProxyURL() string {
	if c != nil && strings.TrimSpace(c.LazyCatProxyURL) != "" {
		return strings.TrimSpace(c.LazyCatProxyURL)
	}
	if c != nil {
		return strings.TrimSpace(c.ProxyURL)
	}
	return ""
}

var (
	Global   = &Config{}
	globalMu sync.RWMutex
)

// GetToken 安全读取内存配置中的服务商 Token
func GetProviderToken(providerID string) string {
	globalMu.RLock()
	defer globalMu.RUnlock()
	if Global == nil {
		return ""
	}
	switch strings.ToLower(providerID) {
	case "akile":
		return Global.AkileToken
	case "acck":
		return Global.ACCKToken
	case "lazycat":
		return Global.LazyCatToken
	}
	return ""
}

// SetProviderToken 并发安全更新内存配置中的服务商 Token
func SetProviderToken(providerID, token string) {
	globalMu.Lock()
	defer globalMu.Unlock()
	if Global == nil {
		Global = &Config{}
	}
	switch strings.ToLower(providerID) {
	case "akile":
		Global.AkileToken = token
	case "acck":
		Global.ACCKToken = token
	case "lazycat":
		Global.LazyCatToken = token
	}
}

func getTokensCandidatePaths() []string {
	paths := []string{"./data/tokens.json"}
	if runtime.GOOS == "linux" {
		paths = append(paths, "/app/data/tokens.json", "/mnt/user/appdata/personal-vps-monitor/tokens.json")
	}
	return paths
}

func getTokensWritePath() string {
	if runtime.GOOS == "linux" {
		for _, p := range []string{"/app/data", "/mnt/user/appdata/personal-vps-monitor"} {
			if fi, err := os.Stat(p); err == nil && fi.IsDir() {
				return filepath.Join(p, "tokens.json")
			}
		}
	}
	_ = os.MkdirAll("./data", 0755)
	return "./data/tokens.json"
}

func LoadSavedTokens() *SavedTokens {
	for _, p := range getTokensCandidatePaths() {
		if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
			var st SavedTokens
			if err := json.Unmarshal(b, &st); err == nil {
				return &st
			}
		}
	}
	return &SavedTokens{}
}

func SaveTokens(akile, acck, lazycat, proxy string) error {
	var proxyPtr *string
	if proxy != "" {
		proxyPtr = &proxy
	}
	return SaveTokensWithProxies(akile, acck, lazycat, proxyPtr, nil)
}

// SaveTokensWithProxies 持久化保存各厂商 Token 与代理设置；指针参数为非 nil 时按传入值覆盖（支持传入空字符串清除）
func SaveTokensWithProxies(akile, acck, lazycat string, proxy, lazycatProxy *string) error {
	target := getTokensWritePath()
	_ = os.MkdirAll(filepath.Dir(target), 0755)

	st := LoadSavedTokens()
	if akile != "" {
		st.AkileToken = akile
		SetProviderToken("akile", akile)
	}
	if acck != "" {
		st.ACCKToken = acck
		SetProviderToken("acck", acck)
	}
	if lazycat != "" {
		st.LazyCatToken = lazycat
		SetProviderToken("lazycat", lazycat)
	}
	if proxy != nil {
		cleanedProxy := strings.TrimSpace(*proxy)
		st.ProxyURL = cleanedProxy
		Global.ProxyURL = cleanedProxy
	}
	if lazycatProxy != nil {
		cleaned := strings.TrimSpace(*lazycatProxy)
		st.LazyCatProxyURL = cleaned
		Global.LazyCatProxyURL = cleaned
	}

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(target, data, 0644)
}

func Load() *Config {
	candidates := []string{
		"/app/data/.env",
		"/mnt/user/appdata/personal-vps-monitor/.env",
		"/opt/data/.env",
		".env",
		filepath.Join(os.Getenv("HOME"), ".env"),
	}

	for _, p := range candidates {
		if envMap, err := godotenv.Read(p); err == nil {
			for k, v := range envMap {
				val := strings.Trim(strings.TrimSpace(v), "\"'")
				if val != "" {
					_ = os.Setenv(k, val)
				}
			}
		}
	}

	Global = &Config{
		LazyCatToken:     getEnv("LAZYCAT_TOKEN", ""),
		LazyCatInstance:  getEnv("LAZY_INSTANCE", "11433"),
		AkileToken:       getEnv("AKILE_TOKEN", ""),
		ACCKToken:        getEnv("ACCK_TOKEN", ""),
		ProxyURL:           getEnv("PROXY_URL", getEnv("HTTPS_PROXY", getEnv("HTTP_PROXY", "http://192.168.8.11:7890"))),
		LazyCatProxyURL:    getEnv("LAZYCAT_PROXY_URL", ""),
		TelegramBotToken:   getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramChatID:   getEnv("TELEGRAM_CHAT_ID", ""),
		HTTPPort:           getEnv("VPS_HUB_HTTP_PORT", "3338"),
		MCPPort:            getEnv("VPS_HUB_MCP_PORT", "3337"),
		PublicReadToken:    getEnv("PUBLIC_READ_TOKEN", ""),
		PublicControlToken: getEnv("PUBLIC_CONTROL_TOKEN", ""),
	}

	// 尝试从持久化 tokens.json 载入覆盖 (文件优先级高于旧环境变量)
	st := LoadSavedTokens()
	if st.AkileToken != "" {
		Global.AkileToken = st.AkileToken
	}
	if st.ACCKToken != "" {
		Global.ACCKToken = st.ACCKToken
	}
	if st.LazyCatToken != "" {
		Global.LazyCatToken = st.LazyCatToken
	}
	if st.ProxyURL != "" {
		Global.ProxyURL = st.ProxyURL
	}
	if st.LazyCatProxyURL != "" {
		Global.LazyCatProxyURL = st.LazyCatProxyURL
	}

	if Global.LazyCatToken == "" {
		for _, tf := range []string{"/app/data/lazycat_token.txt", "/mnt/user/appdata/personal-vps-monitor/lazycat_token.txt"} {
			if b, err := os.ReadFile(tf); err == nil {
				Global.LazyCatToken = strings.Trim(strings.TrimSpace(string(b)), "\"'")
				if Global.LazyCatToken != "" {
					break
				}
			}
		}
	}

	return Global
}

func getEnv(key, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return strings.Trim(strings.TrimSpace(val), "\"'")
}
