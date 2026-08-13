// Package spotify is a minimal client for Spotify's OAuth authorization-code
// flow plus the three Web API endpoints needed for song-request donations:
// identify who connected, search for a track, and queue it. Verified
// against Spotify's own docs (developer.spotify.com/documentation/web-api)
// rather than assumed, same reasoning as internal/twitch.
package spotify

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrTokenExpired signals a caller should refresh the user access token
// (via RefreshUserToken) and retry.
var ErrTokenExpired = errors.New("spotify access token expired")

// ErrNoActiveDevice means the queue call reached Spotify fine, but the
// user doesn't have an active playback session (app closed, nothing
// playing) for it to queue onto -- not a real error, just nothing to do.
var ErrNoActiveDevice = errors.New("no active spotify playback device")

// ErrTrackNotFound means the search returned no results for the query.
var ErrTrackNotFound = errors.New("no matching spotify track found")

const (
	authorizeURL = "https://accounts.spotify.com/authorize"
	tokenURL     = "https://accounts.spotify.com/api/token"
	meURL        = "https://api.spotify.com/v1/me"
	searchURL    = "https://api.spotify.com/v1/search"
	queueURL     = "https://api.spotify.com/v1/me/player/queue"
)

// Scope requests just enough to identify the connected account and queue
// tracks onto whatever's actively playing -- not playlist or library
// access, which this feature has no use for.
const Scope = "user-read-email user-modify-playback-state user-read-playback-state"

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
		// Spotify doesn't support a Google-style multi-account chooser (the
		// browser only ever holds one active Spotify session), but without
		// this it silently reuses that session and skips straight to the
		// redirect if the user already approved the app once -- show_dialog
		// forces the consent screen to render every time, so at minimum the
		// user sees which account is active and can log out/switch there
		// before approving, instead of it happening invisibly.
		"show_dialog": {"true"},
	}
	return authorizeURL + "?" + q.Encode()
}

func (c *Client) basicAuthHeader() string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(c.ClientID+":"+c.ClientSecret))
}

type Tokens struct {
	AccessToken  string
	RefreshToken string
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func (c *Client) doTokenRequest(ctx context.Context, form url.Values) (*tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", c.basicAuthHeader())

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call spotify token endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("spotify token request returned %d: %s", resp.StatusCode, string(body))
	}

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, fmt.Errorf("decode spotify token response: %w", err)
	}
	return &tr, nil
}

func (c *Client) ExchangeCode(ctx context.Context, code string) (*Tokens, error) {
	tr, err := c.doTokenRequest(ctx, url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {c.RedirectURI},
	})
	if err != nil {
		return nil, err
	}
	return &Tokens{AccessToken: tr.AccessToken, RefreshToken: tr.RefreshToken}, nil
}

// RefreshUserToken exchanges a refresh token for a fresh access token.
// Spotify access tokens last 1 hour, and doesn't always return a new
// refresh token on refresh -- callers should keep the old one if this
// response's RefreshToken comes back empty.
func (c *Client) RefreshUserToken(ctx context.Context, refreshToken string) (*Tokens, error) {
	tr, err := c.doTokenRequest(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
	if err != nil {
		return nil, err
	}
	return &Tokens{AccessToken: tr.AccessToken, RefreshToken: tr.RefreshToken}, nil
}

type User struct {
	ID          string
	DisplayName string
}

type meResponse struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

func (c *Client) FetchUser(ctx context.Context, accessToken string) (*User, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, meURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call spotify me endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrTokenExpired
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("spotify me request returned %d", resp.StatusCode)
	}

	var mr meResponse
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, fmt.Errorf("decode spotify me response: %w", err)
	}
	return &User{ID: mr.ID, DisplayName: mr.DisplayName}, nil
}

type Track struct {
	URI    string
	Name   string
	Artist string
}

type searchResponse struct {
	Tracks struct {
		Items []struct {
			URI     string `json:"uri"`
			Name    string `json:"name"`
			Artists []struct {
				Name string `json:"name"`
			} `json:"artists"`
		} `json:"items"`
	} `json:"tracks"`
}

// SearchTrack returns Spotify's top match for a free-text query (as typed
// by a donor -- "artist - song", just a song title, etc.). Spotify's own
// search relevance ranking picks the best match; there's no reasonable way
// for us to do better than that from a single text field.
func (c *Client) SearchTrack(ctx context.Context, accessToken, query string) (*Track, error) {
	q := url.Values{"q": {query}, "type": {"track"}, "limit": {"1"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call spotify search endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrTokenExpired
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("spotify search request returned %d", resp.StatusCode)
	}

	var sr searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("decode spotify search response: %w", err)
	}
	if len(sr.Tracks.Items) == 0 {
		return nil, ErrTrackNotFound
	}
	item := sr.Tracks.Items[0]
	artist := ""
	if len(item.Artists) > 0 {
		artist = item.Artists[0].Name
	}
	return &Track{URI: item.URI, Name: item.Name, Artist: artist}, nil
}

// QueueTrack adds a track to the end of the user's current playback queue.
// Requires an actively playing (or at least active-device) session on
// their end -- returns ErrNoActiveDevice if there isn't one, which is a
// routine, expected outcome (streamer's Spotify isn't open right now), not
// a failure worth surfacing as an error to the donor.
func (c *Client) QueueTrack(ctx context.Context, accessToken, trackURI string) error {
	q := url.Values{"uri": {trackURI}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, queueURL+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("call spotify queue endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrTokenExpired
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrNoActiveDevice
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("spotify queue request returned %d", resp.StatusCode)
	}
	return nil
}
