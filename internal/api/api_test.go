package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ggurkal/funneltap/internal/store"
)

func TestListAfter(t *testing.T) {
	st := store.New(10)
	st.Add("GET", "/a", nil, nil)
	st.UpdateProxy(1, store.ProxyInfo{Status: 200, DurationMs: 1})
	st.Add("POST", "/b", nil, []byte("hi"))
	st.UpdateProxy(2, store.ProxyInfo{Status: 201, DurationMs: 2})

	srv := httptest.NewServer(New(st).Handler())
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

func TestDetailBodyBase64(t *testing.T) {
	st := store.New(10)
	st.Add("POST", "/x", nil, []byte("hello"))
	st.UpdateProxy(1, store.ProxyInfo{Status: 200, DurationMs: 1})

	srv := httptest.NewServer(New(st).Handler())
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
