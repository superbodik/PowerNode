package handlers

import (
	"context"
	"net/http"

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

	resp := analyticsResponse{
		DonationsToday:   []currencyTotal{},
		DonationsAllTime: []currencyTotal{},
	}

	var twitchUserID string
	if err := h.DB.QueryRow(r.Context(),
		`SELECT twitch_user_id FROM twitch_connections WHERE user_id = $1`, claims.UserID,
	).Scan(&twitchUserID); err == nil {
		resp.TwitchConnected = true
		if h.Twitch.Enabled() {
			if appToken, aerr := h.Twitch.AppAccessToken(r.Context()); aerr == nil {
				if vc, live, verr := h.Twitch.GetViewerCount(r.Context(), appToken, twitchUserID); verr == nil {
					resp.Live = live
					resp.ViewerCount = vc
				}
			}
		}
	}

	resp.DonationsAllTime = h.donationTotals(r.Context(), claims.UserID, "")
	resp.DonationsToday = h.donationTotals(r.Context(), claims.UserID, "AND created_at >= date_trunc('day', now())")

	writeJSON(w, http.StatusOK, resp)
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
