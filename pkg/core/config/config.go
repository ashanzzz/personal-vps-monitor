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

	// 通知凭证
	TelegramBotToken string
	TelegramChatID   string

	// 服务端口配置
	HTTPPort string
	MCPPort  string
}

var Global = &Config{}

func Load() *Config {
	// 尝试优先读取 /opt/data/.env 或当前目录 .env
	candidates := []string{
		"/opt/data/.env",
		".env",
		filepath.Join(os.Getenv("HOME"), ".env"),
	}

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			_ = godotenv.Load(p)
			break
		}
	}

	Global = &Config{
		LazyCatToken:     getEnv("LAZYCAT_TOKEN", ""),
		LazyCatInstance:  getEnv("LAZY_INSTANCE", "11433"),
		AkileToken:       getEnv("AKILE_TOKEN", ""),
		ACCKToken:        getEnv("ACCK_TOKEN", ""),
		TelegramBotToken: getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramChatID:   getEnv("TELEGRAM_CHAT_ID", ""),
		HTTPPort:         getEnv("VPS_HUB_HTTP_PORT", "3338"),
		MCPPort:          getEnv("VPS_HUB_MCP_PORT", "3337"),
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
