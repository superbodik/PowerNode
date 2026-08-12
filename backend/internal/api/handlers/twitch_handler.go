package handlers

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/panel/internal/auth"
	"github.com/yourorg/panel/internal/crypto"
	"github.com/yourorg/panel/internal/twitch"
	"github.com/yourorg/panel/internal/ws"
)

// subscriptionEventTypes are the EventSub types registered when a user
// enables the subscription-alert widget. channel.subscribe covers direct
// subs (is_gift is ignored there to avoid double-alerting a gifted sub --
// channel.subscription.gift carries the richer "gifted N subs" payload).
var subscriptionEventTypes = []string{"channel.subscribe", "channel.subscription.gift"}

type TwitchHandler struct {
	DB             *pgxpool.Pool
	Client         *twitch.Client
	Hub            *ws.Hub
	EncryptionKey  string
	EventSubSecret string
	PublicURL      string
}

type twitchStartResponse struct {
	AuthorizeURL string `json:"authorize_url"`
}

// Start issues a one-time state token tying this OAuth attempt to the
// logged-in PowerNode user, then hands back the URL to send the browser to.
// The frontend navigates there directly (window.location, not fetch) --
// Twitch's own login/consent screen has to render in the top-level page.
func (h *TwitchHandler) Start(w http.ResponseWriter, r *http.Request) {
	h.start(w, r, h.Client.AuthorizeURL)
}

// StartExtended is the same flow, requesting the broader
// channel:read:subscriptions scope needed for the subscription-alert
// widget. Kept as an explicit separate button/endpoint rather than folded
// into the base connect flow -- see twitch.Scope's doc comment.
func (h *TwitchHandler) StartExtended(w http.ResponseWriter, r *http.Request) {
	h.start(w, r, h.Client.AuthorizeExtendedURL)
}

func (h *TwitchHandler) start(w http.ResponseWriter, r *http.Request, buildURL func(state string) string) {
	if !h.Client.Enabled() {
		http.Error(w, "twitch integration is not configured on this panel", http.StatusServiceUnavailable)
		return
	}
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	state, err := randomToken(24)
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

	writeJSON(w, http.StatusOK, twitchStartResponse{AuthorizeURL: buildURL(state)})
}

// Callback is hit by Twitch's own redirect, not the panel's frontend --
// there's no Authorization header here, which is exactly why Start had to
// stash the user id server-side against the state token in the first place.
// It only saves the connection; enabling the subscription widget is a
// separate, explicit step (EnableSubscriptions) so a scope upgrade doesn't
// silently register webhooks as a side effect of a redirect.
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
	Enabled            bool   `json:"enabled"`
	Connected          bool   `json:"connected"`
	TwitchLogin        string `json:"twitch_login,omitempty"`
	HasSubscriptions   bool   `json:"has_subscriptions_scope"`
	SubscriptionWidget string `json:"subscription_widget_url,omitempty"`
}

func (h *TwitchHandler) Status(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	resp := twitchStatusResponse{Enabled: h.Client.Enabled()}
	var login, scopes string
	err := h.DB.QueryRow(r.Context(),
		`SELECT twitch_login, scopes FROM twitch_connections WHERE user_id = $1`, claims.UserID,
	).Scan(&login, &scopes)
	if err == nil {
		resp.Connected = true
		resp.TwitchLogin = login
		resp.HasSubscriptions = hasScope(scopes, "channel:read:subscriptions")
	} else if err != pgx.ErrNoRows {
		http.Error(w, "failed to load twitch status", http.StatusInternalServerError)
		return
	}

	var widgetToken string
	if err := h.DB.QueryRow(r.Context(),
		`SELECT token FROM twitch_widget_tokens WHERE user_id = $1`, claims.UserID,
	).Scan(&widgetToken); err == nil {
		resp.SubscriptionWidget = h.widgetURL(widgetToken)
	}

	writeJSON(w, http.StatusOK, resp)
}

