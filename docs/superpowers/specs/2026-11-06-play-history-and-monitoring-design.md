# Play history and monitoring design

Date: 2026-11-06
Status: draft, awaiting review

## Goal

Beatlytics currently reflects only the last ~50 plays Spotify returns, keeps nothing between polls, and exports no useful Prometheus data. This design adds a persistent play history, a window selector on the dashboard, Prometheus counters and operational metrics, a Prometheus and Grafana stack, and a one-time importer for Spotify's extended streaming history.

Success criteria:
- Every live play is stored once and survives restarts.
- The dashboard can show top artists and songs for 24h, 7 days, 30 days and all time.
- Prometheus exposes play counters and poller health metrics, and a provisioned Grafana dashboard shows them.
- A user can import their Spotify extended streaming history and see it in the longer dashboard windows.
- Pollers resume for all known users when the server starts.

## Decisions made

- **Window approach (option A):** SQLite is the source of truth. Prometheus receives live counters only. Grafana windows come from PromQL (`increase(...[range])`); web dashboard windows come from SQLite and include imported history.
- **Importer in scope.** Imported history appears in the web dashboard but not in Grafana, because Prometheus cannot ingest backdated samples.
- **Primary artist only.** Plays are attributed to the first-listed artist. This matches Spotify's export format and replaces today's behaviour of crediting every artist on a track.
- **Moods removed.** The Spotify `/v1/audio-features` endpoint returns 403 for this app. The mood gauge, mood endpoint, audio-features client call and mood UI are deleted.
- **Backend stays on the host.** Only Prometheus and Grafana run in Docker.

## 1. Play store and ingestion

### Schema (SQLite, auto-migrated at startup)

`plays`
- `user_id`, `played_at` (UTC), `track_id`, `track_name`, `artist_name` (primary artist)
- `ms_played` (nullable; null for live plays because the live API does not report it)
- `source` (`live` or `import`)
- Unique on `(user_id, track_id, played_at)`.

`poll_state`
- `user_id` (primary key), `last_played_at_ms` (the cursor).

Tables are created with `CREATE TABLE IF NOT EXISTS`, so an existing `beatlytics.db` upgrades in place.

### Live ingestion

- Each poll calls `recently-played?limit=50&after=<cursor>`. With no cursor it fetches the latest 50.
- New plays are inserted in one transaction. After the commit succeeds, the cursor advances and counters increment, only for rows actually inserted.
- A small interface wraps the Spotify client so the poller can be tested with a fake.
- On startup the server starts a poller for every user in SQLite, using stored refresh tokens. Sign-in still starts a poller for a new user.

### Importer

- Command: `go run ./cmd/import <path> --user <id>`, reading `Streaming_History_Audio_*.json`.
- Skips podcast/episode entries, entries with `ms_played` under 30000, and entries at or after that user's first live play.
- Track ID comes from `spotify_track_uri`; artist is the export's primary artist field.
- Idempotent. Runs each file in a transaction, aborts that file on malformed input with the filename and reason, and prints imported and skipped counts by reason.

## 2. Metrics and API

### Prometheus metrics

- `spotify_plays_total{user_id,artist}`: counter, incremented per newly stored live play.
- `beatlytics_poll_duration_seconds`: histogram.
- `beatlytics_poll_errors_total{stage}`: stage is `token`, `spotify_fetch` or `store`.
- `beatlytics_spotify_requests_total{endpoint,code}`.
- `beatlytics_last_successful_poll_timestamp_seconds{user_id}`: gauge.
- `beatlytics_pollers_running`: gauge.
- `spotify_mood_plays_total` is removed.
- Tracks are not exported to Prometheus. The `artist` label is unbounded; this is acceptable at personal or small multi-user scale and is noted in the README.

### API

- `GET /api/artists?window=24h|7d|30d|all` and `GET /api/tracks?window=...`: top 25 by play count from SQLite. Default `7d`. An invalid window returns 400 listing the valid values.
- `/api/health` reads the last successful poll time from the poller.
- `/api/genres` is removed. `/api/me` and auth routes are unchanged.

### Dashboard

- A window selector (24h, 7 days, 30 days, all time) at the top. The hero and both lists follow it, and the hero sentence names the window.
- The mood strip and related code are removed.
- Until history accumulates or is imported, the longer windows show only what has been collected.

## 3. Prometheus and Grafana

Files in `monitoring/`: `docker-compose.yml`, `prometheus/prometheus.yml`, `prometheus/alerts.yml`, `grafana/provisioning/` (datasource and dashboard provider), `grafana/dashboards/beatlytics.json`.

- Prometheus scrapes `host.docker.internal:8082/metrics` every 15s. Compose adds `extra_hosts: host.docker.internal:host-gateway` for Linux.
- Retention 1 year (`--storage.tsdb.retention.time=1y`) on a named volume.
- Alert rules (visible in the Prometheus UI, no Alertmanager): `PollerStale` (no successful poll for 15 minutes) and `SpotifyRateLimited` (any 429).
- Ports: Prometheus 9090, Grafana 3001. Grafana admin password from `GF_SECURITY_ADMIN_PASSWORD`, default `admin` for local use.
- Dashboard: `user_id` variable; top artists bar gauge using `topk(10, sum by (artist) (increase(spotify_plays_total{user_id="$user"}[$__range])))`; plays over time; total plays; poll duration p95; poll errors by stage; Spotify requests by status code; time since last successful poll; active pollers. The Grafana time picker is the window.
- Run with `docker compose -f monitoring/docker-compose.yml up`. The README gets a Monitoring section.
- Limitation: Grafana shows only data scraped since monitoring started. Imported history is not visible there.

## 4. Testing, error handling, rollout

### Tests (Go, written first)

- Store: idempotent inserts, window queries, cursor advance, using a temporary SQLite database.
- Poller: fake Spotify client; only new plays counted; cursor moves only after a successful write; each failure type increments the right error counter.
- Importer: fixture JSON; podcasts, short plays and post-live plays skipped; idempotent; correct summary counts.
- API: invalid window returns 400; unauthenticated requests return 401.
- Metrics: counters verified with the Prometheus client `testutil` package.
- Frontend: no test framework added; typecheck, lint and a manual run.
- Monitoring: `promtool check config` and `check rules`; dashboard JSON validity; smoke test that the compose stack scrapes the backend and `spotify_plays_total` is queryable.

### Error handling

- Spotify 429: honour `Retry-After`, skip the cycle, count under `code="429"`, do not advance the cursor.
- Token failure: log, count under `stage="token"`, retry next tick. On `invalid_grant`, stop that user's poller and log that they must sign in again.
- DB write failure: one transaction per batch; cursor and counters change only after commit, so no plays are lost.
- Importer: a malformed file aborts that file only, with filename and reason.

### Rollout

Tables are created automatically at startup. The first poll after deploy has no cursor and fetches the latest 50; later polls are incremental.

## Out of scope

Trends and discovery stats, shareable card, AI personality summary, friend comparison, a SQLite datasource in Grafana, containerizing the backend, and authentication for `/metrics`. These go in `TODO.md`. `/metrics` is currently public and exposes `user_id` and artist labels; this must be addressed before any deployment.
