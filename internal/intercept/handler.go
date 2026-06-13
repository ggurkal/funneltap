package intercept

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/ggurkal/funneltap/internal/store"
)

type Handler struct {
	Target       *url.URL
	Store        *store.Store
	ProxyTimeout time.Duration
	MaxBodyBytes int64
	Client       *http.Client
}

func NewHandler(target *url.URL, st *store.Store, timeout time.Duration, maxBody int64) *Handler {
	return &Handler{
		Target:       target,
		Store:        st,
		ProxyTimeout: timeout,
		MaxBodyBytes: maxBody,
		Client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, h.MaxBodyBytes+1))
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	if int64(len(body)) > h.MaxBodyBytes {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}

	path := r.URL.Path
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}

	id := h.Store.Add(r.Method, path, r.Header, body)

	targetURL := buildTargetURL(h.Target, r)
	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, bytes.NewReader(body))
	if err != nil {
		h.finishError(id, w, err)
		return
	}
	outReq.Header = r.Header.Clone()
	if outReq.Header == nil {
		outReq.Header = make(http.Header)
	}

	start := time.Now()
	resp, err := h.Client.Do(outReq)
	duration := time.Since(start)

	if err != nil {
		proxy := store.ProxyInfo{
			DurationMs: duration.Milliseconds(),
			Error:      err.Error(),
		}
		if isTimeout(err) {
			proxy.Status = http.StatusGatewayTimeout
			h.Store.UpdateProxy(id, proxy)
			http.Error(w, "gateway timeout", http.StatusGatewayTimeout)
			return
		}
		proxy.Status = http.StatusBadGateway
		h.Store.UpdateProxy(id, proxy)
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	h.Store.UpdateProxy(id, store.ProxyInfo{
		Status:     resp.StatusCode,
		DurationMs: duration.Milliseconds(),
	})

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (h *Handler) finishError(id uint64, w http.ResponseWriter, err error) {
	h.Store.UpdateProxy(id, store.ProxyInfo{
		Status:     http.StatusBadGateway,
		DurationMs: 0,
		Error:      err.Error(),
	})
	http.Error(w, "bad gateway", http.StatusBadGateway)
}

func buildTargetURL(origin *url.URL, incoming *http.Request) string {
	u := *origin
	u.Path = incoming.URL.Path
	u.RawQuery = incoming.URL.RawQuery
	u.Fragment = ""
	return u.String()
}

func InterceptPort(addr string) (int, error) {
	_, portStr, err := splitHostPort(addr)
	if err != nil {
		return 0, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, fmt.Errorf("invalid intercept port: %w", err)
	}
	return port, nil
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var urlErr *url.Error
	return errors.As(err, &urlErr) && urlErr.Timeout()
}

func splitHostPort(addr string) (host, port string, err error) {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i], addr[i+1:], nil
		}
	}
	return "", "", fmt.Errorf("address missing port: %s", addr)
}
