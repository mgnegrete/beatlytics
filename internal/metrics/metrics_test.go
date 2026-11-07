package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecordPlaysIncrementsPerArtist(t *testing.T) {
	RecordPlays("mt-u1", []string{"Radiohead", "Radiohead", "Bjork"})

	if got := testutil.ToFloat64(playsTotal.WithLabelValues("mt-u1", "Radiohead")); got != 2 {
		t.Errorf("Radiohead = %v, want 2", got)
	}
	if got := testutil.ToFloat64(playsTotal.WithLabelValues("mt-u1", "Bjork")); got != 1 {
		t.Errorf("Bjork = %v, want 1", got)
	}
	if got := testutil.ToFloat64(playsTotal.WithLabelValues("mt-other", "Radiohead")); got != 0 {
		t.Errorf("other user's counter = %v, want 0", got)
	}
}

func TestPollErrorByStage(t *testing.T) {
	before := testutil.ToFloat64(pollErrors.WithLabelValues("token"))
	PollError("token")
	PollError("token")
	if got := testutil.ToFloat64(pollErrors.WithLabelValues("token")) - before; got != 2 {
		t.Errorf("token errors delta = %v, want 2", got)
	}
	if got := testutil.ToFloat64(pollErrors.WithLabelValues("store")); got != 0 {
		t.Errorf("store errors = %v, want 0", got)
	}
}

func TestSpotifyRequestLabelsCode(t *testing.T) {
	SpotifyRequest("recently_played", 429)
	if got := testutil.ToFloat64(spotifyRequests.WithLabelValues("recently_played", "429")); got != 1 {
		t.Errorf("429 count = %v, want 1", got)
	}
}

func TestPollSucceededSetsLastIngest(t *testing.T) {
	if LastIngestUnix("mt-u2") != 0 {
		t.Fatal("expected 0 before any poll")
	}
	at := time.Unix(1700000000, 0)
	PollSucceeded("mt-u2", at)
	if got := LastIngestUnix("mt-u2"); got != 1700000000 {
		t.Errorf("LastIngestUnix = %d", got)
	}
	if got := testutil.ToFloat64(lastSuccess.WithLabelValues("mt-u2")); got != 1700000000 {
		t.Errorf("gauge = %v", got)
	}
}

func TestPollersRunningGauge(t *testing.T) {
	before := testutil.ToFloat64(pollersRunning)
	PollerStarted()
	PollerStarted()
	PollerStopped()
	if got := testutil.ToFloat64(pollersRunning) - before; got != 1 {
		t.Errorf("delta = %v, want 1", got)
	}
}

func TestPossibleGapCounter(t *testing.T) {
	before := testutil.ToFloat64(pollPossibleGap)
	PossibleGap()
	if got := testutil.ToFloat64(pollPossibleGap) - before; got != 1 {
		t.Errorf("delta = %v, want 1", got)
	}
}
