package store

import (
	"net/http"
	"sort"
	"sync"
	"time"
)

type ProxyInfo struct {
	Status     int    `json:"status"`
	DurationMs int64  `json:"durationMs"`
	Error      string `json:"error,omitempty"`
}

type Entry struct {
	ID         uint64      `json:"id"`
	ReceivedAt time.Time   `json:"receivedAt"`
	Method     string      `json:"method"`
	Path       string      `json:"path"`
	Headers    http.Header `json:"headers,omitempty"`
	Body       []byte      `json:"-"`
	Proxy      ProxyInfo   `json:"proxy"`
}

type Summary struct {
	ID         uint64    `json:"id"`
	ReceivedAt time.Time `json:"receivedAt"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	Proxy      ProxyInfo `json:"proxy"`
}

type Store struct {
	mu         sync.RWMutex
	entries    []*Entry
	nextID     uint64
	maxEntries int
}

func New(maxEntries int) *Store {
	return &Store{maxEntries: maxEntries}
}

func (s *Store) Add(method, path string, headers http.Header, body []byte) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	entry := &Entry{
		ID:         s.nextID,
		ReceivedAt: time.Now().UTC(),
		Method:     method,
		Path:       path,
		Headers:    cloneHeader(headers),
		Body:       append([]byte(nil), body...),
	}

	s.entries = append(s.entries, entry)
	if len(s.entries) > s.maxEntries {
		s.entries = s.entries[len(s.entries)-s.maxEntries:]
	}

	return entry.ID
}

func (s *Store) UpdateProxy(id uint64, proxy ProxyInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, e := range s.entries {
		if e.ID == id {
			e.Proxy = proxy
			return
		}
	}
}

func (s *Store) Get(id uint64) (*Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for i := len(s.entries) - 1; i >= 0; i-- {
		if s.entries[i].ID == id {
			return s.entries[i], true
		}
	}
	return nil, false
}

func (s *Store) List() []Summary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return summariesFrom(s.entries, 0)
}

func (s *Store) ListAfter(after uint64) []Summary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return summariesFrom(s.entries, after)
}

func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = nil
}

func summariesFrom(entries []*Entry, after uint64) []Summary {
	var out []Summary
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.ID <= after {
			continue
		}
		out = append(out, e.summary())
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID > out[j].ID
	})
	return out
}

func (e *Entry) summary() Summary {
	return Summary{
		ID:         e.ID,
		ReceivedAt: e.ReceivedAt,
		Method:     e.Method,
		Path:       e.Path,
		Proxy:      e.Proxy,
	}
}

func cloneHeader(h http.Header) http.Header {
	if h == nil {
		return make(http.Header)
	}
	dst := make(http.Header, len(h))
	for k, vv := range h {
		dst[k] = append([]string(nil), vv...)
	}
	return dst
}
