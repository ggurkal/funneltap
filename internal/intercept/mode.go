package intercept

import (
	"net/http"
	"strings"
)

type Mode int

const (
	ModeInspect Mode = iota
	ModeTunnel
)

func Classify(r *http.Request) Mode {
	if isWebSocketUpgrade(r) {
		return ModeTunnel
	}
	if acceptsEventStream(r) {
		return ModeTunnel
	}
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return ModeInspect
	default:
		return ModeTunnel
	}
}

func isWebSocketUpgrade(r *http.Request) bool {
	if !headerHasToken(r.Header, "Connection", "upgrade") {
		return false
	}
	return headerHasToken(r.Header, "Upgrade", "websocket")
}

func acceptsEventStream(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	if accept == "" {
		return false
	}
	for _, part := range strings.Split(accept, ",") {
		if strings.TrimSpace(strings.Split(part, ";")[0]) == "text/event-stream" {
			return true
		}
	}
	return false
}

func headerHasToken(h http.Header, key, token string) bool {
	for _, v := range h[key] {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}
