package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ggurkal/funneltap/internal/routes"
	"github.com/ggurkal/funneltap/internal/store"
)

func TestListAfter(t *testing.T) {
	st := store.New(10)
	reg := routes.NewRegistry(8080, &recordingFunnel{}, func() (string, error) { return "https://x.ts.net", nil }, &routes.RecoveryFile{Path: t.TempDir() + "/r.json"})
	rt1, err := reg.Add("/hooks", "http://localhost:1")
	if err != nil {
		t.Fatal(err)
	}
	rt2, err := reg.Add("/deploy", "http://localhost:2")
	if err != nil {
		t.Fatal(err)
	}

	st.Add("GET", "/a", nil, nil, rt1.ID, "/hooks", "http://localhost:1")
	st.UpdateProxy(1, store.ProxyInfo{Status: 200, DurationMs: 1})
	st.Add("POST", "/b", nil, []byte("hi"), rt2.ID, "/deploy", "http://localhost:2")
	st.UpdateProxy(2, store.ProxyInfo{Status: 201, DurationMs: 2})

	srv := httptest.NewServer(New(st, reg, &routes.RecoveryFile{Path: t.TempDir() + "/r.json"}).Handler())
	defer srv.Close()

	res, err := http.Get(srv.URL + "/requests?after=1")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	var items []struct {
		ID uint64 `json:"id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != 2 {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestListHidesRequestsWithoutActiveRoutes(t *testing.T) {
	st := store.New(10)
	st.Add("GET", "/a", nil, nil, "gone", "/hooks", "http://localhost:1")
	reg := routes.NewRegistry(8080, &recordingFunnel{}, func() (string, error) { return "", nil }, &routes.RecoveryFile{Path: t.TempDir() + "/r.json"})
	srv := httptest.NewServer(New(st, reg, &routes.RecoveryFile{Path: t.TempDir() + "/r.json"}).Handler())
	defer srv.Close()

	res, err := http.Get(srv.URL + "/requests")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	var items []store.Summary
	if err := json.NewDecoder(res.Body).Decode(&items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no items, got %+v", items)
	}
}

func TestDetailBodyBase64(t *testing.T) {
	st := store.New(10)
	reg := routes.NewRegistry(8080, &recordingFunnel{}, func() (string, error) { return "", nil }, &routes.RecoveryFile{Path: t.TempDir() + "/r.json"})
	rt, err := reg.Add("/x", "http://localhost:1")
	if err != nil {
		t.Fatal(err)
	}
	st.Add("POST", "/x", nil, []byte("hello"), rt.ID, "/x", "http://localhost:1")
	st.UpdateProxy(1, store.ProxyInfo{Status: 200, DurationMs: 1})

	srv := httptest.NewServer(New(st, reg, &routes.RecoveryFile{Path: t.TempDir() + "/r.json"}).Handler())
	defer srv.Close()

	res, err := http.Get(srv.URL + "/requests/1")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	var detail struct {
		BodyBase64 string `json:"bodyBase64"`
	}
	if err := json.NewDecoder(res.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail.BodyBase64 != "aGVsbG8=" {
		t.Fatalf("bodyBase64 = %q", detail.BodyBase64)
	}
}

func TestCreateRoute(t *testing.T) {
	f := &recordingFunnel{}
	recoveryPath := filepath.Join(t.TempDir(), "routes.json")
	reg := routes.NewRegistry(9090, f, func() (string, error) { return "https://node.ts.net", nil }, &routes.RecoveryFile{Path: recoveryPath})
	srv := httptest.NewServer(New(store.New(10), reg, &routes.RecoveryFile{Path: recoveryPath}).Handler())
	defer srv.Close()

	body := []byte(`{"path":"/hooks","target":"8080"}`)
	res, err := http.Post(srv.URL+"/routes", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status %d", res.StatusCode)
	}
	if len(f.started) != 1 {
		t.Fatalf("funnel starts: %v", f.started)
	}
}

func TestRecoveryAvailable(t *testing.T) {
	recoveryPath := filepath.Join(t.TempDir(), "routes.json")
	recovery := &routes.RecoveryFile{Path: recoveryPath}
	if err := recovery.Write([]routes.Checkpoint{{Path: "/hooks", Target: "http://localhost:3000"}}); err != nil {
		t.Fatal(err)
	}

	reg := routes.NewRegistry(8080, &recordingFunnel{}, func() (string, error) { return "", nil }, recovery)
	srv := httptest.NewServer(New(store.New(10), reg, recovery).Handler())
	defer srv.Close()

	res, err := http.Get(srv.URL + "/recovery")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	var payload struct {
		Available bool `json:"available"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Available {
		t.Fatal("expected available recovery")
	}
}

type recordingFunnel struct {
	started []string
	stopped []string
}

func (f *recordingFunnel) StartPath(mountPath, backendURL string) error {
	f.started = append(f.started, mountPath)
	return nil
}

func (f *recordingFunnel) StopPath(mountPath string) error {
	f.stopped = append(f.stopped, mountPath)
	return nil
}
