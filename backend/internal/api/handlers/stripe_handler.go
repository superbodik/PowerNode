package handlers

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	stripe "github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/account"
	"github.com/stripe/stripe-go/v81/accountlink"
	checkoutsession "github.com/stripe/stripe-go/v81/checkout/session"
	"github.com/stripe/stripe-go/v81/webhook"

	"github.com/yourorg/panel/internal/auth"
	"github.com/yourorg/panel/internal/spotify"
	"github.com/yourorg/panel/internal/ws"
)

// StripeHandler powers donations. Non-custodial by design: each streamer
// connects their own Stripe Express account (Stripe hosts onboarding/KYC
// entirely -- we never see bank/identity details), viewers pay through a
// platform-created Checkout Session, and funds transfer straight to the
// streamer's account minus PlatformFeeBps. stripe.Key is set globally at
// startup from PANEL_STRIPE_SECRET_KEY (see cmd/panel/main.go); this
// handler is a no-op (Enabled() == false) until that's configured.
type StripeHandler struct {
	DB             *pgxpool.Pool
	Hub            *ws.Hub
	PublicURL      string
	WebhookSecret  string
	PlatformFeeBps int
	// Spotify is optional (nil-safe) -- only used to queue a donor's
	// requested track once a donation completes. Donations work fine
	// without it ever being set.
	Spotify *SpotifyHandler
	enabled bool
}

func NewStripeHandler(db *pgxpool.Pool, hub *ws.Hub, publicURL, webhookSecret string, enabled bool, platformFeeBps int) *StripeHandler {
	return &StripeHandler{
		DB:             db,
		Hub:            hub,
		PublicURL:      publicURL,
		WebhookSecret:  webhookSecret,
		PlatformFeeBps: platformFeeBps,
		enabled:        enabled,
	}
}

func (h *StripeHandler) Enabled() bool { return h.enabled }

type stripeStatusResponse struct {
	Connected        bool   `json:"connected"`
	ChargesEnabled   bool   `json:"charges_enabled"`
	PayoutsEnabled   bool   `json:"payouts_enabled"`
	DetailsSubmitted bool   `json:"details_submitted"`
	DonationPageURL  string `json:"donation_page_url,omitempty"`
}

// donationPageURL uses the short public path (/donate/..., not
// /api/v1/donate/...) -- nginx aliases it to the backend (see
// scripts/panel.sh's "location /donate/" block), so it's what actually
// gets shared with viewers. The backend's real routes are still under
// /api/v1/donate for consistency with everything else it serves; this is
// just the externally-facing address.
func (h *StripeHandler) donationPageURL(username string) string {
	return h.PublicURL + "/donate/" + username
}

// ConnectStart creates (or reuses) this user's Stripe Express account and
// hands back a one-time onboarding URL to redirect the browser to --
// same "navigate the top-level page, not fetch" shape as the Twitch OAuth
// start, since Stripe's onboarding UI has to render at the top level too.
func (h *StripeHandler) ConnectStart(w http.ResponseWriter, r *http.Request) {
	if !h.enabled {
		http.Error(w, "donations are not configured on this panel", http.StatusServiceUnavailable)
		return
	}
	if h.PublicURL == "" {
		http.Error(w, "PANEL_PUBLIC_URL is not set -- required for Stripe to redirect back here", http.StatusServiceUnavailable)
		return
	}
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var accountID string
	err := h.DB.QueryRow(r.Context(),
		`SELECT stripe_account_id FROM stripe_connections WHERE user_id = $1`, claims.UserID,
	).Scan(&accountID)
	if err == pgx.ErrNoRows {
		acct, err := account.New(&stripe.AccountParams{
			Type: stripe.String(string(stripe.AccountTypeExpress)),
		})
		if err != nil {
			http.Error(w, "failed to create stripe account: "+err.Error(), http.StatusBadGateway)
			return
		}
		accountID = acct.ID
		if _, err := h.DB.Exec(r.Context(),
			`INSERT INTO stripe_connections (user_id, stripe_account_id) VALUES ($1, $2)`,
			claims.UserID, accountID,
		); err != nil {
			http.Error(w, "failed to save stripe connection", http.StatusInternalServerError)
			return
		}
	} else if err != nil {
		http.Error(w, "failed to load stripe connection", http.StatusInternalServerError)
		return
	}

	refreshURL := h.PublicURL + "/streamers?stripe=refresh"
	returnURL := h.PublicURL + "/api/v1/integrations/stripe/connect/return?account=" + accountID
	link, err := accountlink.New(&stripe.AccountLinkParams{
		Account:    stripe.String(accountID),
		RefreshURL: stripe.String(refreshURL),
		ReturnURL:  stripe.String(returnURL),
		Type:       stripe.String(string(stripe.AccountLinkTypeAccountOnboarding)),
	})
	if err != nil {
		http.Error(w, "failed to create onboarding link: "+err.Error(), http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"authorize_url": link.URL})
}

