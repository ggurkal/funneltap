package api

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"

	"github.com/ggurkal/funneltap/internal/routes"
	"github.com/ggurkal/funneltap/internal/store"
)

//go:embed ui/*
var uiFS embed.FS

type Server struct {
	Store    *store.Store
	Routes   *routes.Registry
	Recovery *routes.RecoveryFile
}

func New(st *store.Store, reg *routes.Registry, recovery *routes.RecoveryFile) *Server {
	return &Server{Store: st, Routes: reg, Recovery: recovery}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /requests", s.handleList)
	mux.HandleFunc("GET /requests/{id}", s.handleGet)
	mux.HandleFunc("DELETE /requests", s.handleClear)
	mux.HandleFunc("GET /routes", s.handleRoutesList)
	mux.HandleFunc("POST /routes", s.handleRoutesCreate)
	mux.HandleFunc("DELETE /routes/{id}", s.handleRoutesDelete)
	mux.HandleFunc("GET /recovery", s.handleRecoveryGet)
	mux.HandleFunc("POST /recovery/restore", s.handleRecoveryRestore)
	mux.HandleFunc("POST /recovery/dismiss", s.handleRecoveryDismiss)
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
	routeID := r.URL.Query().Get("route")
	var items []store.Summary
	if afterStr := r.URL.Query().Get("after"); afterStr != "" {
		after, err := strconv.ParseUint(afterStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid after parameter", http.StatusBadRequest)
			return
		}
		items = s.Store.ListAfter(after, routeID)
	} else {
		if routeID != "" {
			items = s.Store.ListAfter(0, routeID)
		} else {
			items = s.Store.List()
		}
	}
	if items == nil {
		items = []store.Summary{}
	}
	items = s.filterActiveRoutes(items, routeID)
	writeJSON(w, items)
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	entry, ok := s.Store.Get(id)
	if !ok || !s.isActiveRoute(entry.RouteID) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	type detail struct {
		ID         uint64            `json:"id"`
		ReceivedAt interface{}       `json:"receivedAt"`
		Method     string            `json:"method"`
		Path       string            `json:"path"`
		RouteID    string            `json:"routeId,omitempty"`
		RoutePath  string            `json:"routePath,omitempty"`
		Target     string            `json:"target,omitempty"`
		Headers    map[string]string `json:"headers"`
		BodyBase64 string            `json:"bodyBase64"`
		Proxy      store.ProxyInfo   `json:"proxy"`
	}

	writeJSON(w, detail{
		ID:         entry.ID,
		ReceivedAt: entry.ReceivedAt,
		Method:     entry.Method,
		Path:       entry.Path,
		RouteID:    entry.RouteID,
		RoutePath:  entry.RoutePath,
		Target:     entry.Target,
		Headers:    flattenHeader(entry.Headers),
		BodyBase64: base64.StdEncoding.EncodeToString(entry.Body),
		Proxy:      entry.Proxy,
	})
}

func (s *Server) handleClear(w http.ResponseWriter, r *http.Request) {
	s.Store.Clear()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRoutesList(w http.ResponseWriter, r *http.Request) {
	items := s.Routes.List()
	if items == nil {
		items = []routes.Route{}
	}
	writeJSON(w, items)
}

func (s *Server) handleRoutesCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path   string `json:"path"`
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	rt, err := s.Routes.Add(body.Path, body.Target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, rt)
}

func (s *Server) handleRoutesDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Routes.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRecoveryGet(w http.ResponseWriter, r *http.Request) {
	if !s.Recovery.Exists() {
		writeJSON(w, map[string]any{"available": false})
		return
	}
	checkpoints, err := s.Recovery.Read()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	preview, err := routes.RestoreFromCheckpoint(checkpoints)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"available": true,
		"routes":    preview,
	})
}

func (s *Server) handleRecoveryRestore(w http.ResponseWriter, r *http.Request) {
	checkpoints, err := s.Recovery.Read()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(checkpoints) == 0 {
		writeJSON(w, []routes.Route{})
		return
	}
	activated, err := s.Routes.ActivateCheckpoint(checkpoints)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, activated)
}

func (s *Server) handleRecoveryDismiss(w http.ResponseWriter, r *http.Request) {
	if err := s.Recovery.Delete(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) isActiveRoute(routeID string) bool {
	if routeID == "" {
		return false
	}
	_, ok := s.Routes.Get(routeID)
	return ok
}

func (s *Server) filterActiveRoutes(items []store.Summary, routeFilter string) []store.Summary {
	active := s.Routes.List()
	if len(active) == 0 {
		return []store.Summary{}
	}

	activeIDs := make(map[string]struct{}, len(active))
	for _, rt := range active {
		activeIDs[rt.ID] = struct{}{}
	}

	var out []store.Summary
	for _, item := range items {
		if _, ok := activeIDs[item.RouteID]; !ok {
			continue
		}
		if routeFilter != "" && item.RouteID != routeFilter {
			continue
		}
		out = append(out, item)
	}
	if out == nil {
		return []store.Summary{}
	}
	return out
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
