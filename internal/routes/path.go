package routes

import (
	"fmt"
	"strings"
)

// NormalizePath cleans a mount path for storage and comparison.
func NormalizePath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("path is required")
	}
	if !strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("path must start with /")
	}
	if p != "/" {
		p = strings.TrimSuffix(p, "/")
	}
	return p, nil
}

// PathsOverlap reports whether two normalized paths conflict.
func PathsOverlap(a, b string) bool {
	if a == b {
		return true
	}
	return strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

// InternalPrefix returns the intercept path prefix for a mount path.
func InternalPrefix(mountPath string) string {
	if mountPath == "/" {
		return "/.ft"
	}
	return "/.ft" + mountPath
}

// InternalBackendURL is the funnel proxy target for a route mount.
func InternalBackendURL(interceptPort int, mountPath string) string {
	p := InternalPrefix(mountPath)
	return fmt.Sprintf("http://127.0.0.1:%d%s", interceptPort, p)
}
