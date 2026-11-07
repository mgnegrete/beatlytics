package store

import (
	"path/filepath"
	"testing"
	"time"
)

func openTest(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func play(user, track, artist string, at time.Time, source string) Play {
	return Play{UserID: user, TrackID: track, TrackName: "name-" + track, ArtistName: artist, PlayedAt: at, Source: source}
}

func TestInsertPlaysIsIdempotent(t *testing.T) {
	db := openTest(t)
	now := time.Now().UTC().Truncate(time.Second)
	plays := []Play{play("u1", "t1", "A", now, "live"), play("u1", "t2", "B", now.Add(-time.Minute), "live")}

	got, err := db.InsertPlays("u1", plays, 0)
	if err != nil || len(got) != 2 {
		t.Fatalf("first insert: got %d rows, err %v; want 2", len(got), err)
	}
	got, err = db.InsertPlays("u1", plays, 0)
	if err != nil || len(got) != 0 {
		t.Fatalf("second insert: got %d rows, err %v; want 0", len(got), err)
	}
}

func TestInsertPlaysSetsCursor(t *testing.T) {
	db := openTest(t)
	now := time.Now().UTC().Truncate(time.Second)
	p := []Play{play("u1", "t1", "A", now, "live")}

	if _, err := db.InsertPlays("u1", p, 5000); err != nil {
		t.Fatal(err)
	}
	if c, _ := db.GetCursor("u1"); c != 5000 {
		t.Fatalf("cursor = %d, want 5000", c)
	}
	if _, err := db.InsertPlays("u1", nil, 0); err != nil {
		t.Fatal(err)
	}
	if c, _ := db.GetCursor("u1"); c != 5000 {
		t.Fatalf("cursor changed with cursorMs=0: %d", c)
	}
	if _, err := db.InsertPlays("u1", nil, 4000); err != nil {
		t.Fatal(err)
	}
	if c, _ := db.GetCursor("u1"); c != 5000 {
		t.Fatalf("cursor moved backwards: %d", c)
	}
	if c, _ := db.GetCursor("nobody"); c != 0 {
		t.Fatalf("unknown user cursor = %d, want 0", c)
	}
}

func TestFirstLivePlay(t *testing.T) {
	db := openTest(t)
	now := time.Now().UTC().Truncate(time.Second)

	if _, ok, err := db.FirstLivePlay("u1"); err != nil || ok {
		t.Fatalf("empty user: ok=%v err=%v; want false,nil", ok, err)
	}
	db.InsertPlays("u1", []Play{
		play("u1", "t1", "A", now.Add(-48*time.Hour), "import"),
		play("u1", "t2", "A", now.Add(-2*time.Hour), "live"),
		play("u1", "t3", "A", now.Add(-1*time.Hour), "live"),
	}, 0)
	got, ok, err := db.FirstLivePlay("u1")
	if err != nil || !ok || !got.Equal(now.Add(-2*time.Hour)) {
		t.Fatalf("got %v ok=%v err=%v; want %v", got, ok, err, now.Add(-2*time.Hour))
	}
}

func TestListUsers(t *testing.T) {
	db := openTest(t)
	if _, err := db.UpsertUser("sp1", "One", "r1"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertUser("sp2", "Two", "r2"); err != nil {
		t.Fatal(err)
	}
	users, err := db.ListUsers()
	if err != nil || len(users) != 2 {
		t.Fatalf("got %d users, err %v; want 2", len(users), err)
	}
	if users[0].RefreshToken == "" {
		t.Fatal("refresh token not loaded")
	}
}

func TestParseWindow(t *testing.T) {
	for _, s := range []string{"24h", "7d", "30d", "all"} {
		if w, ok := ParseWindow(s); !ok || string(w) != s {
			t.Errorf("ParseWindow(%q) = %q,%v", s, w, ok)
		}
	}
	for _, s := range []string{"", "1y", "7D"} {
		if _, ok := ParseWindow(s); ok {
			t.Errorf("ParseWindow(%q) accepted", s)
		}
	}
}

func TestSinceBoundaries(t *testing.T) {
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	if got := Window24h.Since(now); !got.Equal(now.Add(-24 * time.Hour)) {
		t.Errorf("24h since = %v", got)
	}
	if got := Window7d.Since(now); !got.Equal(now.Add(-7 * 24 * time.Hour)) {
		t.Errorf("7d since = %v", got)
	}
	if got := Window30d.Since(now); !got.Equal(now.Add(-30 * 24 * time.Hour)) {
		t.Errorf("30d since = %v", got)
	}
	if !WindowAll.Since(now).IsZero() {
		t.Error("all should have zero since")
	}
}

func TestTopArtistsRespectsWindow(t *testing.T) {
	db := openTest(t)
	now := time.Now().UTC().Truncate(time.Second)
	since := now.Add(-24 * time.Hour)
	db.InsertPlays("u1", []Play{
		play("u1", "t1", "Old", since.Add(-time.Second), "live"),
		play("u1", "t2", "Edge", since, "live"), // exactly at the boundary: included
		play("u1", "t3", "New", now, "live"),
		play("u1", "t4", "New", now.Add(-time.Minute), "live"),
	}, 0)

	got, err := db.TopArtists("u1", since, 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != (ArtistCount{"New", 2}) || got[1] != (ArtistCount{"Edge", 1}) {
		t.Fatalf("got %+v", got)
	}
	all, _ := db.TopArtists("u1", time.Time{}, 25)
	if len(all) != 3 {
		t.Fatalf("all window: got %+v, want 3 artists", all)
	}
}

func TestTopTracksGroupsByTrackAndArtist(t *testing.T) {
	db := openTest(t)
	now := time.Now().UTC().Truncate(time.Second)
	db.InsertPlays("u1", []Play{
		play("u1", "t1", "A", now, "live"),
		play("u1", "t1", "A", now.Add(-time.Hour), "live"), // same track, different time: counts twice
		play("u1", "t2", "B", now.Add(-2*time.Hour), "live"),
	}, 0)
	got, err := db.TopTracks("u1", time.Time{}, 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != (TrackCount{"name-t1", "A", 2}) || got[1] != (TrackCount{"name-t2", "B", 1}) {
		t.Fatalf("got %+v", got)
	}
}

func TestTopQueriesIsolateUsers(t *testing.T) {
	db := openTest(t)
	now := time.Now().UTC().Truncate(time.Second)
	db.InsertPlays("u1", []Play{play("u1", "t1", "Mine", now, "live")}, 0)
	db.InsertPlays("u2", []Play{play("u2", "t1", "Theirs", now, "live")}, 0)

	a, _ := db.TopArtists("u1", time.Time{}, 25)
	if len(a) != 1 || a[0].Name != "Mine" {
		t.Fatalf("artists leaked across users: %+v", a)
	}
	tr, _ := db.TopTracks("u1", time.Time{}, 25)
	if len(tr) != 1 || tr[0].Artist != "Mine" {
		t.Fatalf("tracks leaked across users: %+v", tr)
	}
}

func TestTopQueriesEmptyReturnsNonNilSlice(t *testing.T) {
	db := openTest(t)
	a, err := db.TopArtists("nobody", time.Time{}, 25)
	if err != nil || a == nil || len(a) != 0 {
		t.Fatalf("artists: %#v err %v", a, err)
	}
	tr, err := db.TopTracks("nobody", time.Time{}, 25)
	if err != nil || tr == nil || len(tr) != 0 {
		t.Fatalf("tracks: %#v err %v", tr, err)
	}
}

func TestLimitApplied(t *testing.T) {
	db := openTest(t)
	now := time.Now().UTC().Truncate(time.Second)
	var plays []Play
	for i := 0; i < 30; i++ {
		id := string(rune('a' + i))
		plays = append(plays, play("u1", "t"+id, "artist-"+id, now.Add(-time.Duration(i)*time.Minute), "live"))
	}
	db.InsertPlays("u1", plays, 0)
	a, _ := db.TopArtists("u1", time.Time{}, 25)
	if len(a) != 25 {
		t.Fatalf("got %d artists, want 25", len(a))
	}
}
