package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/panel/internal/auth"
	"github.com/yourorg/panel/internal/twitch"
)

// AnalyticsHandler pulls together the numbers that live in different
// systems -- live viewer count from Twitch, donation totals from our own
// donations table -- into one summary for the Streamers hub. Chat message
// counts aren't here yet: unlike viewer count, that needs a chat-reading
// EventSub subscription (a new scope, a new user consent step), not just
// an extra Helix call.
type AnalyticsHandler struct {
	DB     *pgxpool.Pool
	Twitch *twitch.Client
}

type currencyTotal struct {
	Currency   string `json:"currency"`
	TotalCents int64  `json:"total_cents"`
	Count      int    `json:"count"`
}

type analyticsResponse struct {
	TwitchConnected  bool            `json:"twitch_connected"`
	Live             bool            `json:"live"`
	ViewerCount      int             `json:"viewer_count,omitempty"`
	DonationsToday   []currencyTotal `json:"donations_today"`
	DonationsAllTime []currencyTotal `json:"donations_all_time"`
}

func (h *AnalyticsHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, h.compute(r.Context(), claims.UserID))
}

// PublicSummary is a trimmed, public (unauthenticated) version keyed by
// username instead of a logged-in caller -- exists specifically for the
// overlay render page, which is a plain OBS Browser Source with no way to
// carry an API key. Donation totals are collapsed into a single display
// string rather than the raw per-currency breakdown Get returns, since
// an anonymous overlay viewer has no use for the structured form.
func (h *AnalyticsHandler) PublicSummary(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	var userID int64
	if err := h.DB.QueryRow(r.Context(), `SELECT id FROM users WHERE username = $1`, username).Scan(&userID); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	full := h.compute(r.Context(), userID)

	var summary string
	if len(full.DonationsToday) == 0 {
		summary = "0"
	} else {
		for i, ct := range full.DonationsToday {
			if i > 0 {
				summary += " + "
			}
			summary += fmt.Sprintf("%.2f %s", float64(ct.TotalCents)/100, ct.Currency)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"live":                    full.Live,
		"viewer_count":            full.ViewerCount,
		"donations_today_summary": summary,
	})
}

func (h *AnalyticsHandler) compute(ctx context.Context, userID int64) analyticsResponse {
	resp := analyticsResponse{
		DonationsToday:   []currencyTotal{},
		DonationsAllTime: []currencyTotal{},
	}

	var twitchUserID string
	if err := h.DB.QueryRow(ctx,
		`SELECT twitch_user_id FROM twitch_connections WHERE user_id = $1`, userID,
	).Scan(&twitchUserID); err == nil {
		resp.TwitchConnected = true
		if h.Twitch.Enabled() {
			if appToken, aerr := h.Twitch.AppAccessToken(ctx); aerr == nil {
				if vc, live, verr := h.Twitch.GetViewerCount(ctx, appToken, twitchUserID); verr == nil {
					resp.Live = live
					resp.ViewerCount = vc
				}
			}
		}
	}

	resp.DonationsAllTime = h.donationTotals(ctx, userID, "")
	resp.DonationsToday = h.donationTotals(ctx, userID, "AND created_at >= date_trunc('day', now())")
	return resp
}

func (h *AnalyticsHandler) donationTotals(ctx context.Context, userID int64, extraWhere string) []currencyTotal {
	rows, err := h.DB.Query(ctx, `
		SELECT currency, COALESCE(SUM(amount_cents), 0), COUNT(*)
		FROM donations
		WHERE recipient_user_id = $1 `+extraWhere+`
		GROUP BY currency ORDER BY currency`, userID)
	if err != nil {
		return []currencyTotal{}
	}
	defer rows.Close()

	out := []currencyTotal{}
	for rows.Next() {
		var ct currencyTotal
		if err := rows.Scan(&ct.Currency, &ct.TotalCents, &ct.Count); err == nil {
			out = append(out, ct)
		}
	}
	return out
}
