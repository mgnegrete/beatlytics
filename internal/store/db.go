package store

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type DB struct {
	sql *sql.DB
}

func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		return nil, err
	}
	return &DB{sql: db}, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id           TEXT PRIMARY KEY,
			spotify_id   TEXT UNIQUE NOT NULL,
			display_name TEXT NOT NULL,
			refresh_token TEXT NOT NULL,
			created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

func (d *DB) UpsertUser(spotifyID, displayName, refreshToken string) (*User, error) {
	existing, err := d.GetUserBySpotifyID(spotifyID)
	if err == nil {
		_, err = d.sql.Exec(
			`UPDATE users SET display_name=?, refresh_token=?, updated_at=? WHERE spotify_id=?`,
			displayName, refreshToken, time.Now(), spotifyID,
		)
		if err != nil {
			return nil, err
		}
		existing.DisplayName = displayName
		existing.RefreshToken = refreshToken
		return existing, nil
	}

	id := uuid.New().String()
	now := time.Now()
	_, err = d.sql.Exec(
		`INSERT INTO users (id, spotify_id, display_name, refresh_token, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, spotifyID, displayName, refreshToken, now, now,
	)
	if err != nil {
		return nil, err
	}
	return &User{
		ID:           id,
		SpotifyID:    spotifyID,
		DisplayName:  displayName,
		RefreshToken: refreshToken,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func (d *DB) GetUserByID(id string) (*User, error) {
	row := d.sql.QueryRow(
		`SELECT id, spotify_id, display_name, refresh_token, created_at, updated_at FROM users WHERE id=?`, id,
	)
	return scanUser(row)
}

func (d *DB) GetUserBySpotifyID(spotifyID string) (*User, error) {
	row := d.sql.QueryRow(
		`SELECT id, spotify_id, display_name, refresh_token, created_at, updated_at FROM users WHERE spotify_id=?`, spotifyID,
	)
	return scanUser(row)
}

func scanUser(row *sql.Row) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.SpotifyID, &u.DisplayName, &u.RefreshToken, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}
