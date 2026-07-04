package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/yourorg/panel-daemon/internal/console"
	"github.com/yourorg/panel-daemon/internal/docker"
)

func NewRouter(dockerManager *docker.Manager, consoleHub *console.Hub, daemonToken string) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(RequireDaemonToken(daemonToken))

	h := &Handlers{Docker: dockerManager, Console: consoleHub}

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/servers", h.CreateServer)
		r.Post("/servers/{uuid}/power", h.Power)
		r.Delete("/servers/{uuid}", h.Delete)
	})
	r.Get("/ws/servers/{uuid}", h.ConsoleSocket)

	return r
}