// ConnectReturn is where Stripe redirects the browser after onboarding
// (success or not -- account state is the source of truth, not the fact
// the user landed back here). Public: identifies the connection purely by
// the account ID in the query string, which isn't a bearer credential --
// nothing sensitive can be done with it without our platform secret key.
func (h *StripeHandler) ConnectReturn(w http.ResponseWriter, r *http.Request) {
	accountID := r.URL.Query().Get("account")
	if accountID != "" && h.enabled {
		if acct, err := account.GetByID(accountID, nil); err == nil {
			h.DB.Exec(r.Context(), `
				UPDATE stripe_connections
				SET charges_enabled = $1, payouts_enabled = $2, details_submitted = $3, updated_at = now()
				WHERE stripe_account_id = $4`,
				acct.ChargesEnabled, acct.PayoutsEnabled, acct.DetailsSubmitted, accountID,
			)
		}
	}
	http.Redirect(w, r, "/streamers?stripe=success", http.StatusFound)
}

func (h *StripeHandler) Status(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var resp stripeStatusResponse
	var username string
	var accountID string
	err := h.DB.QueryRow(r.Context(), `
		SELECT u.username, c.stripe_account_id, c.charges_enabled, c.payouts_enabled, c.details_submitted
		FROM stripe_connections c JOIN users u ON u.id = c.user_id
		WHERE c.user_id = $1`, claims.UserID,
	).Scan(&username, &accountID, &resp.ChargesEnabled, &resp.PayoutsEnabled, &resp.DetailsSubmitted)
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusOK, resp)
		return
	} else if err != nil {
		http.Error(w, "failed to load stripe connection", http.StatusInternalServerError)
		return
	}

	resp.Connected = true
	if resp.ChargesEnabled {
		resp.DonationPageURL = h.donationPageURL(username)
	}
	writeJSON(w, http.StatusOK, resp)
}

