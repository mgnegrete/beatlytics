# Play History and Monitoring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist every play in SQLite, serve windowed top artists/songs, export Prometheus counters and poller health metrics, provision Prometheus and Grafana, and import Spotify extended streaming history.

**Architecture:** SQLite (`plays`, `poll_state`) is the source of truth. A new `internal/poller` package polls Spotify incrementally (`after=<cursor>`), writes new plays in one transaction, then bumps Prometheus counters for the rows actually inserted. HTTP handlers query SQLite by window. A separate `monitoring/` compose stack runs Prometheus and Grafana; the Go backend stays on the host.

**Tech Stack:** Go 1.25, `modernc.org/sqlite`, `prometheus/client_golang` (incl. `testutil`), Next.js 15, Docker Compose, Prometheus, Grafana.

**Spec:** `docs/superpowers/specs/2026-11-06-play-history-and-monitoring-design.md`

## Global Constraints

- Plays are attributed to the **primary (first-listed) artist** only.
- Windows: `24h`, `7d`, `30d`, `all`; API default `7d`; lists return the top **25**.
- Import skips podcasts/episodes, `ms_played < 30000`, and entries at or after the user's first live play.
- Metric names exactly: `spotify_plays_total{user_id,artist}`, `beatlytics_poll_duration_seconds`, `beatlytics_poll_errors_total{stage}` (stage: `token`, `spotify_fetch`, `store`), `beatlytics_spotify_requests_total{endpoint,code}`, `beatlytics_last_successful_poll_timestamp_seconds{user_id}`, `beatlytics_pollers_running`. `spotify_mood_plays_total`, `/api/genres`, `GetAudioFeatures` and the mood UI are deleted. Tracks are not exported to Prometheus.
- Cursor and counters change only after the DB transaction commits.
- Prometheus: scrape `host.docker.internal:8082/metrics` every 15s, retention `1y`, port 9090, `extra_hosts: host.docker.internal:host-gateway`. Grafana: port 3001, admin password from `GF_SECURITY_ADMIN_PASSWORD` (default `admin`).
- Alert rules: `PollerStale` (no successful poll for 15m), `SpotifyRateLimited` (any 429). No Alertmanager.
- Backend port stays `PORT` from `.env` (8082 locally). Module path `github.com/mgnegrete/beatlytics`.

## Review Focus

- Empty database: window endpoints return `[]`, never `null` (Task 3).
- User isolation: one user's plays never appear in another user's results or counters (Task 2).
- Recently-played items with no artists or an empty track are skipped, not a panic (Task 4).
- A poll that returns no new plays leaves the cursor and counters unchanged (Task 4).
- Export timestamps in any UTC offset parse to UTC, and a malformed timestamp aborts only that file with its name in the error (Task 5).

---

## File Structure

- `internal/store/db.go` (modify): migrations only. `internal/store/plays.go` (create): `Play`, `InsertPlays`, cursor, `FirstLivePlay`, `ListUsers`, windows, top queries.
- `internal/spotify/client.go`, `models.go` (modify): incremental fetch, typed errors, request hook; audio features removed.
- `internal/metrics/metrics.go` (rewrite): collectors only; snapshots and old poller deleted.
- `internal/poller/poller.go` (create): `PollOnce`, `Manager`.
- `internal/importer/importer.go` (create), `cmd/import/main.go` (create).
- `internal/http/handlers.go`, `router.go`, `cmd/server/main.go` (modify).
- `web/lib/api.ts`, `web/components/BeatlyticsDashboard.tsx` (modify).
- `monitoring/**` (create). `README.md`, `CLAUDE.md`, `TODO.md`, `change.log` (modify).

---

### Task 1: Play store, cursor, user listing

**Files:**
- Modify: `internal/store/db.go` (add tables to `migrate`)
- Create: `internal/store/plays.go`, `internal/store/plays_test.go`

**Interfaces:**
- Produces:
  - `type Play struct { UserID, TrackID, TrackName, ArtistName string; PlayedAt time.Time; MsPlayed *int; Source string }` (`Source` is `"live"` or `"import"`)
  - `func (d *DB) InsertPlays(userID string, plays []Play, cursorMs int64) ([]Play, error)`: one transaction, `INSERT OR IGNORE`, returns only the rows actually inserted; if `cursorMs > 0` it also upserts `poll_state` in the same transaction.
  - `func (d *DB) GetCursor(userID string) (int64, error)`: 0 when none.
  - `func (d *DB) FirstLivePlay(userID string) (time.Time, bool, error)`
  - `func (d *DB) ListUsers() ([]User, error)`
  - Tests use `Open(filepath.Join(t.TempDir(), "t.db"))`.

