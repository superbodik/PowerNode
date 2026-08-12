package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/panel/internal/auth"
	"github.com/yourorg/panel/internal/crypto"
	"github.com/yourorg/panel/internal/twitch"
)

type TwitchHandler struct {
	DB            *pgxpool.Pool
	Client        *twitch.Client
	EncryptionKey string
}

type twitchStartResponse struct {
	AuthorizeURL string `json:"authorize_url"`
}

// Start issues a one-time state token tying this OAuth attempt to the
// logged-in PowerNode user, then hands back the URL to send the browser to.
// The frontend navigates there directly (window.location, not fetch) --
// Twitch's own login/consent screen has to render in the top-level page.
func (h *TwitchHandler) Start(w http.ResponseWriter, r *http.Request) {
	if !h.Client.Enabled() {
		http.Error(w, "twitch integration is not configured on this panel", http.StatusServiceUnavailable)
		return
	}
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	state, err := randomState()
	if err != nil {
		http.Error(w, "failed to start twitch connect", http.StatusInternalServerError)
		return
	}

	// Opportunistic cleanup so abandoned flows (closed tab mid-redirect,
	// declined consent) don't grow this table forever -- no cron needed.
	_, _ = h.DB.Exec(r.Context(), `DELETE FROM twitch_oauth_states WHERE created_at < now() - interval '15 minutes'`)

	if _, err := h.DB.Exec(r.Context(),
		`INSERT INTO twitch_oauth_states (state, user_id) VALUES ($1, $2)`, state, claims.UserID,
	); err != nil {
		http.Error(w, "failed to start twitch connect", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, twitchStartResponse{AuthorizeURL: h.Client.AuthorizeURL(state)})
}

// Callback is hit by Twitch's own redirect, not the panel's frontend --
// there's no Authorization header here, which is exactly why Start had to
// stash the user id server-side against the state token in the first place.
func (h *TwitchHandler) Callback(w http.ResponseWriter, r *http.Request) {
	fail := func() { http.Redirect(w, r, "/streamers?twitch=error", http.StatusFound) }

	if !h.Client.Enabled() {
		fail()
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		fail()
		return
	}

	var userID int64
	err := h.DB.QueryRow(r.Context(),
		`DELETE FROM twitch_oauth_states WHERE state = $1 AND created_at > now() - interval '15 minutes' RETURNING user_id`,
		state,
	).Scan(&userID)
	if err != nil {
		// Covers both "no such state" and "expired" (already deleted by the
		// interval check failing to match) identically -- neither is
		// recoverable, and there's nothing sensitive to leak either way.
		fail()
		return
	}

	tokens, err := h.Client.ExchangeCode(r.Context(), code)
	if err != nil {
		fail()
		return
	}
	twitchUser, err := h.Client.FetchUser(r.Context(), tokens.AccessToken)
	if err != nil {
		fail()
		return
	}

	accessEnc, err := crypto.Encrypt(h.EncryptionKey, tokens.AccessToken)
	if err != nil {
		fail()
		return
	}
	refreshEnc, err := crypto.Encrypt(h.EncryptionKey, tokens.RefreshToken)
	if err != nil {
		fail()
		return
	}

	_, err = h.DB.Exec(r.Context(), `
		INSERT INTO twitch_connections (user_id, twitch_user_id, twitch_login, access_token_encrypted, refresh_token_encrypted, scopes, connected_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, now(), now())
		ON CONFLICT (user_id) DO UPDATE SET
			twitch_user_id = EXCLUDED.twitch_user_id,
			twitch_login = EXCLUDED.twitch_login,
			access_token_encrypted = EXCLUDED.access_token_encrypted,
			refresh_token_encrypted = EXCLUDED.refresh_token_encrypted,
			scopes = EXCLUDED.scopes,
			updated_at = now()`,
		userID, twitchUser.ID, twitchUser.Login, accessEnc, refreshEnc, strings.Join(tokens.Scopes, " "),
	)
	if err != nil {
		fail()
		return
	}

	http.Redirect(w, r, "/streamers?twitch=connected", http.StatusFound)
}

type twitchStatusResponse struct {
	Enabled     bool   `json:"enabled"`
	Connected   bool   `json:"connected"`
	TwitchLogin string `json:"twitch_login,omitempty"`
}

func (h *TwitchHandler) Status(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	resp := twitchStatusResponse{Enabled: h.Client.Enabled()}
	var login string
	err := h.DB.QueryRow(r.Context(),
		`SELECT twitch_login FROM twitch_connections WHERE user_id = $1`, claims.UserID,
	).Scan(&login)
	if err == nil {
		resp.Connected = true
		resp.TwitchLogin = login
	} else if err != pgx.ErrNoRows {
		http.Error(w, "failed to load twitch status", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *TwitchHandler) Disconnect(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if _, err := h.DB.Exec(r.Context(), `DELETE FROM twitch_connections WHERE user_id = $1`, claims.UserID); err != nil {
		http.Error(w, "failed to disconnect twitch", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func randomState() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
