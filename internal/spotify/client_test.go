package spotify

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func testClient(srv *httptest.Server) *Client {
	c := NewClientForUser("id", "secret", "refresh")
	c.apiBase = srv.URL
	c.tokenURL = srv.URL + "/api/token"
	return c
}

func TestGetRecentlyPlayedSendsAfter(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Write([]byte(`{"items":[{"track":{"id":"t1","name":"S","artists":[{"id":"a1","name":"A"}]},"played_at":"2026-11-06T10:00:00.123Z"}]}`))
	}))
	defer srv.Close()
	c := testClient(srv)

	items, err := c.GetRecentlyPlayed("tok", 0)
	if err != nil || len(items) != 1 {
		t.Fatalf("items %d err %v", len(items), err)
	}
	if gotQuery != "limit=50" {
		t.Fatalf("query without cursor = %q", gotQuery)
	}
	if _, err := c.GetRecentlyPlayed("tok", 1700000000123); err != nil {
		t.Fatal(err)
	}
	if gotQuery != "limit=50&after=1700000000123" {
		t.Fatalf("query with cursor = %q", gotQuery)
	}
}

func TestAPIErrorCarriesRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		http.Error(w, "slow down", http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := testClient(srv)
	var seen []int
	c.Observe = func(endpoint string, code int) { seen = append(seen, code) }

	_, err := c.GetRecentlyPlayed("tok", 0)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err %v is not *APIError", err)
	}
	if apiErr.Status != 429 || apiErr.RetryAfter != 7*time.Second {
		t.Fatalf("got %+v", apiErr)
	}
	if len(seen) != 1 || seen[0] != 429 {
		t.Fatalf("observe calls %v", seen)
	}
}

func TestGetTokenInvalidGrant(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid_grant","error_description":"Refresh token revoked"}`))
	}))
	defer srv.Close()

	_, err := testClient(srv).GetToken()
	if !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("err %v does not wrap ErrInvalidGrant", err)
	}
}

func TestClientsHaveRequestTimeout(t *testing.T) {
	if NewClientForUser("id", "secret", "refresh").http.Timeout <= 0 {
		t.Fatal("per-user client has no HTTP timeout; a hung request would wedge its poller forever")
	}
	if NewClient().http.Timeout <= 0 {
		t.Fatal("NewClient has no HTTP timeout")
	}
}
