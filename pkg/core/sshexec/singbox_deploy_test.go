package sshexec

import (
	"context"
	"strings"
	"testing"

	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
)

type fakePortMapper struct {
	list    []providers.PortMapping
	added   int
	lastReq struct {
		port, public  int
		proto, remark string
	}
}

func (f *fakePortMapper) ListPortMappings(ctx context.Context, instanceID string) ([]providers.PortMapping, error) {
	return f.list, nil
}

func (f *fakePortMapper) AddPortMapping(ctx context.Context, instanceID string, privatePort int, protocol, remark string) (*providers.PortMapping, error) {
	f.added++
	f.lastReq.port, f.lastReq.proto, f.lastReq.remark = privatePort, protocol, remark
	m := providers.PortMapping{ID: 900 + f.added, Protocol: protocol, PrivatePort: privatePort, PublicPort: 41000 + f.added, Remark: remark}
	f.list = append(f.list, m)
	return &m, nil
}

func (f *fakePortMapper) DeletePortMapping(ctx context.Context, instanceID string, mappingID int) error {
	return nil
}

func TestEnsurePortMapping_ReusesExisting(t *testing.T) {
	pm := &fakePortMapper{list: []providers.PortMapping{{ID: 50285, Protocol: "tcp", PrivatePort: 30021, PublicPort: 30021}}}
	pub, id, err := ensurePortMapping(context.Background(), pm, "10674", 30021, "vless")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pub != 30021 || id != 50285 {
		t.Fatalf("must reuse existing mapping, got pub=%d id=%d", pub, id)
	}
	if pm.added != 0 {
		t.Fatalf("must not add a new mapping, added=%d", pm.added)
	}
}

func TestEnsurePortMapping_CreatesWhenMissing(t *testing.T) {
	pm := &fakePortMapper{}
	pub, id, err := ensurePortMapping(context.Background(), pm, "10674", 30022, "vless")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pub <= 0 || id <= 0 {
		t.Fatalf("expected new mapping, got pub=%d id=%d", pub, id)
	}
	if pm.lastReq.port != 30022 || pm.lastReq.proto != "tcp" {
		t.Fatalf("unexpected request: %+v", pm.lastReq)
	}
}

func TestBuildVLESSURL(t *testing.T) {
	res := &NativeSingBoxDeployResult{
		UUID:       "11111111-2222-3333-4444-555555555555",
		PublicKey:  "PUBKEY",
		ShortID:    "ab12cd34",
		PublicPort: 30021,
		ServerIP:   "216.236.60.76",
	}
	got := buildVLESSURL(res, "www.microsoft.com", "HK-HKG3-NAT")
	for _, want := range []string{
		"vless://11111111-2222-3333-4444-555555555555@216.236.60.76:30021?",
		"pbk=PUBKEY",
		"sid=ab12cd34",
		"flow=xtls-rprx-vision",
		"security=reality",
		"sni=www.microsoft.com",
		"#HK-HKG3-NAT",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("vless url missing %q: %s", want, got)
		}
	}
}

func TestDeployNativeSingBox_ValidatesInput(t *testing.T) {
	if _, err := DeployNativeSingBox(context.Background(), nil, NativeSingBoxDeployRequest{ListenPort: 30021}); err == nil {
		t.Fatal("nil instance must be rejected")
	}
	inst := &providers.Instance{ID: "10674", ProviderID: "lazycat"}
	if _, err := DeployNativeSingBox(context.Background(), inst, NativeSingBoxDeployRequest{ListenPort: 0}); err == nil {
		t.Fatal("invalid listen port must be rejected before any remote call")
	}
}
