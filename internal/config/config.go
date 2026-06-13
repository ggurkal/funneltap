package config

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultAPIPort      = 9000
	DefaultMaxRequests  = 500
	DefaultMaxBodyBytes = 10 << 20 // 10 MiB
	DefaultProxyTimeout = 30 * time.Second
)

type Config struct {
	Target        *url.URL
	InterceptAddr string
	APIAddr       string
	ProxyTimeout  time.Duration
	MaxRequests   int
	MaxBodyBytes  int64
}

func Load() (*Config, error) {
	targetFlag := flag.String("target", "", "backend to proxy to (required)")
	flag.Parse()

	if *targetFlag == "" {
		return nil, errors.New("--target is required")
	}

	target, err := ParseTarget(*targetFlag)
	if err != nil {
		return nil, fmt.Errorf("parse target: %w", err)
	}

	interceptPort, err := resolvePort(os.Getenv("PORT"), true)
	if err != nil {
		return nil, fmt.Errorf("intercept port: %w", err)
	}

	apiPort, err := resolvePort(os.Getenv("API_PORT"), false)
	if err != nil {
		return nil, fmt.Errorf("api port: %w", err)
	}
	if apiPort == 0 {
		apiPort = DefaultAPIPort
	}

	proxyTimeout := DefaultProxyTimeout
	if v := os.Getenv("PROXY_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("PROXY_TIMEOUT: %w", err)
		}
		proxyTimeout = d
	}

	maxRequests := DefaultMaxRequests
	if v := os.Getenv("MAX_REQUESTS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("MAX_REQUESTS: %w", err)
		}
		if n < 1 {
			return nil, errors.New("MAX_REQUESTS must be at least 1")
		}
		maxRequests = n
	}

	maxBody := int64(DefaultMaxBodyBytes)
	if v := os.Getenv("MAX_BODY_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("MAX_BODY_BYTES: %w", err)
		}
		if n < 1 {
			return nil, errors.New("MAX_BODY_BYTES must be at least 1")
		}
		maxBody = n
	}

	return &Config{
		Target:        target,
		InterceptAddr: net.JoinHostPort("127.0.0.1", strconv.Itoa(interceptPort)),
		APIAddr:       net.JoinHostPort("0.0.0.0", strconv.Itoa(apiPort)),
		ProxyTimeout:  proxyTimeout,
		MaxRequests:   maxRequests,
		MaxBodyBytes:  maxBody,
	}, nil
}

func resolvePort(env string, allowRandom bool) (int, error) {
	if env == "" {
		if !allowRandom {
			return 0, nil
		}
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, err
		}
		defer ln.Close()
		return ln.Addr().(*net.TCPAddr).Port, nil
	}
	port, err := strconv.Atoi(env)
	if err != nil {
		return 0, fmt.Errorf("invalid port %q", env)
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("port out of range: %d", port)
	}
	return port, nil
}

func ParseTarget(s string) (*url.URL, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("empty target")
	}

	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		u, err := url.Parse(s)
		if err != nil {
			return nil, err
		}
		if u.Host == "" {
			return nil, errors.New("invalid target URL: missing host")
		}
		if u.Port() == "" {
			port := "80"
			if u.Scheme == "https" {
				port = "443"
			}
			u.Host = net.JoinHostPort(u.Hostname(), port)
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
		return url.Parse("http://127.0.0.1:" + port)
	}

	if _, err := strconv.Atoi(s); err == nil {
		return url.Parse("http://127.0.0.1:" + s)
	}

	if host, port, err := net.SplitHostPort(s); err == nil {
		if _, err := strconv.Atoi(port); err != nil {
			return nil, fmt.Errorf("invalid port in %q", s)
		}
		return url.Parse("http://" + net.JoinHostPort(host, port))
	}

	return url.Parse("http://" + net.JoinHostPort(s, "80"))
}

func (c *Config) TargetOrigin() string {
	u := *c.Target
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
