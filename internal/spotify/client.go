package spotify

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Client struct {
	clientID     string
	clientSecret string
	refreshToken string
	http         *http.Client

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time

	apiBase  string
	tokenURL string

	// Observe, if set, is called after every Spotify request with a short
	// endpoint name and the HTTP status code (0 if the request itself failed).
	Observe func(endpoint string, code int)
}

// requestTimeout bounds every Spotify request so a hung connection (e.g. after
// the machine sleeps) cannot wedge a poller forever.
const requestTimeout = 30 * time.Second

const (
	defaultAPIBase  = "https://api.spotify.com"
	defaultTokenURL = "https://accounts.spotify.com/api/token"
)

// ErrInvalidGrant is wrapped by GetToken when Spotify rejects the refresh
// token (revoked or expired); the user must sign in again.
var ErrInvalidGrant = errors.New("invalid_grant")

// APIError is returned for any non-200 response from the Spotify Web API.
type APIError struct {
	Status     int
	RetryAfter time.Duration
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("spotify api error (%d): %s", e.Status, e.Body)
}

func (c *Client) observe(endpoint string, code int) {
	if c.Observe != nil {
		c.Observe(endpoint, code)
	}
}

func NewClient() *Client {
	return &Client{
		clientID:     os.Getenv("SPOTIFY_CLIENT_ID"),
		clientSecret: os.Getenv("SPOTIFY_CLIENT_SECRET"),
		refreshToken: os.Getenv("SPOTIFY_REFRESH_TOKEN"),
		http:         &http.Client{Timeout: requestTimeout},
		apiBase:      defaultAPIBase,
		tokenURL:     defaultTokenURL,
	}
}

func NewClientForUser(clientID, clientSecret, refreshToken string) *Client {
	return &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		refreshToken: refreshToken,
		http:         &http.Client{Timeout: requestTimeout},
		apiBase:      defaultAPIBase,
		tokenURL:     defaultTokenURL,
	}
}

// SetToken seeds the token cache with an already-obtained access token.
func (c *Client) SetToken(accessToken string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.accessToken = accessToken
	c.tokenExpiry = time.Now().Add(ttl)
}

// ExchangeCode exchanges an authorization code for access + refresh tokens.
func ExchangeCode(clientID, clientSecret, code, redirectURI string) (*TokenPair, error) {
	body := url.Values{}
	body.Set("grant_type", "authorization_code")
	body.Set("code", code)
	body.Set("redirect_uri", redirectURI)

	req, err := http.NewRequest("POST", "https://accounts.spotify.com/api/token", strings.NewReader(body.Encode()))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(clientID, clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("code exchange failed (%d): %s", resp.StatusCode, b)
	}

	var t tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		ExpiresIn:    t.ExpiresIn,
	}, nil
}

// GetProfile fetches the current user's Spotify profile.
func GetProfile(accessToken string) (*Profile, error) {
	req, err := http.NewRequest("GET", "https://api.spotify.com/v1/me", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("profile fetch failed (%d): %s", resp.StatusCode, b)
	}

	var p Profile
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// TokenPair holds the tokens returned by the authorization code exchange.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}

// Profile is the subset of /v1/me used during sign-in.
type Profile struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// GetToken returns a cached access token, refreshing only when within 60s of expiry.
func (c *Client) GetToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.accessToken != "" && time.Now().Before(c.tokenExpiry.Add(-60*time.Second)) {
		return c.accessToken, nil
	}

	body := url.Values{}
	body.Set("grant_type", "refresh_token")
	body.Set("refresh_token", c.refreshToken)

	req, err := http.NewRequest("POST", c.tokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(c.clientID, c.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		c.observe("token", 0)
		return "", err
	}
	defer resp.Body.Close()
	c.observe("token", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		if strings.Contains(string(b), "invalid_grant") {
			return "", fmt.Errorf("token request failed (%d): %w: %s", resp.StatusCode, ErrInvalidGrant, b)
		}
		return "", fmt.Errorf("token request failed (%d): %s", resp.StatusCode, b)
	}

	var t tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return "", err
	}

	c.accessToken = t.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(t.ExpiresIn) * time.Second)
	return c.accessToken, nil
}

// GetRecentlyPlayed returns up to 50 recent plays. If afterMs > 0 only plays
// after that unix-millisecond cursor are returned.
func (c *Client) GetRecentlyPlayed(token string, afterMs int64) ([]TrackItem, error) {
	u := c.apiBase + "/v1/me/player/recently-played?limit=50"
	if afterMs > 0 {
		u += "&after=" + strconv.FormatInt(afterMs, 10)
	}
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		c.observe("recently_played", 0)
		return nil, err
	}
	defer resp.Body.Close()
	c.observe("recently_played", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		apiErr := &APIError{Status: resp.StatusCode, Body: strings.TrimSpace(string(b))}
		if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil {
			apiErr.RetryAfter = time.Duration(secs) * time.Second
		}
		return nil, apiErr
	}

	var result RecentlyPlayedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Items, nil
}
