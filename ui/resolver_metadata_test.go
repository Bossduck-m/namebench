package ui

import "testing"

func TestCountryFromLabel(t *testing.T) {
	t.Parallel()

	label := "1600 Amphitheatre Parkway\nMountain View\nCA\n94043\nUnited States"
	got := countryFromLabel(label)
	if got != "United States" {
		t.Fatalf("countryFromLabel() = %q, want %q", got, "United States")
	}
}

func TestParseVCardEntity(t *testing.T) {
	t.Parallel()

	raw := []interface{}{
		"vcard",
		[]interface{}{
			[]interface{}{"fn", map[string]interface{}{}, "text", "Google LLC"},
			[]interface{}{"adr", map[string]interface{}{"label": "1600 Amphitheatre Parkway\nMountain View\nCA\n94043\nUnited States"}, "text", "1600 Amphitheatre Parkway\nMountain View\nCA\n94043\nUnited States"},
		},
	}

	org, country := parseVCardEntity(raw)
	if org != "Google LLC" {
		t.Fatalf("parseVCardEntity() org = %q, want %q", org, "Google LLC")
	}
	if country != "United States" {
		t.Fatalf("parseVCardEntity() country = %q, want %q", country, "United States")
	}
}

func TestResolverServerIP(t *testing.T) {
	t.Parallel()

	if got := resolverServerIP("8.8.8.8:53"); got != "8.8.8.8" {
		t.Fatalf("resolverServerIP() = %q, want %q", got, "8.8.8.8")
	}
	if got := resolverServerIP("[2606:4700:4700::1111]:53"); got != "2606:4700:4700::1111" {
		t.Fatalf("resolverServerIP() ipv6 = %q", got)
	}
}
