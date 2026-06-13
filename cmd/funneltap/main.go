package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/ggurkal/funneltap/internal/api"
	"github.com/ggurkal/funneltap/internal/config"
	"github.com/ggurkal/funneltap/internal/funnel"
	"github.com/ggurkal/funneltap/internal/intercept"
	"github.com/ggurkal/funneltap/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "funneltap: %v\n", err)
		os.Exit(1)
	}

	st := store.New(cfg.MaxRequests)

	interceptHandler := intercept.NewHandler(cfg.Target, st, cfg.ProxyTimeout, cfg.MaxBodyBytes)
	interceptServer := &http.Server{
		Addr:    cfg.InterceptAddr,
		Handler: interceptHandler,
	}

	apiServer := &http.Server{
		Addr:    cfg.APIAddr,
		Handler: api.New(st).Handler(),
	}

	go func() {
		log.Printf("intercept server listening on http://%s", cfg.InterceptAddr)
		if err := interceptServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("intercept server: %v", err)
		}
	}()

	go func() {
		log.Printf("api server listening on http://%s", cfg.APIAddr)
		if err := apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("api server: %v", err)
		}
	}()

	port, err := intercept.InterceptPort(cfg.InterceptAddr)
	if err != nil {
		log.Fatalf("intercept port: %v", err)
	}

	if err := funnel.Start(port); err != nil {
		log.Fatalf("%v", err)
	}

	log.Printf("target: %s", cfg.TargetOrigin())
	log.Printf("ui: http://%s/ui/", joinHostForLog(cfg.APIAddr))

	select {}
}

func splitHostPort(addr string) (host, port string) {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i], addr[i+1:]
		}
	}
	return addr, ""
}

func joinHostForLog(addr string) string {
	host, port := splitHostPort(addr)
	if host == "0.0.0.0" {
		return "localhost:" + port
	}
	return addr
}
