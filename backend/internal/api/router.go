package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

	authHandler := &handlers.AuthHandler{DB: deps.DB, Token: deps.Token, EncryptionKey: deps.EncryptionKey}
	nodeHandler := &handlers.NodeHandler{DB: deps.DB, EncryptionKey: deps.EncryptionKey, NodeClient: deps.NodeClient}
	serverHandler := &handlers.ServerHandler{DB: deps.DB, NodeClient: deps.NodeClient}
	versionHandler := &handlers.VersionHandler{
		Commit:    deps.Commit,
		BuildDate: deps.BuildDate,
		SourceDir: deps.SourceDir,
		RepoSlug:  deps.RepoSlug,
	}
	activityHandler := &handlers.ActivityHandler{DB: deps.DB}
	eggHandler := &handlers.EggHandler{DB: deps.DB}
	allocationHandler := &handlers.AllocationHandler{DB: deps.DB}
	apiKeyHandler := &handlers.APIKeyHandler{DB: deps.DB}
	fileHandler := &handlers.FileHandler{DB: deps.DB, NodeClient: deps.NodeClient}
	scheduleHandler := &handlers.ScheduleHandler{DB: deps.DB}
	twofaHandler := &handlers.TwoFAHandler{DB: deps.DB, EncryptionKey: deps.EncryptionKey}

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/login", authHandler.Login)
		r.Post("/auth/refresh", authHandler.Refresh)

		r.Group(func(r chi.Router) {
			r.Use(auth.Middleware(deps.Token, resolveAPIKey(deps.DB)))

			r.Get("/auth/me", authHandler.Me)

			r.Get("/nodes", nodeHandler.List)
			r.With(auth.RequireAdmin).Post("/nodes", nodeHandler.Create)
			r.Get("/nodes/{id}/status", nodeHandler.Status)

			r.Get("/servers", serverHandler.List)
			r.Post("/servers", serverHandler.Create)
			r.Get("/servers/{uuid}", serverHandler.Get)
			r.Post("/servers/{uuid}/power", serverHandler.Power)
			r.Delete("/servers/{uuid}", serverHandler.Delete)

			r.Get("/servers/{uuid}/files", fileHandler.List)
			r.Get("/servers/{uuid}/files/contents", fileHandler.Read)
			r.Put("/servers/{uuid}/files/contents", fileHandler.Write)
			r.Delete("/servers/{uuid}/files", fileHandler.Delete)
			r.Post("/servers/{uuid}/files/directory", fileHandler.CreateDirectory)
			r.Post("/servers/{uuid}/files/rename", fileHandler.Rename)

			r.Get("/servers/{uuid}/schedules", scheduleHandler.List)
			r.Post("/servers/{uuid}/schedules", scheduleHandler.Create)
			r.Post("/servers/{uuid}/schedules/{id}/toggle", scheduleHandler.Toggle)
			r.Delete("/servers/{uuid}/schedules/{id}", scheduleHandler.Delete)

			r.Get("/eggs", eggHandler.List)

			r.Get("/allocations", allocationHandler.List)
			r.With(auth.RequireAdmin).Post("/allocations", allocationHandler.Create)

			r.Get("/version", versionHandler.Get)
			r.Get("/version/check", versionHandler.CheckUpdate)

			r.Get("/activity", activityHandler.List)

			r.Get("/account/api-keys", apiKeyHandler.List)
			r.Post("/account/api-keys", apiKeyHandler.Create)
			r.Delete("/account/api-keys/{id}", apiKeyHandler.Delete)

			r.Get("/account/2fa/status", twofaHandler.Status)
			r.Post("/account/2fa/setup", twofaHandler.Setup)
			r.Post("/account/2fa/verify", twofaHandler.Verify)
			r.Post("/account/2fa/disable", twofaHandler.Disable)
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
	claims, err := tm.Parse(token)
	return err == nil && claims.Type == auth.TokenAccess
}

func resolveAPIKey(pool *pgxpool.Pool) auth.APIKeyResolver {
	return func(ctx context.Context, rawToken string) (*auth.Claims, error) {
		sum := sha256.Sum256([]byte(rawToken))
		tokenHash := hex.EncodeToString(sum[:])

		var userID int64
		var email string
		var isAdmin, isActive bool
		err := pool.QueryRow(ctx, `
			SELECT u.id, u.email, u.is_admin, u.is_active
			FROM api_keys k JOIN users u ON u.id = k.user_id
			WHERE k.token_hash = $1`, tokenHash,
		).Scan(&userID, &email, &isAdmin, &isActive)
		if err != nil || !isActive {
			return nil, auth.ErrInvalidToken
		}

		_, _ = pool.Exec(ctx, `UPDATE api_keys SET last_used_at = now() WHERE token_hash = $1`, tokenHash)

		return &auth.Claims{UserID: userID, Email: email, IsAdmin: isAdmin, Type: auth.TokenAccess}, nil
	}
}
