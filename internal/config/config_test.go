package config

import (
	"testing"
)

func TestParseTarget(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"8080", "http://localhost:8080"},
		{":8080", "http://localhost:8080"},
		{"192.168.0.1", "http://192.168.0.1"},
		{"192.168.0.1:8080", "http://192.168.0.1:8080"},
		{"https://test.com", "https://test.com"},
		{"https://test.com:8080", "https://test.com:8080"},
		{"localhost:3002", "http://localhost:3002"},
		{"api.example.com", "http://api.example.com"},
		{"http://api.internal:8080", "http://api.internal:8080"},
	}

	for _, tt := range tests {
		u, err := ParseTarget(tt.in)
		if err != nil {
			t.Fatalf("ParseTarget(%q): %v", tt.in, err)
		}
		if got := FormatTarget(u); got != tt.want {
			t.Fatalf("ParseTarget(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseTargetRejectsEmpty(t *testing.T) {
	if _, err := ParseTarget(""); err == nil {
		t.Fatal("expected error")
	}
}
