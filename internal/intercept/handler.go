package intercept

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"time"

	"github.com/ggurkal/funneltap/internal/routes"
	"github.com/ggurkal/funneltap/internal/store"
)

var errBodyTooLarge = errors.New("request body too large")

type ctxKey struct{}

type reqCtx struct {
	id         uint64
	mode       Mode
	start      time.Time
	target     *url.URL
	statusCode int
}

type Handler struct {
	Routes       *routes.Registry
	Store        *store.Store
	MaxBodyBytes int64
	proxy        *httputil.ReverseProxy
}

func NewHandler(reg *routes.Registry, st *store.Store, maxBody int64) *Handler {
	h := &Handler{
		Routes:       reg,
		Store:        st,
		MaxBodyBytes: maxBody,
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 0

	h.proxy = &httputil.ReverseProxy{
		Transport:      transport,
		Rewrite:        h.rewrite,
		ModifyResponse: h.modifyResponse,
		ErrorHandler:   h.errorHandler,
	}
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route, upstreamPath, ok := h.Routes.Match(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	mode := Classify(r)

	storedPath := upstreamPath
	if r.URL.RawQuery != "" {
		storedPath += "?" + r.URL.RawQuery
	}

	targetURLStr, err := routes.BuildUpstreamURL(route.Target, upstreamPath, r.URL.RawQuery)
	if err != nil {
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	targetURL, err := url.Parse(targetURLStr)
	if err != nil {
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}

	if mode == ModeInspect && r.ContentLength > h.MaxBodyBytes {
		id := h.Store.Add(r.Method, storedPath, r.Header, nil, route.ID, route.Path, route.Target)
		h.Store.UpdateProxy(id, store.ProxyInfo{
			Status:     http.StatusRequestEntityTooLarge,
			DurationMs: 0,
			Error:      errBodyTooLarge.Error(),
		})
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}

	var capture *captureReader
	if mode == ModeInspect {
		capture = newCaptureReader(r.Body, h.MaxBodyBytes)
		r.Body = capture
	}

	id := h.Store.Add(r.Method, storedPath, r.Header, nil, route.ID, route.Path, route.Target)
	rcx := &reqCtx{
		id:     id,
		mode:   mode,
		start:  time.Now(),
		target: targetURL,
	}
	if mode == ModeTunnel {
		h.Store.UpdateProxy(id, store.ProxyInfo{Streaming: true})
	}

	r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, rcx))
	h.proxy.ServeHTTP(w, r)

	if capture != nil {
		h.Store.FinalizeBody(id, capture.bytes())
	}
	if mode == ModeTunnel {
		h.finishStream(rcx)
	}
}

func (h *Handler) rewrite(pr *httputil.ProxyRequest) {
	rcx := pr.In.Context().Value(ctxKey{}).(*reqCtx)
	// SetURL joins the incoming path with the target base path; we already
	// computed the full upstream URL, so assign it directly.
	pr.Out.URL.Scheme = rcx.target.Scheme
	pr.Out.URL.Host = rcx.target.Host
	pr.Out.URL.Path = rcx.target.Path
	pr.Out.URL.RawPath = rcx.target.RawPath
	pr.Out.URL.RawQuery = rcx.target.RawQuery
	pr.Out.Host = rcx.target.Host
}

func (h *Handler) modifyResponse(resp *http.Response) error {
	rcx := resp.Request.Context().Value(ctxKey{}).(*reqCtx)
	rcx.statusCode = resp.StatusCode

	if rcx.mode == ModeTunnel {
		h.Store.UpdateProxy(rcx.id, store.ProxyInfo{
			Status:    resp.StatusCode,
			Streaming: true,
		})
		return nil
	}

	h.Store.UpdateProxy(rcx.id, store.ProxyInfo{
		Status:     resp.StatusCode,
		DurationMs: time.Since(rcx.start).Milliseconds(),
	})
	return nil
}

func (h *Handler) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	rcx, _ := r.Context().Value(ctxKey{}).(*reqCtx)
	if rcx == nil {
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}

	status := http.StatusBadGateway
	if errors.Is(err, errBodyTooLarge) {
		status = http.StatusRequestEntityTooLarge
	}

	h.Store.UpdateProxy(rcx.id, store.ProxyInfo{
		Status:     status,
		DurationMs: time.Since(rcx.start).Milliseconds(),
		Error:      err.Error(),
	})

	if capture, ok := r.Body.(*captureReader); ok {
		h.Store.FinalizeBody(rcx.id, capture.bytes())
	}

	http.Error(w, http.StatusText(status), status)
}

func (h *Handler) finishStream(rcx *reqCtx) {
	now := time.Now()
	closed := now
	status := rcx.statusCode
	if status == 0 {
		status = http.StatusOK
	}
	h.Store.UpdateProxy(rcx.id, store.ProxyInfo{
		Status:     status,
		DurationMs: now.Sub(rcx.start).Milliseconds(),
		Streaming:  false,
		ClosedAt:   &closed,
	})
}

type captureReader struct {
	r       io.ReadCloser
	buf     bytes.Buffer
	max     int64
	n       int64
	tooLarge bool
}

func newCaptureReader(r io.ReadCloser, max int64) *captureReader {
	if r == nil {
		r = io.NopCloser(http.NoBody)
	}
	return &captureReader{r: r, max: max}
}

func (c *captureReader) Read(p []byte) (int, error) {
	if c.tooLarge {
		return 0, errBodyTooLarge
	}
	n, err := c.r.Read(p)
	if n > 0 {
		c.n += int64(n)
		if c.n > c.max {
			c.tooLarge = true
			return n, errBodyTooLarge
		}
		_, _ = c.buf.Write(p[:n])
	}
	return n, err
}

func (c *captureReader) Close() error {
	return c.r.Close()
}

func (c *captureReader) bytes() []byte {
	return append([]byte(nil), c.buf.Bytes()...)
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

func splitHostPort(addr string) (host, port string, err error) {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i], addr[i+1:], nil
		}
	}
	return "", "", fmt.Errorf("address missing port: %s", addr)
}
