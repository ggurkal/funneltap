package intercept

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ggurkal/funneltap/internal/routes"
	"github.com/ggurkal/funneltap/internal/store"
)

func TestHandlerRecordsAndProxies(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/github" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	}))
	defer backend.Close()

	reg := routes.NewRegistry(8080, &noopFunnel{}, func() (string, error) { return "", nil }, &routes.RecoveryFile{Path: t.TempDir() + "/r.json"})
	if _, err := reg.Add("/hooks", backend.URL); err != nil {
		t.Fatal(err)
	}

	st := store.New(10)
	h := NewHandler(reg, st, 5*time.Second, 1<<20)

	intercept := httptest.NewServer(h)
	defer intercept.Close()

	req, err := http.NewRequest(http.MethodPost, intercept.URL+"/.ft/hooks/github", strings.NewReader(`{"ok":true}`))
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	items := st.List()
	if len(items) != 1 {
		t.Fatalf("stored %d items", len(items))
	}
	if items[0].RoutePath != "/hooks" {
		t.Fatalf("routePath = %q", items[0].RoutePath)
	}
	if items[0].Path != "/github" {
		t.Fatalf("path = %q, want /github", items[0].Path)
	}
}

func TestHandlerUnmatchedReturns404WithoutStore(t *testing.T) {
	reg := routes.NewRegistry(8080, &noopFunnel{}, func() (string, error) { return "", nil }, &routes.RecoveryFile{Path: t.TempDir() + "/r.json"})
	st := store.New(10)
	h := NewHandler(reg, st, 5*time.Second, 1<<20)
	intercept := httptest.NewServer(h)
	defer intercept.Close()

	resp, err := http.Get(intercept.URL + "/.ft/missing")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if len(st.List()) != 0 {
		t.Fatal("expected no stored requests")
	}
}

type noopFunnel struct{}

func (n *noopFunnel) StartPath(mountPath, backendURL string) error { return nil }
func (n *noopFunnel) StopPath(mountPath string) error              { return nil }
