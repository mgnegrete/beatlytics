'use client';

import { useEffect, useState } from 'react';
import { motion, useReducedMotion } from 'framer-motion';
import { RefreshCw, LogOut } from 'lucide-react';
import { api, type Me, type Window } from '@/lib/api';

type ArtistDatum = { name: string; plays: number };
type TrackDatum  = { name: string; artist: string; plays: number };
type Health      = { status: 'ok' | 'degraded' | 'down'; lastIngestUnix?: number };

const PREVIEW_COUNT = 10;

const WINDOWS: { value: Window; label: string; phrase: string; empty: string }[] = [
  { value: '24h', label: '24 hours', phrase: 'today',       empty: 'in the last 24 hours' },
  { value: '7d',  label: '7 days',   phrase: 'this week',   empty: 'in the last 7 days' },
  { value: '30d', label: '30 days',  phrase: 'this month',  empty: 'in the last 30 days' },
  { value: 'all', label: 'All time', phrase: 'of all time', empty: 'yet' },
];

function fmtTimeAgo(unix?: number) {
  if (!unix) return 'not yet';
  const delta = Math.max(0, Date.now() / 1000 - unix);
  if (delta < 60) return 'just now';
  if (delta < 3600) return `${Math.floor(delta / 60)}m ago`;
  return `${Math.floor(delta / 3600)}h ago`;
}

const plural = (n: number) => `${n} ${n === 1 ? 'play' : 'plays'}`;

function ChartRow({
  rank, title, subtitle, plays, max, reduceMotion,
}: {
  rank: number; title: string; subtitle?: string; plays: number; max: number; reduceMotion: boolean;
}) {
  const pct = max > 0 ? (plays / max) * 100 : 0;
  return (
    <li className="grid grid-cols-[2rem_1fr_auto] items-baseline gap-x-3 border-b border-edge py-3.5">
      <span className="text-sm tabular-nums text-dust">{rank}</span>
      <div className="min-w-0">
        <div className="truncate font-medium">{title}</div>
        {subtitle && <div className="truncate text-sm text-dust">{subtitle}</div>}
        <div className="mt-2 h-[3px] overflow-hidden rounded-full bg-edge">
          <motion.div
            className={`h-full rounded-full ${rank === 1 ? 'bg-signal' : 'bg-bone/35'}`}
            initial={{ width: reduceMotion ? `${pct}%` : 0 }}
            animate={{ width: `${pct}%` }}
            transition={{
              duration: reduceMotion ? 0 : 0.8,
              ease: [0.2, 0.7, 0.2, 1],
              delay: reduceMotion ? 0 : Math.min(rank, 10) * 0.04,
            }}
          />
        </div>
      </div>
      <span className="font-semibold tabular-nums">{plays}</span>
    </li>
  );
}

function ChartSection({
  title, children, total, expanded, onToggle,
}: {
  title: string; children: React.ReactNode; total: number; expanded: boolean; onToggle: () => void;
}) {
  return (
    <section>
      <div className="flex items-baseline justify-between border-b border-bone/30 pb-3">
        <h2 className="text-2xl font-semibold tracking-tight">{title}</h2>
        <span className="text-sm text-dust">Plays</span>
      </div>
      <ol>{children}</ol>
      {total > PREVIEW_COUNT && (
        <button
          onClick={onToggle}
          className="mt-4 text-sm font-medium text-dust underline-offset-4 hover:text-bone hover:underline"
        >
          {expanded ? `Show top ${PREVIEW_COUNT}` : `Show all ${total}`}
        </button>
      )}
    </section>
  );
}

