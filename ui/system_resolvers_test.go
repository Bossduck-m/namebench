package ui

import "testing"

func TestParseResolvConfNameservers(t *testing.T) {
	t.Parallel()

	raw := []byte(`
# comment
nameserver 127.0.0.1
nameserver 1.1.1.1
nameserver 2606:4700:4700::1111
search localdomain
`)

	got := parseResolvConfNameservers(raw)
	if len(got) != 2 {
		t.Fatalf("parseResolvConfNameservers() len = %d, want 2", len(got))
	}
	if got[0] != "1.1.1.1:53" {
		t.Fatalf("parseResolvConfNameservers() first = %q, want %q", got[0], "1.1.1.1:53")
	}
}

func TestMergeNameServersIncludesSystemResolvers(t *testing.T) {
	t.Parallel()

	got := mergeNameServers(
		[]string{"1.1.1.1:53"},
		[]string{"8.8.8.8:53", "8.8.8.8:53"},
		false,
		false,
		"none",
	)

	if len(got) != 2 {
		t.Fatalf("mergeNameServers() len = %d, want 2", len(got))
	}
	if got[1] != "8.8.8.8:53" {
		t.Fatalf("mergeNameServers() second = %q, want %q", got[1], "8.8.8.8:53")
	}
}
