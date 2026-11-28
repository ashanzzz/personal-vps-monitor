package sshexec

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
)

func sampleInbound() SingBoxInbound {
	return SingBoxInbound{
		Tag:        "hkg3-vless",
		ListenPort: 30021,
		UUID:       "11111111-2222-3333-4444-555555555555",
		Flow:       "xtls-rprx-vision",
		SNI:        "www.apple.com",
		PrivateKey: "PRIVATE_KEY_PLACEHOLDER",
		ShortID:    "deadbeef",
	}
}

func TestRenderSingBoxConfig_HKG3(t *testing.T) {
	raw, err := RenderSingBoxConfig(sampleInbound())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var cfg struct {
		Log struct {
			Level  string `json:"level"`
			Output string `json:"output"`
		} `json:"log"`
		Inbounds []struct {
			Type       string `json:"type"`
			Tag        string `json:"tag"`
			Listen     string `json:"listen"`
			ListenPort int    `json:"listen_port"`
			Users      []struct {
				UUID string `json:"uuid"`
				Flow string `json:"flow"`
			} `json:"users"`
			TLS struct {
				Enabled    bool   `json:"enabled"`
				ServerName string `json:"server_name"`
				Reality    struct {
					Enabled    bool     `json:"enabled"`
					PrivateKey string   `json:"private_key"`
					ShortID    []string `json:"short_id"`
					Handshake  struct {
						Server     string `json:"server"`
						ServerPort int    `json:"server_port"`
					} `json:"handshake"`
				} `json:"reality"`
			} `json:"tls"`
		} `json:"inbounds"`
		Outbounds []map[string]string `json:"outbounds"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("rendered config is not valid json: %v", err)
	}

	if len(cfg.Inbounds) != 1 {
		t.Fatalf("expected exactly 1 inbound, got %d", len(cfg.Inbounds))
	}
	in := cfg.Inbounds[0]
	if in.Type != "vless" {
		t.Errorf("type = %q, want vless", in.Type)
	}
	if in.ListenPort != 30021 {
		t.Errorf("listen_port = %d, want 30021", in.ListenPort)
	}
	if in.Listen != "0.0.0.0" {
		t.Errorf("listen = %q, want 0.0.0.0", in.Listen)
	}
	if len(in.Users) != 1 || in.Users[0].Flow != "xtls-rprx-vision" {
		t.Errorf("users flow not set correctly: %+v", in.Users)
	}
	if !in.TLS.Enabled || !in.TLS.Reality.Enabled {
		t.Errorf("tls/reality must be enabled")
	}
	if in.TLS.Reality.Handshake.Server != "www.apple.com" || in.TLS.Reality.Handshake.ServerPort != 443 {
		t.Errorf("reality handshake wrong: %+v", in.TLS.Reality.Handshake)
	}
	if len(in.TLS.Reality.ShortID) != 1 || in.TLS.Reality.ShortID[0] != "deadbeef" {
		t.Errorf("short_id wrong: %+v", in.TLS.Reality.ShortID)
	}
	if cfg.Log.Output != "/var/log/sing-box.log" {
		t.Errorf("log output = %q", cfg.Log.Output)
	}
	if len(cfg.Outbounds) != 1 || cfg.Outbounds[0]["type"] != "direct" {
		t.Errorf("outbounds wrong: %+v", cfg.Outbounds)
	}
}

func TestRenderSingBoxConfig_RejectsBadInput(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*SingBoxInbound)
	}{
		{"empty tag", func(in *SingBoxInbound) { in.Tag = " " }},
		{"tag with space", func(in *SingBoxInbound) { in.Tag = "hkg 3" }},
		{"tag with slash", func(in *SingBoxInbound) { in.Tag = "hkg/3" }},
		{"port zero", func(in *SingBoxInbound) { in.ListenPort = 0 }},
		{"port too large", func(in *SingBoxInbound) { in.ListenPort = 70000 }},
		{"empty uuid", func(in *SingBoxInbound) { in.UUID = "" }},
		{"empty sni", func(in *SingBoxInbound) { in.SNI = "" }},
		{"empty private key", func(in *SingBoxInbound) { in.PrivateKey = "" }},
		{"empty short id", func(in *SingBoxInbound) { in.ShortID = "" }},
		{"empty flow", func(in *SingBoxInbound) { in.Flow = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := sampleInbound()
			tc.mutate(&in)
			if _, err := RenderSingBoxConfig(in); err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestSingBoxDownloadURL(t *testing.T) {
	u, err := SingBoxDownloadURL("1.11.15", "x86_64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(u, "sing-box-1.11.15-linux-amd64.tar.gz") {
		t.Errorf("unexpected url: %s", u)
	}
	if _, err := SingBoxDownloadURL("1.11.15", "mips"); err == nil {
		t.Error("expected error for unsupported arch")
	}
}

func TestBuildInstallScript_NoForbiddenTools(t *testing.T) {
	cfg, err := RenderSingBoxConfig(sampleInbound())
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	script, err := BuildInstallScript(cfg, "x86_64", "1.11.15")
	if err != nil {
		t.Fatalf("build script failed: %v", err)
	}

	forbidden := []string{"python3", "python ", "docker", "podman", "jq ", "sed -i"}
	for _, f := range forbidden {
		if strings.Contains(script, f) {
			t.Errorf("script must not depend on %q", f)
		}
	}
	required := []string{
		"rc-service", "rc-update", "/usr/local/bin/sing-box",
		"sing-box check", "/etc/sing-box/config.json", "base64 -d",
		"/etc/init.d/sing-box", "UNSUPPORTED_ARCH",
	}
	for _, r := range required {
		if !strings.Contains(script, r) {
			t.Errorf("script missing required fragment %q", r)
		}
	}
	if strings.Contains(script, "PRIVATE_KEY_PLACEHOLDER") {
		t.Error("script must embed base64 payload, not raw json")
	}
}

func TestBuildInstallScript_RejectsEmptyOrInvalidConfig(t *testing.T) {
	if _, err := BuildInstallScript(nil, "x86_64", "1.11.15"); err == nil {
		t.Error("expected error for empty config")
	}
	if _, err := BuildInstallScript([]byte("{not-json"), "x86_64", "1.11.15"); err == nil {
		t.Error("expected error for invalid json")
	}
}

func TestParseRealityKeyPair(t *testing.T) {
	out := "PrivateKey: aBcD1234_-xyz\nPublicKey: ZyXw9876_-abc\n"
	priv, pub, err := ParseRealityKeyPair(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if priv != "aBcD1234_-xyz" || pub != "ZyXw9876_-abc" {
		t.Errorf("parsed pair wrong: %q / %q", priv, pub)
	}
}

func TestParseRealityKeyPair_Invalid(t *testing.T) {
	if _, _, err := ParseRealityKeyPair("garbage"); err == nil {
		t.Error("expected error for garbage output")
	}
	if _, _, err := ParseRealityKeyPair("PrivateKey: only-one\n"); err == nil {
		t.Error("expected error when PublicKey missing")
	}
}

func TestParseNativeHealth_Running(t *testing.T) {
	out := strings.Join([]string{
		"VER:sing-box version 1.11.15",
		"SVC: * status: started",
		"LISTEN 0 4096 0.0.0.0:30021 0.0.0.0:* users:((\"sing-box\",pid=1,fd=3))",
		"LISTEN 0 4096 0.0.0.0:22 0.0.0.0:*",
	}, "\n")
	h := ParseNativeHealth(out)
	if !h.Installed || h.Version != "1.11.15" {
		t.Errorf("version not parsed: %+v", h)
	}
	if !h.Running {
		t.Errorf("expected running, got status %q", h.ServiceStatus)
	}
	if len(h.ListeningPorts) != 2 {
		t.Errorf("expected 2 ports, got %v", h.ListeningPorts)
	}
}

func TestParseNativeHealth_Stopped(t *testing.T) {
	out := "VER:sing-box version 1.11.15\nSVC: * status: stopped\n"
	h := ParseNativeHealth(out)
	if h.Running {
		t.Errorf("expected stopped, got %q", h.ServiceStatus)
	}
	if !h.Installed {
		t.Error("expected installed")
	}
}

func TestParseNativeHealth_NotInstalled(t *testing.T) {
	h := ParseNativeHealth("VER:\nSVC:\n")
	if h.Installed || h.Running {
		t.Errorf("expected not installed, got %+v", h)
	}
}

// ---- fake PortMapper ----

type fakeMapper struct {
	nextID       int
	added        []int
	deleted      []int
	failDeleteID int
	failAdd      bool
	emptyPublic  bool
}

func (f *fakeMapper) ListPortMappings(ctx context.Context, instanceID string) ([]providers.PortMapping, error) {
	return nil, nil
}

func (f *fakeMapper) AddPortMapping(ctx context.Context, instanceID string, privatePort int, protocol, remark string) (*providers.PortMapping, error) {
	if f.failAdd {
		return nil, errors.New("add failed")
	}
	f.nextID++
	f.added = append(f.added, f.nextID)
	pub := 30021 + f.nextID
	if f.emptyPublic {
		pub = 0
	}
	return &providers.PortMapping{
		ID:          f.nextID,
		Protocol:    protocol,
		PrivatePort: privatePort,
		PublicPort:  pub,
		Remark:      remark,
	}, nil
}

func (f *fakeMapper) DeletePortMapping(ctx context.Context, instanceID string, mappingID int) error {
	if f.failDeleteID > 0 && mappingID == f.failDeleteID {
		return errors.New("delete failed")
	}
	f.deleted = append(f.deleted, mappingID)
	return nil
}

func TestRepublishNATPort_Success(t *testing.T) {
	f := &fakeMapper{}
	pub, id, err := republishNATPortWith(context.Background(), f, "10674", 30021, 50285, "hkg3-vless")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pub <= 0 || id <= 0 {
		t.Fatalf("expected new public port and id, got %d/%d", pub, id)
	}
	if len(f.deleted) != 1 || f.deleted[0] != 50285 {
		t.Errorf("old mapping must be deleted after add succeeded: %v", f.deleted)
	}
}

func TestRepublishNATPort_RollbackOnDeleteFailure(t *testing.T) {
	f := &fakeMapper{failDeleteID: 50285}
	if _, _, err := republishNATPortWith(context.Background(), f, "10674", 30021, 50285, "hkg3-vless"); err == nil {
		t.Fatal("expected error when deleting old mapping fails")
	}
	// 新映射必须被删掉（回滚），避免端口配额泄漏。
	if len(f.deleted) != 1 || f.deleted[0] != 1 {
		t.Errorf("expected rollback delete of new mapping id=1, got %v", f.deleted)
	}
}

func TestRepublishNATPort_AddFailureLeavesNoTrace(t *testing.T) {
	f := &fakeMapper{failAdd: true}
	if _, _, err := republishNATPortWith(context.Background(), f, "10674", 30021, 50285, "x"); err == nil {
		t.Fatal("expected error when add fails")
	}
	if len(f.deleted) != 0 {
		t.Errorf("nothing should be deleted when add fails, got %v", f.deleted)
	}
}

func TestRepublishNATPort_RejectsBadPrivatePort(t *testing.T) {
	f := &fakeMapper{}
	if _, _, err := republishNATPortWith(context.Background(), f, "10674", 0, 50285, "x"); err == nil {
		t.Fatal("expected error for zero private port")
	}
}
