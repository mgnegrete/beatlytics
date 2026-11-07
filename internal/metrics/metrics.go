// Package metrics defines the Prometheus collectors for Beatlytics.
package metrics

import (
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	playsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "spotify_plays_total",
		Help: "Plays recorded from Spotify, by user and primary artist.",
	}, []string{"user_id", "artist"})

	pollDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "beatlytics_poll_duration_seconds",
		Help:    "Time taken by one poll of one user.",
		Buckets: prometheus.DefBuckets,
	})

	pollErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "beatlytics_poll_errors_total",
		Help: "Poll failures by stage (token, spotify_fetch, store).",
	}, []string{"stage"})

	spotifyRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "beatlytics_spotify_requests_total",
		Help: "Requests to Spotify by endpoint and HTTP status code (0 = transport error).",
	}, []string{"endpoint", "code"})

	lastSuccess = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "beatlytics_last_successful_poll_timestamp_seconds",
		Help: "Unix time of the last successful poll, by user.",
	}, []string{"user_id"})

	pollersRunning = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "beatlytics_pollers_running",
		Help: "Number of active per-user pollers.",
	})

	muIngest         sync.RWMutex
	lastIngestByUser = map[string]int64{}
)

func init() {
	prometheus.MustRegister(playsTotal, pollDuration, pollErrors, spotifyRequests, lastSuccess, pollersRunning)
}

// RecordPlays increments the per-artist play counter once per entry.
func RecordPlays(userID string, artists []string) {
	for _, a := range artists {
		playsTotal.WithLabelValues(userID, a).Inc()
	}
}

func ObservePoll(d time.Duration) { pollDuration.Observe(d.Seconds()) }

func PollError(stage string) { pollErrors.WithLabelValues(stage).Inc() }

func SpotifyRequest(endpoint string, code int) {
	spotifyRequests.WithLabelValues(endpoint, strconv.Itoa(code)).Inc()
}

// PollSucceeded records the time of the user's last successful poll.
func PollSucceeded(userID string, at time.Time) {
	lastSuccess.WithLabelValues(userID).Set(float64(at.Unix()))
	muIngest.Lock()
	lastIngestByUser[userID] = at.Unix()
	muIngest.Unlock()
}

// LastIngestUnix returns the unix time of the user's last successful poll, or 0.
func LastIngestUnix(userID string) int64 {
	muIngest.RLock()
	defer muIngest.RUnlock()
	return lastIngestByUser[userID]
}

func PollerStarted() { pollersRunning.Inc() }
func PollerStopped() { pollersRunning.Dec() }
