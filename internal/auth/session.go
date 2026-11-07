package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
	"sync"
)

const cookieName = "beatlytics_session"

type SessionStore struct {
	secret []byte
	mu     sync.RWMutex
	data   map[string]string // sessionID -> userID
}

func NewSessionStore(secret string) *SessionStore {
	return &SessionStore{
		secret: []byte(secret),
		data:   make(map[string]string),
	}
}

func (s *SessionStore) CreateSession(w http.ResponseWriter, userID string) error {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	sessionID := base64.URLEncoding.EncodeToString(b)

	s.mu.Lock()
	s.data[sessionID] = userID
	s.mu.Unlock()

	signed := sessionID + "." + s.sign(sessionID)
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    signed,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   60 * 60 * 24 * 30, // 30 days
	})
	return nil
}

func (s *SessionStore) GetUserID(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return "", false
	}

	parts := strings.SplitN(cookie.Value, ".", 2)
	if len(parts) != 2 {
		return "", false
	}
	sessionID, sig := parts[0], parts[1]

	if !hmac.Equal([]byte(sig), []byte(s.sign(sessionID))) {
		return "", false
	}

	s.mu.RLock()
	userID, ok := s.data[sessionID]
	s.mu.RUnlock()
	return userID, ok
}

func (s *SessionStore) DeleteSession(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return
	}
	parts := strings.SplitN(cookie.Value, ".", 2)
	if len(parts) == 2 {
		s.mu.Lock()
		delete(s.data, parts[0])
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:   cookieName,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}

func (s *SessionStore) sign(sessionID string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(sessionID))
	return base64.URLEncoding.EncodeToString(mac.Sum(nil))
}
