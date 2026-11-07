# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Beatlytics is a cloud-native observability platform that monitors Spotify listening habits. Users sign in with Spotify; a Go backend polls each user's recently played tracks, stores every play in SQLite, exposes Prometheus metrics, and serves a windowed JSON API to a Next.js dashboard. A Docker Compose stack provides Prometheus and Grafana.

## Commands

### Run both services together (recommended)
```bash
cd web && npm run dev:all
```
This uses `concurrently` to start the Go server and the Next.js dev server in parallel.

### Backend (Go)
```bash
go run ./cmd/server   # run the server
go build ./cmd/server # build
go mod tidy           # tidy dependencies
```

### Frontend (Next.js)
```bash
cd web
npm install       # install deps
npm run dev       # Next.js dev server (Turbopack)
npm run build     # production build
npm run lint      # ESLint
```

```bash
go test ./cmd/... ./internal/...            # Go tests (store, poller, importer, spotify client, metrics, http)
go run ./cmd/import --user <id> <files>     # import Spotify extended streaming history
docker compose -f monitoring/docker-compose.yml up -d   # Prometheus :9090, Grafana :3001
```

The frontend has no test framework; check it with `npx tsc --noEmit` and `npm run lint`.

## Architecture

### Backend (`cmd/` + `internal/`)
- **`cmd/server/main.go`** — loads `.env`, opens SQLite (`beatlytics.db`), builds the poller manager, **resumes pollers for every stored user**, and serves the router on `PORT` (default `8081`).
- **`internal/store/`** — SQLite via `modernc.org/sqlite` (WAL). Tables: `users`, `plays` (unique on user, track, `played_at_ms`; primary artist only; `source` is `live` or `import`), `poll_state` (per-user cursor). `InsertPlays` writes in one transaction, ignores duplicates, returns only new rows and raises the cursor atomically. Windowed `TopArtists`/`TopTracks` and the `Window` type (`24h`, `7d`, `30d`, `all`) live here.
- **`internal/auth/`** — HMAC-SHA256 signed cookie sessions keyed by `SESSION_SECRET`.
- **`internal/spotify/`** — per-user client with cached access token, OAuth code exchange, `GetProfile`, and `GetRecentlyPlayed(token, afterMs)`. Non-200 API responses are `*APIError` (with `RetryAfter`); a revoked refresh token wraps `ErrInvalidGrant`. `Client.Observe` reports each request to metrics. `/v1/audio-features` returns 403 for this app, so moods were removed.
- **`internal/poller/`** — `PollOnce` (fetch plays after the cursor, store them, then bump counters only for newly inserted rows) and `Manager` (one goroutine per user; honors 429 `Retry-After`; stops a user's poller on `invalid_grant`).
- **`internal/metrics/`** — Prometheus collectors only: `spotify_plays_total{user_id,artist}`, `beatlytics_poll_duration_seconds`, `beatlytics_poll_errors_total{stage}`, `beatlytics_spotify_requests_total{endpoint,code}`, `beatlytics_last_successful_poll_timestamp_seconds{user_id}`, `beatlytics_pollers_running`.
- **`internal/importer/`** + **`cmd/import/`** — loads Spotify extended streaming history (skips podcasts, plays under 30s, and anything at/after the user's first live play; idempotent; a malformed file inserts nothing).
- **`internal/http/`** — `Handlers` (Sessions, Store, Pollers), `RequireAuth`, `NewRouter`.
- **`monitoring/`** — Compose stack: Prometheus (scrapes `host.docker.internal:8082`, 1y retention, alert rules) and Grafana (provisioned datasource and dashboard, port 3001).
- `internal/config/`, `internal/ingest/`, and top-level `api/` are empty placeholders.

### Routes
- Public: `/` (health text), `/metrics`, `/auth/login`, `/auth/callback`, `/auth/logout`
- Authenticated (`RequireAuth`): `/api/me`, `/api/artists?window=`, `/api/tracks?window=` (window default `7d`, top 25, invalid value returns 400), `/api/health`

### Frontend (`web/`)
- Next.js 15 app router with Tailwind CSS 4, Framer Motion, Recharts, and Lucide icons.
- `web/app/page.tsx` checks auth via `getMe()` and redirects to `/login` if unauthenticated, then renders `web/components/BeatlyticsDashboard.tsx` (hero, window selector, ranked artist/song lists).
- `web/app/login/page.tsx` — "Sign in with Spotify" button.
- API calls are centralized in `web/lib/api.ts` (`getMe`, `api.artists(window)`, `api.tracks(window)`, `api.health()`), with mock-data fallbacks if the backend is unreachable.
- **Dev proxy**: `web/next.config.ts` rewrites `/api/*` and `/auth/*` → `http://localhost:8082` in development, so cookies are same-origin. Keep the port in sync with `PORT` in `.env`.
- `NEXT_PUBLIC_API_BASE` env var can override the API base URL for production deployments.

### Data flow
1. User signs in via `/auth/login` → Spotify → `/auth/callback`, which exchanges the code, upserts the user, sets the session cookie, and starts that user's poller. After a restart, pollers resume from stored refresh tokens.
2. Each poller (every `SPOTIFY_POLL_INTERVAL`, default 5m) asks Spotify for plays after the stored cursor, writes new plays to SQLite, then increments Prometheus counters for those rows.
3. The API answers windowed queries from SQLite; the frontend refreshes every 30 seconds.
4. Grafana reads the Prometheus counters (only data scraped since monitoring started; imported history is SQLite-only).

## Environment
- `.env` at repo root (gitignored) is read by the Go server. Variables: `PORT` (currently `8082`), `SPOTIFY_CLIENT_ID`, `SPOTIFY_CLIENT_SECRET`, `SPOTIFY_REDIRECT_URI`, `SESSION_SECRET`, `SPOTIFY_POLL_INTERVAL`.
- No `.env` file is needed for the frontend in local dev.
- `beatlytics.db` (SQLite, plus `-wal`/`-shm` files) is created in the working directory and is gitignored.
- Sign in at `http://127.0.0.1:3000`, not `localhost` (redirect URI host must match the cookie host).
- `change.log` records the history of larger changes; `TODO.md` (gitignored) tracks the plan.
