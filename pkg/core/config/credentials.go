package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type ProviderCredential struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	AutoRefresh bool   `json:"auto_refresh"`
}

type SavedCredentials struct {
	Akile   *ProviderCredential `json:"akile,omitempty"`
	LazyCat *ProviderCredential `json:"lazycat,omitempty"`
	ACCK    *ProviderCredential `json:"acck,omitempty"`
}

var (
	credMu sync.RWMutex
)

func getCredentialsPath() string {
	paths := []string{"/app/data/credentials.json", "./data/credentials.json"}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if _, err := os.Stat("/app/data"); err == nil {
		return "/app/data/credentials.json"
	}
	_ = os.MkdirAll("./data", 0755)
	return "./data/credentials.json"
}

// MaskUsername 脱敏用户名展示 (例如 ashanzzz -> ash***)
func MaskUsername(u string) string {
	u = strings.TrimSpace(u)
	if len(u) <= 3 {
		return "***"
	}
	return u[:3] + "***"
}

// LoadSavedCredentials 读取持久化保存的账号密码凭据 (支持 credentials.json 与环境变量回退)
func LoadSavedCredentials() *SavedCredentials {
	credMu.RLock()
	defer credMu.RUnlock()

	p := getCredentialsPath()
	b, err := os.ReadFile(p)
	var sc SavedCredentials
	if err == nil && len(b) > 0 {
		_ = json.Unmarshal(b, &sc)
	}

	// 环境变量回退补充
	if (sc.Akile == nil || sc.Akile.Password == "") && os.Getenv("AKILE_PASSWORD") != "" {
		sc.Akile = &ProviderCredential{
			Username:    os.Getenv("AKILE_USERNAME"),
			Password:    os.Getenv("AKILE_PASSWORD"),
			AutoRefresh: true,
		}
	}
	if (sc.LazyCat == nil || sc.LazyCat.Password == "") && os.Getenv("LAZYCAT_PASSWORD") != "" {
		sc.LazyCat = &ProviderCredential{
			Username:    os.Getenv("LAZYCAT_USERNAME"),
			Password:    os.Getenv("LAZYCAT_PASSWORD"),
			AutoRefresh: true,
		}
	}
	if (sc.ACCK == nil || sc.ACCK.Password == "") && os.Getenv("ACCK_PASSWORD") != "" {
		sc.ACCK = &ProviderCredential{
			Username:    os.Getenv("ACCK_USERNAME"),
			Password:    os.Getenv("ACCK_PASSWORD"),
			AutoRefresh: true,
		}
	}

	return &sc
}

// SaveProviderCredential 保存或更新指定厂商的免密登录账号凭据
func SaveProviderCredential(provider, username, password string, autoRefresh bool) error {
	credMu.Lock()
	defer credMu.Unlock()

	sc := LoadSavedCredentials()
	cred := &ProviderCredential{
		Username:    username,
		Password:    password,
		AutoRefresh: autoRefresh,
	}

	switch provider {
	case "akile":
		sc.Akile = cred
	case "lazycat":
		sc.LazyCat = cred
	case "acck":
		sc.ACCK = cred
	}

	data, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		return err
	}

	p := getCredentialsPath()
	_ = os.MkdirAll(filepath.Dir(p), 0755)
	return os.WriteFile(p, data, 0600)
}
