package vault

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/config"
	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/pbkdf2"
)

var (
	vaultMu sync.Mutex
)

// Decryptor 处理 Bitwarden/Vaultwarden AES-CBC + HMAC-SHA256 密文
type Decryptor struct {
	EncKey []byte
	MacKey []byte
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty data")
	}
	padLen := int(data[len(data)-1])
	if padLen == 0 || padLen > len(data) || padLen > aes.BlockSize {
		return nil, errors.New("invalid padding")
	}
	for i := len(data) - padLen; i < len(data); i++ {
		if data[i] != byte(padLen) {
			return nil, errors.New("invalid padding byte")
		}
	}
	return data[:len(data)-padLen], nil
}

// Raw 解密 encString 格式: '<type>.<b64 iv>|<b64 data>|<b64 mac>'
func (d *Decryptor) Raw(ct string) ([]byte, error) {
	if ct == "" {
		return nil, nil
	}
	parts := strings.SplitN(ct, ".", 2)
	if len(parts) != 2 {
		return []byte(ct), nil
	}
	encType := parts[0]
	subParts := strings.Split(parts[1], "|")
	if len(subParts) < 2 {
		return nil, errors.New("malformed cipher string")
	}

	iv, err := base64.StdEncoding.DecodeString(subParts[0])
	if err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(subParts[1])
	if err != nil {
		return nil, err
	}

	if len(subParts) > 2 && subParts[2] != "" {
		mac, err := base64.StdEncoding.DecodeString(subParts[2])
		if err == nil && len(mac) > 0 {
			h := hmac.New(sha256.New, d.MacKey)
			h.Write(iv)
			h.Write(data)
			expectedMac := h.Sum(nil)
			if !hmac.Equal(expectedMac, mac) {
				return nil, errors.New("mac verification failed")
			}
		}
	}

	key := d.EncKey
	if encType == "0" && len(key) >= 16 {
		key = key[:16]
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(data)%block.BlockSize() != 0 {
		return nil, errors.New("ciphertext is not a multiple of block size")
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	decrypted := make([]byte, len(data))
	mode.CryptBlocks(decrypted, data)

	return pkcs7Unpad(decrypted)
}

// Text 解密并转为 UTF-8 字符串
func (d *Decryptor) Text(ct string) string {
	if ct == "" {
		return ""
	}
	if !strings.Contains(ct, ".") {
		return ct
	}
	raw, err := d.Raw(ct)
	if err != nil {
		return ""
	}
	return string(raw)
}

func hkdfExpand(prk, info []byte, length int) ([]byte, error) {
	r := hkdf.Expand(sha256.New, prk, info)
	out := make([]byte, length)
	if _, err := io.ReadFull(r, out); err != nil {
		return nil, err
	}
	return out, nil
}

// GenerateTOTP 从 secret 或 otpauth URI 中计算当前 6 位 2FA 动态码 (RFC 6238)
func GenerateTOTP(secretOrURI string) (string, error) {
	secret := strings.TrimSpace(secretOrURI)
	if secret == "" {
		return "", errors.New("empty totp secret")
	}

	if strings.HasPrefix(secret, "otpauth://") {
		u, err := url.Parse(secret)
		if err == nil {
			if s := u.Query().Get("secret"); s != "" {
				secret = s
			}
		}
	}

	// 清理格式并转换为大写
	secret = strings.ToUpper(strings.ReplaceAll(secret, " ", ""))
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		key, err = base32.StdEncoding.DecodeString(secret)
		if err != nil {
			return "", fmt.Errorf("base32 decode totp secret failed: %w", err)
		}
	}

	counter := uint64(time.Now().Unix() / 30)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)

	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0xf
	code := (binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff) % 1000000
	return fmt.Sprintf("%06d", code), nil
}

// VaultItem 解密后的凭据项
type VaultItem struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	TOTPSecret string `json:"totp_secret,omitempty"`
	Notes      string `json:"notes,omitempty"`
}

// CipherSummary 密码条目摘要，用于前端安全展示与下拉选择（不含明文密码）
type CipherSummary struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Username       string `json:"username,omitempty"`
	UsernameMasked string `json:"username_masked"`
	HasPassword    bool   `json:"has_password"`
	HasTOTP        bool   `json:"has_totp"`
	Notes          string `json:"notes,omitempty"`
}

