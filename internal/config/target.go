package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// ParseTarget normalizes an upstream target from user input. Origin only (no path/query).
func ParseTarget(s string) (*url.URL, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty target")
	}

	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		u, err := url.Parse(s)
		if err != nil {
			return nil, err
		}
		if u.Host == "" {
			return nil, fmt.Errorf("invalid target URL: missing host")
		}
		u.Path = ""
		u.RawPath = ""
		u.RawQuery = ""
		u.Fragment = ""
		return u, nil
	}

	if strings.HasPrefix(s, ":") {
		port := s[1:]
		if _, err := strconv.Atoi(port); err != nil {
			return nil, fmt.Errorf("invalid port %q", s)
		}
		return url.Parse("http://localhost:" + port)
	}

	if _, err := strconv.Atoi(s); err == nil {
		return url.Parse("http://localhost:" + s)
	}

	if host, port, err := net.SplitHostPort(s); err == nil {
		if _, err := strconv.Atoi(port); err != nil {
			return nil, fmt.Errorf("invalid port in %q", s)
		}
		return url.Parse("http://" + net.JoinHostPort(host, port))
	}

	return url.Parse("http://" + s)
}

// FormatTarget returns a stable string form without default ports.
func FormatTarget(u *url.URL) string {
	if u == nil {
		return ""
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		return u.Scheme + "://" + host
	}
	return u.Scheme + "://" + net.JoinHostPort(host, port)
}
