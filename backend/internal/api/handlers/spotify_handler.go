package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/panel/internal/auth"
	"github.com/yourorg/panel/internal/crypto"
	"github.com/yourorg/panel/internal/spotify"
)

// SpotifyHandler powers song-request donations: a streamer connects their
// own Spotify account (same non-custodial shape as Stripe -- we only ever
// search the public catalog and queue onto their existing playback, never
// touch their library/playlists), and a donor's requested track gets
// queued automatically when their donation completes.
type SpotifyHandler struct {
	DB            *pgxpool.Pool
	Client        *spotify.Client
	EncryptionKey string
}

type spotifyStartResponse struct {
	AuthorizeURL string `json:"authorize_url"`
}

func (h *SpotifyHandler) Start(w http.ResponseWriter, r *http.Request) {
	if !h.Client.Enabled() {
		http.Error(w, "spotify integration is not configured on this panel", http.StatusServiceUnavailable)
		return
	}
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	state, err := randomToken(24)
	if err != nil {
		http.Error(w, "failed to start spotify connect", http.StatusInternalServerError)
		return
	}

	_, _ = h.DB.Exec(r.Context(), `DELETE FROM spotify_oauth_states WHERE created_at < now() - interval '15 minutes'`)

	if _, err := h.DB.Exec(r.Context(),
		`INSERT INTO spotify_oauth_states (state, user_id) VALUES ($1, $2)`, state, claims.UserID,
	); err != nil {
		http.Error(w, "failed to start spotify connect", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, spotifyStartResponse{AuthorizeURL: h.Client.AuthorizeURL(state)})
}

func (h *SpotifyHandler) Callback(w http.ResponseWriter, r *http.Request) {
	// fail always redirects the same way to the user (nothing sensitive to
	// differentiate for them), but logs exactly which step broke -- this
	// path previously failed completely silently, which made a real
	// connect failure impossible to diagnose from the panel's own logs.
	fail := func(step string, err error) {
		log.Printf("spotify callback failed at %s: %v", step, err)
		http.Redirect(w, r, "/streamers?spotify=error", http.StatusFound)
	}

	if !h.Client.Enabled() {
		fail("enabled-check", errors.New("spotify client not configured"))
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		fail("query-params", errors.New("missing code or state"))
		return
	}

	var userID int64
	err := h.DB.QueryRow(r.Context(),
		`DELETE FROM spotify_oauth_states WHERE state = $1 AND created_at > now() - interval '15 minutes' RETURNING user_id`,
		state,
	).Scan(&userID)
	if err != nil {
		fail("state-lookup", err)
		return
	}

	tokens, err := h.Client.ExchangeCode(r.Context(), code)
	if err != nil {
		fail("exchange-code", err)
		return
	}
	spotifyUser, err := h.Client.FetchUser(r.Context(), tokens.AccessToken)
	if err != nil {
		fail("fetch-user", err)
		return
	}

	accessEnc, err := crypto.Encrypt(h.EncryptionKey, tokens.AccessToken)
	if err != nil {
		fail("encrypt-access-token", err)
		return
	}
	refreshEnc, err := crypto.Encrypt(h.EncryptionKey, tokens.RefreshToken)
	if err != nil {
		fail("encrypt-refresh-token", err)
		return
	}

	_, err = h.DB.Exec(r.Context(), `
		INSERT INTO spotify_connections (user_id, spotify_user_id, display_name, access_token_encrypted, refresh_token_encrypted, connected_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, now(), now())
		ON CONFLICT (user_id) DO UPDATE SET
			spotify_user_id = EXCLUDED.spotify_user_id,
			display_name = EXCLUDED.display_name,
			access_token_encrypted = EXCLUDED.access_token_encrypted,
			refresh_token_encrypted = EXCLUDED.refresh_token_encrypted,
			updated_at = now()`,
		userID, spotifyUser.ID, spotifyUser.DisplayName, accessEnc, refreshEnc,
	)
	if err != nil {
		fail("save-connection", err)
		return
	}

	http.Redirect(w, r, "/streamers?spotify=connected", http.StatusFound)
}

type spotifyStatusResponse struct {
	Enabled     bool   `json:"enabled"`
	Connected   bool   `json:"connected"`
	DisplayName string `json:"display_name,omitempty"`
}

func (h *SpotifyHandler) Status(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	resp := spotifyStatusResponse{Enabled: h.Client.Enabled()}
	var displayName string
	err := h.DB.QueryRow(r.Context(),
		`SELECT display_name FROM spotify_connections WHERE user_id = $1`, claims.UserID,
	).Scan(&displayName)
	if err == nil {
		resp.Connected = true
		resp.DisplayName = displayName
	} else if err != pgx.ErrNoRows {
		http.Error(w, "failed to load spotify connection", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *SpotifyHandler) Disconnect(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if _, err := h.DB.Exec(r.Context(), `DELETE FROM spotify_connections WHERE user_id = $1`, claims.UserID); err != nil {
		http.Error(w, "failed to disconnect", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// validAccessToken returns a usable access token for userID, transparently
// refreshing and persisting it first if the stored one has expired. Shared
// by anything that needs to call the Spotify API on a user's behalf outside
// a request they initiated themselves (here: the donation webhook).
func (h *SpotifyHandler) validAccessToken(ctx context.Context, userID int64) (string, error) {
	var accessEnc, refreshEnc string
	if err := h.DB.QueryRow(ctx,
		`SELECT access_token_encrypted, refresh_token_encrypted FROM spotify_connections WHERE user_id = $1`, userID,
	).Scan(&accessEnc, &refreshEnc); err != nil {
		return "", err
	}
	accessToken, err := crypto.Decrypt(h.EncryptionKey, accessEnc)
	if err != nil {
		return "", err
	}
	refreshToken, err := crypto.Decrypt(h.EncryptionKey, refreshEnc)
	if err != nil {
		return "", err
	}

	// Cheapest possible validity check: try the caller's actual request
	// first and only refresh reactively on a 401, rather than tracking a
	// separate expires_at column that could drift from reality. Probe with
	// FetchUser since it's the lightest authenticated call available.
	if _, err := h.Client.FetchUser(ctx, accessToken); err == nil {
		return accessToken, nil
	} else if !errors.Is(err, spotify.ErrTokenExpired) {
		return "", err
	}

	tokens, err := h.Client.RefreshUserToken(ctx, refreshToken)
	if err != nil {
		return "", err
	}
	newAccessEnc, err := crypto.Encrypt(h.EncryptionKey, tokens.AccessToken)
	if err != nil {
		return "", err
	}
	// Spotify doesn't always rotate the refresh token -- keep the existing
	// one encrypted rather than re-encrypting an empty string over it.
	newRefreshToken := tokens.RefreshToken
	if newRefreshToken == "" {
		newRefreshToken = refreshToken
	}
	newRefreshEnc, err := crypto.Encrypt(h.EncryptionKey, newRefreshToken)
	if err != nil {
		return "", err
	}
	if _, err := h.DB.Exec(ctx,
		`UPDATE spotify_connections SET access_token_encrypted = $1, refresh_token_encrypted = $2, updated_at = now() WHERE user_id = $3`,
		newAccessEnc, newRefreshEnc, userID,
	); err != nil {
		return "", err
	}
	return tokens.AccessToken, nil
}

// QueueForDonation searches for and queues a donor's requested track onto
// the recipient's active Spotify playback. Every failure mode here (not
// connected, search miss, no active device, expired session) is routine
// and silent by design -- called from the donation webhook, where the
// donation itself has already succeeded regardless of whether this part
// works, and there's no user waiting on a response to show an error to.
func (h *SpotifyHandler) QueueForDonation(ctx context.Context, recipientUserID int64, query string) (queued bool, track *spotify.Track) {
	if !h.Client.Enabled() || query == "" {
		return false, nil
	}
	accessToken, err := h.validAccessToken(ctx, recipientUserID)
	if err != nil {
		return false, nil
	}
	t, err := h.Client.SearchTrack(ctx, accessToken, query)
	if err != nil {
		return false, nil
	}
	if err := h.Client.QueueTrack(ctx, accessToken, t.URI); err != nil {
		return false, t
	}
	return true, t
}
