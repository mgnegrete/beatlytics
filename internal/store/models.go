package store

import "time"

type User struct {
	ID           string
	SpotifyID    string
	DisplayName  string
	RefreshToken string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
