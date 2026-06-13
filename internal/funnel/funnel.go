package funnel

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

func Start(port int) error {
	cmd := exec.Command("tailscale", "funnel", "--bg", strconv.Itoa(port))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tailscale funnel: %w", err)
	}
	return nil
}
