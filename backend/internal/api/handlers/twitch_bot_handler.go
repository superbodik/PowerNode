package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/panel/internal/auth"
	"github.com/yourorg/panel/internal/crypto"
	"github.com/yourorg/panel/internal/twitch"
)

const twitchBotEventType = "channel.chat.message"

type TwitchBotHandler struct {
	DB             *pgxpool.Pool
	Client         *twitch.Client
	EncryptionKey  string
	EventSubSecret string
	PublicURL      string
}

type twitchBotStatusResponse struct {
	Connected           bool   `json:"connected"`
	BotLogin            string `json:"bot_login,omitempty"`
	NeedsMainConnection bool   `json:"needs_main_connection"`
}

func (h *TwitchBotHandler) Status(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var login string
	err := h.DB.QueryRow(r.Context(),
		`SELECT bot_twitch_login FROM twitch_bot_connections WHERE user_id = $1`, claims.UserID,
	).Scan(&login)
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusOK, twitchBotStatusResponse{Connected: false})
		return
	} else if err != nil {
		http.Error(w, "failed to load bot status", http.StatusInternalServerError)
		return
	}

	var hasMain bool
	_ = h.DB.QueryRow(r.Context(), `SELECT true FROM twitch_connections WHERE user_id = $1`, claims.UserID).Scan(&hasMain)

	writeJSON(w, http.StatusOK, twitchBotStatusResponse{Connected: true, BotLogin: login, NeedsMainConnection: !hasMain})
}

func (h *TwitchBotHandler) Disconnect(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ctx := r.Context()

	var subID string
	if err := h.DB.QueryRow(ctx,
		`SELECT twitch_subscription_id FROM twitch_eventsub_subscriptions WHERE user_id = $1 AND event_type = $2`,
		claims.UserID, twitchBotEventType,
	).Scan(&subID); err == nil {
		if appToken, aerr := h.Client.AppAccessToken(ctx); aerr == nil {
			_ = h.Client.DeleteEventSubSubscription(ctx, appToken, subID)
		}
	}

	_, _ = h.DB.Exec(ctx, `DELETE FROM twitch_eventsub_subscriptions WHERE user_id = $1 AND event_type = $2`, claims.UserID, twitchBotEventType)
	_, _ = h.DB.Exec(ctx, `DELETE FROM twitch_bot_connections WHERE user_id = $1`, claims.UserID)

	w.WriteHeader(http.StatusNoContent)
}

func (h *TwitchBotHandler) EnsureSubscription(ctx context.Context, userID int64) {
	var broadcasterTwitchID string
	if err := h.DB.QueryRow(ctx, `SELECT twitch_user_id FROM twitch_connections WHERE user_id = $1`, userID).Scan(&broadcasterTwitchID); err != nil {
		return
	}
	var botTwitchID string
	if err := h.DB.QueryRow(ctx, `SELECT bot_twitch_user_id FROM twitch_bot_connections WHERE user_id = $1`, userID).Scan(&botTwitchID); err != nil {
		return
	}

	var already bool
	if err := h.DB.QueryRow(ctx,
		`SELECT true FROM twitch_eventsub_subscriptions WHERE user_id = $1 AND event_type = $2 AND status = 'enabled'`,
		userID, twitchBotEventType,
	).Scan(&already); err == nil && already {
		return
	}

	appToken, err := h.Client.AppAccessToken(ctx)
	if err != nil {
		return
	}
	subID, err := h.Client.CreateEventSubSubscription(ctx, appToken,
		twitchBotEventType, "1",
		map[string]string{"broadcaster_user_id": broadcasterTwitchID, "user_id": botTwitchID},
		h.PublicURL+"/api/v1/twitch/eventsub", h.EventSubSecret,
	)
	if err != nil {
		return
	}

	_, _ = h.DB.Exec(ctx, `
		INSERT INTO twitch_eventsub_subscriptions (user_id, event_type, twitch_subscription_id, status)
		VALUES ($1, $2, $3, 'enabled')
		ON CONFLICT (user_id, event_type) DO UPDATE SET twitch_subscription_id = EXCLUDED.twitch_subscription_id, status = 'enabled'`,
		userID, twitchBotEventType, subID,
	)
}

type botCommand struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Response string `json:"response"`
}

