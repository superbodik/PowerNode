package twitch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var ErrTokenExpired = errors.New("twitch access token expired")

const (
	authorizeURL = "https://id.twitch.tv/oauth2/authorize"
	tokenURL     = "https://id.twitch.tv/oauth2/token"
	usersURL     = "https://api.twitch.tv/helix/users"
	eventSubURL  = "https://api.twitch.tv/helix/eventsub/subscriptions"
	streamsURL   = "https://api.twitch.tv/helix/streams"
)

const (
	Scope         = "user:read:email"
	ScopeExtended = "user:read:email channel:read:subscriptions channel:read:stream_key moderator:read:followers channel:manage:redemptions"
	ScopeBot      = "user:read:chat chat:edit"
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

func (c *Client) AuthorizeExtendedURL(state string) string {
	return c.authorizeURL(state, ScopeExtended)
}

func (c *Client) AuthorizeBotURL(state string) string {
	return c.authorizeURL(state, ScopeBot)
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

type streamsResponse struct {
	Data []struct {
		ViewerCount int `json:"viewer_count"`
	} `json:"data"`
}

func (c *Client) GetViewerCount(ctx context.Context, appToken, broadcasterUserID string) (int, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		streamsURL+"?user_id="+url.QueryEscape(broadcasterUserID), nil)
	if err != nil {
		return 0, false, err
	}
	req.Header.Set("Authorization", "Bearer "+appToken)
	req.Header.Set("Client-Id", c.ClientID)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, false, fmt.Errorf("call twitch streams endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return 0, false, fmt.Errorf("twitch get-streams returned %d", resp.StatusCode)
	}

	var sr streamsResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return 0, false, fmt.Errorf("decode twitch streams response: %w", err)
	}
	if len(sr.Data) == 0 {
		return 0, false, nil
	}
	return sr.Data[0].ViewerCount, true, nil
}

func (c *Client) RefreshUserToken(ctx context.Context, refreshToken string) (*Tokens, error) {
	form := url.Values{
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call twitch refresh-token endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("twitch refresh-token request returned %d", resp.StatusCode)
	}

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, fmt.Errorf("decode twitch refresh-token response: %w", err)
	}
	return &Tokens{AccessToken: tr.AccessToken, RefreshToken: tr.RefreshToken, Scopes: tr.Scope}, nil
}

type streamKeyResponse struct {
	Data []struct {
		StreamKey string `json:"stream_key"`
	} `json:"data"`
}

func (c *Client) GetStreamKey(ctx context.Context, userAccessToken, broadcasterUserID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.twitch.tv/helix/streams/key?broadcaster_id="+url.QueryEscape(broadcasterUserID), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+userAccessToken)
	req.Header.Set("Client-Id", c.ClientID)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("call twitch stream-key endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return "", ErrTokenExpired
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("twitch stream-key request returned %d", resp.StatusCode)
	}

	var sr streamKeyResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return "", fmt.Errorf("decode twitch stream-key response: %w", err)
	}
	if len(sr.Data) == 0 {
		return "", fmt.Errorf("twitch stream-key request returned no data")
	}
	return sr.Data[0].StreamKey, nil
}

type customRewardsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func (c *Client) CreateCustomReward(ctx context.Context, userAccessToken, broadcasterUserID, title string, costPoints int) (string, error) {
	body := map[string]any{
		"title":                  title,
		"cost":                   costPoints,
		"is_user_input_required": true,
		"prompt":                 "Paste a YouTube, SoundCloud, or Yandex Music link",
		"is_enabled":             true,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	reqURL := "https://api.twitch.tv/helix/channel_points/custom_rewards?broadcaster_id=" + url.QueryEscape(broadcasterUserID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(string(payload)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+userAccessToken)
	req.Header.Set("Client-Id", c.ClientID)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("call twitch create-custom-reward endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return "", ErrTokenExpired
	}
	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("twitch create-custom-reward returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var cr customRewardsResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", fmt.Errorf("decode twitch create-custom-reward response: %w", err)
	}
	if len(cr.Data) == 0 {
		return "", fmt.Errorf("twitch create-custom-reward returned no data")
	}
	return cr.Data[0].ID, nil
}

func (c *Client) DeleteCustomReward(ctx context.Context, userAccessToken, broadcasterUserID, rewardID string) error {
	reqURL := "https://api.twitch.tv/helix/channel_points/custom_rewards?broadcaster_id=" +
		url.QueryEscape(broadcasterUserID) + "&id=" + url.QueryEscape(rewardID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+userAccessToken)
	req.Header.Set("Client-Id", c.ClientID)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("call twitch delete-custom-reward endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrTokenExpired
	}
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("twitch delete-custom-reward returned %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) UpdateRedemptionStatus(ctx context.Context, userAccessToken, broadcasterUserID, rewardID, redemptionID, status string) error {
	reqURL := "https://api.twitch.tv/helix/channel_points/custom_rewards/redemptions?" +
		url.Values{
			"broadcaster_id": {broadcasterUserID},
			"reward_id":      {rewardID},
			"id":             {redemptionID},
		}.Encode()
	payload, err := json.Marshal(map[string]string{"status": status})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, reqURL, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+userAccessToken)
	req.Header.Set("Client-Id", c.ClientID)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("call twitch update-redemption-status endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrTokenExpired
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("twitch update-redemption-status returned %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) SendChatMessage(ctx context.Context, userAccessToken, broadcasterUserID, senderUserID, message string) error {
	payload, err := json.Marshal(map[string]string{
		"broadcaster_id": broadcasterUserID,
		"sender_id":      senderUserID,
		"message":        message,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.twitch.tv/helix/chat/messages", strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+userAccessToken)
	req.Header.Set("Client-Id", c.ClientID)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("call twitch send-chat-message endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrTokenExpired
	}
	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("twitch send-chat-message returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

type appTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

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

func (c *Client) CreateEventSubSubscription(ctx context.Context, appToken, eventType, version string, condition map[string]string, callbackURL, secret string) (string, error) {
	body := eventSubCreateRequest{
		Type:      eventType,
		Version:   version,
		Condition: condition,
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
