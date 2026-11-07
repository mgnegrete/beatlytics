# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Beatlytics is a cloud-native observability platform that monitors Spotify listening habits. Users sign in with Spotify; a Go backend polls each user's recently played tracks, classifies them by mood using audio features, exposes Prometheus metrics, and serves a JSON API to a Next.js dashboard.

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

There are currently no tests.

## Architecture

### Backend (`cmd/` + `internal/`)
- **`cmd/server/main.go`** — loads `.env`, opens SQLite (`beatlytics.db`), builds the session store and `Handlers`, serves the router on `PORT` (default `8081`).
- **`internal/store/`** — SQLite via `modernc.org/sqlite`; `User` model; `UpsertUser`, `GetUserByID`, `GetUserBySpotifyID`. Auto-migrates the `users` table on startup.
- **`internal/auth/`** — HMAC-SHA256 signed cookie sessions (`SessionStore`), keyed by `SESSION_SECRET`.
- **`internal/spotify/`** — Spotify client (`client.go`: per-user client, cached access token, OAuth code exchange, `GetProfile`, `GetRecentlyPlayed`, `GetAudioFeatures`) and models (`models.go`).
- **`internal/metrics/`** — per-user snapshots (moods, top artists, top tracks), `classifyMood()` (9 buckets: Hype, Energetic, Aggressive, Happy, Danceable, Focus, Melancholic, Chill, Neutral), `StartPollerForUser`, and the Prometheus gauge `spotify_mood_plays_total{user_id,...}`.
- **`internal/http/`** — `Handlers` struct (injected deps), `RequireAuth` middleware (validates session cookie, puts `userID` in context), `NewRouter`.
- `internal/config/`, `internal/ingest/`, and top-level `api/` are empty placeholders.

### Routes
- Public: `/` (health text), `/metrics`, `/auth/login`, `/auth/callback`, `/auth/logout`
- Authenticated (`RequireAuth`): `/api/me`, `/api/genres` (returns **moods**; name is legacy), `/api/artists`, `/api/tracks`, `/api/health`

### Frontend (`web/`)
- Next.js 15 app router with Tailwind CSS 4, Framer Motion, Recharts, and Lucide icons.
- `web/app/page.tsx` checks auth via `getMe()` and redirects to `/login` if unauthenticated, then renders `web/components/BeatlyticsDashboard.tsx`.
- `web/app/login/page.tsx` — "Sign in with Spotify" button.
- API calls are centralized in `web/lib/api.ts` (`me`, `moods`, `artists`, `tracks`, health), with mock-data fallbacks if the backend is unreachable.
- **Dev proxy**: `web/next.config.ts` rewrites `/api/*` and `/auth/*` → `http://localhost:8082` in development, so cookies are same-origin. Keep the port in sync with `PORT` in `.env`.
- `NEXT_PUBLIC_API_BASE` env var can override the API base URL for production deployments.

### Data flow
1. User signs in via `/auth/login` → Spotify → `/auth/callback`, which exchanges the code, upserts the user, sets the session cookie, and starts that user's poller.
2. Each poller (interval `SPOTIFY_POLL_INTERVAL`, default 5m) fetches recently played tracks, batch-fetches audio features, and updates per-user snapshots and Prometheus gauges.
3. The frontend polls the `/api/*` endpoints every 30 seconds.
4. Dashboard renders mood bar + pie charts, plus Top Artists and Top Songs tables (top 25 each).

## Environment
- `.env` at repo root (gitignored) is read by the Go server. Variables: `PORT` (currently `8082`), `SPOTIFY_CLIENT_ID`, `SPOTIFY_CLIENT_SECRET`, `SPOTIFY_REDIRECT_URI`, `SESSION_SECRET`, `SPOTIFY_POLL_INTERVAL`.
- No `.env` file is needed for the frontend in local dev.
- `beatlytics.db` (SQLite) is created in the working directory and is gitignored.
- `change.log` records the history of larger changes; `TODO.md` (gitignored) tracks the plan.
