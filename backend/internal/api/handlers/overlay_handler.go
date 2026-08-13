package handlers

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/panel/internal/auth"
)

// OverlayHandler powers the moderator overlay constructor: a streamer (or
// anyone holding the moderator link -- token-based access, no login,
// same trust model as the Twitch alert widget) arranges widgets on a
// canvas, and a separate public render page turns that into the actual
// OBS Browser Source.
type OverlayHandler struct {
	DB        *pgxpool.Pool
	PublicURL string
}

type overlayWidget struct {
	ID         int64           `json:"id,omitempty"`
	WidgetType string          `json:"widget_type"`
	X          float64         `json:"x"`
	Y          float64         `json:"y"`
	Width      float64         `json:"width"`
	Height     float64         `json:"height"`
	ZIndex     int             `json:"z_index"`
	Config     json.RawMessage `json:"config"`
}

type overlayLayoutResponse struct {
	Name         string          `json:"name"`
	ModeratorURL string          `json:"moderator_url"`
	RenderURL    string          `json:"render_url"`
	Widgets      []overlayWidget `json:"widgets"`
}

// resolveOwner identifies whose layout is being worked on -- either the
// authenticated caller (normal panel login), or anyone presenting a valid
// moderator_token (?token=... query param, no login required). Returns 0
// with ok=false if neither checks out.
func (h *OverlayHandler) resolveOwner(r *http.Request) (userID int64, ok bool) {
	if claims, authed := auth.FromContext(r.Context()); authed {
		return claims.UserID, true
	}
	token := r.URL.Query().Get("token")
	if token == "" {
		return 0, false
	}
	err := h.DB.QueryRow(r.Context(),
		`SELECT owner_user_id FROM overlay_layouts WHERE moderator_token = $1`, token,
	).Scan(&userID)
	if err != nil {
		return 0, false
	}
	return userID, true
}

func (h *OverlayHandler) moderatorURL(token string) string {
	return h.PublicURL + "/overlay-editor?token=" + token
}

func (h *OverlayHandler) renderURL(token string) string {
	return h.PublicURL + "/api/v1/overlay/render/" + token
}

