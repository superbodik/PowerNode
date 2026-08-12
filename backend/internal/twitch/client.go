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
	"time"
)

const (
	authorizeURL = "https://id.twitch.tv/oauth2/authorize"
	tokenURL     = "https://id.twitch.tv/oauth2/token"
	usersURL     = "https://api.twitch.tv/helix/users"
)

// Scope is kept intentionally small for the initial "connect an account"
// pass: just enough to identify who's connected. Stream title/category
// management (channel:manage:broadcast) and subscription/donation alerts
// (channel:read:subscriptions, ...) are deliberate follow-ups once this
// exists to build on, not bundled in here.
const Scope = "user:read:email"

type Client struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	HTTP         *http.Client
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

func (c *Client) AuthorizeURL(state string) string {
	q := url.Values{
		"client_id":     {c.ClientID},
		"redirect_uri":  {c.RedirectURI},
		"response_type": {"code"},
		"scope":         {Scope},
		"state":         {state},
	}
	return authorizeURL + "?" + q.Encode()
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
