# Beatlytics

Beatlytics is an observability platform for your Spotify listening habits. Sign in with Spotify and a Go backend polls your recently played tracks, buckets them into moods using Spotify audio features, and exposes the results as Prometheus metrics and a JSON API. A Next.js dashboard visualizes it all.

## Features

- Sign in with Spotify (per-user OAuth, signed-cookie sessions, SQLite user storage)
- Mood breakdown of your recent listening (Hype, Energetic, Aggressive, Happy, Danceable, Focus, Melancholic, Chill, Neutral) as bar and pie charts
- Top 25 artists and top 25 songs by play count
- Prometheus metrics at `/metrics` (`spotify_mood_plays_total`, labeled by user)
- Dashboard auto-refreshes every 30 seconds

## Planned

- AI-generated listening personality summary
- Docker images and Kubernetes manifests
- Grafana dashboard on top of the Prometheus metrics

## Tech stack

- **Backend:** Go, Prometheus client, SQLite (`modernc.org/sqlite`)
- **Frontend:** Next.js 15 (app router), Tailwind CSS 4, Recharts, Framer Motion

## Getting started

### Prerequisites

- Go
- Node.js and npm
- A Spotify app from the [Spotify Developer Dashboard](https://developer.spotify.com/dashboard)

### Spotify app setup

1. Create an app in the Spotify Developer Dashboard.
2. Add a redirect URI that points at the backend's `/auth/callback` route. It must exactly match `SPOTIFY_REDIRECT_URI` below.
3. Copy the client ID and client secret.

The app requests the `user-read-recently-played` and `user-read-private` scopes.

### Configuration

Create a `.env` file in the repo root (it is gitignored):

```
PORT=8082
SPOTIFY_CLIENT_ID=...
SPOTIFY_CLIENT_SECRET=...
SPOTIFY_REDIRECT_URI=...   # must match the URI registered in Spotify
SESSION_SECRET=...         # long random string used to sign session cookies
SPOTIFY_POLL_INTERVAL=5m
```

`PORT` defaults to `8081` if unset. In development the Next.js proxy forwards to `localhost:8082`, so keep `PORT=8082` or update `web/next.config.ts`.

### Run

```bash
cd web
npm install
npm run dev:all
```

This starts the Go server and the Next.js dev server together. Open the Next.js URL (usually http://localhost:3000) and sign in with Spotify.

To run them separately:

```bash
go run ./cmd/server       # backend
cd web && npm run dev     # frontend
```

The SQLite database (`beatlytics.db`) is created in the working directory on first run.

## API

| Route | Auth | Description |
|-------|------|-------------|
| `GET /metrics` | no | Prometheus metrics |
| `GET /auth/login` | no | Start the Spotify OAuth flow |
| `GET /auth/callback` | no | OAuth callback; creates the session |
| `GET /auth/logout` | no | Clear the session |
| `GET /api/me` | yes | Current user |
| `GET /api/genres` | yes | Mood counts (legacy route name) |
| `GET /api/artists` | yes | Top 25 artists |
| `GET /api/tracks` | yes | Top 25 songs |
| `GET /api/health` | yes | Status and last ingest time |

## Project layout

```
cmd/server/        entry point
internal/auth/     signed-cookie sessions
internal/http/     handlers, middleware, router
internal/metrics/  mood classification, per-user pollers, Prometheus gauges
internal/spotify/  Spotify API client and models
internal/store/    SQLite user storage
web/               Next.js dashboard
```

See `change.log` for a history of larger changes.
