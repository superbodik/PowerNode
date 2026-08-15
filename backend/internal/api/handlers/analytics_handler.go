package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/panel/internal/auth"
	"github.com/yourorg/panel/internal/twitch"
)

const (
	streamOnlineEventType  = "stream.online"
	streamOfflineEventType = "stream.offline"
)

type AnalyticsHandler struct {
	DB             *pgxpool.Pool
	Twitch         *twitch.Client
	EventSubSecret string
	PublicURL      string
}

type currencyTotal struct {
	Currency   string `json:"currency"`
	TotalCents int64  `json:"total_cents"`
	Count      int    `json:"count"`
}

type analyticsResponse struct {
	TwitchConnected   bool            `json:"twitch_connected"`
	Live              bool            `json:"live"`
	ViewerCount       int             `json:"viewer_count,omitempty"`
	DonationsToday    []currencyTotal `json:"donations_today"`
	DonationsAllTime  []currencyTotal `json:"donations_all_time"`
	ChatMessagesToday int             `json:"chat_messages_today"`
}

func (h *AnalyticsHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, h.compute(r.Context(), claims.UserID))
}

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

	_ = h.DB.QueryRow(ctx,
		`SELECT COALESCE(SUM(chat_messages), 0) FROM stream_sessions WHERE user_id = $1 AND started_at >= date_trunc('day', now())`,
		userID,
	).Scan(&resp.ChatMessagesToday)

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

func (h *AnalyticsHandler) EnsureSessionTracking(ctx context.Context, userID int64) {
	var twitchUserID string
	if err := h.DB.QueryRow(ctx, `SELECT twitch_user_id FROM twitch_connections WHERE user_id = $1`, userID).Scan(&twitchUserID); err != nil {
		return
	}

	callbackURL := h.PublicURL + "/api/v1/twitch/eventsub"
	for _, eventType := range []string{streamOnlineEventType, streamOfflineEventType} {
		var already bool
		if err := h.DB.QueryRow(ctx,
			`SELECT true FROM twitch_eventsub_subscriptions WHERE user_id = $1 AND event_type = $2 AND status = 'enabled'`,
			userID, eventType,
		).Scan(&already); err == nil && already {
			continue
		}
		appToken, err := h.Twitch.AppAccessToken(ctx)
		if err != nil {
			return
		}
		subID, err := h.Twitch.CreateEventSubSubscription(ctx, appToken,
			eventType, "1", map[string]string{"broadcaster_user_id": twitchUserID},
			callbackURL, h.EventSubSecret,
		)
		if err != nil {
			continue
		}
		_, _ = h.DB.Exec(ctx, `
			INSERT INTO twitch_eventsub_subscriptions (user_id, event_type, twitch_subscription_id, status)
			VALUES ($1, $2, $3, 'enabled')
			ON CONFLICT (user_id, event_type) DO UPDATE SET twitch_subscription_id = EXCLUDED.twitch_subscription_id, status = 'enabled'`,
			userID, eventType, subID,
		)
	}
}

type streamSession struct {
	ID            int64             `json:"id"`
	StartedAt     time.Time         `json:"started_at"`
	EndedAt       *time.Time        `json:"ended_at"`
	PeakViewers   int               `json:"peak_viewers"`
	ChatMessages  int               `json:"chat_messages"`
	DonationTotal []currencyTotal   `json:"donation_total"`
	Samples       []viewerSample    `json:"samples,omitempty"`
	Donations     []sessionDonation `json:"donations,omitempty"`
}

type viewerSample struct {
	SampledAt time.Time `json:"sampled_at"`
	Viewers   int       `json:"viewers"`
}

type sessionDonation struct {
	DonorName   string    `json:"donor_name"`
	Message     string    `json:"message"`
	AmountCents int64     `json:"amount_cents"`
	Currency    string    `json:"currency"`
	CreatedAt   time.Time `json:"created_at"`
}

