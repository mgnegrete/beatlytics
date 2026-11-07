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
