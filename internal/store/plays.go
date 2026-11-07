package store

import (
	"database/sql"
	"errors"
	"time"
)

// Play is one listen of a track. Only the primary artist is recorded.
type Play struct {
	UserID     string
	TrackID    string
	TrackName  string
	ArtistName string
	PlayedAt   time.Time
	MsPlayed   *int   // nil for live plays: the live API does not report it
	Source     string // "live" or "import"
}

// InsertPlays stores plays in one transaction, ignoring ones already stored,
// and returns only the rows that were newly inserted. If cursorMs > 0 the
// user's cursor is raised to it in the same transaction (it never moves back).
func (d *DB) InsertPlays(userID string, plays []Play, cursorMs int64) ([]Play, error) {
	tx, err := d.sql.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO plays
		(user_id, track_id, played_at_ms, track_name, artist_name, ms_played, source)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	inserted := make([]Play, 0, len(plays))
	for _, p := range plays {
		var ms any
		if p.MsPlayed != nil {
			ms = *p.MsPlayed
		}
		res, err := stmt.Exec(userID, p.TrackID, p.PlayedAt.UnixMilli(), p.TrackName, p.ArtistName, ms, p.Source)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			inserted = append(inserted, p)
		}
	}

	if cursorMs > 0 {
		_, err := tx.Exec(`INSERT INTO poll_state (user_id, last_played_at_ms) VALUES (?, ?)
			ON CONFLICT(user_id) DO UPDATE SET last_played_at_ms = MAX(last_played_at_ms, excluded.last_played_at_ms)`,
			userID, cursorMs)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return inserted, nil
}

// GetCursor returns the latest played_at (unix ms) seen for the user, or 0.
func (d *DB) GetCursor(userID string) (int64, error) {
	var ms int64
	err := d.sql.QueryRow(`SELECT last_played_at_ms FROM poll_state WHERE user_id=?`, userID).Scan(&ms)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return ms, err
}

// FirstLivePlay returns the time of the user's earliest live play.
func (d *DB) FirstLivePlay(userID string) (time.Time, bool, error) {
	var ms sql.NullInt64
	err := d.sql.QueryRow(`SELECT MIN(played_at_ms) FROM plays WHERE user_id=? AND source='live'`, userID).Scan(&ms)
	if err != nil || !ms.Valid {
		return time.Time{}, false, err
	}
	return time.UnixMilli(ms.Int64).UTC(), true, nil
}

// ListUsers returns every known user, oldest first.
func (d *DB) ListUsers() ([]User, error) {
	rows, err := d.sql.Query(`SELECT id, spotify_id, display_name, refresh_token, created_at, updated_at FROM users ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.SpotifyID, &u.DisplayName, &u.RefreshToken, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// Window is a time range for top-N queries.
type Window string

const (
	Window24h Window = "24h"
	Window7d  Window = "7d"
	Window30d Window = "30d"
	WindowAll Window = "all"
)

// ParseWindow validates a window string from a request.
func ParseWindow(s string) (Window, bool) {
	switch w := Window(s); w {
	case Window24h, Window7d, Window30d, WindowAll:
		return w, true
	}
	return "", false
}

// Since returns the start of the window relative to now; zero means unbounded.
func (w Window) Since(now time.Time) time.Time {
	switch w {
	case Window24h:
		return now.Add(-24 * time.Hour)
	case Window7d:
		return now.Add(-7 * 24 * time.Hour)
	case Window30d:
		return now.Add(-30 * 24 * time.Hour)
	}
	return time.Time{}
}

type ArtistCount struct {
	Name  string
	Plays int
}

type TrackCount struct {
	Name   string
	Artist string
	Plays  int
}

func sinceMs(since time.Time) int64 {
	if since.IsZero() {
		return 0
	}
	return since.UnixMilli()
}

// TopArtists returns up to limit artists by play count since the given time.
func (d *DB) TopArtists(userID string, since time.Time, limit int) ([]ArtistCount, error) {
	rows, err := d.sql.Query(`SELECT artist_name, COUNT(*) AS n FROM plays
		WHERE user_id=? AND played_at_ms >= ?
		GROUP BY artist_name ORDER BY n DESC, artist_name ASC LIMIT ?`,
		userID, sinceMs(since), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ArtistCount{}
	for rows.Next() {
		var a ArtistCount
		if err := rows.Scan(&a.Name, &a.Plays); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// TopTracks returns up to limit tracks by play count since the given time.
func (d *DB) TopTracks(userID string, since time.Time, limit int) ([]TrackCount, error) {
	rows, err := d.sql.Query(`SELECT track_name, artist_name, COUNT(*) AS n FROM plays
		WHERE user_id=? AND played_at_ms >= ?
		GROUP BY track_id, track_name, artist_name ORDER BY n DESC, track_name ASC LIMIT ?`,
		userID, sinceMs(since), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []TrackCount{}
	for rows.Next() {
		var t TrackCount
		if err := rows.Scan(&t.Name, &t.Artist, &t.Plays); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
