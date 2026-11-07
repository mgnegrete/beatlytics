package metrics

import (
	"log"
	"sort"
	"sync"
	"time"

	"github.com/mgnegrete/beatlytics/internal/spotify"
	"github.com/prometheus/client_golang/prometheus"
)

type ArtistCount struct {
	ID    string
	Name  string
	Plays int
}

type TrackCount struct {
	ID     string
	Name   string
	Artist string
	Plays  int
}

var MoodCount = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "spotify_mood_plays_total",
		Help: "Number of recently-played tracks classified into each mood bucket",
	},
	[]string{"user_id", "mood"},
)

func init() {
	prometheus.MustRegister(MoodCount)
}

var (
	muMood     sync.RWMutex
	moodByUser = map[string]map[string]int{}

	muArtists     sync.RWMutex
	artistsByUser = map[string][]ArtistCount{}

	muTracks     sync.RWMutex
	tracksByUser = map[string][]TrackCount{}

	muIngest         sync.RWMutex
	lastIngestByUser = map[string]int64{}

	muPoller      sync.Mutex
	pollerRunning = map[string]bool{}
)

func RecordData(userID string, moods map[string]int, artists []ArtistCount, tracks []TrackCount) {
	muMood.RLock()
	existing := moodByUser[userID]
	muMood.RUnlock()
	for mood := range existing {
		MoodCount.DeleteLabelValues(userID, mood)
	}
	for mood, count := range moods {
		MoodCount.WithLabelValues(userID, mood).Set(float64(count))
	}

	muMood.Lock()
	moodByUser[userID] = moods
	muMood.Unlock()

	muArtists.Lock()
	artistsByUser[userID] = artists
	muArtists.Unlock()

	muTracks.Lock()
	tracksByUser[userID] = tracks
	muTracks.Unlock()

	muIngest.Lock()
	lastIngestByUser[userID] = time.Now().Unix()
	muIngest.Unlock()
}

func MoodSnapshot(userID string) map[string]int {
	muMood.RLock()
	defer muMood.RUnlock()
	src := moodByUser[userID]
	out := make(map[string]int, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func ArtistsSnapshot(userID string) []ArtistCount {
	muArtists.RLock()
	defer muArtists.RUnlock()
	src := artistsByUser[userID]
	out := make([]ArtistCount, len(src))
	copy(out, src)
	return out
}

func TracksSnapshot(userID string) []TrackCount {
	muTracks.RLock()
	defer muTracks.RUnlock()
	src := tracksByUser[userID]
	out := make([]TrackCount, len(src))
	copy(out, src)
	return out
}

func LastIngestUnix(userID string) int64 {
	muIngest.RLock()
	defer muIngest.RUnlock()
	return lastIngestByUser[userID]
}

func StartPollerForUser(userID string, client *spotify.Client, interval time.Duration) {
	muPoller.Lock()
	defer muPoller.Unlock()
	if pollerRunning[userID] {
		return
	}
	pollerRunning[userID] = true
	go func() {
		pollUser(userID, client)
		for range time.Tick(interval) {
			pollUser(userID, client)
		}
	}()
}

func pollUser(userID string, client *spotify.Client) {
	token, err := client.GetToken()
	if err != nil {
		log.Printf("poller[%s]: get token: %v", userID, err)
		return
	}

	items, err := client.GetRecentlyPlayed(token)
	if err != nil {
		log.Printf("poller[%s]: recently played: %v", userID, err)
		return
	}

	// Count plays per track and per artist.
	trackPlays := map[string]int{}
	trackMeta := map[string]spotify.Track{}
	artistPlays := map[string]int{}
	artistMeta := map[string]spotify.Artist{}

	for _, item := range items {
		t := item.Track
		trackPlays[t.ID]++
		trackMeta[t.ID] = t
		for _, a := range t.Artists {
			artistPlays[a.ID]++
			artistMeta[a.ID] = a
		}
	}

	// Collect unique track IDs for batch audio-features call.
	trackIDs := make([]string, 0, len(trackPlays))
	for id := range trackPlays {
		trackIDs = append(trackIDs, id)
	}

	features, err := client.GetAudioFeatures(token, trackIDs)
	if err != nil {
		log.Printf("poller[%s]: audio features: %v", userID, err)
		return
	}

	// Classify each track into a mood bucket.
	moods := map[string]int{}
	for _, f := range features {
		if f.ID == "" {
			continue
		}
		mood := classifyMood(f)
		moods[mood] += trackPlays[f.ID]
	}

	// Build sorted top-25 artist list.
	artists := make([]ArtistCount, 0, len(artistPlays))
	for id, plays := range artistPlays {
		a := artistMeta[id]
		artists = append(artists, ArtistCount{ID: id, Name: a.Name, Plays: plays})
	}
	sort.Slice(artists, func(i, j int) bool { return artists[i].Plays > artists[j].Plays })
	if len(artists) > 25 {
		artists = artists[:25]
	}

	// Build sorted top-25 track list.
	tracks := make([]TrackCount, 0, len(trackPlays))
	for id, plays := range trackPlays {
		t := trackMeta[id]
		artist := ""
		if len(t.Artists) > 0 {
			artist = t.Artists[0].Name
		}
		tracks = append(tracks, TrackCount{ID: id, Name: t.Name, Artist: artist, Plays: plays})
	}
	sort.Slice(tracks, func(i, j int) bool { return tracks[i].Plays > tracks[j].Plays })
	if len(tracks) > 25 {
		tracks = tracks[:25]
	}

	RecordData(userID, moods, artists, tracks)
	log.Printf("poller[%s]: recorded %d moods, %d artists, %d tracks",
		userID, len(moods), len(artists), len(tracks))
}

func classifyMood(f spotify.AudioFeatures) string {
	switch {
	case f.Energy >= 0.7 && f.Danceability >= 0.7:
		return "Hype"
	case f.Energy >= 0.7 && f.Valence >= 0.5:
		return "Energetic"
	case f.Energy >= 0.7 && f.Valence < 0.5:
		return "Aggressive"
	case f.Valence >= 0.7:
		return "Happy"
	case f.Danceability >= 0.7:
		return "Danceable"
	case f.Instrumentalness >= 0.35:
		return "Focus"
	case f.Valence <= 0.35:
		return "Melancholic"
	case f.Energy <= 0.4:
		return "Chill"
	default:
		return "Neutral"
	}
}

