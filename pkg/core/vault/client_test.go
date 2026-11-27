package vault

import "testing"

func TestSelectBestProviderItemsPrefersNewestAndTOTP(t *testing.T) {
	items := []*VaultItem{
		{
			ID:           "old-no-totp",
			Name:         "acck.io",
			Username:     "a@example.com",
			Password:     "old-pass-1234",
			RevisionDate: "2026-07-27T14:14:50Z",
		},
		{
			ID:           "new-with-totp",
			Name:         "acck.io",
			Username:     "a@example.com",
			Password:     "new-pass-12345678",
			TOTPSecret:   "JBSWY3DPEHPK3PXP",
			RevisionDate: "2026-09-05T06:08:09Z",
		},
		{
			ID:           "akile-old",
			Name:         "Akile",
			Username:     "ak@example.com",
			Password:     "ak-old",
			TOTPSecret:   "JBSWY3DPEHPK3PXP",
			RevisionDate: "2026-06-01T00:00:00Z",
		},
		{
			ID:           "akile-new",
			Name:         "Akile",
			Username:     "ak@example.com",
			Password:     "ak-new",
			TOTPSecret:   "JBSWY3DPEHPK3PXP",
			RevisionDate: "2026-09-14T09:01:01Z",
		},
		{
			ID:           "lz",
			Name:         "LazyCatCloud",
			Username:     "lz@example.com",
			Password:     "lz-pass",
			RevisionDate: "2026-11-14T10:38:04Z",
		},
	}

	best := SelectBestProviderItems(items)
	if best["acck"] == nil || best["acck"].ID != "new-with-totp" {
		t.Fatalf("expected acck to pick new-with-totp, got %+v", best["acck"])
	}
	if best["akile"] == nil || best["akile"].ID != "akile-new" {
		t.Fatalf("expected akile to pick akile-new, got %+v", best["akile"])
	}
	if best["lazycat"] == nil || best["lazycat"].ID != "lz" {
		t.Fatalf("expected lazycat to pick lz, got %+v", best["lazycat"])
	}
}
