package api

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"

	"github.com/ggurkal/funneltap/internal/store"
)

//go:embed ui/*
var uiFS embed.FS

type Server struct {
	Store *store.Store
}

func New(st *store.Store) *Server {
	return &Server{Store: st}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /requests", s.handleList)
	mux.HandleFunc("GET /requests/{id}", s.handleGet)
	mux.HandleFunc("DELETE /requests", s.handleClear)
	mux.HandleFunc("GET /ui", s.redirectUI)
	mux.Handle("GET /ui/", s.uiHandler())
	return mux
}

func (s *Server) redirectUI(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/ui/", http.StatusFound)
}

func (s *Server) uiHandler() http.Handler {
	sub, err := fs.Sub(uiFS, "ui")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/ui/", http.FileServer(http.FS(sub)))
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	var items []store.Summary
	if afterStr := r.URL.Query().Get("after"); afterStr != "" {
		after, err := strconv.ParseUint(afterStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid after parameter", http.StatusBadRequest)
			return
		}
		items = s.Store.ListAfter(after)
	} else {
		items = s.Store.List()
	}
	if items == nil {
		items = []store.Summary{}
	}
	writeJSON(w, items)
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	entry, ok := s.Store.Get(id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	type detail struct {
		ID         uint64            `json:"id"`
		ReceivedAt interface{}       `json:"receivedAt"`
		Method     string            `json:"method"`
		Path       string            `json:"path"`
		Headers    map[string]string `json:"headers"`
		BodyBase64 string            `json:"bodyBase64"`
		Proxy      store.ProxyInfo   `json:"proxy"`
	}

	writeJSON(w, detail{
		ID:         entry.ID,
		ReceivedAt: entry.ReceivedAt,
		Method:     entry.Method,
		Path:       entry.Path,
		Headers:    flattenHeader(entry.Headers),
		BodyBase64: base64.StdEncoding.EncodeToString(entry.Body),
		Proxy:      entry.Proxy,
	})
}

func (s *Server) handleClear(w http.ResponseWriter, r *http.Request) {
	s.Store.Clear()
	w.WriteHeader(http.StatusNoContent)
}

func flattenHeader(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, vv := range h {
		if len(vv) == 1 {
			out[k] = vv[0]
		} else {
			out[k] = joinHeaderValues(vv)
		}
	}
	return out
}

func joinHeaderValues(vv []string) string {
	if len(vv) == 0 {
		return ""
	}
	s := vv[0]
	for i := 1; i < len(vv); i++ {
		s += ", " + vv[i]
	}
	return s
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
