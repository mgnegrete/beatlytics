package poller

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mgnegrete/beatlytics/internal/spotify"
	"github.com/mgnegrete/beatlytics/internal/store"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

type fakeSource struct {
	tokenErr  error
	fetchErr  error
	items     []spotify.TrackItem
	lastAfter int64
	calls     atomic.Int32
}

func (f *fakeSource) GetToken() (string, error) {
	f.calls.Add(1)
	if f.tokenErr != nil {
		return "", f.tokenErr
	}
	return "tok", nil
}

func (f *fakeSource) GetRecentlyPlayed(token string, afterMs int64) ([]spotify.TrackItem, error) {
	f.lastAfter = afterMs
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	return f.items, nil
}

type failingStore struct{ Store }

func (failingStore) InsertPlays(string, []store.Play, int64) ([]store.Play, error) {
	return nil, errors.New("disk full")
}

func newDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func item(id, name, artist, playedAt string) spotify.TrackItem {
	var artists []spotify.Artist
	if artist != "" {
		artists = []spotify.Artist{{ID: "a-" + artist, Name: artist}, {ID: "a-other", Name: "Second Artist"}}
	}
	return spotify.TrackItem{Track: spotify.Track{ID: id, Name: name, Artists: artists}, PlayedAt: playedAt}
}

