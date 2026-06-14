package intercept

import (
	"net/http"
	"strings"
	"testing"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		req  *http.Request
		want Mode
	}{
		{
			name: "post inspect",
			req:  httpreq(http.MethodPost, "/", `{"x":1}`),
			want: ModeInspect,
		},
		{
			name: "get tunnel",
			req:  httpreq(http.MethodGet, "/", ""),
			want: ModeTunnel,
		},
		{
			name: "websocket",
			req: func() *http.Request {
				r := httpreq(http.MethodGet, "/ws", "")
				r.Header.Set("Connection", "Upgrade")
				r.Header.Set("Upgrade", "websocket")
				return r
			}(),
			want: ModeTunnel,
		},
		{
			name: "sse",
			req: func() *http.Request {
				r := httpreq(http.MethodGet, "/sse", "")
				r.Header.Set("Accept", "text/event-stream")
				return r
			}(),
			want: ModeTunnel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.req); got != tt.want {
				t.Fatalf("Classify() = %v, want %v", got, tt.want)
			}
		})
	}
}

func httpreq(method, path, body string) *http.Request {
	r, _ := http.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		r.ContentLength = int64(len(body))
	}
	return r
}
