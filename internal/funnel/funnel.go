package funnel

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const macOSTailscaleAppCLI = "/Applications/Tailscale.app/Contents/MacOS/Tailscale"

// ResolveTailscaleBinary returns the tailscale CLI path.
// It checks TAILSCALE_BIN, PATH, then the default macOS app bundle location.
func ResolveTailscaleBinary() (string, error) {
	if v := os.Getenv("TAILSCALE_BIN"); v != "" {
		if _, err := os.Stat(v); err != nil {
			return "", fmt.Errorf("TAILSCALE_BIN %q: %w", v, err)
		}
		return v, nil
	}
	if p, err := exec.LookPath("tailscale"); err == nil {
		return p, nil
	}
	if runtime.GOOS == "darwin" {
		if _, err := os.Stat(macOSTailscaleAppCLI); err == nil {
			return macOSTailscaleAppCLI, nil
		}
	}
	return "", fmt.Errorf("tailscale CLI not found in PATH (install Tailscale or set TAILSCALE_BIN)")
}

func tailscaleCommand(args ...string) (*exec.Cmd, error) {
	bin, err := ResolveTailscaleBinary()
	if err != nil {
		return nil, err
	}
	return exec.Command(bin, args...), nil
}

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
	cmd, err := tailscaleCommand(args...)
	if err != nil {
		return err
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func (c *CLI) StopPath(mountPath string) error {
	args := BuildStopArgs(mountPath)
	cmd, err := tailscaleCommand(args...)
	if err != nil {
		return err
	}
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
	cmd, err := tailscaleCommand("status", "--json")
	if err != nil {
		return "", err
	}
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
