package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	// 懒猫云凭证
	LazyCatToken    string
	LazyCatInstance string

	// Akile 凭证
	AkileToken string

	// ACCK 凭证
	ACCKToken string

	// 网络代理配置 (避免境内直连 Cloudflare 403)
	ProxyURL string

	// 通知凭证
	TelegramBotToken string
	TelegramChatID   string

	// 服务端口配置
	HTTPPort string
	MCPPort  string
}

var Global = &Config{}

func Load() *Config {
	candidates := []string{
		"/app/data/.env",
		"/mnt/user/appdata/akile-monitor/.env",
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
		ProxyURL:         getEnv("PROXY_URL", getEnv("HTTPS_PROXY", getEnv("HTTP_PROXY", "http://192.168.8.11:7890"))),
		TelegramBotToken: getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramChatID:   getEnv("TELEGRAM_CHAT_ID", ""),
		HTTPPort:         getEnv("VPS_HUB_HTTP_PORT", "3338"),
		MCPPort:          getEnv("VPS_HUB_MCP_PORT", "3337"),
	}

	if Global.LazyCatToken == "" {
		for _, tf := range []string{"/app/data/lazycat_token.txt", "/mnt/user/appdata/akile-monitor/lazycat_token.txt"} {
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
