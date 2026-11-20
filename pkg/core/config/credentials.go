package config

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	_ = os.MkdirAll("./data", 0755)
	return "./data/credentials.json"
}

// LoadSavedCredentials 读取持久化保存的账号密码凭据
func LoadSavedCredentials() *SavedCredentials {
	credMu.RLock()
	defer credMu.RUnlock()

	p := getCredentialsPath()
	b, err := os.ReadFile(p)
	if err != nil || len(b) == 0 {
		return &SavedCredentials{}
	}

	var sc SavedCredentials
	if err := json.Unmarshal(b, &sc); err != nil {
		return &SavedCredentials{}
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
