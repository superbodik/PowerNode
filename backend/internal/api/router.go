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
	DB            *pgxpool.Pool
	Token         *auth.TokenManager
	Hub           *ws.Hub
	NodeClient    func(nodeID int64) (*daemonclient.Client, error)
	EncryptionKey string
	Commit        string
	BuildDate     string
	SourceDir     string
	RepoSlug      string
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
	nodeHandler := &handlers.NodeHandler{DB: deps.DB, EncryptionKey: deps.EncryptionKey}
	serverHandler := &handlers.ServerHandler{DB: deps.DB, NodeClient: deps.NodeClient}
	versionHandler := &handlers.VersionHandler{
		Commit:    deps.Commit,
		BuildDate: deps.BuildDate,
		SourceDir: deps.SourceDir,
		RepoSlug:  deps.RepoSlug,
	}

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

			r.Get("/version", versionHandler.Get)
			r.Get("/version/check", versionHandler.CheckUpdate)
		})
	})

	r.Get("/ws/servers/{uuid}", func(w http.ResponseWriter, r *http.Request) {
		if !authenticateWS(r, deps.Token) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		id, err := parseUUIDParam(r, "uuid")
		if err != nil {
			http.Error(w, "invalid server uuid", http.StatusBadRequest)
			return
		}
		deps.Hub.ServeServerSocket(w, r, id)
	})

	r.Get("/ws/servers/{uuid}/console", func(w http.ResponseWriter, r *http.Request) {
		if !authenticateWS(r, deps.Token) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		id, err := parseUUIDParam(r, "uuid")
		if err != nil {
			http.Error(w, "invalid server uuid", http.StatusBadRequest)
			return
		}
		deps.Hub.ServeConsoleSocket(w, r, id)
	})

	return r
}

func authenticateWS(r *http.Request, tm *auth.TokenManager) bool {
	token := r.URL.Query().Get("token")
	if token == "" {
		return false
	}
	_, err := tm.Parse(token)
	return err == nil
}
