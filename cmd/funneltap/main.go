package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/ggurkal/funneltap/internal/api"
	"github.com/ggurkal/funneltap/internal/config"
	"github.com/ggurkal/funneltap/internal/funnel"
	"github.com/ggurkal/funneltap/internal/intercept"
	"github.com/ggurkal/funneltap/internal/routes"
	"github.com/ggurkal/funneltap/internal/store"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("funneltap %s (%s) %s\n", version, commit, date)
		return
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "funneltap: %v\n", err)
		os.Exit(1)
	}

	st := store.New(cfg.MaxRequests)
	funnelCLI := funnel.NewCLI()
	recovery := &routes.RecoveryFile{Path: cfg.RoutesFile}
	reg := routes.NewRegistry(cfg.InterceptPort, funnelCLI, funnel.MachineHTTPSURL, recovery)

	interceptHandler := intercept.NewHandler(reg, st, cfg.ProxyTimeout, cfg.MaxBodyBytes)
	interceptServer := &http.Server{
		Addr:    cfg.InterceptAddr,
		Handler: interceptHandler,
	}

	apiServer := &http.Server{
		Addr:    cfg.APIAddr,
		Handler: api.New(st, reg, recovery).Handler(),
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

	log.Printf("ui: http://%s/ui/", joinHostForLog(cfg.APIAddr))

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Printf("shutting down...")
	if err := reg.StopAll(); err != nil {
		log.Printf("stop funnel: %v", err)
	}
	if err := recovery.Delete(); err != nil {
		log.Printf("delete recovery file: %v", err)
	}
	_ = interceptServer.Close()
	_ = apiServer.Close()
}

func joinHostForLog(addr string) string {
	host, port := splitHostPort(addr)
	if host == "0.0.0.0" {
		return "localhost:" + port
	}
	return addr
}

func splitHostPort(addr string) (host, port string) {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i], addr[i+1:]
		}
	}
	return addr, ""
}
