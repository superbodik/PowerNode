package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/yourorg/panel-daemon/internal/api"
	"github.com/yourorg/panel-daemon/internal/config"
	"github.com/yourorg/panel-daemon/internal/console"
	"github.com/yourorg/panel-daemon/internal/docker"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	if cfg.DaemonToken == "" {
		log.Fatal("WINGSD_DAEMON_TOKEN is required (issued by the panel when the node is created)")
	}

	dockerManager, err := docker.NewManager(cfg.DockerSocket, cfg.DataDir)
	if err != nil {
		log.Fatalf("docker: %v", err)
	}
	consoleHub := console.NewHub(dockerManager)

	router := api.NewRouter(dockerManager, consoleHub, cfg.DaemonToken)

	srv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: router,
	}

	go func() {
		log.Printf("wingsd listening on %s", cfg.HTTPAddr)
		var err error
		if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
			err = srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
		} else {
			log.Println("warning: running without TLS — set WINGSD_TLS_CERT/WINGSD_TLS_KEY in production")
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
