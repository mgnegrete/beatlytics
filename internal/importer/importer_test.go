package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mgnegrete/beatlytics/internal/store"
)

func openStore(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// seedLive records a live play so imports are cut off at 2026-10-01.
func seedLive(t *testing.T, db *store.DB) {
	t.Helper()
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	_, err := db.InsertPlays("u1", []store.Play{{UserID: "u1", TrackID: "live1", TrackName: "Live", ArtistName: "Zed", PlayedAt: at, Source: "live"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T) *os.File {
	t.Helper()
	f, err := os.Open("testdata/streaming_history.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestImportSkipsAndCounts(t *testing.T) {
	db := openStore(t)
	seedLive(t, db)

	res, err := ImportFile(db, "u1", "streaming_history.json", fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 2 {
		t.Errorf("Imported = %d, want 2", res.Imported)
	}
	want := map[string]int{"podcast": 1, "short": 1, "after_live": 1}
	for reason, n := range want {
		if res.Skipped[reason] != n {
			t.Errorf("Skipped[%s] = %d, want %d (all: %v)", reason, res.Skipped[reason], n, res.Skipped)
		}
	}
	artists, _ := db.TopArtists("u1", time.Time{}, 25)
	names := map[string]bool{}
	for _, a := range artists {
		names[a.Name] = true
	}
	if !names["Alpha"] || !names["Beta"] || names["Gamma"] || names["Delta"] {
		t.Errorf("unexpected artists after import: %+v", artists)
	}
}

func TestImportIsIdempotent(t *testing.T) {
	db := openStore(t)
	seedLive(t, db)
	first, err := ImportFile(db, "u1", "f.json", fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	f2, _ := os.Open("testdata/streaming_history.json")
	defer f2.Close()
	second, err := ImportFile(db, "u1", "f.json", f2)
	if err != nil {
		t.Fatal(err)
	}
	if second.Imported != 0 || second.Skipped["duplicate"] != first.Imported {
		t.Fatalf("second run: Imported=%d duplicate=%d; want 0 and %d", second.Imported, second.Skipped["duplicate"], first.Imported)
	}
}

func TestImportParsesOffsetTimestampsToUTC(t *testing.T) {
	db := openStore(t)
	if _, err := ImportFile(db, "u1", "f.json", fixture(t)); err != nil {
		t.Fatal(err)
	}
	// Song B is "2026-09-02T12:00:00+02:00" == 10:00:00Z. Only Beta's window should contain it.
	since := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	got, _ := db.TopArtists("u1", since, 25)
	found := false
	for _, a := range got {
		if a.Name == "Beta" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Beta not found at/after 10:00Z: %+v", got)
	}
	got, _ = db.TopArtists("u1", since.Add(time.Second), 25)
	for _, a := range got {
		if a.Name == "Beta" {
			t.Fatalf("Beta played later than 10:00:00Z: offset was not applied")
		}
	}
}

func TestImportMalformedTimestampAbortsWithFilename(t *testing.T) {
	db := openStore(t)
	bad := `[
	  {"ts":"2026-09-01T10:00:00Z","ms_played":200000,"master_metadata_track_name":"Ok","master_metadata_album_artist_name":"Fine","spotify_track_uri":"spotify:track:OK1"},
	  {"ts":"yesterday-ish","ms_played":200000,"master_metadata_track_name":"Bad","master_metadata_album_artist_name":"Broken","spotify_track_uri":"spotify:track:BAD"}
	]`
	_, err := ImportFile(db, "u1", "Streaming_History_Audio_2024.json", strings.NewReader(bad))
	if err == nil || !strings.Contains(err.Error(), "Streaming_History_Audio_2024.json") {
		t.Fatalf("err = %v, want one naming the file", err)
	}
	if a, _ := db.TopArtists("u1", time.Time{}, 25); len(a) != 0 {
		t.Fatalf("rows inserted despite malformed file: %+v", a)
	}
}

func TestImportMalformedJSONAbortsWithFilename(t *testing.T) {
	_, err := ImportFile(openStore(t), "u1", "broken.json", strings.NewReader(`{not json`))
	if err == nil || !strings.Contains(err.Error(), "broken.json") {
		t.Fatalf("err = %v, want one naming the file", err)
	}
}

func TestImportWithoutLivePlaysImportsEverythingEligible(t *testing.T) {
	db := openStore(t)
	res, err := ImportFile(db, "u1", "f.json", fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 3 || res.Skipped["after_live"] != 0 {
		t.Fatalf("Imported=%d skipped=%v; want 3 imported (Alpha, Beta, Delta) and no after_live", res.Imported, res.Skipped)
	}
}
