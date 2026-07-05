package auth

import (
	"context"
	"net/http"
	"strings"
)

type ctxKey string

const claimsCtxKey ctxKey = "auth_claims"

type APIKeyResolver func(ctx context.Context, rawToken string) (*Claims, error)

func Middleware(tm *TokenManager, resolveAPIKey APIKeyResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			token, ok := strings.CutPrefix(header, "Bearer ")
			if !ok || token == "" {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}

			if strings.HasPrefix(token, "panel_") {
				if resolveAPIKey == nil {
					http.Error(w, "invalid token", http.StatusUnauthorized)
					return
				}
				claims, err := resolveAPIKey(r.Context(), token)
				if err != nil {
					http.Error(w, "invalid api key", http.StatusUnauthorized)
					return
				}
				ctx := context.WithValue(r.Context(), claimsCtxKey, claims)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			claims, err := tm.Parse(token)
			if err != nil || claims.Type != TokenAccess {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), claimsCtxKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func FromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(claimsCtxKey).(*Claims)
	return claims, ok
}
