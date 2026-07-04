package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/panel/internal/api/handlers"
	"github.com/yourorg/panel/internal/auth"
	"github.com/yourorg/panel/internal/daemonclient"
	"github.com/yourorg/panel/internal/ws"
)

type Dependencies struct {
	DB         *pgxpool.Pool
	Token      *auth.TokenManager
	Hub        *ws.Hub
	NodeClient func(nodeID int64) (*daemonclient.Client, error)
}

func NewRouter(deps Dependencies) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	authHandler := &handlers.AuthHandler{DB: deps.DB, Token: deps.Token}
	nodeHandler := &handlers.NodeHandler{DB: deps.DB}
	serverHandler := &handlers.ServerHandler{DB: deps.DB, NodeClient: deps.NodeClient}

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/login", authHandler.Login)

		r.Group(func(r chi.Router) {
			r.Use(auth.Middleware(deps.Token))

			r.Get("/auth/me", authHandler.Me)

			r.Get("/nodes", nodeHandler.List)
			r.With(auth.RequireAdmin).Post("/nodes", nodeHandler.Create)

			r.Get("/servers", serverHandler.List)
			r.Get("/servers/{uuid}", serverHandler.Get)
			r.Post("/servers/{uuid}/power", serverHandler.Power)
		})
	})

	r.Get("/ws/servers/{uuid}", func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUIDParam(r, "uuid")
		if err != nil {
			http.Error(w, "invalid server uuid", http.StatusBadRequest)
			return
		}
		deps.Hub.ServeServerSocket(w, r, id)
	})

	return r
}
