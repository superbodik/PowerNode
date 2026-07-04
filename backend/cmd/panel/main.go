package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/yourorg/panel/internal/api"
	"github.com/yourorg/panel/internal/auth"
	"github.com/yourorg/panel/internal/config"
	"github.com/yourorg/panel/internal/daemonclient"
	"github.com/yourorg/panel/internal/db"
	"github.com/yourorg/panel/internal/ws"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()

	pool, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	tokenManager := auth.NewTokenManager(cfg.JWTSecret, cfg.AccessTokenTTL)
	hub := ws.NewHub()

	router := api.NewRouter(api.Dependencies{
		DB:    pool,
		Token: tokenManager,
		Hub:   hub,
		NodeClient: func(nodeID int64) (*daemonclient.Client, error) {
			return nil, errNodeClientNotConfigured
		},
	})

	srv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: router,
	}

	go func() {
		log.Printf("panel listening on %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

var errNodeClientNotConfigured = &nodeClientError{}

type nodeClientError struct{}

func (*nodeClientError) Error() string {
	return "node client resolver not configured: wire up internal/daemonclient lookups in cmd/panel/main.go"
}
