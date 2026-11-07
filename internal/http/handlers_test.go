package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mgnegrete/beatlytics/internal/auth"
	"github.com/mgnegrete/beatlytics/internal/store"
)

func testHandlers(t *testing.T) *Handlers {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	return &Handlers{Sessions: auth.NewSessionStore("test-secret"), Store: db}
}

func authedGet(h http.HandlerFunc, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", target, nil)
	req = req.WithContext(context.WithValue(req.Context(), userIDKey, "u1"))
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func seed(t *testing.T, h *Handlers, artist, track string, at time.Time) {
	t.Helper()
	_, err := h.Store.InsertPlays("u1", []store.Play{{
		UserID: "u1", TrackID: track, TrackName: "Song " + track, ArtistName: artist, PlayedAt: at, Source: "live",
	}}, 0)
	if err != nil {
		t.Fatal(err)
	}
}

func TestArtistsDefaultsTo7d(t *testing.T) {
	h := testHandlers(t)
	now := time.Now()
	seed(t, h, "Recent", "t1", now.Add(-24*time.Hour))
	seed(t, h, "Old", "t2", now.Add(-10*24*time.Hour))

	rec := authedGet(h.ArtistsHandler, "/api/artists")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var got []artistEntry
	json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 || got[0].Name != "Recent" || got[0].Plays != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestArtistsInvalidWindowReturns400(t *testing.T) {
	h := testHandlers(t)
	rec := authedGet(h.ArtistsHandler, "/api/artists?window=1y")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "24h, 7d, 30d or all") {
		t.Fatalf("body %q does not list valid windows", rec.Body.String())
	}
}

func TestArtistsEmptyReturnsEmptyArray(t *testing.T) {
	h := testHandlers(t)
	rec := authedGet(h.ArtistsHandler, "/api/artists")
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("body %q, want []", rec.Body.String())
	}
	rec = authedGet(h.TracksHandler, "/api/tracks")
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("tracks body %q, want []", rec.Body.String())
	}
}

func TestTracksAllWindow(t *testing.T) {
	h := testHandlers(t)
	seed(t, h, "Ancient", "t1", time.Now().Add(-400*24*time.Hour))

	rec := authedGet(h.TracksHandler, "/api/tracks?window=all")
	var got []trackEntry
	json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 || got[0].Name != "Song t1" || got[0].Artist != "Ancient" {
		t.Fatalf("got %+v", got)
	}
	rec = authedGet(h.TracksHandler, "/api/tracks?window=30d")
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("30d should exclude a 400-day-old play, got %q", rec.Body.String())
	}
}

func TestUnauthenticatedReturns401(t *testing.T) {
	h := testHandlers(t)
	router := NewRouter(h)
	for _, path := range []string{"/api/artists", "/api/tracks", "/api/me", "/api/health"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401", path, rec.Code)
		}
	}
}

func TestGenresRouteRemoved(t *testing.T) {
	router := NewRouter(testHandlers(t))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/api/genres", nil))
	if body := rec.Body.String(); strings.Contains(body, "unauthorized") {
		t.Fatalf("/api/genres still routed behind auth: %q", body)
	}
}
