// Package poller incrementally ingests each user's recent Spotify plays.
package poller

import (
	"errors"
	"log"
	"sync"
	"time"

	"github.com/mgnegrete/beatlytics/internal/metrics"
	"github.com/mgnegrete/beatlytics/internal/spotify"
	"github.com/mgnegrete/beatlytics/internal/store"
)

// maxPage is the most plays Spotify returns per request (and in total).
const maxPage = 50

// Source is the part of the Spotify client the poller needs.
type Source interface {
	GetToken() (string, error)
	GetRecentlyPlayed(token string, afterMs int64) ([]spotify.TrackItem, error)
}

// Store is the part of the play store the poller needs.
type Store interface {
	GetCursor(userID string) (int64, error)
	InsertPlays(userID string, plays []store.Play, cursorMs int64) ([]store.Play, error)
}

// PollOnce fetches plays newer than the user's cursor, stores them in one
// transaction, and only then updates counters. A failed or rate-limited poll
// leaves the cursor untouched.
func PollOnce(userID string, src Source, st Store) error {
	start := time.Now()
	defer func() { metrics.ObservePoll(time.Since(start)) }()

	token, err := src.GetToken()
	if err != nil {
		metrics.PollError("token")
		return err
	}

	cursor, err := st.GetCursor(userID)
	if err != nil {
		metrics.PollError("store")
		return err
	}

	items, err := src.GetRecentlyPlayed(token, cursor)
	if err != nil {
		metrics.PollError("spotify_fetch")
		return err
	}

	// Spotify keeps only the last 50 plays. A full page after the first poll
	// means older plays since the cursor may be gone for good.
	if cursor > 0 && len(items) >= maxPage {
		metrics.PossibleGap()
		log.Printf("poller[%s]: full page of %d plays; some plays since the last poll may have been missed", userID, len(items))
	}

	plays := make([]store.Play, 0, len(items))
	var maxMs int64
	for _, it := range items {
		if it.Track.ID == "" || len(it.Track.Artists) == 0 {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, it.PlayedAt)
		if err != nil {
			continue
		}
		plays = append(plays, store.Play{
			UserID:     userID,
			TrackID:    it.Track.ID,
			TrackName:  it.Track.Name,
			ArtistName: it.Track.Artists[0].Name,
			PlayedAt:   at.UTC(),
			Source:     "live",
		})
		if ms := at.UnixMilli(); ms > maxMs {
			maxMs = ms
		}
	}

	inserted, err := st.InsertPlays(userID, plays, maxMs)
	if err != nil {
		metrics.PollError("store")
		return err
	}

	artists := make([]string, 0, len(inserted))
	for _, p := range inserted {
		artists = append(artists, p.ArtistName)
	}
	metrics.RecordPlays(userID, artists)
	metrics.PollSucceeded(userID, time.Now())
	if len(inserted) > 0 {
		log.Printf("poller[%s]: stored %d new plays", userID, len(inserted))
	}
	return nil
}

// Manager runs one polling goroutine per user.
type Manager struct {
	st       Store
	interval time.Duration

	mu      sync.Mutex
	running map[string]bool
	sources map[string]Source
}

func NewManager(st Store, interval time.Duration) *Manager {
	return &Manager{st: st, interval: interval, running: map[string]bool{}, sources: map[string]Source{}}
}

// Running reports whether the user's poller is active.
func (m *Manager) Running(userID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running[userID]
}

func (m *Manager) source(userID string) Source {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sources[userID]
}

// Start begins polling for the user. If a poller is already running it keeps
// running but switches to src, so fresh credentials from a new sign-in are used.
func (m *Manager) Start(userID string, src Source) {
	m.mu.Lock()
	m.sources[userID] = src
	if m.running[userID] {
		m.mu.Unlock()
		return
	}
	m.running[userID] = true
	m.mu.Unlock()

	metrics.PollerStarted()
	go m.loop(userID)
}

// StartAll starts a poller for every given user.
func (m *Manager) StartAll(users []store.User, sourceFor func(store.User) Source) {
	for _, u := range users {
		m.Start(u.ID, sourceFor(u))
	}
}

func (m *Manager) loop(userID string) {
	defer func() {
		m.mu.Lock()
		m.running[userID] = false
		m.mu.Unlock()
		metrics.PollerStopped()
	}()

	for {
		err := PollOnce(userID, m.source(userID), m.st)
		wait := m.interval
		if err != nil {
			if errors.Is(err, spotify.ErrInvalidGrant) {
				log.Printf("poller[%s]: Spotify rejected the refresh token; user must sign in again. Stopping.", userID)
				return
			}
			var apiErr *spotify.APIError
			if errors.As(err, &apiErr) && apiErr.RetryAfter > wait {
				wait = apiErr.RetryAfter
			}
			log.Printf("poller[%s]: %v", userID, err)
		}
		time.Sleep(wait)
	}
}
