package funnel

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestBuildStartArgs(t *testing.T) {
	got := BuildStartArgs("/hooks", "http://127.0.0.1:8080/.ft/hooks")
	want := []string{"funnel", "--bg", "--yes", "--set-path", "/hooks", "http://127.0.0.1:8080/.ft/hooks"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestBuildStopArgs(t *testing.T) {
	got := BuildStopArgs("/hooks")
	want := []string{"funnel", "--yes", "--set-path", "/hooks", "off"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestResolveTailscaleBinaryEnv(t *testing.T) {
	t.Setenv("TAILSCALE_BIN", "")
	dir := t.TempDir()
	bin := filepath.Join(dir, "tailscale")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TAILSCALE_BIN", bin)

	got, err := ResolveTailscaleBinary()
	if err != nil {
		t.Fatal(err)
	}
	if got != bin {
		t.Fatalf("got %q want %q", got, bin)
	}
}

func TestResolveTailscaleBinaryMissingEnv(t *testing.T) {
	t.Setenv("TAILSCALE_BIN", "/definitely/missing/tailscale")

	_, err := ResolveTailscaleBinary()
	if err == nil {
		t.Fatal("expected error for missing TAILSCALE_BIN")
	}
}

func TestResolveTailscaleBinaryPATH(t *testing.T) {
	t.Setenv("TAILSCALE_BIN", "")

	dir := t.TempDir()
	bin := filepath.Join(dir, "tailscale")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	got, err := ResolveTailscaleBinary()
	if err != nil {
		t.Fatal(err)
	}
	if got != bin {
		t.Fatalf("got %q want %q", got, bin)
	}
}

func TestResolveTailscaleBinaryNotFound(t *testing.T) {
	t.Setenv("TAILSCALE_BIN", "")
	t.Setenv("PATH", t.TempDir())

	if runtime.GOOS == "darwin" {
		if _, err := os.Stat(macOSTailscaleAppCLI); err == nil {
			t.Skip("macOS app bundle provides fallback")
		}
	}

	_, err := ResolveTailscaleBinary()
	if err == nil {
		t.Fatal("expected error when tailscale is not on PATH")
	}
}

func TestResolveTailscaleBinaryMacOSApp(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	if _, err := os.Stat(macOSTailscaleAppCLI); err != nil {
		t.Skip("Tailscale.app not installed")
	}

	t.Setenv("TAILSCALE_BIN", "")
	t.Setenv("PATH", t.TempDir())

	got, err := ResolveTailscaleBinary()
	if err != nil {
		t.Fatal(err)
	}
	if got != macOSTailscaleAppCLI {
		t.Fatalf("got %q want %q", got, macOSTailscaleAppCLI)
	}
}