// EnableSubscriptions registers the EventSub subscriptions (idempotently --
// safe to call again) and mints a widget token if one doesn't exist yet.
// Requires the connection to already carry channel:read:subscriptions,
// obtained via StartExtended/Callback first.
func (h *TwitchHandler) EnableSubscriptions(w http.ResponseWriter, r *http.Request) {
	if !h.Client.Enabled() {
		http.Error(w, "twitch integration is not configured on this panel", http.StatusServiceUnavailable)
		return
	}
	if h.PublicURL == "" {
		http.Error(w, "PANEL_PUBLIC_URL is not set -- required so Twitch can reach the webhook callback", http.StatusServiceUnavailable)
		return
	}
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var twitchUserID, scopes string
	err := h.DB.QueryRow(r.Context(),
		`SELECT twitch_user_id, scopes FROM twitch_connections WHERE user_id = $1`, claims.UserID,
	).Scan(&twitchUserID, &scopes)
	if err == pgx.ErrNoRows {
		http.Error(w, "connect a twitch account first", http.StatusBadRequest)
		return
	} else if err != nil {
		http.Error(w, "failed to load twitch connection", http.StatusInternalServerError)
		return
	}
	if !hasScope(scopes, "channel:read:subscriptions") {
		http.Error(w, "reconnect twitch with subscription access first", http.StatusBadRequest)
		return
	}

	appToken, err := h.Client.AppAccessToken(r.Context())
	if err != nil {
		http.Error(w, "failed to authenticate with twitch", http.StatusBadGateway)
		return
	}

	callbackURL := h.PublicURL + "/api/v1/twitch/eventsub"
	for _, eventType := range subscriptionEventTypes {
		var exists bool
		_ = h.DB.QueryRow(r.Context(),
			`SELECT true FROM twitch_eventsub_subscriptions WHERE user_id = $1 AND event_type = $2 AND status = 'enabled'`,
			claims.UserID, eventType,
		).Scan(&exists)
		if exists {
			continue
		}

		subID, err := h.Client.CreateEventSubSubscription(r.Context(), appToken, eventType, twitchUserID, callbackURL, h.EventSubSecret)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to register %s with twitch: %v", eventType, err), http.StatusBadGateway)
			return
		}
		if _, err := h.DB.Exec(r.Context(), `
			INSERT INTO twitch_eventsub_subscriptions (user_id, event_type, twitch_subscription_id, status)
			VALUES ($1, $2, $3, 'enabled')
			ON CONFLICT (user_id, event_type) DO UPDATE SET
				twitch_subscription_id = EXCLUDED.twitch_subscription_id,
				status = 'enabled'`,
			claims.UserID, eventType, subID,
		); err != nil {
			http.Error(w, "failed to save eventsub subscription", http.StatusInternalServerError)
			return
		}
	}

	var widgetToken string
	err = h.DB.QueryRow(r.Context(), `SELECT token FROM twitch_widget_tokens WHERE user_id = $1`, claims.UserID).Scan(&widgetToken)
	if err == pgx.ErrNoRows {
		widgetToken, err = randomToken(24)
		if err != nil {
			http.Error(w, "failed to create widget token", http.StatusInternalServerError)
			return
		}
		if _, err := h.DB.Exec(r.Context(),
			`INSERT INTO twitch_widget_tokens (user_id, token) VALUES ($1, $2)`, claims.UserID, widgetToken,
		); err != nil {
			http.Error(w, "failed to save widget token", http.StatusInternalServerError)
			return
		}
	} else if err != nil {
		http.Error(w, "failed to load widget token", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"subscription_widget_url": h.widgetURL(widgetToken)})
}

type testAlertRequest struct {
	Kind string `json:"kind"`
}

