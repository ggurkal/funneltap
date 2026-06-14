package intercept

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ggurkal/funneltap/internal/routes"
	"github.com/ggurkal/funneltap/internal/store"
	"golang.org/x/net/websocket"
)

func TestHandlerRecordsAndProxies(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/github" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"ok":true}` {
			t.Fatalf("body = %q", body)
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
	h := NewHandler(reg, st, 1<<20)

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

	entry, ok := st.Get(items[0].ID)
	if !ok {
		t.Fatal("entry not found")
	}
	if string(entry.Body) != `{"ok":true}` {
		t.Fatalf("stored body = %q", entry.Body)
	}
}

func TestHandlerUnmatchedReturns404WithoutStore(t *testing.T) {
	reg := routes.NewRegistry(8080, &noopFunnel{}, func() (string, error) { return "", nil }, &routes.RecoveryFile{Path: t.TempDir() + "/r.json"})
	st := store.New(10)
	h := NewHandler(reg, st, 1<<20)
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

func TestHandlerSSEStream(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("no flusher")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for i := 0; i < 3; i++ {
			fmt.Fprintf(w, "data: %d\n\n", i)
			flusher.Flush()
			time.Sleep(200 * time.Millisecond)
		}
	}))
	defer backend.Close()

	reg, st, intercept := testIntercept(t, backend.URL)
	defer intercept.Close()

	resp, err := http.Get(intercept.URL + "/.ft/one/sse")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	var lines int
	for scanner.Scan() {
		if scanner.Text() != "" {
			lines++
		}
	}

	if lines < 3 {
		t.Fatalf("got %d lines, want >= 3", lines)
	}

	entry := latestEntry(t, st)
	if len(entry.Body) != 0 {
		t.Fatalf("sse body = %q, want empty", entry.Body)
	}
	if !entry.Proxy.Streaming && entry.Proxy.Status != http.StatusOK {
		t.Fatalf("proxy = %+v", entry.Proxy)
	}
	_ = reg
}

func TestHandlerWebSocketEcho(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/ws", websocket.Handler(func(ws *websocket.Conn) {
		var msg string
		if err := websocket.Message.Receive(ws, &msg); err != nil {
			return
		}
		_ = websocket.Message.Send(ws, "echo:"+msg)
	}))
	backend := httptest.NewServer(mux)
	defer backend.Close()

	_, st, intercept := testIntercept(t, backend.URL)
	defer intercept.Close()

	wsURL := "ws" + strings.TrimPrefix(intercept.URL, "http") + "/.ft/one/ws?token=abc"
	ws, err := websocket.Dial(wsURL, "", intercept.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	if err := websocket.Message.Send(ws, "ping"); err != nil {
		t.Fatal(err)
	}
	var recv string
	if err := websocket.Message.Receive(ws, &recv); err != nil {
		t.Fatal(err)
	}
	if recv != "echo:ping" {
		t.Fatalf("recv = %q", recv)
	}

	entry := latestEntry(t, st)
	if entry.Path != "/ws?token=abc" && entry.Path != "/ws" {
		t.Fatalf("path = %q", entry.Path)
	}
	if len(entry.Body) != 0 {
		t.Fatalf("ws body = %q, want empty", entry.Body)
	}
}

func TestHandlerMixedRouteInspectAndTunnel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(body)
	})
	mux.Handle("/ws", websocket.Handler(func(ws *websocket.Conn) {
		var msg string
		_ = websocket.Message.Receive(ws, &msg)
		_ = websocket.Message.Send(ws, "ok")
	}))
	backend := httptest.NewServer(mux)
	defer backend.Close()

	_, st, intercept := testIntercept(t, backend.URL)
	defer intercept.Close()

	postResp, err := http.Post(intercept.URL+"/.ft/one/test", "application/json", strings.NewReader(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	postResp.Body.Close()

	wsURL := "ws" + strings.TrimPrefix(intercept.URL, "http") + "/.ft/one/ws"
	ws, err := websocket.Dial(wsURL, "", intercept.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = websocket.Message.Send(ws, "x")
	var msg string
	_ = websocket.Message.Receive(ws, &msg)
	ws.Close()

	if len(st.List()) != 2 {
		t.Fatalf("stored %d items, want 2", len(st.List()))
	}

	var postID, wsID uint64
	for _, item := range st.List() {
		switch {
		case item.Method == http.MethodPost:
			postID = item.ID
		case item.Path == "/ws":
			wsID = item.ID
		}
	}

	post, ok := st.Get(postID)
	if !ok {
		t.Fatal("post entry missing")
	}
	if string(post.Body) != `{"a":1}` {
		t.Fatalf("post body = %q", post.Body)
	}

	wsEntry, ok := st.Get(wsID)
	if !ok {
		t.Fatal("ws entry missing")
	}
	if len(wsEntry.Body) != 0 {
		t.Fatalf("ws body = %q", wsEntry.Body)
	}
}

func TestHandlerLongResponseNotAborted(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("slow"))
	}))
	defer backend.Close()

	_, _, intercept := testIntercept(t, backend.URL)
	defer intercept.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(intercept.URL + "/.ft/one/slow")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "slow" {
		t.Fatalf("body = %q", body)
	}
}

func testIntercept(t *testing.T, backendURL string) (*routes.Registry, *store.Store, *httptest.Server) {
	t.Helper()
	reg := routes.NewRegistry(8080, &noopFunnel{}, func() (string, error) { return "", nil }, &routes.RecoveryFile{Path: t.TempDir() + "/r.json"})
	if _, err := reg.Add("/one", backendURL); err != nil {
		t.Fatal(err)
	}
	st := store.New(10)
	h := NewHandler(reg, st, 1<<20)
	return reg, st, httptest.NewServer(h)
}

func latestEntry(t *testing.T, st *store.Store) *store.Entry {
	t.Helper()
	items := st.List()
	if len(items) == 0 {
		t.Fatal("no entries")
	}
	entry, ok := st.Get(items[0].ID)
	if !ok {
		t.Fatal("entry missing")
	}
	return entry
}

func TestDecodeBodyRoundTrip(t *testing.T) {
	raw := `{"hello":"world"}`
	encoded := base64.StdEncoding.EncodeToString([]byte(raw))
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != raw {
		t.Fatalf("got %q", decoded)
	}
}

type noopFunnel struct{}

func (n *noopFunnel) StartPath(mountPath, backendURL string) error { return nil }
func (n *noopFunnel) StopPath(mountPath string) error              { return nil }