func (h *TwitchBotHandler) ListCommands(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	rows, err := h.DB.Query(r.Context(), `SELECT id, name, response FROM twitch_bot_commands WHERE user_id = $1 ORDER BY name`, claims.UserID)
	if err != nil {
		http.Error(w, "failed to load commands", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	commands := []botCommand{}
	for rows.Next() {
		var c botCommand
		if rows.Scan(&c.ID, &c.Name, &c.Response) == nil {
			commands = append(commands, c)
		}
	}
	writeJSON(w, http.StatusOK, commands)
}

type saveCommandRequest struct {
	Name     string `json:"name"`
	Response string `json:"response"`
}

func (h *TwitchBotHandler) SaveCommand(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req saveCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	name := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(req.Name), "!")))
	if name == "" || strings.ContainsAny(name, " \t\n") {
		http.Error(w, "command name must be a single word", http.StatusBadRequest)
		return
	}
	response := strings.TrimSpace(req.Response)
	if response == "" {
		http.Error(w, "response text is required", http.StatusBadRequest)
		return
	}
	if len(response) > 450 {
		http.Error(w, "response must be 450 characters or fewer", http.StatusBadRequest)
		return
	}

	var cmd botCommand
	err := h.DB.QueryRow(r.Context(), `
		INSERT INTO twitch_bot_commands (user_id, name, response) VALUES ($1, $2, $3)
		ON CONFLICT (user_id, name) DO UPDATE SET response = EXCLUDED.response, updated_at = now()
		RETURNING id, name, response`,
		claims.UserID, name, response,
	).Scan(&cmd.ID, &cmd.Name, &cmd.Response)
	if err != nil {
		http.Error(w, "failed to save command", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, cmd)
}

func (h *TwitchBotHandler) DeleteCommand(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid command id", http.StatusBadRequest)
		return
	}
	tag, err := h.DB.Exec(r.Context(), `DELETE FROM twitch_bot_commands WHERE id = $1 AND user_id = $2`, id, claims.UserID)
	if err != nil {
		http.Error(w, "failed to delete command", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "command not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *TwitchBotHandler) HandleChatMessage(ctx context.Context, userID int64, chatterUserID, text string) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "!") {
		return
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return
	}
	name := strings.ToLower(strings.TrimPrefix(fields[0], "!"))
	if name == "" {
		return
	}

	var response string
	if err := h.DB.QueryRow(ctx, `SELECT response FROM twitch_bot_commands WHERE user_id = $1 AND name = $2`, userID, name).Scan(&response); err != nil {
		return
	}

	var broadcasterTwitchID, botTwitchID, accessEnc, refreshEnc string
	err := h.DB.QueryRow(ctx, `
		SELECT tc.twitch_user_id, bc.bot_twitch_user_id, bc.access_token_encrypted, bc.refresh_token_encrypted
		FROM twitch_bot_connections bc JOIN twitch_connections tc ON tc.user_id = bc.user_id
		WHERE bc.user_id = $1`, userID,
	).Scan(&broadcasterTwitchID, &botTwitchID, &accessEnc, &refreshEnc)
	if err != nil {
		return
	}
	if chatterUserID == botTwitchID {
		return
	}

	accessToken, err := crypto.Decrypt(h.EncryptionKey, accessEnc)
	if err != nil {
		return
	}

	err = h.Client.SendChatMessage(ctx, accessToken, broadcasterTwitchID, botTwitchID, response)
	if errors.Is(err, twitch.ErrTokenExpired) {
		refreshToken, derr := crypto.Decrypt(h.EncryptionKey, refreshEnc)
		if derr != nil {
			return
		}
		tokens, rerr := h.Client.RefreshUserToken(ctx, refreshToken)
		if rerr != nil {
			return
		}
		newAccessEnc, aerr := crypto.Encrypt(h.EncryptionKey, tokens.AccessToken)
		newRefreshEnc, rerr2 := crypto.Encrypt(h.EncryptionKey, tokens.RefreshToken)
		if aerr == nil && rerr2 == nil {
			_, _ = h.DB.Exec(ctx,
				`UPDATE twitch_bot_connections SET access_token_encrypted = $1, refresh_token_encrypted = $2, updated_at = now() WHERE user_id = $3`,
				newAccessEnc, newRefreshEnc, userID)
		}
		_ = h.Client.SendChatMessage(ctx, tokens.AccessToken, broadcasterTwitchID, botTwitchID, response)
	}
}