// SendTestAlert pushes a fake event through the exact same WS broadcast
// path a real EventSub notification would use, so the OBS Browser Source
// can be checked without waiting for an actual subscription. Never touches
// Twitch's API -- it's purely internal, gated only on the widget already
// existing.
func (h *TwitchHandler) SendTestAlert(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req testAlertRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	var widgetToken string
	err := h.DB.QueryRow(r.Context(),
		`SELECT token FROM twitch_widget_tokens WHERE user_id = $1`, claims.UserID,
	).Scan(&widgetToken)
	if err == pgx.ErrNoRows {
		http.Error(w, "create the subscription widget first", http.StatusBadRequest)
		return
	} else if err != nil {
		http.Error(w, "failed to load widget token", http.StatusInternalServerError)
		return
	}

	if req.Kind == "gift" {
		h.Hub.BroadcastWidget(widgetToken, map[string]any{
			"type":      "gift",
			"user_name": "TestSubscriber",
			"tier":      "1000",
			"count":     5,
		})
	} else {
		h.Hub.BroadcastWidget(widgetToken, map[string]any{
			"type":      "sub",
			"user_name": "TestSubscriber",
			"tier":      "1000",
		})
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetStreamKey fetches the connected broadcaster's own RTMP stream key, so
// the create-server form can fill in TWITCH_KEY without the user copying it
// from Twitch's dashboard by hand. Requires channel:read:stream_key,
// obtained via StartExtended/Callback -- the same upgrade as the
// subscription-alert widget.
func (h *TwitchHandler) GetStreamKey(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var twitchUserID, scopes, accessEnc, refreshEnc string
	err := h.DB.QueryRow(r.Context(),
		`SELECT twitch_user_id, scopes, access_token_encrypted, refresh_token_encrypted FROM twitch_connections WHERE user_id = $1`,
		claims.UserID,
	).Scan(&twitchUserID, &scopes, &accessEnc, &refreshEnc)
	if err == pgx.ErrNoRows {
		http.Error(w, "connect a twitch account first", http.StatusBadRequest)
		return
	} else if err != nil {
		http.Error(w, "failed to load twitch connection", http.StatusInternalServerError)
		return
	}
	if !hasScope(scopes, "channel:read:stream_key") {
		http.Error(w, "reconnect twitch with stream key access first", http.StatusBadRequest)
		return
	}

	accessToken, err := crypto.Decrypt(h.EncryptionKey, accessEnc)
	if err != nil {
		http.Error(w, "failed to decrypt stored token", http.StatusInternalServerError)
		return
	}

	key, err := h.Client.GetStreamKey(r.Context(), accessToken, twitchUserID)
	if errors.Is(err, twitch.ErrTokenExpired) {
		refreshToken, derr := crypto.Decrypt(h.EncryptionKey, refreshEnc)
		if derr != nil {
			http.Error(w, "failed to decrypt stored token", http.StatusInternalServerError)
			return
		}
		tokens, rerr := h.Client.RefreshUserToken(r.Context(), refreshToken)
		if rerr != nil {
			http.Error(w, "twitch session expired -- reconnect your account", http.StatusBadRequest)
			return
		}
		newAccessEnc, aerr := crypto.Encrypt(h.EncryptionKey, tokens.AccessToken)
		newRefreshEnc, rerr2 := crypto.Encrypt(h.EncryptionKey, tokens.RefreshToken)
		if aerr != nil || rerr2 != nil {
			http.Error(w, "failed to store refreshed token", http.StatusInternalServerError)
			return
		}
		if _, err := h.DB.Exec(r.Context(),
			`UPDATE twitch_connections SET access_token_encrypted = $1, refresh_token_encrypted = $2, updated_at = now() WHERE user_id = $3`,
			newAccessEnc, newRefreshEnc, claims.UserID,
		); err != nil {
			http.Error(w, "failed to store refreshed token", http.StatusInternalServerError)
			return
		}
		key, err = h.Client.GetStreamKey(r.Context(), tokens.AccessToken, twitchUserID)
	}
	if err != nil {
		http.Error(w, "failed to fetch stream key from twitch", http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"stream_key": key})
}

func (h *TwitchHandler) Disconnect(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Best-effort: also tell Twitch to stop sending events. Not fatal if
	// this fails (Twitch also revokes on its own once the grant is gone) --
	// the panel-side rows are the source of truth for whether we still act
	// on notifications from these subscription IDs.
	rows, _ := h.DB.Query(r.Context(),
		`SELECT twitch_subscription_id FROM twitch_eventsub_subscriptions WHERE user_id = $1`, claims.UserID)
	var subIDs []string
	if rows != nil {
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				subIDs = append(subIDs, id)
			}
		}
		rows.Close()
	}
	if len(subIDs) > 0 {
		if appToken, err := h.Client.AppAccessToken(r.Context()); err == nil {
			for _, id := range subIDs {
				_ = h.Client.DeleteEventSubSubscription(r.Context(), appToken, id)
			}
		}
	}

	_, _ = h.DB.Exec(r.Context(), `DELETE FROM twitch_eventsub_subscriptions WHERE user_id = $1`, claims.UserID)
	_, _ = h.DB.Exec(r.Context(), `DELETE FROM twitch_widget_tokens WHERE user_id = $1`, claims.UserID)
	if _, err := h.DB.Exec(r.Context(), `DELETE FROM twitch_connections WHERE user_id = $1`, claims.UserID); err != nil {
		http.Error(w, "failed to disconnect twitch", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// EventSubWebhook is Twitch's own delivery endpoint -- public, no
// PowerNode auth, no Authorization header. The HMAC signature over the
// message id + timestamp + raw body is the only thing standing between
// this and anyone posting fake subscription alerts, so it's checked before
// anything else runs.
func (h *TwitchHandler) EventSubWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	messageID := r.Header.Get("Twitch-Eventsub-Message-Id")
	timestamp := r.Header.Get("Twitch-Eventsub-Message-Timestamp")
	signature := r.Header.Get("Twitch-Eventsub-Message-Signature")
	messageType := r.Header.Get("Twitch-Eventsub-Message-Type")
	if messageID == "" || timestamp == "" || signature == "" {
		http.Error(w, "missing eventsub headers", http.StatusBadRequest)
		return
	}

	if ts, err := time.Parse(time.RFC3339, timestamp); err != nil || time.Since(ts) > 10*time.Minute {
		http.Error(w, "stale or invalid timestamp", http.StatusBadRequest)
		return
	}

	if !verifyEventSubSignature(h.EventSubSecret, messageID, timestamp, body, signature) {
		http.Error(w, "signature mismatch", http.StatusForbidden)
		return
	}

	switch messageType {
	case "webhook_callback_verification":
		var payload struct {
			Challenge string `json:"challenge"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(w, "bad challenge payload", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(payload.Challenge))

	case "notification":
		h.handleEventSubNotification(r, body)
		w.WriteHeader(http.StatusOK)

	case "revocation":
		var payload struct {
			Subscription struct {
				ID string `json:"id"`
			} `json:"subscription"`
		}
		_ = json.Unmarshal(body, &payload)
		_, _ = h.DB.Exec(r.Context(),
			`UPDATE twitch_eventsub_subscriptions SET status = 'revoked' WHERE twitch_subscription_id = $1`,
			payload.Subscription.ID)
		w.WriteHeader(http.StatusOK)

	default:
		w.WriteHeader(http.StatusOK)
	}
}

func (h *TwitchHandler) handleEventSubNotification(r *http.Request, body []byte) {
	var payload struct {
		Subscription struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"subscription"`
		Event struct {
			UserName    string `json:"user_name"`
			Tier        string `json:"tier"`
			IsGift      bool   `json:"is_gift"`
			Total       int    `json:"total"`
			IsAnonymous bool   `json:"is_anonymous"`
		} `json:"event"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return
	}

	var userID int64
	if err := h.DB.QueryRow(r.Context(),
		`SELECT user_id FROM twitch_eventsub_subscriptions WHERE twitch_subscription_id = $1`, payload.Subscription.ID,
	).Scan(&userID); err != nil {
		return
	}
	var widgetToken string
	if err := h.DB.QueryRow(r.Context(),
		`SELECT token FROM twitch_widget_tokens WHERE user_id = $1`, userID,
	).Scan(&widgetToken); err != nil {
		return
	}

	switch payload.Subscription.Type {
	case "channel.subscribe":
		if payload.Event.IsGift {
			// Covered by the channel.subscription.gift notification instead --
			// alerting on both would double up for the same event.
			return
		}
		h.Hub.BroadcastWidget(widgetToken, map[string]any{
			"type":      "sub",
			"user_name": payload.Event.UserName,
			"tier":      payload.Event.Tier,
		})
	case "channel.subscription.gift":
		name := payload.Event.UserName
		if payload.Event.IsAnonymous || name == "" {
			name = "Anonymous"
		}
		h.Hub.BroadcastWidget(widgetToken, map[string]any{
			"type":      "gift",
			"user_name": name,
			"tier":      payload.Event.Tier,
			"count":     payload.Event.Total,
		})
	}
}

// WidgetPage serves the OBS Browser Source page -- a small, standalone,
// unauthenticated HTML+JS page (the token in the URL is the only "auth").
// Deliberately not part of the React app: OBS's embedded browser has no
// logged-in PowerNode session to share, and this needs to work as a bare
// URL pasted into a Browser Source, not behind the SPA's login gate.
func (h *TwitchHandler) WidgetPage(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, widgetPageHTML(token))
}

func (h *TwitchHandler) widgetURL(token string) string {
	base := h.PublicURL
	return base + "/api/v1/widgets/subs/" + token
}

func widgetPageHTML(token string) string {
	return `<!doctype html>
<html><head><meta charset="utf-8"><title>PowerNode subscription alerts</title>
<style>
  html, body { margin: 0; background: transparent; overflow: hidden; font-family: -apple-system, Segoe UI, sans-serif; }
  #alert {
    position: fixed; top: 40px; left: 50%; transform: translate(-50%, -20px);
    padding: 16px 28px; border-radius: 12px;
    background: rgba(10, 8, 12, 0.85); border: 1px solid rgba(232, 168, 184, 0.4);
    color: #ece4e8; text-align: center; opacity: 0; transition: opacity .3s, transform .3s;
  }
  #alert.show { opacity: 1; transform: translate(-50%, 0); }
  #alert .name { color: #e8a8b8; font-size: 22px; font-weight: 600; }
  #alert .msg { font-size: 14px; color: #b8a8b0; margin-top: 4px; }
</style></head>
<body>
  <div id="alert"><div class="name"></div><div class="msg"></div></div>
  <script>
    var token = ` + jsStringLiteral(token) + `;
    var el = document.getElementById('alert');
    var nameEl = el.querySelector('.name');
    var msgEl = el.querySelector('.msg');
    var hideTimer = null;

    function showAlert(name, msg) {
      nameEl.textContent = name;
      msgEl.textContent = msg;
      el.classList.add('show');
      if (hideTimer) clearTimeout(hideTimer);
      hideTimer = setTimeout(function () { el.classList.remove('show'); }, 6000);
    }

    function connect() {
      var proto = location.protocol === 'https:' ? 'wss' : 'ws';
      var socket = new WebSocket(proto + '://' + location.host + '/ws/widgets/' + encodeURIComponent(token));
      socket.onmessage = function (event) {
        try {
          var data = JSON.parse(event.data);
          if (data.type === 'sub') {
            showAlert(data.user_name, 'just subscribed' + (data.tier ? ' (tier ' + (data.tier / 1000) + ')' : '') + '!');
          } else if (data.type === 'gift') {
            showAlert(data.user_name, 'gifted ' + data.count + ' sub' + (data.count === 1 ? '' : 's') + '!');
          }
        } catch (e) { /* ignore malformed messages */ }
      };
      socket.onclose = function () { setTimeout(connect, 3000); };
      socket.onerror = function () { socket.close(); };
    }
    connect();
  </script>
</body></html>`
}

// jsStringLiteral safely embeds a server-controlled token (hex from
// randomToken, never user input) into an inline <script> block.
func jsStringLiteral(s string) string {
	return "\"" + html.EscapeString(s) + "\""
}

// verifyEventSubSignature implements Twitch's documented algorithm exactly:
// HMAC-SHA256 over the concatenation of the message ID, the timestamp, and
// the raw request body (in that order), prefixed "sha256=" and hex-encoded.
// Compared with hmac.Equal (constant-time) rather than ==, per Twitch's own
// recommendation to use a time-safe comparison.
func verifyEventSubSignature(secret, messageID, timestamp string, body []byte, signature string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(messageID + timestamp))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func hasScope(scopes, want string) bool {
	for _, s := range strings.Fields(scopes) {
		if s == want {
			return true
		}
	}
	return false
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