- [ ] **Step 1: Write failing tests** in `plays_test.go`: `TestInsertPlaysIsIdempotent` (inserting the same two plays twice returns 2 then 0 inserted rows); `TestInsertPlaysSetsCursor` (cursor equals `cursorMs` after insert; unchanged when `cursorMs == 0`; cursor only moves forward if a smaller `cursorMs` is passed later); `TestFirstLivePlay` (ignores `import` rows, `false` for a user with none); `TestListUsers`.
- [ ] **Step 2:** Run `go test ./internal/store/ -run 'Insert|FirstLive|ListUsers'`. Expected: FAIL, undefined symbols.
- [ ] **Step 3:** Add to `migrate`: `plays` (columns per spec; `UNIQUE(user_id, track_id, played_at)`; index on `(user_id, played_at)`) and `poll_state(user_id TEXT PRIMARY KEY, last_played_at_ms INTEGER NOT NULL)`. Implement the functions above in `plays.go`; store `played_at` as UTC.
- [ ] **Step 4:** Run `go test ./internal/store/`. Expected: PASS.
- [ ] **Step 5:** `git add internal/store && git commit -m "feat(store): add plays table, cursor, user listing"`

### Task 2: Windowed top queries

**Files:**
- Modify: `internal/store/plays.go`; Test: `internal/store/plays_test.go`

**Interfaces:**
- Produces:
  - `type Window string` with consts `Window24h="24h"`, `Window7d="7d"`, `Window30d="30d"`, `WindowAll="all"`
  - `func ParseWindow(s string) (Window, bool)`; `func (w Window) Since(now time.Time) time.Time` (zero time for `all`)
  - `type ArtistCount struct { Name string; Plays int }`, `type TrackCount struct { Name, Artist string; Plays int }`
  - `func (d *DB) TopArtists(userID string, since time.Time, limit int) ([]ArtistCount, error)`; `TopTracks` likewise. Ordered by plays desc, then name asc; always non-nil slices.

