package routes

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/ggurkal/funneltap/internal/config"
)

// Funnel controls tailscale funnel mounts for routes.
type Funnel interface {
	StartPath(mountPath, backendURL string) error
	StopPath(mountPath string) error
}

// PublicURLFunc returns the HTTPS base URL for this node (no trailing slash).
type PublicURLFunc func() (string, error)

// Route is an active path-based funnel route.
type Route struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	Target    string `json:"target"`
	PublicURL string `json:"publicURL"`
}

// Registry holds in-memory routes.
type Registry struct {
	mu            sync.RWMutex
	interceptPort int
	funnel        Funnel
	publicURL     PublicURLFunc
	recovery      *RecoveryFile
	byID          map[string]*Route
	order         []*Route
}

func NewRegistry(interceptPort int, funnel Funnel, publicURL PublicURLFunc, recovery *RecoveryFile) *Registry {
	return &Registry{
		interceptPort: interceptPort,
		funnel:        funnel,
		publicURL:     publicURL,
		recovery:      recovery,
		byID:          make(map[string]*Route),
	}
}

func (r *Registry) InterceptPort() int {
	return r.interceptPort
}

func (r *Registry) List() []Route {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Route, len(r.order))
	for i, rt := range r.order {
		out[i] = *rt
	}
	return out
}

func (r *Registry) Get(id string) (*Route, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rt, ok := r.byID[id]
	if !ok {
		return nil, false
	}
	cp := *rt
	return &cp, true
}

func (r *Registry) Add(pathRaw, targetRaw string) (*Route, error) {
	path, err := NormalizePath(pathRaw)
	if err != nil {
		return nil, err
	}
	targetURL, err := config.ParseTarget(targetRaw)
	if err != nil {
		return nil, fmt.Errorf("parse target: %w", err)
	}
	target := config.FormatTarget(targetURL)

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existing := range r.order {
		if PathsOverlap(path, existing.Path) {
			return nil, fmt.Errorf("path overlaps existing route %q", existing.Path)
		}
	}

	backend := InternalBackendURL(r.interceptPort, path)
	if err := r.funnel.StartPath(path, backend); err != nil {
		return nil, fmt.Errorf("tailscale funnel: %w", err)
	}

	publicBase, err := r.publicURL()
	publicURL := ""
	if err == nil {
		publicURL = publicBase + path
	}

	rt := &Route{
		ID:        newRouteID(),
		Path:      path,
		Target:    target,
		PublicURL: publicURL,
	}
	r.byID[rt.ID] = rt
	r.order = append(r.order, rt)

	if err := r.persistLocked(); err != nil {
		_ = r.funnel.StopPath(path)
		delete(r.byID, rt.ID)
		r.order = r.order[:len(r.order)-1]
		return nil, err
	}
	cp := *rt
	return &cp, nil
}

func (r *Registry) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	idx := -1
	var rt *Route
	for i, candidate := range r.order {
		if candidate.ID == id {
			idx = i
			rt = candidate
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("route not found")
	}

	if err := r.funnel.StopPath(rt.Path); err != nil {
		return fmt.Errorf("tailscale funnel: %w", err)
	}

	delete(r.byID, rt.ID)
	r.order = append(r.order[:idx], r.order[idx+1:]...)
	return r.persistLocked()
}

func (r *Registry) StopAll() error {
	r.mu.RLock()
	paths := make([]string, len(r.order))
	for i, rt := range r.order {
		paths[i] = rt.Path
	}
	r.mu.RUnlock()

	var first error
	for _, path := range paths {
		if err := r.funnel.StopPath(path); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// RestoreFromCheckpoint loads routes from recovery without funnel; used for preview.
func RestoreFromCheckpoint(checkpoints []Checkpoint) ([]Route, error) {
	out := make([]Route, 0, len(checkpoints))
	for _, cp := range checkpoints {
		path, err := NormalizePath(cp.Path)
		if err != nil {
			return nil, err
		}
		targetURL, err := config.ParseTarget(cp.Target)
		if err != nil {
			return nil, fmt.Errorf("parse target %q: %w", cp.Target, err)
		}
		out = append(out, Route{
			ID:     newRouteID(),
			Path:   path,
			Target: config.FormatTarget(targetURL),
		})
	}
	return out, nil
}

// ActivateCheckpoint routes start funnel mounts and enter the registry.
func (r *Registry) ActivateCheckpoint(checkpoints []Checkpoint) ([]Route, error) {
	preview, err := RestoreFromCheckpoint(checkpoints)
	if err != nil {
		return nil, err
	}
	activated := make([]Route, 0, len(preview))
	for _, cp := range preview {
		rt, err := r.Add(cp.Path, cp.Target)
		if err != nil {
			for _, done := range activated {
				_ = r.Delete(done.ID)
			}
			return nil, err
		}
		activated = append(activated, *rt)
	}
	return activated, nil
}

// Match finds the route for an intercept request path and returns upstream path.
func (r *Registry) Match(requestPath string) (*Route, string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	type candidate struct {
		rt     *Route
		prefix string
	}
	var matches []candidate
	for _, rt := range r.order {
		prefix := InternalPrefix(rt.Path)
		if requestPath == prefix || strings.HasPrefix(requestPath, prefix+"/") {
			matches = append(matches, candidate{rt: rt, prefix: prefix})
		}
	}
	if len(matches) == 0 {
		return nil, "", false
	}
	sort.Slice(matches, func(i, j int) bool {
		return len(matches[i].prefix) > len(matches[j].prefix)
	})
	best := matches[0]
	suffix := strings.TrimPrefix(requestPath, best.prefix)
	if suffix == "" {
		suffix = "/"
	} else if !strings.HasPrefix(suffix, "/") {
		suffix = "/" + suffix
	}
	cp := *best.rt
	return &cp, suffix, true
}

func (r *Registry) persistLocked() error {
	cps := make([]Checkpoint, len(r.order))
	for i, rt := range r.order {
		cps[i] = Checkpoint{Path: rt.Path, Target: rt.Target}
	}
	return r.recovery.Write(cps)
}

func newRouteID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// BuildUpstreamURL joins a route target origin with a path suffix and optional query.
func BuildUpstreamURL(target string, path string, rawQuery string) (string, error) {
	u, err := url.Parse(target)
	if err != nil {
		return "", err
	}
	if path == "" {
		path = "/"
	}
	u.Path = path
	u.RawQuery = rawQuery
	u.Fragment = ""
	return u.String(), nil
}
