// Test backend for proxy/streaming manual integration tests.
//
// Routes:
//   POST /test  — simple echo (inspect mode)
//   GET  /sse   — Server-Sent Events (tunnel mode)
//   GET  /ws    — WebSocket echo (tunnel mode)
//
// Usage:
//   go run . -addr 127.0.0.1:9876
//   go run . -client ws://127.0.0.1:PORT/.ft/one/ws
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"golang.org/x/net/websocket"
)

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "-client" {
		runWSClient(os.Args[2])
		return
	}

	addr := flag.String("addr", "127.0.0.1:9876", "listen address")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/test", testHandler)
	mux.HandleFunc("/sse", sseHandler)
	mux.Handle("/ws", websocket.Handler(wsHandler))

	log.Printf("test backend listening on http://%s", *addr)
	log.Printf("  POST /test")
	log.Printf("  GET  /sse?count=N   (default 5 events, 1s apart)")
	log.Printf("  GET  /ws")
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func testHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(append([]byte("ok:"), body...))
}

func sseHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	count := 5
	if _, err := fmt.Sscanf(r.URL.Query().Get("count"), "%d", &count); err != nil || count < 1 {
		count = 5
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	for i := 1; i <= count; i++ {
		select {
		case <-r.Context().Done():
			return
		default:
			_, _ = fmt.Fprintf(w, "data: event %d\n\n", i)
			flusher.Flush()
			time.Sleep(time.Second)
		}
	}
}

func wsHandler(ws *websocket.Conn) {
	defer ws.Close()
	var msg string
	for {
		if err := websocket.Message.Receive(ws, &msg); err != nil {
			return
		}
		if err := websocket.Message.Send(ws, "echo:"+msg); err != nil {
			return
		}
	}
}

func runWSClient(url string) {
	ws, err := websocket.Dial(url, "", "http://localhost/")
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer ws.Close()

	const send = "ping"
	if err := websocket.Message.Send(ws, send); err != nil {
		log.Fatalf("send: %v", err)
	}
	var recv string
	if err := websocket.Message.Receive(ws, &recv); err != nil {
		log.Fatalf("recv: %v", err)
	}
	want := "echo:" + send
	if recv != want {
		log.Fatalf("got %q, want %q", recv, want)
	}
	fmt.Println(recv)
}
