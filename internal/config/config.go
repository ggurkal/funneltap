package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
)

const (
	DefaultAPIPort      = 9000
	DefaultMaxRequests  = 500
	DefaultMaxBodyBytes = 10 << 20 // 10 MiB
)

type Config struct {
	InterceptAddr string
	InterceptPort int
	APIAddr       string
	MaxRequests   int
	MaxBodyBytes  int64
	RoutesFile    string
}

func Load() (*Config, error) {
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

	routesFile := os.Getenv("FUNNELTAP_ROUTES_FILE")
	if routesFile == "" {
		routesFile = "/tmp/funneltap-routes.json"
	}

	return &Config{
		InterceptAddr: net.JoinHostPort("127.0.0.1", strconv.Itoa(interceptPort)),
		InterceptPort: interceptPort,
		APIAddr:       net.JoinHostPort("0.0.0.0", strconv.Itoa(apiPort)),
		MaxRequests:   maxRequests,
		MaxBodyBytes:  maxBody,
		RoutesFile:    routesFile,
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
