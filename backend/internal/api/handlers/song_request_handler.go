package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/panel/internal/auth"
	"github.com/yourorg/panel/internal/crypto"
	"github.com/yourorg/panel/internal/twitch"
	"github.com/yourorg/panel/internal/ws"
)

// SongRequestHandler runs song requests off Twitch Channel Points: a viewer
// redeems a custom reward this panel creates on the broadcaster's channel,
// pastes a YouTube/SoundCloud/Yandex Music link as the reward's required
// text input, and it lands in a queue an OBS Browser Source plays through --
// an alternative to the Spotify-based flow (spotify_handler.go) that needs
// neither Premium nor an app allowlist, just Channel Points.
type SongRequestHandler struct {
	DB             *pgxpool.Pool
	Client         *twitch.Client
	Hub            *ws.Hub
	EncryptionKey  string
	EventSubSecret string
	PublicURL      string
}

const songRequestEventType = "channel.channel_points_custom_reward_redemption.add"

func (h *SongRequestHandler) widgetURL(token string) string {
	return h.PublicURL + "/api/v1/widgets/song-requests/" + token
}

// resolveOwner mirrors OverlayHandler's dual auth: either the logged-in
// owner (panel UI) or anyone presenting the widget's own render_token
// (?token=... -- the OBS Browser Source has no panel session to carry a
// JWT with).
func (h *SongRequestHandler) resolveOwner(r *http.Request) (userID int64, ok bool) {
	if claims, authed := auth.FromContext(r.Context()); authed {
		return claims.UserID, true
	}
	token := r.URL.Query().Get("token")
	if token == "" {
		return 0, false
	}
	err := h.DB.QueryRow(r.Context(),
		`SELECT user_id FROM song_request_rewards WHERE render_token = $1`, token,
	).Scan(&userID)
	if err != nil {
		return 0, false
	}
	return userID, true
}

type songRequestStatusResponse struct {
	Enabled   bool   `json:"enabled"`
	WidgetURL string `json:"widget_url,omitempty"`
}

func (h *SongRequestHandler) Status(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var token string
	err := h.DB.QueryRow(r.Context(),
		`SELECT render_token FROM song_request_rewards WHERE user_id = $1`, claims.UserID,
	).Scan(&token)
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusOK, songRequestStatusResponse{Enabled: false})
		return
	} else if err != nil {
		http.Error(w, "failed to load song requests status", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, songRequestStatusResponse{Enabled: true, WidgetURL: h.widgetURL(token)})
}

type enableSongRequestsRequest struct {
	CostPoints int `json:"cost_points"`
}

