# Beatlytics

Beatlytics is an observability platform for your Spotify listening habits. Sign in with Spotify and a Go backend polls your recently played tracks, stores every play in SQLite, and exposes Prometheus metrics and a JSON API. A Next.js dashboard and a provisioned Grafana dashboard visualize it.

## Features

- Sign in with Spotify (per-user OAuth, signed-cookie sessions, SQLite user storage)
- Every play is stored, so you can look back over 24 hours, 7 days, 30 days or all time
- Top 25 artists and top 25 songs for the selected window (plays are credited to the primary artist)
- Import your full Spotify history from the extended streaming history export
- Prometheus metrics at `/metrics`: play counters plus poller health, Spotify request codes and poll latency
- A Grafana dashboard and two alert rules, provisioned by Docker Compose

## Planned

- AI-generated listening personality summary
- Docker images and Kubernetes manifests for the backend and frontend
- Trends, discovery stats and shareable cards

## Tech stack

- **Backend:** Go, Prometheus client, SQLite (`modernc.org/sqlite`)
- **Monitoring:** Prometheus, Grafana (Docker Compose)
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

This starts the Go server and the Next.js dev server together. Open **http://127.0.0.1:3000** (not `localhost`) and sign in with Spotify. The redirect URI uses `127.0.0.1`, and the sign-in cookie is host-specific, so starting from `localhost` fails with "invalid state".

To run them separately:

```bash
go run ./cmd/server       # backend
cd web && npm run dev     # frontend
```

The SQLite database (`beatlytics.db`) is created in the working directory on first run. On startup the server resumes polling for every user already in the database, so you only need to sign in once.

## Import your Spotify history

Spotify's API only returns your last ~50 plays, so Beatlytics builds history by polling. To load your older listening:

1. Request your **extended streaming history** at https://www.spotify.com/account/privacy. Spotify emails a download link, which can take up to 30 days.
2. Unzip it and find the `Streaming_History_Audio_*.json` files.
3. Run the importer (run it without `--user` to list user IDs):

```bash
go run ./cmd/import --user <your-user-id> path/to/Streaming_History_Audio_*.json
```

It skips podcasts, plays under 30 seconds and anything after your first live-polled play, and it is safe to run twice. Imported plays show up in the dashboard's longer windows. They do not appear in Grafana, because Prometheus cannot ingest backdated samples.

## Monitoring (Prometheus and Grafana)

With the backend running on port 8082:

```bash
docker compose -f monitoring/docker-compose.yml up -d
```

- Prometheus: http://localhost:9090 (scrapes `host.docker.internal:8082/metrics` every 15s, keeps data for 1 year)
- Grafana: http://localhost:3001 (user `admin`, password from `GF_SECURITY_ADMIN_PASSWORD`, default `admin`). The **Beatlytics** dashboard is provisioned automatically, and the time picker is the window.

Metrics:

| Metric | Meaning |
|--------|---------|
| `spotify_plays_total{user_id,artist}` | Plays recorded, by primary artist |
| `beatlytics_poll_duration_seconds` | Histogram of poll time |
| `beatlytics_poll_errors_total{stage}` | Failures at `token`, `spotify_fetch` or `store` |
| `beatlytics_spotify_requests_total{endpoint,code}` | Spotify calls by status code (watch for 429) |
| `beatlytics_last_successful_poll_timestamp_seconds{user_id}` | Last good poll |
| `beatlytics_pollers_running` | Active per-user pollers |

Things to know:
- `artist` is an unbounded label. That is fine for personal or small multi-user use (hundreds to low thousands of series).
- Grafana uses `increase()`, which cannot see a series' first count. Plays recorded before the first scrape, and the first play of each new artist, are not counted in Grafana. The web dashboard reads from SQLite and is not affected.
- `/metrics` is unauthenticated and exposes `user_id` and artist labels. Fine on localhost; lock it down before deploying.
- Alert rules (`PollerStale`, `SpotifyRateLimited`) are visible in the Prometheus UI. There is no Alertmanager.

## API

| Route | Auth | Description |
|-------|------|-------------|
| `GET /metrics` | no | Prometheus metrics |
| `GET /auth/login` | no | Start the Spotify OAuth flow |
| `GET /auth/callback` | no | OAuth callback; creates the session |
| `GET /auth/logout` | no | Clear the session |
| `GET /api/me` | yes | Current user |
| `GET /api/artists?window=` | yes | Top 25 artists; `window` is `24h`, `7d` (default), `30d` or `all` |
| `GET /api/tracks?window=` | yes | Top 25 songs; same `window` values |
| `GET /api/health` | yes | Status and last ingest time |

## Project layout

```
cmd/server/        entry point
internal/auth/     signed-cookie sessions
internal/http/     handlers, middleware, router
internal/metrics/  Prometheus collectors
internal/poller/   incremental per-user polling
internal/importer/ Spotify extended-history import
internal/spotify/  Spotify API client and models
internal/store/    SQLite: users, plays, poll cursor, windowed queries
cmd/import/        history import command
monitoring/        Prometheus + Grafana (Docker Compose)
web/               Next.js dashboard
```

See `change.log` for a history of larger changes.
