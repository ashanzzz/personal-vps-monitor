package sshexec

import (
	"encoding/base64"
	"strings"
	"testing"
)

// 测试 x25519 Reality 密钥生成是否符合 Xray 43 字符 RawURLEncoding 标准
func TestGenerateRealityKeyPair(t *testing.T) {
	priv, pub, sid, err := GenerateRealityKeyPair()
	if err != nil {
		t.Fatalf("GenerateRealityKeyPair failed: %v", err)
	}

	if len(priv) != 43 {
		t.Errorf("expected private key length 43, got %d (%s)", len(priv), priv)
	}
	if len(pub) != 43 {
		t.Errorf("expected public key length 43, got %d (%s)", len(pub), pub)
	}
	if len(sid) != 8 {
		t.Errorf("expected short ID length 8, got %d (%s)", len(sid), sid)
	}

	// 验证 Base64 RawURLEncoding 可正确解码为 32 字节
	privBytes, err := base64.RawURLEncoding.DecodeString(priv)
	if err != nil || len(privBytes) != 32 {
		t.Errorf("invalid private key bytes: %v (len: %d)", err, len(privBytes))
	}
	pubBytes, err := base64.RawURLEncoding.DecodeString(pub)
	if err != nil || len(pubBytes) != 32 {
		t.Errorf("invalid public key bytes: %v (len: %d)", err, len(pubBytes))
	}
}

// 测试 3x-ui SQLite JSON 解析与监听匹配
func TestParse3xUIInbounds(t *testing.T) {
	rawJSON := `[
  {
    "id": 1,
    "port": 36563,
    "protocol": "vless",
    "settings": "{\"clients\": [{\"id\": \"5b84d275-d328-4565-9b47-20e9121fee7e\", \"flow\": \"xtls-rprx-vision\", \"email\": \"ashan\"}], \"decryption\": \"none\"}",
    "stream_settings": "{\"network\":\"tcp\",\"security\":\"reality\",\"realitySettings\":{\"serverNames\":[\"aws.amazon.com\"],\"shortIds\":[\"3c4ffcc0\"],\"settings\":{\"publicKey\":\"Djl4lJnk4RIOWfywQJxvCRwhGyZdUdSk9Xquv-35TzI\"}}}",
    "enable": 1,
    "remark": "美国-acck-lax4837isp"
  },
  {
    "id": 2,
    "port": 10085,
    "protocol": "vless",
    "settings": "{\"clients\": [{\"id\": \"b8867c3d-b661-4274-b86a-391f171196bc\"}]}",
    "stream_settings": "{\"network\":\"ws\",\"security\":\"none\",\"wsSettings\":{\"path\":\"/cf-vless\"}}",
    "enable": 1,
    "remark": "美国-LAX-CF救活-WS"
  }
]`

	listeningMap := map[int]bool{
		36563: true,
		10085: true,
	}

	proxies := parse3xUIInbounds(rawJSON, listeningMap)
	if len(proxies) != 2 {
		t.Fatalf("expected 2 proxies, got %d", len(proxies))
	}

	// 验证节点 1
	p1 := proxies[0]
	if p1.Port != 36563 || p1.Protocol != "vless" || p1.Security != "reality" {
		t.Errorf("unexpected p1: %+v", p1)
	}
	if p1.UUID != "5b84d275-d328-4565-9b47-20e9121fee7e" {
		t.Errorf("unexpected uuid: %s", p1.UUID)
	}
	if p1.PublicKey != "Djl4lJnk4RIOWfywQJxvCRwhGyZdUdSk9Xquv-35TzI" {
		t.Errorf("unexpected pubkey: %s", p1.PublicKey)
	}
	if p1.SNI != "aws.amazon.com" || p1.ShortID != "3c4ffcc0" {
		t.Errorf("unexpected sni/shortid: %s / %s", p1.SNI, p1.ShortID)
	}
	if p1.Status != "listening" {
		t.Errorf("expected listening, got %s", p1.Status)
	}

	// 验证节点 2 (WebSocket)
	p2 := proxies[1]
	if p2.Port != 10085 || p2.Protocol != "vless_ws" || p2.Path != "/cf-vless" {
		t.Errorf("unexpected p2: %+v", p2)
	}
	if p2.Status != "listening" {
		t.Errorf("expected listening, got %s", p2.Status)
	}
}

// 测试 Clash / Mihomo 与 Sing-box 链式代理配置生成
func TestChainProxyGeneration(t *testing.T) {
	relay := RelayNode{
		Name:      "🇺🇸 美西-4837-中继",
		Tag:       "relay-lax",
		Server:    "38.175.222.44",
		Port:      36563,
		Protocol:  "vless",
		UUID:      "5b84d275-d328-4565-9b47-20e9121fee7e",
		SNI:       "aws.amazon.com",
		PublicKey: "Djl4lJnk4RIOWfywQJxvCRwhGyZdUdSk9Xquv-35TzI",
		ShortID:   "3c4ffcc0",
	}

	chains := []ChainNode{
		{
			Name:        "🔗 链式|美西4837 ➔ 🇺🇸 美国-Pavlov家宽",
			Tag:         "chain-lax-to-us",
			RelayName:   relay.Name,
			RelayTag:    relay.Tag,
			DialerProxy: relay.Name,
			Detour:      relay.Tag,
			Server:      "167.104.167.151",
			Port:        49040,
			Protocol:    "socks",
			Username:    "user5",
			Password:    "i5uHEX9OhLbDhoDc3PfNLQ",
		},
	}

	clashYAML := generateClashConfig([]RelayNode{relay}, chains)
	if !strings.Contains(clashYAML, "dialer-proxy: \"🇺🇸 美西-4837-中继\"") {
		t.Errorf("clash yaml missing dialer-proxy: %s", clashYAML)
	}
	if !strings.Contains(clashYAML, "reality-opts:") || !strings.Contains(clashYAML, "public-key: Djl4lJnk4RIOWfywQJxvCRwhGyZdUdSk9Xquv-35TzI") {
		t.Errorf("clash yaml missing reality opts: %s", clashYAML)
	}

	singboxJSON := generateSingBoxConfig([]RelayNode{relay}, chains)
	if !strings.Contains(singboxJSON, "\"detour\": \"relay-lax\"") {
		t.Errorf("singbox json missing detour: %s", singboxJSON)
	}
	if !strings.Contains(singboxJSON, "\"public_key\": \"Djl4lJnk4RIOWfywQJxvCRwhGyZdUdSk9Xquv-35TzI\"") {
		t.Errorf("singbox json missing reality public key: %s", singboxJSON)
	}
}

// 测试实例快速查询容错
func TestFindInstance(t *testing.T) {
	_, err := FindInstance("")
	if err == nil {
		t.Errorf("expected error on empty input")
	}

	// 查找 13483 或 38.175.222.44
	inst, err := FindInstance("13483")
	if err == nil {
		if inst.IP != "38.175.222.44" {
			t.Errorf("expected IP 38.175.222.44, got %s", inst.IP)
		}
		if inst.SSHUser != "root" || inst.SSHPort != 22 {
			t.Errorf("unexpected ssh defaults: %s@%d", inst.SSHUser, inst.SSHPort)
		}
	}
}
