package funnel

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Funnel runs tailscale funnel commands.
type Funnel interface {
	StartPath(mountPath, backendURL string) error
	StopPath(mountPath string) error
}

// CLI implements Funnel using the tailscale binary.
type CLI struct{}

func NewCLI() *CLI { return &CLI{} }

func (c *CLI) StartPath(mountPath, backendURL string) error {
	args := BuildStartArgs(mountPath, backendURL)
	cmd := exec.Command("tailscale", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func (c *CLI) StopPath(mountPath string) error {
	args := BuildStopArgs(mountPath)
	cmd := exec.Command("tailscale", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// BuildStartArgs constructs tailscale funnel args for tests.
func BuildStartArgs(mountPath, backendURL string) []string {
	return []string{"funnel", "--bg", "--yes", "--set-path", mountPath, backendURL}
}

// BuildStopArgs constructs tailscale funnel teardown args for tests.
func BuildStopArgs(mountPath string) []string {
	return []string{"funnel", "--yes", "--set-path", mountPath, "off"}
}

// MachineHTTPSURL returns https://<machine>.<tailnet>.ts.net without trailing slash.
func MachineHTTPSURL() (string, error) {
	cmd := exec.Command("tailscale", "status", "--json")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("tailscale status: %w", err)
	}
	var status struct {
		Self struct {
			DNSName string `json:"DNSName"`
		} `json:"Self"`
	}
	if err := json.Unmarshal(out, &status); err != nil {
		return "", err
	}
	name := strings.TrimSuffix(status.Self.DNSName, ".")
	if name == "" {
		return "", fmt.Errorf("missing DNS name in tailscale status")
	}
	return "https://" + name, nil
}
