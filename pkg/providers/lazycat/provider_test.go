package lazycat

import (
	"context"
	"testing"
)

func TestDetectLazyCatRegion(t *testing.T) {
	cases := []struct {
		name        string
		countryCode string
		pkg         string
		plan        string
		instName    string
		rawRegion   string
		want        string
	}{
		{"HKG package", "", "HKG-Premium 3", "HKG-Premium Basic", "HKG 0.1折", "", "香港"},
		{"HK country code", "HK", "SomePkg", "", "node-1", "", "香港"},
		{"JP country code", "JP", "Global", "", "大杯02", "", "日本"},
		{"US name", "", "", "", "US-Frontier", "", "美国"},
		{"SG pkg", "SG", "StarHub", "", "sg-1", "", "新加坡"},
		{"Global residential fallback", "", "Global-Residential", "", "0.1折 全球家宽 大杯02", "", "全球家宽"},
		{"Unknown fallback", "", "", "", "custom-box", "", "NAT节点"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := detectLazyCatRegion(tc.countryCode, tc.pkg, tc.plan, tc.instName, tc.rawRegion)
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestEnrichInstance_NATSSHPortAndSpecs(t *testing.T) {
	p := &LazyCatProvider{}

	item := InstanceListItem{
		ID:           10674,
		Name:         "HKG 0.1折",
		Status:       "running",
		PackageName:  "HKG-Premium 3",
		PlanName:     "HKG-Premium Basic",
		CPUCores:     1,
		Memory:       512,
		HostPublicIP: "216.236.60.76",
		PortLimit:    10,
	}

	// Case 1: Only 30021 mapped, no port 22 mapping -> SSHPort must be 0 (not hardcoded 22)
	detailNoSSH := &InstanceDetailResponse{
		ID: 10674,
		PortMappings: []PortMappingItem{
			{ID: 50285, Protocol: "tcp", PrivatePort: 30021, PublicPort: 30021},
		},
	}
	inst1 := p.enrichInstance(context.Background(), item, detailNoSSH)
	if inst1.Region != "香港" {
		t.Errorf("expected Region=香港, got %q", inst1.Region)
	}
	if inst1.Category != "固定出口" {
		t.Errorf("expected Category=固定出口, got %q", inst1.Category)
	}
	if inst1.SSHPort != 0 {
		t.Errorf("expected SSHPort=0 when port 22 is unmapped, got %d", inst1.SSHPort)
	}
	if inst1.CPU != "1核" || inst1.Memory != "512 MB" {
		t.Errorf("unexpected CPU/Memory: %q / %q", inst1.CPU, inst1.Memory)
	}
	if inst1.PortCount != 1 {
		t.Errorf("expected PortCount=1, got %d", inst1.PortCount)
	}

	// Case 2: Port 22 mapped to 30408 -> SSHPort must be 30408
	detailWithSSH := &InstanceDetailResponse{
		ID: 10674,
		PortMappings: []PortMappingItem{
			{ID: 50285, Protocol: "tcp", PrivatePort: 22, PublicPort: 30408},
		},
	}
	inst2 := p.enrichInstance(context.Background(), item, detailWithSSH)
	if inst2.SSHPort != 30408 {
		t.Errorf("expected SSHPort=30408, got %d", inst2.SSHPort)
	}
}