// DefaultVaultServer 默认服务器地址
func DefaultVaultServer() string {
	if s := os.Getenv("VAULT_SERVER"); s != "" {
		return strings.TrimRight(s, "/")
	}
	if s := os.Getenv("VAULT_URL"); s != "" {
		return strings.TrimRight(s, "/")
	}
	if sc := config.LoadSavedCredentials(); sc != nil && sc.Vaultwarden != nil && sc.Vaultwarden.Server != "" {
		return strings.TrimRight(sc.Vaultwarden.Server, "/")
	}
	return "http://192.168.8.11:4743"
}

// DefaultVaultEmail 默认账号邮箱
func DefaultVaultEmail() string {
	if s := os.Getenv("VAULT_EMAIL"); s != "" {
		return strings.TrimSpace(s)
	}
	if sc := config.LoadSavedCredentials(); sc != nil && sc.Vaultwarden != nil && sc.Vaultwarden.Username != "" {
		return strings.TrimSpace(sc.Vaultwarden.Username)
	}
	return "ashanzzz1213@gmail.com"
}

// DefaultVaultPassword 默认主密码
func DefaultVaultPassword() string {
	if s := os.Getenv("VAULT_PASSWORD"); s != "" {
		return s
	}
	return "Woo@@@204317"
}

// fetchAndDecryptVault 统一底层：连接 Vaultwarden、解密所有 Cipher 条目
func fetchAndDecryptVault(ctx context.Context, server, email, password string) ([]*VaultItem, error) {
	if server == "" {
		server = DefaultVaultServer()
	}
	if email == "" {
		email = DefaultVaultEmail()
	}
	if password == "" {
		password = DefaultVaultPassword()
	}

	server = strings.TrimRight(server, "/")

	// 构建支持穿透本地 NO_PROXY 的 HTTP Client
	transport := &http.Transport{
		Proxy: nil, // 直连局域网，不走代理以防超时
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
	}

	// 1. Prelogin 获取 KDF 参数
	preReqBody, _ := json.Marshal(map[string]string{"email": email})
	req, err := http.NewRequestWithContext(ctx, "POST", server+"/identity/accounts/prelogin", bytes.NewReader(preReqBody))
	if err != nil {
		return nil, fmt.Errorf("prelogin request build failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connect to vaultwarden prelogin failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("vaultwarden prelogin returned HTTP %d", resp.StatusCode)
	}

	var preResp struct {
		Kdf           int `json:"kdf"`
		KdfIterations int `json:"kdfIterations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&preResp); err != nil {
		return nil, fmt.Errorf("decode prelogin response failed: %w", err)
	}

	iterations := preResp.KdfIterations
	if iterations <= 0 {
		iterations = 600000
	}

	// 2. 派生主密钥与加密密钥
	masterKey := pbkdf2.Key([]byte(password), []byte(strings.ToLower(email)), iterations, 32, sha256.New)
	encKey, _ := hkdfExpand(masterKey, []byte("enc"), 32)
	macKey, _ := hkdfExpand(masterKey, []byte("mac"), 32)

	// 3. 计算用于 Token 签发的 Password Hash
	pwHashRaw := pbkdf2.Key(masterKey, []byte(password), 1, 32, sha256.New)
	pwHash := base64.StdEncoding.EncodeToString(pwHashRaw)

	// 4. 请求 Access Token
	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("username", email)
	form.Set("password", pwHash)
	form.Set("scope", "api offline_access")
	form.Set("client_id", "browser")
	form.Set("deviceType", "10")
	form.Set("deviceIdentifier", "personal-vps-monitor-agent")
	form.Set("deviceName", "personal-vps-monitor")

	req, err = http.NewRequestWithContext(ctx, "POST", server+"/identity/connect/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err = client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vaultwarden token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vaultwarden auth failed HTTP %d: %s", resp.StatusCode, string(b))
	}

	var tokResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokResp); err != nil {
		return nil, err
	}

	// 5. 拉取全量同步数据 (/api/sync)
	req, err = http.NewRequestWithContext(ctx, "GET", server+"/api/sync", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tokResp.AccessToken)
	resp, err = client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vaultwarden sync request failed: %w", err)
	}
	defer resp.Body.Close()

	var syncResp struct {
		Profile struct {
			Key string `json:"key"`
		} `json:"profile"`
		Ciphers []struct {
			ID    string `json:"id"`
			Type  int    `json:"type"`
			Name  string `json:"name"`
			Notes string `json:"notes"`
			Login *struct {
				Username string `json:"username"`
				Password string `json:"password"`
				Totp     string `json:"totp"`
			} `json:"login"`
		} `json:"ciphers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&syncResp); err != nil {
		return nil, fmt.Errorf("decode sync response failed: %w", err)
	}

	// 6. 解密 User Encryption Key
	d := &Decryptor{EncKey: encKey, MacKey: macKey}
	if syncResp.Profile.Key != "" {
		userKey, err := d.Raw(syncResp.Profile.Key)
		if err == nil {
			if len(userKey) != 64 {
				if maybe, err := base64.StdEncoding.DecodeString(string(userKey)); err == nil && len(maybe) == 64 {
					userKey = maybe
				}
			}
			if len(userKey) == 64 {
				d.EncKey = userKey[:32]
				d.MacKey = userKey[32:]
			}
		}
	}

	// 7. 解密所有 Cipher
	var items []*VaultItem
	for _, c := range syncResp.Ciphers {
		name := d.Text(c.Name)
		var u, p, totp string
		if c.Login != nil {
			u = d.Text(c.Login.Username)
			p = d.Text(c.Login.Password)
			totp = d.Text(c.Login.Totp)
		}

		item := &VaultItem{
			ID:         c.ID,
			Name:       name,
			Username:   u,
			Password:   p,
			TOTPSecret: totp,
			Notes:      d.Text(c.Notes),
		}
		items = append(items, item)
	}

	return items, nil
}

// ListVaultCiphers 拉取解密后的所有密码条目摘要，用于前端安全选择（不含明文密码）
func ListVaultCiphers(ctx context.Context, server, email, password string) ([]CipherSummary, error) {
	vaultMu.Lock()
	defer vaultMu.Unlock()

	items, err := fetchAndDecryptVault(ctx, server, email, password)
	if err != nil {
		return nil, err
	}

	var summaries []CipherSummary
	for _, it := range items {
		summaries = append(summaries, CipherSummary{
			ID:             it.ID,
			Name:           it.Name,
			Username:       it.Username,
			UsernameMasked: config.MaskUsername(it.Username),
			HasPassword:    it.Password != "",
			HasTOTP:        it.TOTPSecret != "",
			Notes:          it.Notes,
		})
	}
	return summaries, nil
}

// GetVaultCipherByID 根据 ID 提取指定 Cipher 条目详情（含解密后的密码与 TOTP 密钥）
func GetVaultCipherByID(ctx context.Context, server, email, password, cipherID string) (*VaultItem, error) {
	vaultMu.Lock()
	defer vaultMu.Unlock()

	items, err := fetchAndDecryptVault(ctx, server, email, password)
	if err != nil {
		return nil, err
	}

	for _, it := range items {
		if it.ID == cipherID {
			return it, nil
		}
	}
	return nil, fmt.Errorf("未在密码库中找到 ID 为 %s 的条目", cipherID)
}

// PullAndSyncFromVaultwarden 从 Vaultwarden 登录、同步、解密并自动存入监控系统凭据库
func PullAndSyncFromVaultwarden(ctx context.Context, server, email, password string) ([]*VaultItem, error) {
	vaultMu.Lock()
	defer vaultMu.Unlock()

	items, err := fetchAndDecryptVault(ctx, server, email, password)
	if err != nil {
		return nil, err
	}

	var matchedItems []*VaultItem
	for _, item := range items {
		nameLower := strings.ToLower(item.Name)
		u := item.Username
		p := item.Password
		totp := item.TOTPSecret

		// 智能识别并自动持久化回写至监控凭据库
		if strings.Contains(nameLower, "akile") && p != "" {
			_ = config.SaveProviderCredentialWithTOTP("akile", u, p, totp, true)
			matchedItems = append(matchedItems, item)
		}
		if (strings.Contains(nameLower, "lazycat") || strings.Contains(nameLower, "懒猫")) && p != "" {
			_ = config.SaveProviderCredentialWithTOTP("lazycat", u, p, totp, true)
			matchedItems = append(matchedItems, item)
		}
		if (strings.Contains(nameLower, "acck") || strings.Contains(nameLower, "acck.io")) && p != "" {
			_ = config.SaveProviderCredentialWithTOTP("acck", u, p, totp, true)
			matchedItems = append(matchedItems, item)
		}
	}

	return matchedItems, nil
}
