package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/panel/internal/activity"
	"github.com/yourorg/panel/internal/auth"
	"github.com/yourorg/panel/internal/crypto"
)

type AuthHandler struct {
	DB            *pgxpool.Pool
	Token         *auth.TokenManager
	EncryptionKey string
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	TOTPCode string `json:"totp_code"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	User         struct {
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
		id            int64
		email         string
		username      string
		passwordHash  string
		isAdmin       bool
		isActive      bool
		totpEnabled   bool
		totpSecretEnc *string
	)
	err := h.DB.QueryRow(r.Context(),
		`SELECT id, email, username, password_hash, is_admin, is_active, totp_enabled, totp_secret
		 FROM users WHERE email = $1`, req.Email,
	).Scan(&id, &email, &username, &passwordHash, &isAdmin, &isActive, &totpEnabled, &totpSecretEnc)

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

	if totpEnabled {
		if req.TOTPCode == "" {
			http.Error(w, "totp_required", http.StatusPreconditionRequired)
			return
		}
		secret, err := crypto.Decrypt(h.EncryptionKey, *totpSecretEnc)
		if err != nil || !auth.ValidateTOTPCode(secret, req.TOTPCode) {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
	}

	accessToken, err := h.Token.Issue(id, email, isAdmin)
	if err != nil {
		http.Error(w, "failed to issue token", http.StatusInternalServerError)
		return
	}
	refreshToken, err := h.Token.IssueRefresh(id, email, isAdmin)
	if err != nil {
		http.Error(w, "failed to issue token", http.StatusInternalServerError)
		return
	}

	activity.Record(r.Context(), h.DB, activity.Entry{
		ActorUserID: &id,
		Event:       "user.login",
		IPAddress:   activity.RequestIP(r),
	})

	resp := tokenResponse{AccessToken: accessToken, RefreshToken: refreshToken}
	resp.User.ID = id
	resp.User.Email = email
	resp.User.Username = username

	writeJSON(w, http.StatusOK, resp)
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	claims, err := h.Token.ParseRefresh(req.RefreshToken)
	if err != nil {
		http.Error(w, "invalid or expired refresh token", http.StatusUnauthorized)
		return
	}

	var (
		email    string
		username string
		isAdmin  bool
		isActive bool
	)
	err = h.DB.QueryRow(r.Context(),
		`SELECT email, username, is_admin, is_active FROM users WHERE id = $1`, claims.UserID,
	).Scan(&email, &username, &isAdmin, &isActive)
	if err != nil || !isActive {
		http.Error(w, "account no longer active", http.StatusUnauthorized)
		return
	}

	accessToken, err := h.Token.Issue(claims.UserID, email, isAdmin)
	if err != nil {
		http.Error(w, "failed to issue token", http.StatusInternalServerError)
		return
	}
	refreshToken, err := h.Token.IssueRefresh(claims.UserID, email, isAdmin)
	if err != nil {
		http.Error(w, "failed to issue token", http.StatusInternalServerError)
		return
	}

	resp := tokenResponse{AccessToken: accessToken, RefreshToken: refreshToken}
	resp.User.ID = claims.UserID
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
