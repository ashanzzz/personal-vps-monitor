package lazycat

import (
	"net/url"
	"strings"
	"testing"
)

func TestSortEgressNodesByPriority(t *testing.T) {
	raw := []struct {
		id   int
		name string
	}{
		{12, "菲律宾-DITO动态家宽"},
		{10, "中国台湾-Hinet动态"},
		{8, "中国香港-HKT动态家宽"},
		{11, "韩国-Korea-Telecom动态家宽"},
		{9, "新加坡-StarHub动态家宽"},
		{1, "日本-SoftBank动态家宽"},
		{5, "美国-Pavlov动态家宽"},
		{6, "美国-Frontier动态"},
	}

	var nodes []EgressNode
	for _, item := range raw {
		cc, flag := detectCountryFlag(item.name)
		nodes = append(nodes, EgressNode{
			ID:          item.id,
			Priority:    countryPriorityRank(cc),
			Name:        item.name,
			CountryCode: cc,
			FlagEmoji:   flag,
			PrivatePort: 10000 + item.id,
		})
	}
	sortEgressNodesByPriority(nodes)

	gotOrder := make([]string, len(nodes))
	for i, n := range nodes {
		gotOrder[i] = n.CountryCode
	}
	want := []string{"US", "US", "JP", "SG", "KR", "TW", "HK", "PH"}
	for i := range want {
		if gotOrder[i] != want[i] {
			t.Fatalf("priority mismatch at index %d: got %v, want %v", i, gotOrder, want)
		}
	}
	if nodes[0].ID != 5 || nodes[1].ID != 6 {
		t.Fatalf("expected US nodes ordered by ID (5 then 6), got %d then %d", nodes[0].ID, nodes[1].ID)
	}
}

func TestParseCredentialsJSONExcludesPrivateKeyAndBuildVLESSURL(t *testing.T) {
	rawCreds := `{
	  "5": {"uuid": "813d55b2-fca7-42be-ae66-0bfe14b80f80"},
	  "_reality": {
	    "private_key": "SUPER_SECRET_PRIVATE_KEY",
	    "public_key": "9-J_57550EbUeADMr_lsplIIHTu4C4BtaWZfQtBKwSc",
	    "short_id": "9b4d2b10",
	    "server_name": "www.apple.com"
	  }
	}`
	parsed := parseCredentialsJSON(rawCreds)
	if parsed == nil {
		t.Fatal("expected parsed credentials, got nil")
	}
	if parsed.Reality.PublicKey != "9-J_57550EbUeADMr_lsplIIHTu4C4BtaWZfQtBKwSc" {
		t.Fatalf("unexpected public_key: %s", parsed.Reality.PublicKey)
	}
	if parsed.Reality.ShortID != "9b4d2b10" {
		t.Fatalf("unexpected short_id: %s", parsed.Reality.ShortID)
	}
	if parsed.Nodes["5"].UUID != "813d55b2-fca7-42be-ae66-0bfe14b80f80" {
		t.Fatalf("unexpected node 5 uuid: %s", parsed.Nodes["5"].UUID)
	}

	node := EgressNode{
		ID:          5,
		Name:        "美国-Pavlov动态家宽",
		CountryCode: "US",
		FlagEmoji:   "🇺🇸",
		Protocol:    "vless",
		Flow:        "xtls-rprx-vision",
		SNI:         parsed.Reality.ServerName,
		UUID:        parsed.Nodes["5"].UUID,
		PublicKey:   parsed.Reality.PublicKey,
		ShortID:     parsed.Reality.ShortID,
	}

	vlessURL := node.BuildVLESSURL("167.104.167.151", 49040)
	if !strings.HasPrefix(vlessURL, "vless://813d55b2-fca7-42be-ae66-0bfe14b80f80@167.104.167.151:49040?") {
		t.Fatalf("unexpected vless URL prefix: %s", vlessURL)
	}
	u, err := url.Parse(vlessURL)
	if err != nil {
		t.Fatalf("failed to parse vless URL: %v", err)
	}
	q := u.Query()
	if q.Get("pbk") != "9-J_57550EbUeADMr_lsplIIHTu4C4BtaWZfQtBKwSc" || q.Get("sid") != "9b4d2b10" || q.Get("sni") != "www.apple.com" || q.Get("flow") != "xtls-rprx-vision" {
		t.Fatalf("missing or invalid reality query params: %v", q)
	}
	if strings.Contains(vlessURL, "SUPER_SECRET_PRIVATE_KEY") {
		t.Fatal("private key leaked into VLESS URL")
	}
}

func TestGenerateEgressClashAndSingBox(t *testing.T) {
	nodes := []EgressNode{
		{
			ID:          5,
			Priority:    1,
			Name:        "美国-Pavlov动态家宽",
			CountryCode: "US",
			FlagEmoji:   "🇺🇸",
			HostIP:      "167.104.167.151",
			PublicPort:  49040,
			Mapped:      true,
			SNI:         "www.apple.com",
			Flow:        "xtls-rprx-vision",
			UUID:        "813d55b2-fca7-42be-ae66-0bfe14b80f80",
			PublicKey:   "9-J_57550EbUeADMr_lsplIIHTu4C4BtaWZfQtBKwSc",
			ShortID:     "9b4d2b10",
		},
	}

	yaml := generateEgressClashYAML(nodes, "LazyCat-Test")
	if !strings.Contains(yaml, "type: vless") || !strings.Contains(yaml, "public-key: 9-J_57550EbUeADMr_lsplIIHTu4C4BtaWZfQtBKwSc") || !strings.Contains(yaml, "short-id: 9b4d2b10") || !strings.Contains(yaml, "type: url-test") {
		t.Fatalf("invalid Clash YAML generated:\n%s", yaml)
	}

	sb := generateEgressSingBoxJSON(nodes, "LazyCat-Test")
	if !strings.Contains(sb, `"type": "vless"`) || !strings.Contains(sb, `"tag": "lazycat-us-5"`) || !strings.Contains(sb, `"public_key": "9-J_57550EbUeADMr_lsplIIHTu4C4BtaWZfQtBKwSc"`) || !strings.Contains(sb, `"type": "urltest"`) {
		t.Fatalf("invalid sing-box JSON generated:\n%s", sb)
	}
}
