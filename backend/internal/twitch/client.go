// Package twitch is a minimal client for Twitch's OAuth authorization-code
// flow and the one Helix endpoint needed to identify who just connected.
// Verified against Twitch's own docs (dev.twitch.tv/docs/authentication)
// rather than assumed, since getting a redirect_uri or token exchange
// detail wrong here breaks the whole flow silently for every installed
// panel using it.
package twitch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	authorizeURL    = "https://id.twitch.tv/oauth2/authorize"
	tokenURL        = "https://id.twitch.tv/oauth2/token"
	usersURL        = "https://api.twitch.tv/helix/users"
	eventSubURL     = "https://api.twitch.tv/helix/eventsub/subscriptions"
	eventSubVersion = "1"
)

// Scope is the base "connect an account" scope: just enough to identify who
// connected. ScopeSubscriptions is requested by a separate, explicit
// upgrade step (not bundled into the base connect flow) since it lets the
// app read the broadcaster's subscriber list -- a meaningfully bigger ask
// than proving identity, and users should see that distinction rather than
// consent to it as a side effect of just linking an account.
const (
	Scope              = "user:read:email"
	ScopeSubscriptions = "user:read:email channel:read:subscriptions"
)

type Client struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	HTTP         *http.Client

	appTokenMu     sync.Mutex
	appToken       string
	appTokenExpiry time.Time
}

func New(clientID, clientSecret, redirectURI string) *Client {
	return &Client{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURI:  redirectURI,
		HTTP:         &http.Client{Timeout: 10 * time.Second},
	}
}

// Enabled reports whether this panel install has everything needed to run
// the flow: a registered Twitch app, and a known public URL to redirect
// back to (Twitch redirects the user's browser, so this can't be relative
// or default to localhost).
func (c *Client) Enabled() bool {
	return c.ClientID != "" && c.ClientSecret != "" && c.RedirectURI != ""
}

func (c *Client) authorizeURL(state, scope string) string {
	q := url.Values{
		"client_id":     {c.ClientID},
		"redirect_uri":  {c.RedirectURI},
		"response_type": {"code"},
		"scope":         {scope},
		"state":         {state},
	}
	return authorizeURL + "?" + q.Encode()
}

func (c *Client) AuthorizeURL(state string) string {
	return c.authorizeURL(state, Scope)
}

// AuthorizeSubscriptionsURL requests the broader scope needed for
// subscription-alert EventSub subscriptions. Kept as an explicit separate
// entry point rather than folded into AuthorizeURL -- see the Scope doc
// comment.
func (c *Client) AuthorizeSubscriptionsURL(state string) string {
	return c.authorizeURL(state, ScopeSubscriptions)
}

type Tokens struct {
	AccessToken  string
	RefreshToken string
	Scopes       []string
}

type tokenResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	Scope        []string `json:"scope"`
}

func (c *Client) ExchangeCode(ctx context.Context, code string) (*Tokens, error) {
	form := url.Values{
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {c.RedirectURI},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call twitch token endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("twitch token exchange returned %d", resp.StatusCode)
	}

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, fmt.Errorf("decode twitch token response: %w", err)
	}
	return &Tokens{AccessToken: tr.AccessToken, RefreshToken: tr.RefreshToken, Scopes: tr.Scope}, nil
}

type User struct {
	ID          string
	Login       string
	DisplayName string
}

type usersResponse struct {
	Data []struct {
		ID          string `json:"id"`
		Login       string `json:"login"`
		DisplayName string `json:"display_name"`
	} `json:"data"`
}

// FetchUser looks up the identity of whoever the access token belongs to.
// Called with no query parameters, Twitch's Get Users endpoint returns the
// token owner's own record.
func (c *Client) FetchUser(ctx context.Context, accessToken string) (*User, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, usersURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Client-Id", c.ClientID)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call twitch users endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("twitch get-users returned %d", resp.StatusCode)
	}

	var ur usersResponse
	if err := json.NewDecoder(resp.Body).Decode(&ur); err != nil {
		return nil, fmt.Errorf("decode twitch users response: %w", err)
	}
	if len(ur.Data) == 0 {
		return nil, fmt.Errorf("twitch get-users returned no data")
	}
	u := ur.Data[0]
	return &User{ID: u.ID, Login: u.Login, DisplayName: u.DisplayName}, nil
}

type appTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// AppAccessToken returns a client-credentials app token, caching it until
// shortly before it expires. EventSub webhook subscriptions specifically
// require an app token, not a user token -- confirmed against Twitch's own
// docs ("When subscribing to events using webhooks, you must use an app
// access token. The request fails if you use a user access token.").
func (c *Client) AppAccessToken(ctx context.Context) (string, error) {
	c.appTokenMu.Lock()
	defer c.appTokenMu.Unlock()

	if c.appToken != "" && time.Now().Before(c.appTokenExpiry) {
		return c.appToken, nil
	}

	form := url.Values{
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
		"grant_type":    {"client_credentials"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("call twitch app-token endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("twitch app-token request returned %d", resp.StatusCode)
	}

	var tr appTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("decode twitch app-token response: %w", err)
	}

	c.appToken = tr.AccessToken
	// Refresh a bit early rather than racing an exact expiry.
	c.appTokenExpiry = time.Now().Add(time.Duration(tr.ExpiresIn)*time.Second - 5*time.Minute)
	return c.appToken, nil
}

type eventSubCreateRequest struct {
	Type      string            `json:"type"`
	Version   string            `json:"version"`
	Condition map[string]string `json:"condition"`
	Transport eventSubTransport `json:"transport"`
}

type eventSubTransport struct {
	Method   string `json:"method"`
	Callback string `json:"callback"`
	Secret   string `json:"secret"`
}

type eventSubCreateResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// CreateEventSubSubscription registers a webhook-transport EventSub
// subscription for a broadcaster and returns Twitch's ID for it (needed
// later to delete it on disconnect). appToken must come from
// AppAccessToken, not a user token -- see its doc comment.
func (c *Client) CreateEventSubSubscription(ctx context.Context, appToken, eventType, broadcasterUserID, callbackURL, secret string) (string, error) {
	body := eventSubCreateRequest{
		Type:      eventType,
		Version:   eventSubVersion,
		Condition: map[string]string{"broadcaster_user_id": broadcasterUserID},
		Transport: eventSubTransport{Method: "webhook", Callback: callbackURL, Secret: secret},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, eventSubURL, strings.NewReader(string(payload)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+appToken)
	req.Header.Set("Client-Id", c.ClientID)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("call twitch eventsub create: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("twitch eventsub create returned %d", resp.StatusCode)
	}

	var er eventSubCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		return "", fmt.Errorf("decode twitch eventsub create response: %w", err)
	}
	if len(er.Data) == 0 {
		return "", fmt.Errorf("twitch eventsub create returned no data")
	}
	return er.Data[0].ID, nil
}

// DeleteEventSubSubscription removes a webhook subscription, e.g. when a
// user disconnects their Twitch account.
func (c *Client) DeleteEventSubSubscription(ctx context.Context, appToken, subscriptionID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, eventSubURL+"?id="+url.QueryEscape(subscriptionID), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+appToken)
	req.Header.Set("Client-Id", c.ClientID)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("call twitch eventsub delete: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("twitch eventsub delete returned %d", resp.StatusCode)
	}
	return nil
}