func (h *AnalyticsHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := h.DB.Query(r.Context(), `
		SELECT id, started_at, ended_at, peak_viewers, chat_messages
		FROM stream_sessions WHERE user_id = $1 ORDER BY started_at DESC LIMIT 100`, claims.UserID)
	if err != nil {
		http.Error(w, "failed to load stream sessions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	sessions := []streamSession{}
	for rows.Next() {
		var s streamSession
		if err := rows.Scan(&s.ID, &s.StartedAt, &s.EndedAt, &s.PeakViewers, &s.ChatMessages); err != nil {
			continue
		}
		end := time.Now()
		if s.EndedAt != nil {
			end = *s.EndedAt
		}
		s.DonationTotal = h.donationTotalsBetween(r.Context(), claims.UserID, s.StartedAt, end)
		sessions = append(sessions, s)
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (h *AnalyticsHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid session id", http.StatusBadRequest)
		return
	}

	var s streamSession
	err = h.DB.QueryRow(r.Context(), `
		SELECT id, started_at, ended_at, peak_viewers, chat_messages
		FROM stream_sessions WHERE id = $1 AND user_id = $2`, id, claims.UserID,
	).Scan(&s.ID, &s.StartedAt, &s.EndedAt, &s.PeakViewers, &s.ChatMessages)
	if err == pgx.ErrNoRows {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "failed to load session", http.StatusInternalServerError)
		return
	}

	end := time.Now()
	if s.EndedAt != nil {
		end = *s.EndedAt
	}
	s.DonationTotal = h.donationTotalsBetween(r.Context(), claims.UserID, s.StartedAt, end)

	sampleRows, err := h.DB.Query(r.Context(),
		`SELECT sampled_at, viewers FROM viewer_samples WHERE session_id = $1 ORDER BY sampled_at`, s.ID)
	if err == nil {
		defer sampleRows.Close()
		s.Samples = []viewerSample{}
		for sampleRows.Next() {
			var v viewerSample
			if sampleRows.Scan(&v.SampledAt, &v.Viewers) == nil {
				s.Samples = append(s.Samples, v)
			}
		}
	}

	donationRows, err := h.DB.Query(r.Context(), `
		SELECT donor_name, message, amount_cents, currency, created_at
		FROM donations WHERE recipient_user_id = $1 AND created_at BETWEEN $2 AND $3
		ORDER BY created_at`, claims.UserID, s.StartedAt, end)
	if err == nil {
		defer donationRows.Close()
		s.Donations = []sessionDonation{}
		for donationRows.Next() {
			var d sessionDonation
			if donationRows.Scan(&d.DonorName, &d.Message, &d.AmountCents, &d.Currency, &d.CreatedAt) == nil {
				s.Donations = append(s.Donations, d)
			}
		}
	}

	writeJSON(w, http.StatusOK, s)
}

func (h *AnalyticsHandler) donationTotalsBetween(ctx context.Context, userID int64, start, end time.Time) []currencyTotal {
	rows, err := h.DB.Query(ctx, `
		SELECT currency, COALESCE(SUM(amount_cents), 0), COUNT(*)
		FROM donations WHERE recipient_user_id = $1 AND created_at BETWEEN $2 AND $3
		GROUP BY currency ORDER BY currency`, userID, start, end)
	if err != nil {
		return []currencyTotal{}
	}
	defer rows.Close()
	out := []currencyTotal{}
	for rows.Next() {
		var ct currencyTotal
		if rows.Scan(&ct.Currency, &ct.TotalCents, &ct.Count) == nil {
			out = append(out, ct)
		}
	}
	return out
}

func (h *AnalyticsHandler) HandleStreamOnline(ctx context.Context, userID int64) {
	var existing bool
	if err := h.DB.QueryRow(ctx, `SELECT true FROM stream_sessions WHERE user_id = $1 AND ended_at IS NULL`, userID).Scan(&existing); err == nil && existing {
		return
	}
	_, _ = h.DB.Exec(ctx, `INSERT INTO stream_sessions (user_id, started_at) VALUES ($1, now())`, userID)
}

func (h *AnalyticsHandler) HandleStreamOffline(ctx context.Context, userID int64) {
	_, _ = h.DB.Exec(ctx, `UPDATE stream_sessions SET ended_at = now() WHERE user_id = $1 AND ended_at IS NULL`, userID)
}

func RunViewerSampler(pool *pgxpool.Pool, client *twitch.Client) {
	if !client.Enabled() {
		return
	}
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		sampleOpenSessions(pool, client)
	}
}

func sampleOpenSessions(pool *pgxpool.Pool, client *twitch.Client) {
	ctx := context.Background()

	type openSession struct {
		id           int64
		twitchUserID string
	}

	rows, err := pool.Query(ctx, `
		SELECT ss.id, tc.twitch_user_id
		FROM stream_sessions ss
		JOIN twitch_connections tc ON tc.user_id = ss.user_id
		WHERE ss.ended_at IS NULL`)
	if err != nil {
		return
	}
	var sessions []openSession
	for rows.Next() {
		var s openSession
		if rows.Scan(&s.id, &s.twitchUserID) == nil {
			sessions = append(sessions, s)
		}
	}
	rows.Close()
	if len(sessions) == 0 {
		return
	}

	appToken, err := client.AppAccessToken(ctx)
	if err != nil {
		return
	}
	for _, s := range sessions {
		vc, live, err := client.GetViewerCount(ctx, appToken, s.twitchUserID)
		if err != nil || !live {
			continue
		}
		_, _ = pool.Exec(ctx, `INSERT INTO viewer_samples (session_id, viewers) VALUES ($1, $2)`, s.id, vc)
		_, _ = pool.Exec(ctx, `UPDATE stream_sessions SET peak_viewers = GREATEST(peak_viewers, $1) WHERE id = $2`, vc, s.id)
	}
}
