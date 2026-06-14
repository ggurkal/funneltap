package routes

import (
	"path/filepath"
	"testing"
)

func TestNormalizePath(t *testing.T) {
	got, err := NormalizePath("/hooks/")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/hooks" {
		t.Fatalf("got %q", got)
	}
}

func TestPathsOverlap(t *testing.T) {
	if !PathsOverlap("/api", "/api/v2") {
		t.Fatal("expected overlap")
	}
	if PathsOverlap("/hooks", "/deploy") {
		t.Fatal("unexpected overlap")
	}
}

func TestInternalBackendURL(t *testing.T) {
	got := InternalBackendURL(8080, "/hooks")
	want := "http://127.0.0.1:8080/.ft/hooks"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestRegistryMatch(t *testing.T) {
	reg := NewRegistry(8080, &fakeFunnel{}, func() (string, error) { return "https://x.ts.net", nil }, &RecoveryFile{Path: t.TempDir() + "/r.json"})

	rt, err := reg.Add("/hooks", "localhost:3000")
	if err != nil {
		t.Fatal(err)
	}

	matched, suffix, ok := reg.Match("/.ft/hooks/github")
	if !ok || matched.ID != rt.ID || suffix != "/github" {
		t.Fatalf("match = %#v %q %v", matched, suffix, ok)
	}

	if _, _, ok := reg.Match("/.ft/unknown"); ok {
		t.Fatal("expected no match")
	}
}

func TestRecoveryFileLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routes.json")
	f := &RecoveryFile{Path: path}

	if err := f.Write([]Checkpoint{{Path: "/a", Target: "http://localhost:1"}}); err != nil {
		t.Fatal(err)
	}
	if !f.Exists() {
		t.Fatal("expected file")
	}
	cps, err := f.Read()
	if err != nil || len(cps) != 1 {
		t.Fatalf("read: %#v %v", cps, err)
	}
	if err := f.Delete(); err != nil {
		t.Fatal(err)
	}
	if f.Exists() {
		t.Fatal("expected deleted")
	}
}

type fakeFunnel struct {
	started []string
	stopped []string
}

func (f *fakeFunnel) StartPath(mountPath, backendURL string) error {
	f.started = append(f.started, mountPath+"="+backendURL)
	return nil
}

func (f *fakeFunnel) StopPath(mountPath string) error {
	f.stopped = append(f.stopped, mountPath)
	return nil
}
