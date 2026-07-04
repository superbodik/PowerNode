package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/panel/internal/auth"
)

type AuthHandler struct {
	DB    *pgxpool.Pool
	Token *auth.TokenManager
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	AccessToken string `json:"access_token"`
	User        struct {
		ID       int64  `json:"id"`
		Email    string `json:"email"`
		Username string `json:"username"`
	} `json:"user"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var (
		id           int64
		email        string
		username     string
		passwordHash string
		isAdmin      bool
		isActive     bool
	)
	err := h.DB.QueryRow(r.Context(),
		`SELECT id, email, username, password_hash, is_admin, is_active
		 FROM users WHERE email = $1`, req.Email,
	).Scan(&id, &email, &username, &passwordHash, &isAdmin, &isActive)

	if err == pgx.ErrNoRows {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	} else if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if !isActive || !auth.VerifyPassword(passwordHash, req.Password) {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	token, err := h.Token.Issue(id, email, isAdmin)
	if err != nil {
		http.Error(w, "failed to issue token", http.StatusInternalServerError)
		return
	}

	resp := loginResponse{AccessToken: token}
	resp.User.ID = id
	resp.User.Email = email
	resp.User.Username = username

	writeJSON(w, http.StatusOK, resp)
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":       claims.UserID,
		"email":    claims.Email,
		"is_admin": claims.IsAdmin,
	})
}