// counterValue reads a counter from the default registry by metric name and labels.
func counterValue(t *testing.T, name string, labels map[string]string) float64 {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if matches(m, labels) {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func matches(m *dto.Metric, labels map[string]string) bool {
	got := map[string]string{}
	for _, l := range m.GetLabel() {
		got[l.GetName()] = l.GetValue()
	}
	for k, v := range labels {
		if got[k] != v {
			return false
		}
	}
	return true
}

func TestPollOnceStoresNewPlaysAndAdvancesCursor(t *testing.T) {
	db := newDB(t)
	src := &fakeSource{items: []spotify.TrackItem{
		item("t1", "One", "Alpha", "2026-11-06T10:00:00.500Z"),
		item("t2", "Two", "Beta", "2026-11-06T10:05:00.250Z"),
	}}

	if err := PollOnce("p-u1", src, db); err != nil {
		t.Fatal(err)
	}

	want, _ := time.Parse(time.RFC3339Nano, "2026-11-06T10:05:00.250Z")
	if c, _ := db.GetCursor("p-u1"); c != want.UnixMilli() {
		t.Fatalf("cursor = %d, want %d", c, want.UnixMilli())
	}
	artists, _ := db.TopArtists("p-u1", time.Time{}, 25)
	if len(artists) != 2 {
		t.Fatalf("stored artists %+v; want Alpha and Beta only (primary artist)", artists)
	}
	if got := counterValue(t, "spotify_plays_total", map[string]string{"user_id": "p-u1", "artist": "Alpha"}); got != 1 {
		t.Fatalf("Alpha counter = %v, want 1", got)
	}
	if got := counterValue(t, "spotify_plays_total", map[string]string{"user_id": "p-u1", "artist": "Second Artist"}); got != 0 {
		t.Fatalf("secondary artist counted: %v", got)
	}

	// The next poll must ask only for plays after the stored cursor.
	if err := PollOnce("p-u1", src, db); err != nil {
		t.Fatal(err)
	}
	if src.lastAfter != want.UnixMilli() {
		t.Fatalf("second poll asked after=%d, want %d", src.lastAfter, want.UnixMilli())
	}
}

func TestPollOnceNoNewPlaysLeavesCursorAndCountersUnchanged(t *testing.T) {
	db := newDB(t)
	src := &fakeSource{items: []spotify.TrackItem{item("t1", "One", "Gamma", "2026-11-06T10:00:00Z")}}
	PollOnce("p-u2", src, db)
	cursor, _ := db.GetCursor("p-u2")

	// Spotify returns the same play again (e.g. overlapping poll).
	if err := PollOnce("p-u2", src, db); err != nil {
		t.Fatal(err)
	}
	if c, _ := db.GetCursor("p-u2"); c != cursor {
		t.Fatalf("cursor changed: %d -> %d", cursor, c)
	}
	if got := counterValue(t, "spotify_plays_total", map[string]string{"user_id": "p-u2", "artist": "Gamma"}); got != 1 {
		t.Fatalf("counter = %v, want 1 (no double count)", got)
	}

	// And a poll that returns nothing at all.
	src.items = nil
	if err := PollOnce("p-u2", src, db); err != nil {
		t.Fatal(err)
	}
	if c, _ := db.GetCursor("p-u2"); c != cursor {
		t.Fatalf("cursor changed on empty poll: %d", c)
	}
}

func TestPollOnceSkipsItemsWithoutArtists(t *testing.T) {
	db := newDB(t)
	src := &fakeSource{items: []spotify.TrackItem{
		item("t1", "Local File", "", "2026-11-06T10:00:00Z"),
		item("", "No ID", "Delta", "2026-11-06T10:01:00Z"),
		item("t3", "Good", "Epsilon", "2026-11-06T10:02:00Z"),
	}}
	if err := PollOnce("p-u3", src, db); err != nil {
		t.Fatal(err)
	}
	artists, _ := db.TopArtists("p-u3", time.Time{}, 25)
	if len(artists) != 1 || artists[0].Name != "Epsilon" {
		t.Fatalf("got %+v, want only Epsilon", artists)
	}
}

func TestPollOnceRateLimitedDoesNotAdvanceCursor(t *testing.T) {
	db := newDB(t)
	db.InsertPlays("p-u4", nil, 12345)
	before := counterValue(t, "beatlytics_poll_errors_total", map[string]string{"stage": "spotify_fetch"})
	src := &fakeSource{fetchErr: &spotify.APIError{Status: 429, RetryAfter: 30 * time.Second}}

	err := PollOnce("p-u4", src, db)
	var apiErr *spotify.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 429 {
		t.Fatalf("err = %v, want the 429 APIError", err)
	}
	if c, _ := db.GetCursor("p-u4"); c != 12345 {
		t.Fatalf("cursor moved to %d", c)
	}
	after := counterValue(t, "beatlytics_poll_errors_total", map[string]string{"stage": "spotify_fetch"})
	if after-before != 1 {
		t.Fatalf("spotify_fetch errors delta = %v, want 1", after-before)
	}
}

func TestPollOnceTokenErrorCountsTokenStage(t *testing.T) {
	db := newDB(t)
	before := counterValue(t, "beatlytics_poll_errors_total", map[string]string{"stage": "token"})
	err := PollOnce("p-u5", &fakeSource{tokenErr: errors.New("boom")}, db)
	if err == nil {
		t.Fatal("expected error")
	}
	if d := counterValue(t, "beatlytics_poll_errors_total", map[string]string{"stage": "token"}) - before; d != 1 {
		t.Fatalf("token errors delta = %v, want 1", d)
	}
}

func TestPollOnceStoreFailureDoesNotIncrementCounters(t *testing.T) {
	db := newDB(t)
	before := counterValue(t, "beatlytics_poll_errors_total", map[string]string{"stage": "store"})
	src := &fakeSource{items: []spotify.TrackItem{item("t1", "One", "Zeta", "2026-11-06T10:00:00Z")}}

	if err := PollOnce("p-u6", src, failingStore{db}); err == nil {
		t.Fatal("expected error")
	}
	if d := counterValue(t, "beatlytics_poll_errors_total", map[string]string{"stage": "store"}) - before; d != 1 {
		t.Fatalf("store errors delta = %v, want 1", d)
	}
	if got := counterValue(t, "spotify_plays_total", map[string]string{"user_id": "p-u6", "artist": "Zeta"}); got != 0 {
		t.Fatalf("play counter incremented despite failed write: %v", got)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}

func TestManagerStopsOnInvalidGrant(t *testing.T) {
	m := NewManager(newDB(t), 10*time.Millisecond)
	src := &fakeSource{tokenErr: fmt.Errorf("refresh: %w", spotify.ErrInvalidGrant)}

	m.Start("m-u1", src)
	waitFor(t, func() bool { return src.calls.Load() >= 1 && !m.Running("m-u1") })

	n := src.calls.Load()
	time.Sleep(60 * time.Millisecond)
	if src.calls.Load() != n {
		t.Fatalf("poller kept polling after invalid_grant (%d -> %d calls)", n, src.calls.Load())
	}
}

func TestManagerStartIsIdempotent(t *testing.T) {
	m := NewManager(newDB(t), time.Hour)
	src := &fakeSource{}

	m.Start("m-u2", src)
	m.Start("m-u2", src)
	waitFor(t, func() bool { return src.calls.Load() >= 1 })
	time.Sleep(50 * time.Millisecond)
	if n := src.calls.Load(); n != 1 {
		t.Fatalf("source polled %d times by two Start calls, want 1", n)
	}
	if !m.Running("m-u2") {
		t.Fatal("expected poller running")
	}
}

func TestManagerStartAllStartsEveryUser(t *testing.T) {
	m := NewManager(newDB(t), time.Hour)
	srcs := map[string]*fakeSource{"a": {}, "b": {}}
	users := []store.User{{ID: "a"}, {ID: "b"}}

	m.StartAll(users, func(u store.User) Source { return srcs[u.ID] })
	waitFor(t, func() bool { return srcs["a"].calls.Load() >= 1 && srcs["b"].calls.Load() >= 1 })
}

func fullBatch(n int) []spotify.TrackItem {
	items := make([]spotify.TrackItem, n)
	base := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	for i := range items {
		items[i] = item(fmt.Sprintf("g%d", i), "Song", "GapArtist", base.Add(time.Duration(i)*time.Minute).Format(time.RFC3339Nano))
	}
	return items
}

func TestPollOnceFullBatchWithCursorFlagsPossibleGap(t *testing.T) {
	db := newDB(t)
	db.InsertPlays("g-u1", nil, 1000) // an existing cursor: not the first poll
	before := counterValue(t, "beatlytics_poll_possible_gap_total", nil)

	if err := PollOnce("g-u1", &fakeSource{items: fullBatch(50)}, db); err != nil {
		t.Fatal(err)
	}
	if d := counterValue(t, "beatlytics_poll_possible_gap_total", nil) - before; d != 1 {
		t.Fatalf("possible-gap delta = %v, want 1", d)
	}
}

func TestPollOnceFirstPollOrShortBatchDoesNotFlagGap(t *testing.T) {
	db := newDB(t)
	before := counterValue(t, "beatlytics_poll_possible_gap_total", nil)

	// First poll (no cursor) legitimately returns a full 50.
	if err := PollOnce("g-u2", &fakeSource{items: fullBatch(50)}, db); err != nil {
		t.Fatal(err)
	}
	// Later poll with fewer than 50 items.
	if err := PollOnce("g-u2", &fakeSource{items: fullBatch(49)}, db); err != nil {
		t.Fatal(err)
	}
	if d := counterValue(t, "beatlytics_poll_possible_gap_total", nil) - before; d != 0 {
		t.Fatalf("possible-gap delta = %v, want 0", d)
	}
}

func TestManagerStartSwapsSourceOnRunningPoller(t *testing.T) {
	m := NewManager(newDB(t), 10*time.Millisecond)
	oldSrc, newSrc := &fakeSource{}, &fakeSource{}

	m.Start("s-u1", oldSrc)
	waitFor(t, func() bool { return oldSrc.calls.Load() >= 1 })

	m.Start("s-u1", newSrc) // e.g. user signed in again with fresh credentials
	waitFor(t, func() bool { return newSrc.calls.Load() >= 1 })
}