- [ ] **Step 1: Write failing tests:** `TestParseWindow` (the four values valid, `""` and `"1y"` invalid); `TestSinceBoundaries` (a play exactly at `since` is included); `TestTopArtistsRespectsWindow`; `TestTopTracksGroupsByTrackAndArtist`; `TestTopQueriesIsolateUsers` (user B's plays never counted for user A); `TestTopQueriesEmptyReturnsNonNilSlice`; `TestLimitApplied` (30 distinct artists, limit 25 returns 25).
- [ ] **Step 2:** Run `go test ./internal/store/`. Expected: FAIL on the new symbols.
- [ ] **Step 3:** Implement with `GROUP BY` queries filtered on `user_id` and `played_at >= since` (omit the filter when `since.IsZero()`).
- [ ] **Step 4:** Run `go test ./internal/store/`. Expected: PASS.
- [ ] **Step 5:** `git commit -am "feat(store): windowed top artists and tracks"`

### Task 3: Windowed API, moods removed

**Files:**
- Modify: `internal/http/handlers.go`, `internal/http/router.go`
- Test: `internal/http/handlers_test.go` (create)

**Interfaces:**
- Consumes: Task 2 (`ParseWindow`, `Since`, `TopArtists`, `TopTracks`).
- Produces: `ArtistsHandler`/`TracksHandler` read `?window=` (default `7d`), query `h.Store`, return `[{name,plays}]` / `[{name,artist,plays}]`. Invalid window: 400 with `{"error":"invalid window; use 24h, 7d, 30d or all"}`. Delete `MoodsHandler`, `moodEntry` and the `/api/genres` route. `HealthHandler` unchanged.

- [ ] **Step 1: Write failing tests** using `httptest`, a temp-DB `store.DB`, and a context carrying the user ID (use `context.WithValue(ctx, userIDKey, id)` from inside package `http`): `TestArtistsDefaultsTo7d`, `TestArtistsInvalidWindowReturns400`, `TestArtistsEmptyReturnsEmptyArray` (body is `[]`, not `null`), `TestTracksAllWindow`, `TestUnauthenticatedReturns401` (via `NewRouter`).
- [ ] **Step 2:** Run `go test ./internal/http/`. Expected: FAIL.
- [ ] **Step 3:** Implement the handlers and router change. The metrics package still compiles untouched at this point (the old poller keeps running and is replaced in Task 4).
- [ ] **Step 4:** Run `go build ./... && go test ./...`. Expected: PASS.
- [ ] **Step 5:** `git commit -am "feat(api): windowed artists and tracks from SQLite; remove moods endpoint"`

### Task 4: Live ingestion pipeline

**Files:**
- Modify: `internal/spotify/client.go`, `internal/spotify/models.go`, `internal/http/handlers.go` (callback), `cmd/server/main.go`
- Rewrite: `internal/metrics/metrics.go`
- Create: `internal/poller/poller.go`; Tests: `internal/poller/poller_test.go`, `internal/metrics/metrics_test.go`, `internal/spotify/client_test.go`

**Interfaces:**
- Produces (spotify):
  - `func (c *Client) GetRecentlyPlayed(token string, afterMs int64) ([]TrackItem, error)`: `limit=50`, adds `&after=` when `afterMs > 0`.
  - `type APIError struct { Status int; RetryAfter time.Duration; Body string }` with `Error() string`; returned for any non-200 from the API (parse `Retry-After` seconds on 429).
  - `var ErrInvalidGrant = errors.New("invalid_grant")`; `GetToken` returns an error wrapping it (`errors.Is`) when the token endpoint body contains `invalid_grant`.
  - `Client.Observe func(endpoint string, code int)` (optional hook, called after each API request including token refresh).
  - `GetAudioFeatures`, `AudioFeatures`, `AudioFeaturesResponse` deleted.
- Produces (metrics): collectors registered in `init`, plus `RecordPlays(userID string, artists []string)`, `ObservePoll(d time.Duration)`, `PollError(stage string)`, `SpotifyRequest(endpoint string, code int)`, `PollSucceeded(userID string, at time.Time)`, `LastIngestUnix(userID string) int64`, `PollerStarted()`, `PollerStopped()`. Everything else in the old file (snapshots, `RecordData`, `StartPollerForUser`, `pollUser`, `classifyMood`, mood gauge) is deleted.
- Produces (poller):
  - `type Source interface { GetToken() (string, error); GetRecentlyPlayed(token string, afterMs int64) ([]spotify.TrackItem, error) }`
  - `type Store interface { GetCursor(userID string) (int64, error); InsertPlays(userID string, plays []store.Play, cursorMs int64) ([]store.Play, error) }`
  - `func PollOnce(userID string, src Source, st Store) error`
  - `type Manager`; `func NewManager(st *store.DB, interval time.Duration) *Manager`; `func (m *Manager) Start(userID string, src Source)` (idempotent per user); `func (m *Manager) StartAll(users []store.User, sourceFor func(store.User) Source)`.

- [ ] **Step 1: Write failing tests.**
  - `client_test.go` (point the client at an `httptest` server via an unexported base-URL field): `TestGetRecentlyPlayedSendsAfter`, `TestAPIErrorCarriesRetryAfter`, `TestGetTokenInvalidGrant`.
  - `metrics_test.go` using `testutil.ToFloat64`: `TestRecordPlaysIncrementsPerArtist`, `TestPollErrorByStage`, `TestPollSucceededSetsLastIngest`.
  - `poller_test.go` with a fake `Source` and a real temp-DB store: `TestPollOnceStoresNewPlaysAndAdvancesCursor` (cursor becomes the max `played_at` in ms; counter incremented once per new play, by primary artist); `TestPollOnceNoNewPlaysLeavesCursorAndCountersUnchanged` (the fake returns plays already stored); `TestPollOnceSkipsItemsWithoutArtists` (no panic, not counted); `TestPollOnceRateLimitedDoesNotAdvanceCursor` (fake returns `*spotify.APIError{Status:429}`; cursor unchanged; `spotify_fetch` error counted); `TestPollOnceTokenErrorCountsTokenStage`; `TestPollOnceStoreFailureDoesNotIncrementCounters` (fake `Store` whose `InsertPlays` returns an error; `store` stage counted; no play counters change); `TestManagerStopsOnInvalidGrant`; `TestManagerStartIsIdempotent`.
- [ ] **Step 2:** Run `go test ./internal/...`. Expected: FAIL (undefined).
- [ ] **Step 3:** Implement spotify changes, then the metrics rewrite, then `poller`. `PollOnce` flow: token, `GetCursor`, `GetRecentlyPlayed(token, cursor)`, map items to `store.Play` (`Source:"live"`, primary artist, skip items with no artist or empty ID), `InsertPlays` (cursor = max `played_at` ms, or 0 if nothing fetched), then `metrics.RecordPlays` for the returned inserted rows, `metrics.PollSucceeded`. Time the whole poll with `ObservePoll`. `Manager` loop: poll immediately, then every `interval`; if the error is a 429 `*APIError`, wait `max(interval, RetryAfter)`; on `errors.Is(err, spotify.ErrInvalidGrant)` log "user must sign in again" and stop that poller (`PollerStopped`).
- [ ] **Step 4:** Wire up: the OAuth callback calls `h.Pollers.Start(user.ID, client)` (add `Pollers *poller.Manager` to `Handlers`, drop `Interval`); in `main.go` build the manager, set `client.Observe = metrics.SpotifyRequest` for every client created, then call `StartAll(db.ListUsers(), ...)` using each user's stored refresh token and `SPOTIFY_CLIENT_ID/SECRET`.
- [ ] **Step 5:** Run `go build ./... && go test ./...`. Expected: PASS. Then run the server, confirm `curl localhost:8082/metrics | grep -c beatlytics_` is non-zero.
- [ ] **Step 6:** `git add -A internal cmd && git commit -m "feat: incremental live ingestion with Prometheus counters and poller metrics"`

### Task 5: History importer

**Files:**
- Create: `internal/importer/importer.go`, `internal/importer/importer_test.go`, `internal/importer/testdata/streaming_history.json`, `cmd/import/main.go`

**Interfaces:**
- Consumes: Task 1 (`Play`, `InsertPlays`, `FirstLivePlay`, `ListUsers`).
- Produces:
  - `type Store interface { FirstLivePlay(userID string) (time.Time, bool, error); InsertPlays(userID string, plays []store.Play, cursorMs int64) ([]store.Play, error) }`
  - `type Result struct { Imported int; Skipped map[string]int }`; skip reasons: `podcast`, `short`, `after_live`, `duplicate`.
  - `func ImportFile(st Store, userID, name string, r io.Reader) (Result, error)`: `name` is used in errors. Export fields read: `ts`, `ms_played`, `master_metadata_track_name`, `master_metadata_album_artist_name`, `spotify_track_uri` (track ID is the part after `spotify:track:`), `spotify_episode_uri`. Inserts with `Source:"import"` and `cursorMs` 0.
  - CLI: `go run ./cmd/import --db beatlytics.db --user <id> <file-or-glob>...`; with no `--user` it prints the users from `ListUsers`.

- [ ] **Step 1: Write failing tests** against the fixture (a mix of valid tracks, a podcast entry with null track URI, a 10s play, a play after a fixed first-live time, and one with a `+02:00` offset): `TestImportSkipsAndCounts` (exact `Result`), `TestImportIsIdempotent` (second run: `Imported==0`, `duplicate` count equals the first run's imported), `TestImportParsesOffsetTimestampsToUTC`, `TestImportMalformedTimestampAbortsWithFilename` (error contains `name`, and nothing was inserted), `TestImportWithoutLivePlaysImportsEverythingEligible`.
- [ ] **Step 2:** Run `go test ./internal/importer/`. Expected: FAIL.
- [ ] **Step 3:** Implement: decode the whole file first, validate every entry, and only then make a single `InsertPlays` call so a malformed file inserts nothing. Compute `duplicate` as eligible entries minus inserted. Write the CLI around it, printing totals and skip reasons per file.
- [ ] **Step 4:** Run `go test ./... && go build ./cmd/import`. Expected: PASS.
- [ ] **Step 5:** `git add -A internal cmd docs && git commit -m "feat: import Spotify extended streaming history"`

### Task 6: Dashboard window selector, moods removed

**Files:**
- Modify: `web/lib/api.ts`, `web/components/BeatlyticsDashboard.tsx`

**Interfaces:**
- Produces: `export type Window = '24h' | '7d' | '30d' | 'all'`; `api.artists(w: Window)` and `api.tracks(w: Window)` call `/api/artists?window=${w}` and `/api/tracks?window=${w}`; `api.moods` and the moods mock are deleted.

- [ ] **Step 1:** Update `api.ts`; delete the mood types, state, `moodTotal`/`sortedMoods` memos, the mood section and the `/api/genres` call in the dashboard.
- [ ] **Step 2:** Add a window selector above the hero: four buttons (labels `24 hours`, `7 days`, `30 days`, `All time`) in a `role="group"` with `aria-pressed` on the active one. Default `7d`; changing it re-runs `load()` immediately. The hero sentence uses the window ("this week", "today", "this month", "of all time"), and the empty state says nothing was played in that window.
- [ ] **Step 3:** Run `cd web && npx tsc --noEmit && npx eslint app components`. Expected: no output.
- [ ] **Step 4:** Manual check at `http://127.0.0.1:3000` after signing in: each selector option changes the lists and the hero text, and `all` works with an empty database state (empty message, no crash).
- [ ] **Step 5:** `git commit -am "feat(web): window selector; remove mood UI"`

### Task 7: Prometheus and Grafana stack

**Files:**
- Create: `monitoring/docker-compose.yml`, `monitoring/prometheus/prometheus.yml`, `monitoring/prometheus/alerts.yml`, `monitoring/grafana/provisioning/datasources/prometheus.yml`, `monitoring/grafana/provisioning/dashboards/dashboards.yml`, `monitoring/grafana/dashboards/beatlytics.json`

**Interfaces:** Consumes the metric names in Global Constraints.

- [ ] **Step 1:** Write the compose file and Prometheus config per Global Constraints (named volumes for Prometheus and Grafana data; Prometheus mounts both config files and sets `--storage.tsdb.retention.time=1y`).
- [ ] **Step 2:** Write `alerts.yml` with `PollerStale` (`time() - beatlytics_last_successful_poll_timestamp_seconds > 900`, `for: 5m`) and `SpotifyRateLimited` (`increase(beatlytics_spotify_requests_total{code="429"}[5m]) > 0`).
- [ ] **Step 3:** Write the provisioned datasource and dashboard provider, and `beatlytics.json` with a `user` variable (`label_values(spotify_plays_total, user_id)`) and these panels: top artists bar gauge (`topk(10, sum by (artist) (increase(spotify_plays_total{user_id="$user"}[$__range])))`), plays over time, total plays, poll duration p95, poll errors by stage, Spotify requests by code, time since last successful poll, active pollers.
- [ ] **Step 4:** Validate: `docker run --rm --entrypoint promtool -v "$PWD/monitoring/prometheus:/p" prom/prometheus check config /p/prometheus.yml` and `check rules /p/alerts.yml` both report SUCCESS; `python3 -m json.tool monitoring/grafana/dashboards/beatlytics.json >/dev/null` exits 0.
- [ ] **Step 5:** Smoke test: with the backend running and a user signed in, `docker compose -f monitoring/docker-compose.yml up -d`; `http://localhost:9090/targets` shows the backend UP; a query for `spotify_plays_total` returns series; Grafana at `http://localhost:3001` shows the provisioned dashboard with data.
- [ ] **Step 6:** `git add monitoring && git commit -m "feat: Prometheus and Grafana monitoring stack"`

### Task 8: Docs and cleanup

**Files:** Modify `README.md`, `CLAUDE.md`, `TODO.md`, `change.log`.

- [ ] **Step 1:** README: add a Monitoring section (compose command, ports, Grafana login, retention, the cardinality note, and that Grafana does not show imported history), an "Import your Spotify history" section (request the extended streaming history at spotify.com/account/privacy, up to 30 days to arrive, then the import command), a note to use `127.0.0.1` rather than `localhost` for sign-in, and update the API table (`window` parameter; `/api/genres` removed).
- [ ] **Step 2:** CLAUDE.md: replace the metrics/mood description with the plays store, poller package, new metrics and routes, and the `monitoring/` and importer commands. TODO.md: mark finished items, record the out-of-scope list and the public-`/metrics` risk. `change.log`: add a Phase 5 entry.
- [ ] **Step 3:** Run `go test ./... && go vet ./... && (cd web && npx tsc --noEmit && npx eslint app components)`. Expected: all clean.
- [ ] **Step 4:** `git add -A && git commit -m "docs: monitoring, importer and play-history documentation"` (`TODO.md` stays gitignored).
