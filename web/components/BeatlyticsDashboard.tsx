'use client';

import { useEffect, useMemo, useState } from 'react';
import { motion } from 'framer-motion';
import {
  ResponsiveContainer, BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip as ReTooltip, Legend,
  PieChart, Pie, Cell,
} from 'recharts';
import { RefreshCw, Activity, Circle, Server, Timer, LogOut } from 'lucide-react';
import { api, type Me } from '@/lib/api';

type MoodDatum   = { genre: string; plays: number };
type ArtistDatum = { name: string; plays: number };
type TrackDatum  = { name: string; artist: string; plays: number };
type Health      = { status: 'ok' | 'degraded' | 'down'; lastIngestUnix?: number };

function fmtTimeAgo(unix?: number) {
  if (!unix) return '–';
  const delta = Date.now() / 1000 - unix;
  if (delta < 60) return `${Math.floor(delta)}s ago`;
  if (delta < 3600) return `${Math.floor(delta / 60)}m ago`;
  return `${Math.floor(delta / 3600)}h ago`;
}

export default function BeatlyticsDashboard({ user }: { user: Me }) {
  const [moods, setMoods]     = useState<MoodDatum[]>([]);
  const [artists, setArtists] = useState<ArtistDatum[]>([]);
  const [tracks, setTracks]   = useState<TrackDatum[]>([]);
  const [health, setHealth]   = useState<Health>({ status: 'ok' });
  const [loading, setLoading] = useState(false);

  const load = async () => {
    setLoading(true);
    const [m, a, t, h] = await Promise.all([
      api.moods(), api.artists(), api.tracks(), api.health(),
    ]);
    setMoods(m);
    setArtists(a);
    setTracks(t);
    setHealth(h);
    setLoading(false);
  };

  useEffect(() => {
    load();
    const id = setInterval(load, 30_000);
    return () => clearInterval(id);
  }, []);

  const pieData  = useMemo(() => moods.map((m) => ({ name: m.genre, value: m.plays })), [moods]);
  const topMoods = useMemo(() => [...moods].sort((a, b) => b.plays - a.plays).slice(0, 8), [moods]);

  const statusColor =
    health.status === 'ok' ? 'text-green-600'
    : health.status === 'degraded' ? 'text-amber-600'
    : 'text-red-600';
  const statusLabel =
    health.status === 'ok' ? 'Healthy'
    : health.status === 'degraded' ? 'Degraded'
    : 'Down';

  return (
    <div className="min-h-screen bg-black p-6">
      <div className="mx-auto max-w-6xl space-y-6">

        {/* Header */}
        <div className="flex items-center justify-between">
          <div>
            <motion.h1 layout className="text-3xl font-bold tracking-tight text-emerald-600">
              Beatlytics Dashboard
            </motion.h1>
            <p className="mt-1 text-sm text-gray-400">{user.displayName}</p>
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={load}
              disabled={loading}
              className="inline-flex items-center rounded-2xl bg-gray-900 px-4 py-2 text-sm font-medium text-emerald-400 disabled:opacity-60"
            >
              <RefreshCw className={`mr-2 h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
              Refresh
            </button>
            <a
              href="/auth/logout"
              className="inline-flex items-center rounded-2xl bg-gray-900 px-4 py-2 text-sm font-medium text-gray-400 hover:text-white"
            >
              <LogOut className="mr-2 h-4 w-4" />
              Sign out
            </a>
          </div>
        </div>

        {/* Status Cards */}
        <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
          <div className="rounded-2xl bg-gray-900 p-5">
            <div className="flex items-center gap-3">
              <Server className="h-5 w-5 text-gray-500" />
              <div className="text-sm text-emerald-500">Service Health</div>
            </div>
            <div className={`mt-2 flex items-center gap-2 font-semibold ${statusColor}`}>
              <Circle className="h-3 w-3" />
              {statusLabel}
            </div>
          </div>
          <div className="rounded-2xl bg-gray-900 p-5">
            <div className="flex items-center gap-3">
              <Timer className="h-5 w-5 text-gray-500" />
              <div className="text-sm text-emerald-500">Last Ingest</div>
            </div>
            <div className="mt-2 text-lg font-semibold text-white">{fmtTimeAgo(health.lastIngestUnix)}</div>
          </div>
          <div className="rounded-2xl bg-gray-900 p-5">
            <div className="flex items-center gap-3">
              <Activity className="h-5 w-5 text-gray-500" />
              <div className="text-sm text-emerald-500">Mood Buckets</div>
            </div>
            <div className="mt-2 text-lg font-semibold text-white">{moods.length}</div>
          </div>
        </div>

        {/* Mood Charts */}
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <div className="rounded-2xl bg-gray-900 p-5">
            <div className="mb-3 text-sm font-medium text-emerald-600">Plays by Mood (Top 8)</div>
            <div className="h-72 w-full">
              <ResponsiveContainer>
                <BarChart data={topMoods}>
                  <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
                  <XAxis dataKey="genre" tick={{ fill: '#9ca3af', fontSize: 12 }} />
                  <YAxis allowDecimals={false} tick={{ fill: '#9ca3af' }} />
                  <ReTooltip contentStyle={{ background: '#111827', border: 'none' }} />
                  <Legend />
                  <Bar dataKey="plays" name="Plays" radius={[6, 6, 0, 0]} fill="#059669" />
                </BarChart>
              </ResponsiveContainer>
            </div>
          </div>
          <div className="rounded-2xl bg-gray-900 p-5">
            <div className="mb-3 text-sm font-medium text-emerald-600">Mood Distribution</div>
            <div className="h-72 w-full">
              <ResponsiveContainer>
                <PieChart>
                  <Pie data={pieData} dataKey="value" nameKey="name" outerRadius={100}>
                    {pieData.map((_, idx) => (
                      <Cell key={idx} fill="#059669" opacity={0.6 + (idx % 4) * 0.1} />
                    ))}
                  </Pie>
                  <ReTooltip contentStyle={{ background: '#111827', border: 'none' }} />
                  <Legend />
                </PieChart>
              </ResponsiveContainer>
            </div>
          </div>
        </div>

        {/* Artists + Tracks Tables */}
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">

          {/* Top Artists */}
          <div className="rounded-2xl bg-gray-900 overflow-hidden">
            <div className="px-6 py-4 text-sm font-medium text-emerald-600 border-b border-gray-800">
              Top Artists
            </div>
            <div className="overflow-x-auto">
              <table className="min-w-full divide-y divide-gray-800">
                <thead>
                  <tr>
                    <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">#</th>
                    <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Artist</th>
                    <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Plays</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-800">
                  {artists.map((a, i) => (
                    <tr key={a.name}>
                      <td className="px-6 py-3 text-sm text-gray-500">{i + 1}</td>
                      <td className="px-6 py-3 text-sm font-medium text-emerald-400">{a.name}</td>
                      <td className="px-6 py-3 text-sm text-gray-300">{a.plays}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          {/* Top Tracks */}
          <div className="rounded-2xl bg-gray-900 overflow-hidden">
            <div className="px-6 py-4 text-sm font-medium text-emerald-600 border-b border-gray-800">
              Top Songs
            </div>
            <div className="overflow-x-auto">
              <table className="min-w-full divide-y divide-gray-800">
                <thead>
                  <tr>
                    <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">#</th>
                    <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Song</th>
                    <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Artist</th>
                    <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Plays</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-800">
                  {tracks.map((t, i) => (
                    <tr key={t.name + t.artist}>
                      <td className="px-6 py-3 text-sm text-gray-500">{i + 1}</td>
                      <td className="px-6 py-3 text-sm font-medium text-emerald-400">{t.name}</td>
                      <td className="px-6 py-3 text-sm text-gray-300">{t.artist}</td>
                      <td className="px-6 py-3 text-sm text-gray-300">{t.plays}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>

        <div className="pt-2 text-center text-xs text-gray-600">
          Data refreshes every 30 seconds · Powered by Spotify
        </div>
      </div>
    </div>
  );
}