// Enable creates a "Song Request" Channel Points reward on the
// broadcaster's channel and subscribes to its redemptions. Idempotent: a
// repeat call while already configured just returns the existing widget
// URL instead of creating a second reward.
func (h *SongRequestHandler) Enable(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ctx := r.Context()

	var req enableSongRequestsRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	cost := req.CostPoints
	if cost <= 0 {
		cost = 500
	}

	if existingToken, err := h.currentToken(ctx, claims.UserID); err == nil {
		writeJSON(w, http.StatusOK, songRequestStatusResponse{Enabled: true, WidgetURL: h.widgetURL(existingToken)})
		return
	}

	var twitchUserID, scopes, accessEnc, refreshEnc string
	err := h.DB.QueryRow(ctx,
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
	if !hasScope(scopes, "channel:manage:redemptions") {
		http.Error(w, "reconnect twitch with channel points access first", http.StatusBadRequest)
		return
	}

	accessToken, err := crypto.Decrypt(h.EncryptionKey, accessEnc)
	if err != nil {
		http.Error(w, "failed to decrypt stored token", http.StatusInternalServerError)
		return
	}

	rewardID, err := h.Client.CreateCustomReward(ctx, accessToken, twitchUserID, "Song Request", cost)
	if errors.Is(err, twitch.ErrTokenExpired) {
		accessToken, err = h.refreshStoredToken(ctx, claims.UserID, refreshEnc)
		if err != nil {
			http.Error(w, "twitch session expired -- reconnect your account", http.StatusBadRequest)
			return
		}
		rewardID, err = h.Client.CreateCustomReward(ctx, accessToken, twitchUserID, "Song Request", cost)
	}
	if err != nil {
		http.Error(w, "failed to create twitch reward: "+err.Error(), http.StatusBadGateway)
		return
	}

	appToken, err := h.Client.AppAccessToken(ctx)
	if err != nil {
		_ = h.Client.DeleteCustomReward(ctx, accessToken, twitchUserID, rewardID)
		http.Error(w, "failed to reach twitch", http.StatusBadGateway)
		return
	}
	subID, err := h.Client.CreateEventSubSubscription(ctx, appToken,
		songRequestEventType, "1",
		map[string]string{"broadcaster_user_id": twitchUserID, "reward_id": rewardID},
		h.PublicURL+"/api/v1/twitch/eventsub", h.EventSubSecret,
	)
	if err != nil {
		_ = h.Client.DeleteCustomReward(ctx, accessToken, twitchUserID, rewardID)
		http.Error(w, "failed to subscribe to twitch redemptions: "+err.Error(), http.StatusBadGateway)
		return
	}

	renderToken, err := randomToken(24)
	if err != nil {
		http.Error(w, "failed to create widget token", http.StatusInternalServerError)
		return
	}

	tx, err := h.DB.Begin(ctx)
	if err != nil {
		http.Error(w, "failed to save configuration", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`INSERT INTO song_request_rewards (user_id, twitch_reward_id, render_token) VALUES ($1, $2, $3)`,
		claims.UserID, rewardID, renderToken,
	); err != nil {
		http.Error(w, "failed to save configuration", http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO twitch_eventsub_subscriptions (user_id, event_type, twitch_subscription_id, status)
		VALUES ($1, $2, $3, 'enabled')`,
		claims.UserID, songRequestEventType, subID,
	); err != nil {
		http.Error(w, "failed to save configuration", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		http.Error(w, "failed to save configuration", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, songRequestStatusResponse{Enabled: true, WidgetURL: h.widgetURL(renderToken)})
}

func (h *SongRequestHandler) currentToken(ctx context.Context, userID int64) (string, error) {
	var token string
	err := h.DB.QueryRow(ctx, `SELECT render_token FROM song_request_rewards WHERE user_id = $1`, userID).Scan(&token)
	return token, err
}

// refreshStoredToken exchanges the stored refresh token for a fresh pair,
// persists it, and returns the new access token -- shared by every call
// site here that needs to retry once after twitch.ErrTokenExpired.
func (h *SongRequestHandler) refreshStoredToken(ctx context.Context, userID int64, refreshEnc string) (string, error) {
	refreshToken, err := crypto.Decrypt(h.EncryptionKey, refreshEnc)
	if err != nil {
		return "", err
	}
	tokens, err := h.Client.RefreshUserToken(ctx, refreshToken)
	if err != nil {
		return "", err
	}
	newAccessEnc, aerr := crypto.Encrypt(h.EncryptionKey, tokens.AccessToken)
	newRefreshEnc, rerr := crypto.Encrypt(h.EncryptionKey, tokens.RefreshToken)
	if aerr == nil && rerr == nil {
		_, _ = h.DB.Exec(ctx,
			`UPDATE twitch_connections SET access_token_encrypted = $1, refresh_token_encrypted = $2, updated_at = now() WHERE user_id = $3`,
			newAccessEnc, newRefreshEnc, userID)
	}
	return tokens.AccessToken, nil
}

// Disable tears down everything Enable set up: the EventSub subscription,
// the Twitch reward itself (best-effort -- Twitch only lets the app that
// created a reward delete it, so this can't orphan someone else's), and the
// panel-side rows.
func (h *SongRequestHandler) Disable(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ctx := r.Context()

	var rewardID string
	err := h.DB.QueryRow(ctx, `SELECT twitch_reward_id FROM song_request_rewards WHERE user_id = $1`, claims.UserID).Scan(&rewardID)
	if err == pgx.ErrNoRows {
		w.WriteHeader(http.StatusNoContent)
		return
	} else if err != nil {
		http.Error(w, "failed to load song requests configuration", http.StatusInternalServerError)
		return
	}

	var subID string
	if err := h.DB.QueryRow(ctx,
		`SELECT twitch_subscription_id FROM twitch_eventsub_subscriptions WHERE user_id = $1 AND event_type = $2`,
		claims.UserID, songRequestEventType,
	).Scan(&subID); err == nil {
		if appToken, aerr := h.Client.AppAccessToken(ctx); aerr == nil {
			_ = h.Client.DeleteEventSubSubscription(ctx, appToken, subID)
		}
	}
	var twitchUserID, accessEnc string
	if err := h.DB.QueryRow(ctx,
		`SELECT twitch_user_id, access_token_encrypted FROM twitch_connections WHERE user_id = $1`, claims.UserID,
	).Scan(&twitchUserID, &accessEnc); err == nil {
		if accessToken, derr := crypto.Decrypt(h.EncryptionKey, accessEnc); derr == nil {
			_ = h.Client.DeleteCustomReward(ctx, accessToken, twitchUserID, rewardID)
		}
	}

	_, _ = h.DB.Exec(ctx, `DELETE FROM twitch_eventsub_subscriptions WHERE user_id = $1 AND event_type = $2`,
		claims.UserID, songRequestEventType)
	_, _ = h.DB.Exec(ctx, `DELETE FROM song_request_queue WHERE user_id = $1`, claims.UserID)
	_, _ = h.DB.Exec(ctx, `DELETE FROM song_request_rewards WHERE user_id = $1`, claims.UserID)

	w.WriteHeader(http.StatusNoContent)
}

type queuedSongRequest struct {
	ID       int64  `json:"id"`
	Redeemer string `json:"redeemer_name"`
	Link     string `json:"link"`
	Provider string `json:"provider"`
	Status   string `json:"status"`
}

// Queue lists still-pending requests -- used both by the panel (to show a
// queue view) and by the widget itself on load, so a reloaded OBS Browser
// Source picks up whatever was already queued instead of only what arrives
// over the websocket after that reload.
func (h *SongRequestHandler) Queue(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.resolveOwner(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	rows, err := h.DB.Query(r.Context(), `
		SELECT id, redeemer_name, link, provider, status FROM song_request_queue
		WHERE user_id = $1 AND status IN ('queued', 'playing') ORDER BY id`, userID)
	if err != nil {
		http.Error(w, "failed to load queue", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	queue := []queuedSongRequest{}
	for rows.Next() {
		var q queuedSongRequest
		if err := rows.Scan(&q.ID, &q.Redeemer, &q.Link, &q.Provider, &q.Status); err == nil {
			queue = append(queue, q)
		}
	}
	writeJSON(w, http.StatusOK, queue)
}

// Advance marks a request done -- called by the widget itself (a "Next"
// button an OBS "Interact" click can reach) or the panel's own queue view.
func (h *SongRequestHandler) Advance(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.resolveOwner(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid queue id", http.StatusBadRequest)
		return
	}
	tag, err := h.DB.Exec(r.Context(),
		`UPDATE song_request_queue SET status = 'done' WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		http.Error(w, "failed to update queue", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "queue item not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RenderPage is the actual OBS Browser Source -- public, read-only except
// for its own Advance/Next control, keyed by render_token.
func (h *SongRequestHandler) RenderPage(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	var exists bool
	if err := h.DB.QueryRow(r.Context(),
		`SELECT true FROM song_request_rewards WHERE render_token = $1`, token,
	).Scan(&exists); err != nil {
		http.Error(w, "song request widget not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, songRequestWidgetPageHTML(token))
}

// classifyProvider is a display-only hint stored alongside each request
// (shown as an icon/label in the panel's queue view) -- the widget itself
// re-parses the link independently to build the actual embed src, since
// that depends on exactly how each provider's embed player wants the URL
// shaped, not just which provider it is.
func classifyProvider(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		return "unknown"
	}
	host := strings.ToLower(u.Hostname())
	switch {
	case host == "youtu.be", strings.HasSuffix(host, ".youtube.com"), host == "youtube.com":
		return "youtube"
	case strings.HasSuffix(host, ".soundcloud.com"), host == "soundcloud.com":
		return "soundcloud"
	case strings.HasPrefix(host, "music.yandex."):
		return "yandex_music"
	default:
		return "unknown"
	}
}

// HandleRedemption is called from TwitchHandler.handleEventSubNotification
// for the song-request EventSub type -- kept separate from that switch
// (rather than adding a song_request_rewards-specific lookup inline there)
// since a user who only enabled song requests, never the sub/follow alert
// widget, has no twitch_widget_tokens row at all, and that path's existing
// lookup would otherwise silently swallow every redemption for them.
func (h *SongRequestHandler) HandleRedemption(ctx context.Context, userID int64, redemptionID, redeemerName, userInput string) {
	link := strings.TrimSpace(userInput)
	if link == "" {
		return
	}
	provider := classifyProvider(link)

	renderToken, err := h.currentToken(ctx, userID)
	if err != nil {
		return
	}

	var queueID int64
	err = h.DB.QueryRow(ctx, `
		INSERT INTO song_request_queue (user_id, twitch_redemption_id, redeemer_name, link, provider)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, twitch_redemption_id) DO NOTHING
		RETURNING id`,
		userID, redemptionID, redeemerName, link, provider,
	).Scan(&queueID)
	if err != nil {
		// Either a duplicate delivery (Twitch retries notifications) or a
		// DB error -- either way there's nothing new to broadcast.
		return
	}

	h.Hub.BroadcastWidget(renderToken, map[string]any{
		"type":     "queued",
		"id":       queueID,
		"redeemer": redeemerName,
		"link":     link,
		"provider": provider,
	})

	h.fulfillRedemption(ctx, userID, redemptionID)
}

// fulfillRedemption marks the redemption fulfilled on Twitch's side so it
// doesn't sit as a pending review item in the creator dashboard forever.
// Best-effort: the request is already queued and playable in the widget
// regardless of whether this succeeds.
func (h *SongRequestHandler) fulfillRedemption(ctx context.Context, userID int64, redemptionID string) {
	var twitchUserID, rewardID, accessEnc, refreshEnc string
	err := h.DB.QueryRow(ctx, `
		SELECT tc.twitch_user_id, sr.twitch_reward_id, tc.access_token_encrypted, tc.refresh_token_encrypted
		FROM song_request_rewards sr JOIN twitch_connections tc ON tc.user_id = sr.user_id
		WHERE sr.user_id = $1`, userID,
	).Scan(&twitchUserID, &rewardID, &accessEnc, &refreshEnc)
	if err != nil {
		return
	}
	accessToken, err := crypto.Decrypt(h.EncryptionKey, accessEnc)
	if err != nil {
		return
	}
	err = h.Client.UpdateRedemptionStatus(ctx, accessToken, twitchUserID, rewardID, redemptionID, "FULFILLED")
	if errors.Is(err, twitch.ErrTokenExpired) {
		accessToken, err = h.refreshStoredToken(ctx, userID, refreshEnc)
		if err != nil {
			return
		}
		_ = h.Client.UpdateRedemptionStatus(ctx, accessToken, twitchUserID, rewardID, redemptionID, "FULFILLED")
	}
}

func songRequestWidgetPageHTML(token string) string {
	return `<!doctype html>
<html><head><meta charset="utf-8"><title>PowerNode song requests</title>
<style>
  html, body { margin: 0; padding: 0; width: 100%; height: 100%; overflow: hidden; background: transparent; font-family: -apple-system, Segoe UI, sans-serif; }
  #stage { position: relative; width: 100%; height: 100%; display: flex; flex-direction: column; justify-content: flex-end; }
  #player { width: 100%; }
  #player iframe { display: block; width: 100%; border: 0; }
  #bar { display: flex; align-items: center; gap: 10px; background: rgba(10,8,12,.75); color: #ece4e8; padding: 6px 10px; font-size: 12px; }
  #bar .redeemer { font-weight: 700; color: #e8a8b8; }
  #bar .queued { margin-left: auto; opacity: .7; }
  #next { background: rgba(232,168,184,.15); border: 1px solid rgba(232,168,184,.4); color: #ece4e8; border-radius: 6px; padding: 4px 10px; cursor: pointer; font-size: 12px; }
  #next:hover { background: rgba(232,168,184,.28); }
  .fallback { padding: 10px; color: #ece4e8; word-break: break-all; }
  .fallback a { color: #e8a8b8; }
  #empty { display: none; color: #8a7a82; font-size: 12px; padding: 8px; }
</style>
</head>
<body>
  <div id="stage">
    <div id="player"></div>
    <div id="bar" style="display:none">
      <span class="redeemer"></span>
      <button id="next">Next ▶</button>
      <span class="queued"></span>
    </div>
    <div id="empty">Waiting for song requests…</div>
  </div>
  <script>
    var token = ` + jsStringLiteral(token) + `;
    var queue = [];
    var current = null;

    // Returns a plain descriptor, never an HTML string -- link is
    // arbitrary text a Twitch viewer typed into a redemption, so anything
    // derived from it (including pieces pulled out via URL parsing) gets
    // assigned through DOM properties below, never innerHTML, or it'd be a
    // stored-XSS hole running in the streamer's own OBS Browser Source.
    function embedFor(link) {
      try {
        var u = new URL(link);
        var host = u.hostname.replace(/^www\./, '');
        if (host === 'youtu.be') {
          return { kind: 'youtube', id: u.pathname.slice(1) };
        }
        if (host === 'youtube.com' || host === 'music.youtube.com') {
          var id = u.searchParams.get('v');
          if (!id && u.pathname.indexOf('/shorts/') === 0) id = u.pathname.split('/')[2];
          if (!id && u.pathname.indexOf('/embed/') === 0) id = u.pathname.split('/')[2];
          if (id) return { kind: 'youtube', id: id };
        }
        if (host === 'soundcloud.com') {
          return { kind: 'soundcloud', url: link };
        }
        if (host.indexOf('music.yandex.') === 0) {
          var m = u.pathname.match(/\/album\/(\d+)\/track\/(\d+)/);
          if (m) {
            return { kind: 'yandex', trackId: m[2], albumId: m[1] };
          }
        }
      } catch (e) {}
      return { kind: 'link', url: link };
    }

    function buildPlayer(descriptor) {
      if (descriptor.kind === 'youtube') {
        var f = document.createElement('iframe');
        f.height = '220';
        f.allow = 'autoplay; encrypted-media';
        f.allowFullscreen = true;
        f.src = 'https://www.youtube.com/embed/' + encodeURIComponent(descriptor.id) + '?autoplay=1';
        return f;
      }
      if (descriptor.kind === 'soundcloud') {
        var f2 = document.createElement('iframe');
        f2.height = '166';
        f2.scrolling = 'no';
        f2.setAttribute('allow', 'autoplay');
        f2.src = 'https://w.soundcloud.com/player/?url=' + encodeURIComponent(descriptor.url) + '&auto_play=true&color=%23e8a8b8';
        return f2;
      }
      if (descriptor.kind === 'yandex') {
        var f3 = document.createElement('iframe');
        f3.style.border = 'none';
        f3.height = '180';
        f3.src = 'https://music.yandex.ru/iframe/#track/' + encodeURIComponent(descriptor.trackId) + '/' + encodeURIComponent(descriptor.albumId);
        return f3;
      }
      var div = document.createElement('div');
      div.className = 'fallback';
      var a = document.createElement('a');
      a.href = descriptor.url;
      a.target = '_blank';
      a.rel = 'noopener';
      a.textContent = descriptor.url;
      div.appendChild(a);
      return div;
    }

    function render() {
      var bar = document.getElementById('bar');
      var empty = document.getElementById('empty');
      var player = document.getElementById('player');
      player.textContent = '';
      if (!current) {
        bar.style.display = 'none';
        empty.style.display = 'block';
        return;
      }
      empty.style.display = 'none';
      bar.style.display = 'flex';
      bar.querySelector('.redeemer').textContent = current.redeemer + ' requested:';
      bar.querySelector('.queued').textContent = queue.length ? ('+' + queue.length + ' queued') : '';
      player.appendChild(buildPlayer(embedFor(current.link)));
    }

    function playNext() {
      current = queue.shift() || null;
      render();
    }

    document.getElementById('next').addEventListener('click', function () {
      if (!current) return;
      fetch('/api/v1/twitch/song-requests/queue/' + current.id + '/advance?token=' + encodeURIComponent(token), { method: 'POST' })
        .catch(function () {});
      playNext();
    });

    fetch('/api/v1/twitch/song-requests/queue?token=' + encodeURIComponent(token))
      .then(function (r) { return r.ok ? r.json() : []; })
      .then(function (items) {
        queue = items || [];
        playNext();
      })
      .catch(function () {});

    function connect() {
      var proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
      var ws = new WebSocket(proto + '//' + location.host + '/ws/widgets/' + token);
      ws.onmessage = function (ev) {
        try {
          var msg = JSON.parse(ev.data);
          if (msg.type === 'queued') {
            queue.push({ id: msg.id, redeemer: msg.redeemer, link: msg.link, provider: msg.provider });
            if (!current) playNext();
            else render();
          }
        } catch (e) {}
      };
      ws.onclose = function () { setTimeout(connect, 3000); };
    }
    connect();
  </script>
</body></html>`
}