export default function BeatlyticsDashboard({ user }: { user: Me }) {
  const [artists, setArtists] = useState<ArtistDatum[]>([]);
  const [tracks, setTracks]   = useState<TrackDatum[]>([]);
  const [health, setHealth]   = useState<Health>({ status: 'ok' });
  const [loading, setLoading] = useState(false);
  const [loaded, setLoaded]   = useState(false);
  const [win, setWin]         = useState<Window>('7d');
  const [allArtists, setAllArtists] = useState(false);
  const [allTracks, setAllTracks]   = useState(false);
  const reduceMotion = useReducedMotion() ?? false;

  const load = async (w: Window = win) => {
    setLoading(true);
    const [a, t, h] = await Promise.all([
      api.artists(w), api.tracks(w), api.health(),
    ]);
    setArtists(a ?? []);
    setTracks(t ?? []);
    setHealth(h);
    setLoading(false);
    setLoaded(true);
  };

  useEffect(() => {
    load(win);
    const id = setInterval(() => load(win), 30_000);
    return () => clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [win]);

  const current = WINDOWS.find((w) => w.value === win)!;

  const topArtist = artists[0];
  const topTrack  = tracks[0];
  const maxArtist = topArtist?.plays ?? 0;
  const maxTrack  = topTrack?.plays ?? 0;

  const statusDot =
    health.status === 'ok' ? 'bg-emerald-400'
    : health.status === 'degraded' ? 'bg-amber-400'
    : 'bg-red-400';
  const statusText =
    health.status === 'down' ? 'Backend offline'
    : health.status === 'degraded' ? `Sync delayed, last ${fmtTimeAgo(health.lastIngestUnix)}`
    : `Synced ${fmtTimeAgo(health.lastIngestUnix)}`;

  return (
    <div className="min-h-screen px-6 pb-16 pt-6 sm:px-12">
      <div className="mx-auto max-w-6xl">

        {/* Header */}
        <header className="flex flex-wrap items-center justify-between gap-x-6 gap-y-3">
          <span className="text-lg font-semibold tracking-tight">Beatlytics</span>
          <div className="flex flex-wrap items-center gap-x-5 gap-y-2 text-sm text-dust">
            <span className="flex items-center gap-2">
              <span className={`h-2 w-2 rounded-full ${statusDot}`} aria-hidden />
              {statusText}
            </span>
            <span>{user.displayName}</span>
            <button
              onClick={() => load(win)}
              disabled={loading}
              className="inline-flex items-center gap-1.5 hover:text-bone disabled:opacity-60"
            >
              <RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />
              Refresh
            </button>
            <a href="/auth/logout" className="inline-flex items-center gap-1.5 hover:text-bone">
              <LogOut className="h-3.5 w-3.5" />
              Sign out
            </a>
          </div>
        </header>

        {/* Window selector */}
        <div role="group" aria-label="Time window" className="mt-10 inline-flex rounded-full border border-edge p-1 text-sm">
          {WINDOWS.map((w) => (
            <button
              key={w.value}
              onClick={() => setWin(w.value)}
              aria-pressed={win === w.value}
              className={`rounded-full px-4 py-1.5 font-medium transition ${
                win === w.value ? 'bg-bone text-ink' : 'text-dust hover:text-bone'
              }`}
            >
              {w.label}
            </button>
          ))}
        </div>

        {/* Hero */}
        <section className="min-h-[16rem] pb-14 pt-10 sm:pb-20 sm:pt-16">
          {loaded && topArtist && (
            <>
              <h1 className="display text-[clamp(3rem,11vw,8.5rem)]">{topArtist.name}</h1>
              <p className="mt-6 max-w-xl text-lg text-dust">
                Your most played artist {current.phrase}, with {plural(topArtist.plays)}.
                {topTrack && (
                  <>
                    {' '}On repeat: <span className="text-bone">{topTrack.name}</span> by {topTrack.artist}.
                  </>
                )}
              </p>
            </>
          )}
          {loaded && !topArtist && (
            <>
              <h1 className="display text-[clamp(3rem,9vw,6.5rem)]">Nothing played yet.</h1>
              <p className="mt-6 max-w-xl text-lg text-dust">
                No plays recorded {current.empty}. Beatlytics checks Spotify every few minutes,
                so new plays can take a little while to show up.
              </p>
            </>
          )}
        </section>

        {/* Ranked lists */}
        {topArtist && (
          <div className="grid grid-cols-1 gap-x-16 gap-y-14 lg:grid-cols-2">
            <ChartSection
              title="Artists"
              total={artists.length}
              expanded={allArtists}
              onToggle={() => setAllArtists((v) => !v)}
            >
              {(allArtists ? artists : artists.slice(0, PREVIEW_COUNT)).map((a, i) => (
                <ChartRow
                  key={a.name}
                  rank={i + 1}
                  title={a.name}
                  plays={a.plays}
                  max={maxArtist}
                  reduceMotion={reduceMotion}
                />
              ))}
            </ChartSection>

            <ChartSection
              title="Songs"
              total={tracks.length}
              expanded={allTracks}
              onToggle={() => setAllTracks((v) => !v)}
            >
              {(allTracks ? tracks : tracks.slice(0, PREVIEW_COUNT)).map((t, i) => (
                <ChartRow
                  key={t.name + t.artist}
                  rank={i + 1}
                  title={t.name}
                  subtitle={t.artist}
                  plays={t.plays}
                  max={maxTrack}
                  reduceMotion={reduceMotion}
                />
              ))}
            </ChartSection>
          </div>
        )}

        <footer className="mt-20 text-sm text-dust">
          Updates every 30 seconds. Listening data from Spotify.
        </footer>
      </div>
    </div>
  );
}
