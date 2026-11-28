package lazycat

import "testing"

// 10674 / HKG-3 的真实字段值（来自懒猫云 API 实测）。
const (
	hkg3Package = "HKG-Premium 3"
	hkg3Plan    = "HKG-Premium Basic"
	hkg3Name    = "HKG 0.1折"
	hkg3Country = "hk"
	hkg3Region  = "香港"

	global2Package = "全球家宽大杯02"
	global2Name    = "全球家宽大杯02"
)

func TestIsResidential_FixedExitHKG3(t *testing.T) {
	if isResidential(hkg3Package, hkg3Plan, hkg3Name) {
		t.Fatalf("HKG-3 (%s/%s/%s) must NOT be treated as residential", hkg3Package, hkg3Plan, hkg3Name)
	}
}

func TestIsResidential_GlobalResidential(t *testing.T) {
	if !isResidential(global2Package, "", global2Name) {
		t.Fatalf("global residential instance must be treated as residential")
	}
}

// 节点命名硬规则：HKG-3 的节点名禁止含 LZ-/动态/家宽，否则会被 vpn-probe 误判为家宽。
func TestIsResidential_SafeNodeName(t *testing.T) {
	if isResidential("", "", "HK-HKG3-NAT") {
		t.Fatal("HK-HKG3-NAT must not be classified as residential")
	}
	for _, bad := range []string{"HK-动态家宽", "HK-residential", "HK-Global"} {
		if !isResidential("", "", bad) {
			t.Fatalf("%q should be treated as residential (guard against unsafe naming)", bad)
		}
	}
	// 注意：LZ- 前缀在本服务里不算家宽，但 vpn-probe 的 IsResidentialNode 会算，
	// 因此节点命名仍禁止 LZ-（跨服务一致性由 vpn-probe 的用例守护）。
	if isResidential("", "", "LZ-HKG-3") {
		t.Fatal("this service does not classify LZ- as residential; keep behavior explicit")
	}
}

func TestDetectLazyCatRegion_HKG3(t *testing.T) {
	got := detectLazyCatRegion(hkg3Country, hkg3Package, hkg3Plan, hkg3Name, hkg3Region)
	if got != "香港" {
		t.Fatalf("region = %q, want 香港", got)
	}
}
