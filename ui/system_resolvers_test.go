package ui

import "testing"

func TestParseWindowsResolverJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		raw   string
		want  int
		first string
	}{
		{
			name:  "array payload",
			raw:   `["192.168.1.1","1.1.1.1","2606:4700:4700::1111"]`,
			want:  2,
			first: "1.1.1.1:53",
		},
		{
			name:  "single payload",
			raw:   `"8.8.8.8"`,
			want:  1,
			first: "8.8.8.8:53",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseWindowsResolverJSON([]byte(tt.raw))
			if err != nil {
				t.Fatalf("parseWindowsResolverJSON() error = %v", err)
			}
			if len(got) != tt.want {
				t.Fatalf("parseWindowsResolverJSON() len = %d, want %d", len(got), tt.want)
			}
			if len(got) > 0 && got[0] != tt.first {
				t.Fatalf("parseWindowsResolverJSON() first = %q, want %q", got[0], tt.first)
			}
		})
	}
}

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
