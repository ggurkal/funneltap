package config

import (
	"net/url"
	"testing"
)

func TestParseTarget(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"8000", "http://127.0.0.1:8000"},
		{":8000", "http://127.0.0.1:8000"},
		{"localhost:3002", "http://localhost:3002"},
		{"api.example.com", "http://api.example.com:80"},
		{"https://localhost:3002", "https://localhost:3002"},
		{"https://localhost", "https://localhost:443"},
		{"http://api.internal", "http://api.internal:80"},
		{"https://example.com/api/extra", "https://example.com:443"},
	}

	for _, tt := range tests {
		u, err := ParseTarget(tt.in)
		if err != nil {
			t.Fatalf("ParseTarget(%q): %v", tt.in, err)
		}
		if u.String() != tt.want {
			t.Fatalf("ParseTarget(%q) = %q, want %q", tt.in, u.String(), tt.want)
		}
	}
}

func TestParseTargetOriginOnly(t *testing.T) {
	raw := "https://example.com/ignored/path"
	u, err := ParseTarget(raw)
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "" {
		t.Fatalf("expected empty path, got %q", u.Path)
	}
	origin := &url.URL{Scheme: u.Scheme, Host: u.Host}
	if origin.String() != "https://example.com:443" {
		t.Fatalf("unexpected origin %q", origin.String())
	}
}