// GetLayout returns the caller's layout, creating an empty one on first
// use -- there's nothing meaningful to show for "no layout yet" beyond an
// empty canvas, so this just makes one exist rather than erroring.
func (h *OverlayHandler) GetLayout(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.resolveOwner(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var layoutID int64
	var name, modToken, renderToken string
	err := h.DB.QueryRow(r.Context(),
		`SELECT id, name, moderator_token, render_token FROM overlay_layouts WHERE owner_user_id = $1`, userID,
	).Scan(&layoutID, &name, &modToken, &renderToken)
	if err == pgx.ErrNoRows {
		modToken, err = randomToken(24)
		if err != nil {
			http.Error(w, "failed to create layout", http.StatusInternalServerError)
			return
		}
		renderToken, err = randomToken(24)
		if err != nil {
			http.Error(w, "failed to create layout", http.StatusInternalServerError)
			return
		}
		name = "Overlay"
		err = h.DB.QueryRow(r.Context(), `
			INSERT INTO overlay_layouts (owner_user_id, name, moderator_token, render_token)
			VALUES ($1, $2, $3, $4) RETURNING id`,
			userID, name, modToken, renderToken,
		).Scan(&layoutID)
		if err != nil {
			http.Error(w, "failed to create layout", http.StatusInternalServerError)
			return
		}
	} else if err != nil {
		http.Error(w, "failed to load layout", http.StatusInternalServerError)
		return
	}

	rows, err := h.DB.Query(r.Context(), `
		SELECT id, widget_type, x, y, width, height, z_index, config
		FROM overlay_widgets WHERE layout_id = $1 ORDER BY z_index, id`, layoutID)
	if err != nil {
		http.Error(w, "failed to load widgets", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	widgets := []overlayWidget{}
	for rows.Next() {
		var wg overlayWidget
		if err := rows.Scan(&wg.ID, &wg.WidgetType, &wg.X, &wg.Y, &wg.Width, &wg.Height, &wg.ZIndex, &wg.Config); err != nil {
			http.Error(w, "failed to read widgets", http.StatusInternalServerError)
			return
		}
		widgets = append(widgets, wg)
	}

	writeJSON(w, http.StatusOK, overlayLayoutResponse{
		Name:         name,
		ModeratorURL: h.moderatorURL(modToken),
		RenderURL:    h.renderURL(renderToken),
		Widgets:      widgets,
	})
}

type saveWidgetsRequest struct {
	Widgets []overlayWidget `json:"widgets"`
}

// SaveWidgets replaces the entire widget set in one call -- matches how a
// canvas editor naturally saves state ("here's the current layout"),
// rather than trying to diff individual widget moves against the server.
func (h *OverlayHandler) SaveWidgets(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.resolveOwner(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req saveWidgetsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.Widgets) > 50 {
		http.Error(w, "too many widgets", http.StatusBadRequest)
		return
	}

	var layoutID int64
	if err := h.DB.QueryRow(r.Context(),
		`SELECT id FROM overlay_layouts WHERE owner_user_id = $1`, userID,
	).Scan(&layoutID); err != nil {
		http.Error(w, "layout not found -- load it first", http.StatusBadRequest)
		return
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		http.Error(w, "failed to save widgets", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	if _, err := tx.Exec(r.Context(), `DELETE FROM overlay_widgets WHERE layout_id = $1`, layoutID); err != nil {
		http.Error(w, "failed to save widgets", http.StatusInternalServerError)
		return
	}
	for _, wg := range req.Widgets {
		config := wg.Config
		if len(config) == 0 {
			config = json.RawMessage("{}")
		}
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO overlay_widgets (layout_id, widget_type, x, y, width, height, z_index, config)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			layoutID, wg.WidgetType, wg.X, wg.Y, wg.Width, wg.Height, wg.ZIndex, config,
		); err != nil {
			http.Error(w, "failed to save widgets", http.StatusInternalServerError)
			return
		}
	}
	if _, err := tx.Exec(r.Context(), `UPDATE overlay_layouts SET updated_at = now() WHERE id = $1`, layoutID); err != nil {
		http.Error(w, "failed to save widgets", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "failed to save widgets", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RenderPage is the actual OBS Browser Source -- public, read-only, keyed
// by render_token (deliberately separate from moderator_token so sharing
// this URL, which might end up visible on-stream, can never grant edit
// access). Renders each widget's last-known position/size and live data.
func (h *OverlayHandler) RenderPage(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

	var layoutID int64
	var username string
	err := h.DB.QueryRow(r.Context(), `
		SELECT l.id, u.username FROM overlay_layouts l JOIN users u ON u.id = l.owner_user_id
		WHERE l.render_token = $1`, token,
	).Scan(&layoutID, &username)
	if err != nil {
		http.Error(w, "overlay not found", http.StatusNotFound)
		return
	}

	rows, err := h.DB.Query(r.Context(), `
		SELECT widget_type, x, y, width, height, z_index, config
		FROM overlay_widgets WHERE layout_id = $1 ORDER BY z_index, id`, layoutID)
	if err != nil {
		http.Error(w, "failed to load overlay", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	widgets := []overlayWidget{}
	for rows.Next() {
		var wg overlayWidget
		if err := rows.Scan(&wg.WidgetType, &wg.X, &wg.Y, &wg.Width, &wg.Height, &wg.ZIndex, &wg.Config); err == nil {
			widgets = append(widgets, wg)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, overlayRenderPageHTML(username, widgets))
}

func overlayRenderPageHTML(username string, widgets []overlayWidget) string {
	var boxes string
	for _, wg := range widgets {
		var inner string
		switch wg.WidgetType {
		case "chat":
			src := "/api/v1/widgets/chat/" + username
			boxes += fmt.Sprintf(
				`<iframe class="w" style="left:%.3f%%;top:%.3f%%;width:%.3f%%;height:%.3f%%;z-index:%d" src="%s"></iframe>`,
				wg.X, wg.Y, wg.Width, wg.Height, wg.ZIndex, html.EscapeString(src),
			)
			continue
		case "text":
			var cfg struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(wg.Config, &cfg)
			inner = `<div class="text">` + html.EscapeString(cfg.Text) + `</div>`
		case "image":
			var cfg struct {
				URL string `json:"url"`
			}
			_ = json.Unmarshal(wg.Config, &cfg)
			inner = `<img class="img" src="` + html.EscapeString(cfg.URL) + `" />`
		case "viewer_count":
			inner = `<div class="stat" data-metric="viewers"><span class="num">&mdash;</span><span class="lbl">viewers</span></div>`
		case "donation_total":
			inner = `<div class="stat" data-metric="donations"><span class="num">&mdash;</span><span class="lbl">donated today</span></div>`
		default:
			continue
		}
		boxes += fmt.Sprintf(
			`<div class="w" style="left:%.3f%%;top:%.3f%%;width:%.3f%%;height:%.3f%%;z-index:%d">%s</div>`,
			wg.X, wg.Y, wg.Width, wg.Height, wg.ZIndex, inner,
		)
	}

	return `<!doctype html>
<html><head><meta charset="utf-8"><title>PowerNode overlay</title>
<style>
  html, body { margin: 0; padding: 0; width: 100%; height: 100%; overflow: hidden; background: transparent; font-family: -apple-system, Segoe UI, sans-serif; }
  .stage { position: relative; width: 100%; height: 100%; }
  .w { position: absolute; box-sizing: border-box; overflow: hidden; }
  iframe.w { border: 0; }
  .text { color: #ece4e8; font-size: 20px; text-shadow: 0 1px 3px rgba(0,0,0,.8); }
  .img { width: 100%; height: 100%; object-fit: contain; }
  .stat { color: #ece4e8; text-align: center; }
  .stat .num { display: block; font-size: 28px; font-weight: 700; text-shadow: 0 1px 3px rgba(0,0,0,.8); }
  .stat .lbl { display: block; font-size: 11px; color: #b8a8b0; text-transform: uppercase; letter-spacing: .04em; }
</style>
</head>
<body>
  <div class="stage">` + boxes + `</div>
  <script>
    // Live-bind viewer_count / donation_total placeholders against the
    // panel's own public analytics summary for this channel, polled
    // periodically -- everything else on this page is static once loaded.
    function poll() {
      fetch('/api/v1/streamers/public-analytics/' + ` + jsStringLiteral(username) + `)
        .then(function (r) { return r.ok ? r.json() : null; })
        .then(function (data) {
          if (!data) return;
          document.querySelectorAll('[data-metric="viewers"] .num').forEach(function (el) {
            el.textContent = data.live ? data.viewer_count : '—';
          });
          document.querySelectorAll('[data-metric="donations"] .num').forEach(function (el) {
            el.textContent = data.donations_today_summary || '—';
          });
        })
        .catch(function () {});
    }
    poll();
    setInterval(poll, 15000);
  </script>
</body></html>`
}
