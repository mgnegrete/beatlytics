// Package importer loads Spotify "extended streaming history" exports into the play store.
package importer

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mgnegrete/beatlytics/internal/store"
)

const minPlayMs = 30000

// Store is the part of the play store the importer needs.
type Store interface {
	FirstLivePlay(userID string) (time.Time, bool, error)
	InsertPlays(userID string, plays []store.Play, cursorMs int64) ([]store.Play, error)
}

// Result summarizes one imported file.
type Result struct {
	Imported int
	Skipped  map[string]int // podcast, short, no_artist, after_live, duplicate
}

type entry struct {
	TS         string  `json:"ts"`
	MsPlayed   int     `json:"ms_played"`
	Track      *string `json:"master_metadata_track_name"`
	Artist     *string `json:"master_metadata_album_artist_name"`
	TrackURI   *string `json:"spotify_track_uri"`
	EpisodeURI *string `json:"spotify_episode_uri"`
}

// ImportFile imports one Streaming_History_Audio_*.json file. The whole file is
// validated before anything is written, so a malformed file inserts nothing.
// name is used only in error messages.
func ImportFile(st Store, userID, name string, r io.Reader) (Result, error) {
	res := Result{Skipped: map[string]int{}}

	var entries []entry
	if err := json.NewDecoder(r).Decode(&entries); err != nil {
		return res, fmt.Errorf("%s: invalid JSON: %w", name, err)
	}

	firstLive, hasLive, err := st.FirstLivePlay(userID)
	if err != nil {
		return res, fmt.Errorf("%s: %w", name, err)
	}

	plays := make([]store.Play, 0, len(entries))
	for i, e := range entries {
		at, err := time.Parse(time.RFC3339, e.TS)
		if err != nil {
			return res, fmt.Errorf("%s: entry %d: bad timestamp %q: %w", name, i, e.TS, err)
		}
		at = at.UTC()

		switch {
		case e.EpisodeURI != nil && *e.EpisodeURI != "", e.TrackURI == nil || *e.TrackURI == "":
			res.Skipped["podcast"]++
		case e.MsPlayed < minPlayMs:
			res.Skipped["short"]++
		case e.Artist == nil || *e.Artist == "" || e.Track == nil:
			res.Skipped["no_artist"]++
		case hasLive && !at.Before(firstLive):
			res.Skipped["after_live"]++
		default:
			ms := e.MsPlayed
			plays = append(plays, store.Play{
				UserID:     userID,
				TrackID:    strings.TrimPrefix(*e.TrackURI, "spotify:track:"),
				TrackName:  *e.Track,
				ArtistName: *e.Artist,
				PlayedAt:   at,
				MsPlayed:   &ms,
				Source:     "import",
			})
		}
	}

	inserted, err := st.InsertPlays(userID, plays, 0)
	if err != nil {
		return res, fmt.Errorf("%s: %w", name, err)
	}
	res.Imported = len(inserted)
	if dup := len(plays) - len(inserted); dup > 0 {
		res.Skipped["duplicate"] = dup
	}
	return res, nil
}
