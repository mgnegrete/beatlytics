package http

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/mgnegrete/beatlytics/internal/auth"
	"github.com/mgnegrete/beatlytics/internal/metrics"
	"github.com/mgnegrete/beatlytics/internal/spotify"
	"github.com/mgnegrete/beatlytics/internal/store"
)

type Handlers struct {
	Sessions *auth.SessionStore
	Store    *store.DB
	Interval time.Duration
}

func (h *Handlers) LoginHandler(w http.ResponseWriter, r *http.Request) {
	state := randomState()
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})

	params := url.Values{
		"response_type": {"code"},
		"client_id":     {os.Getenv("SPOTIFY_CLIENT_ID")},
		"scope":         {"user-read-recently-played user-read-private"},
		"redirect_uri":  {os.Getenv("SPOTIFY_REDIRECT_URI")},
		"state":         {state},
	}
	http.Redirect(w, r, "https://accounts.spotify.com/authorize?"+params.Encode(), http.StatusTemporaryRedirect)
}

func (h *Handlers) CallbackHandler(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "oauth_state", Value: "", MaxAge: -1, Path: "/"})

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	clientID := os.Getenv("SPOTIFY_CLIENT_ID")
	clientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")

	tokens, err := spotify.ExchangeCode(clientID, clientSecret, code, os.Getenv("SPOTIFY_REDIRECT_URI"))
	if err != nil {
		http.Error(w, "token exchange failed", http.StatusInternalServerError)
		return
	}

	profile, err := spotify.GetProfile(tokens.AccessToken)
	if err != nil {
		http.Error(w, "profile fetch failed", http.StatusInternalServerError)
		return
	}

	user, err := h.Store.UpsertUser(profile.ID, profile.DisplayName, tokens.RefreshToken)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	client := spotify.NewClientForUser(clientID, clientSecret, tokens.RefreshToken)
	client.SetToken(tokens.AccessToken, time.Duration(tokens.ExpiresIn)*time.Second)
	metrics.StartPollerForUser(user.ID, client, h.Interval)

	if err := h.Sessions.CreateSession(w, user.ID); err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

func (h *Handlers) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	h.Sessions.DeleteSession(w, r)
	http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
}

func (h *Handlers) MeHandler(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserIDFromContext(r.Context())
	user, err := h.Store.GetUserByID(userID)
	if err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"id":          user.ID,
		"displayName": user.DisplayName,
	})
}

type moodEntry struct {
	Genre string `json:"genre"`
	Plays int    `json:"plays"`
}

func (h *Handlers) MoodsHandler(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserIDFromContext(r.Context())
	snapshot := metrics.MoodSnapshot(userID)
	entries := make([]moodEntry, 0, len(snapshot))
	for mood, plays := range snapshot {
		entries = append(entries, moodEntry{Genre: mood, Plays: plays})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

type artistEntry struct {
	Name  string `json:"name"`
	Plays int    `json:"plays"`
}

func (h *Handlers) ArtistsHandler(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserIDFromContext(r.Context())
	artists := metrics.ArtistsSnapshot(userID)
	entries := make([]artistEntry, 0, len(artists))
	for _, a := range artists {
		entries = append(entries, artistEntry{Name: a.Name, Plays: a.Plays})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

type trackEntry struct {
	Name   string `json:"name"`
	Artist string `json:"artist"`
	Plays  int    `json:"plays"`
}

func (h *Handlers) TracksHandler(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserIDFromContext(r.Context())
	tracks := metrics.TracksSnapshot(userID)
	entries := make([]trackEntry, 0, len(tracks))
	for _, t := range tracks {
		entries = append(entries, trackEntry{Name: t.Name, Artist: t.Artist, Plays: t.Plays})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

type healthResponse struct {
	Status         string `json:"status"`
	LastIngestUnix int64  `json:"lastIngestUnix"`
}

func (h *Handlers) HealthHandler(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserIDFromContext(r.Context())
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(healthResponse{
		Status:         "ok",
		LastIngestUnix: metrics.LastIngestUnix(userID),
	})
}

func randomState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}
