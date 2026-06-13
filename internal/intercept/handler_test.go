package intercept

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ggurkal/funneltap/internal/store"
)

func TestHandlerRecordsAndProxies(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.Path != "/hook" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if string(body) != `{"ok":true}` {
			t.Fatalf("body = %s", body)
		}
		if r.Header.Get("X-Test") != "1" {
			t.Fatalf("missing X-Test header")
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	}))
	defer backend.Close()

	target, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatal(err)
	}

	st := store.New(10)
	h := NewHandler(target, st, 5*time.Second, 1<<20)

	intercept := httptest.NewServer(h)
	defer intercept.Close()

	req, err := http.NewRequest(http.MethodPost, intercept.URL+"/hook", strings.NewReader(`{"ok":true}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Test", "1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "created" {
		t.Fatalf("response = %s", body)
	}

	items := st.List()
	if len(items) != 1 {
		t.Fatalf("stored %d items", len(items))
	}
	if items[0].Proxy.Status != http.StatusCreated {
		t.Fatalf("proxy status = %d", items[0].Proxy.Status)
	}
}