// Disconnect only removes our local record -- it never touches the actual
// Stripe account, consistent with never holding custody of it in the
// first place. The streamer keeps their Stripe account regardless.
func (h *StripeHandler) Disconnect(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if _, err := h.DB.Exec(r.Context(), `DELETE FROM stripe_connections WHERE user_id = $1`, claims.UserID); err != nil {
		http.Error(w, "failed to disconnect", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DonatePage serves a minimal public, unauthenticated page for viewers --
// amount/name/message, then hands off to Stripe-hosted Checkout for the
// actual card entry, so this page never touches card data itself.
func (h *StripeHandler) DonatePage(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	var chargesEnabled bool
	var recipientUserID int64
	err := h.DB.QueryRow(r.Context(), `
		SELECT u.id, c.charges_enabled FROM stripe_connections c JOIN users u ON u.id = c.user_id
		WHERE u.username = $1`, username,
	).Scan(&recipientUserID, &chargesEnabled)
	if err != nil || !chargesEnabled {
		http.Error(w, "this streamer isn't accepting donations right now", http.StatusNotFound)
		return
	}

	spotifyEnabled := false
	if h.Spotify != nil {
		var exists bool
		h.DB.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM spotify_connections WHERE user_id = $1)`, recipientUserID,
		).Scan(&exists)
		spotifyEnabled = exists
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, donatePageHTML(username, spotifyEnabled))
}

func (h *StripeHandler) DonateThanksPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, donateThanksPageHTML())
}

type createDonationCheckoutRequest struct {
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
	DonorName   string `json:"donor_name"`
	Message     string `json:"message"`
	TrackQuery  string `json:"track_query"`
}

const (
	minDonationCents = 100        // $1 (or local-currency equivalent) -- below most processors' own minimums anyway
	maxDonationCents = 100_000_00 // $100,000 sanity cap against fat-fingered amounts, not a real limit
)

// CreateCheckout is the public, unauthenticated endpoint the donate page's
// own form posts to. Never trusts the client for anything beyond the
// amount/currency/name/message -- the recipient's Stripe account and the
// platform fee are both resolved server-side from the username in the URL.
func (h *StripeHandler) CreateCheckout(w http.ResponseWriter, r *http.Request) {
	if !h.enabled {
		http.Error(w, "donations are not configured on this panel", http.StatusServiceUnavailable)
		return
	}
	username := chi.URLParam(r, "username")

	var req createDonationCheckoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.AmountCents < minDonationCents || req.AmountCents > maxDonationCents {
		http.Error(w, "amount out of range", http.StatusBadRequest)
		return
	}
	if req.Currency == "" {
		req.Currency = "usd"
	}
	if len(req.DonorName) > 60 {
		req.DonorName = req.DonorName[:60]
	}
	if len(req.Message) > 300 {
		req.Message = req.Message[:300]
	}
	if req.DonorName == "" {
		req.DonorName = "Anonymous"
	}
	if len(req.TrackQuery) > 150 {
		req.TrackQuery = req.TrackQuery[:150]
	}

	var recipientUserID int64
	var stripeAccountID string
	var chargesEnabled bool
	err := h.DB.QueryRow(r.Context(), `
		SELECT u.id, c.stripe_account_id, c.charges_enabled
		FROM stripe_connections c JOIN users u ON u.id = c.user_id
		WHERE u.username = $1`, username,
	).Scan(&recipientUserID, &stripeAccountID, &chargesEnabled)
	if err != nil || !chargesEnabled {
		http.Error(w, "this streamer isn't accepting donations right now", http.StatusNotFound)
		return
	}

	feeCents := req.AmountCents * int64(h.PlatformFeeBps) / 10000

	sess, err := checkoutsession.New(&stripe.CheckoutSessionParams{
		Mode: stripe.String(string(stripe.CheckoutSessionModePayment)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{{
			Quantity: stripe.Int64(1),
			PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
				Currency:   stripe.String(req.Currency),
				UnitAmount: stripe.Int64(req.AmountCents),
				ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
					Name: stripe.String("Donation to " + username),
				},
			},
		}},
		PaymentIntentData: &stripe.CheckoutSessionPaymentIntentDataParams{
			ApplicationFeeAmount: stripe.Int64(feeCents),
			TransferData: &stripe.CheckoutSessionPaymentIntentDataTransferDataParams{
				Destination: stripe.String(stripeAccountID),
			},
		},
		Metadata: map[string]string{
			"recipient_user_id":  fmt.Sprintf("%d", recipientUserID),
			"donor_name":         req.DonorName,
			"message":            req.Message,
			"platform_fee_cents": fmt.Sprintf("%d", feeCents),
			"track_query":        req.TrackQuery,
		},
		SuccessURL: stripe.String(h.PublicURL + "/donate/" + username + "/thanks"),
		CancelURL:  stripe.String(h.PublicURL + "/donate/" + username),
	})
	if err != nil {
		http.Error(w, "failed to start checkout: "+err.Error(), http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"redirect_url": sess.URL})
}

// Webhook is Stripe's server-to-server delivery of what actually happened
// to a payment -- the only place a donation is ever recorded, since a
// viewer merely reaching Checkout proves nothing was paid.
func (h *StripeHandler) Webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	event, err := webhook.ConstructEvent(body, r.Header.Get("Stripe-Signature"), h.WebhookSecret)
	if err != nil {
		http.Error(w, "signature verification failed", http.StatusBadRequest)
		return
	}

	if event.Type != "checkout.session.completed" {
		w.WriteHeader(http.StatusOK)
		return
	}

	var sess stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &sess); err != nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	if sess.PaymentStatus != stripe.CheckoutSessionPaymentStatusPaid {
		w.WriteHeader(http.StatusOK)
		return
	}

	var recipientUserID int64
	fmt.Sscanf(sess.Metadata["recipient_user_id"], "%d", &recipientUserID)
	var feeCents int64
	fmt.Sscanf(sess.Metadata["platform_fee_cents"], "%d", &feeCents)
	if recipientUserID == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}

	var paymentIntentID string
	if sess.PaymentIntent != nil {
		paymentIntentID = sess.PaymentIntent.ID
	}

	trackQuery := sess.Metadata["track_query"]
	var trackQueued bool
	var trackName, trackArtist string
	if h.Spotify != nil && trackQuery != "" {
		var track *spotify.Track
		trackQueued, track = h.Spotify.QueueForDonation(r.Context(), recipientUserID, trackQuery)
		if track != nil {
			trackName, trackArtist = track.Name, track.Artist
		}
	}

	_, err = h.DB.Exec(r.Context(), `
		INSERT INTO donations (recipient_user_id, stripe_checkout_session_id, stripe_payment_intent_id,
		                        donor_name, message, amount_cents, currency, platform_fee_cents,
		                        track_query, track_queued)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (stripe_checkout_session_id) DO NOTHING`,
		recipientUserID, sess.ID, paymentIntentID,
		sess.Metadata["donor_name"], sess.Metadata["message"], sess.AmountTotal, string(sess.Currency), feeCents,
		trackQuery, trackQueued,
	)
	if err != nil {
		http.Error(w, "failed to record donation", http.StatusInternalServerError)
		return
	}

	if widgetToken, err := getOrCreateWidgetToken(r.Context(), h.DB, recipientUserID); err == nil {
		msg := map[string]any{
			"type":      "donation",
			"user_name": sess.Metadata["donor_name"],
			"message":   sess.Metadata["message"],
			"amount":    sess.AmountTotal,
			"currency":  string(sess.Currency),
		}
		if trackQueued {
			msg["track_name"] = trackName
			msg["track_artist"] = trackArtist
		}
		h.Hub.BroadcastWidget(widgetToken, msg)
	}

	w.WriteHeader(http.StatusOK)
}

func donatePageHTML(username string, spotifyEnabled bool) string {
	safeUsername := html.EscapeString(username)
	checkoutURL := "/donate/" + url.PathEscape(username) + "/checkout"
	trackField := ""
	if spotifyEnabled {
		trackField = `
    <label>Song request (optional)</label>
    <input id="track" type="text" maxlength="150" placeholder="Artist - Song title" />
    <span style="display:block;font-size:11px;color:#8a7a82;margin-top:4px;">Queued on their Spotify if they're live and listening.</span>`
	}
	return `<!doctype html>
<html><head><meta charset="utf-8"><title>Donate to ` + safeUsername + `</title>
<style>
  html, body { margin: 0; min-height: 100%; background: #0a080c; color: #ece4e8; font-family: -apple-system, Segoe UI, sans-serif; }
  body { display: flex; align-items: center; justify-content: center; padding: 24px; box-sizing: border-box; }
  .card { max-width: 420px; width: 100%; background: rgba(255,255,255,0.03); border: 1px solid rgba(232,168,184,0.2); border-radius: 16px; padding: 28px; }
  h1 { font-size: 20px; margin: 0 0 4px; color: #e8a8b8; }
  p.sub { margin: 0 0 20px; color: #b8a8b0; font-size: 13px; }
  label { display: block; font-size: 12px; color: #b8a8b0; margin-bottom: 6px; margin-top: 14px; }
  input, textarea { width: 100%; box-sizing: border-box; padding: 10px 12px; border-radius: 8px; border: 1px solid rgba(232,168,184,0.25); background: rgba(0,0,0,0.3); color: #ece4e8; font-size: 14px; font-family: inherit; }
  textarea { resize: vertical; min-height: 60px; }
  .amounts { display: flex; gap: 8px; margin-top: 8px; flex-wrap: wrap; }
  .amounts button { flex: 1; min-width: 60px; padding: 8px; border-radius: 8px; border: 1px solid rgba(232,168,184,0.25); background: transparent; color: #ece4e8; cursor: pointer; font-size: 13px; }
  .amounts button.active { background: #e8a8b8; color: #1a1216; border-color: #e8a8b8; }
  button.submit { width: 100%; margin-top: 20px; padding: 12px; border-radius: 10px; border: none; background: #e8a8b8; color: #1a1216; font-size: 15px; font-weight: 600; cursor: pointer; }
  button.submit:disabled { opacity: 0.6; cursor: default; }
  .error { color: #f23f43; font-size: 13px; margin-top: 10px; display: none; }
</style></head>
<body>
  <div class="card">
    <h1>Donate to ` + safeUsername + `</h1>
    <p class="sub">Paid securely via Stripe. Goes straight to their account.</p>
    <label>Amount (USD)</label>
    <div class="amounts">
      <button type="button" data-amt="5">$5</button>
      <button type="button" data-amt="10" class="active">$10</button>
      <button type="button" data-amt="25">$25</button>
      <button type="button" data-amt="50">$50</button>
    </div>
    <input id="amount" type="number" min="1" step="1" value="10" style="margin-top:8px" />
    <label>Your name (optional)</label>
    <input id="name" type="text" maxlength="60" placeholder="Anonymous" />
    <label>Message (optional)</label>
    <textarea id="message" maxlength="300"></textarea>` + trackField + `
    <button class="submit" id="submit">Donate</button>
    <div class="error" id="error"></div>
  </div>
  <script>
    var amountInput = document.getElementById('amount');
    document.querySelectorAll('.amounts button').forEach(function (btn) {
      btn.addEventListener('click', function () {
        document.querySelectorAll('.amounts button').forEach(function (b) { b.classList.remove('active'); });
        btn.classList.add('active');
        amountInput.value = btn.getAttribute('data-amt');
      });
    });
    document.getElementById('submit').addEventListener('click', function () {
      var btn = this;
      var errEl = document.getElementById('error');
      errEl.style.display = 'none';
      var amount = parseFloat(amountInput.value);
      if (!amount || amount <= 0) { errEl.textContent = 'Enter a valid amount.'; errEl.style.display = 'block'; return; }
      btn.disabled = true;
      btn.textContent = 'Redirecting…';
      fetch(` + jsStringLiteral(checkoutURL) + `, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          amount_cents: Math.round(amount * 100),
          currency: 'usd',
          donor_name: document.getElementById('name').value,
          message: document.getElementById('message').value,
          track_query: document.getElementById('track') ? document.getElementById('track').value : '',
        }),
      }).then(function (res) {
        if (!res.ok) throw new Error('failed');
        return res.json();
      }).then(function (data) {
        window.location = data.redirect_url;
      }).catch(function () {
        errEl.textContent = 'Something went wrong. Please try again.';
        errEl.style.display = 'block';
        btn.disabled = false;
        btn.textContent = 'Donate';
      });
    });
  </script>
</body></html>`
}

func donateThanksPageHTML() string {
	return `<!doctype html>
<html><head><meta charset="utf-8"><title>Thank you!</title>
<style>
  html, body { margin: 0; min-height: 100%; background: #0a080c; color: #ece4e8; font-family: -apple-system, Segoe UI, sans-serif; }
  body { display: flex; align-items: center; justify-content: center; text-align: center; }
</style></head>
<body>
  <div>
    <h1 style="color:#e8a8b8;">Thank you! 💜</h1>
    <p style="color:#b8a8b0;">Your donation is on its way.</p>
  </div>
</body></html>`
}
