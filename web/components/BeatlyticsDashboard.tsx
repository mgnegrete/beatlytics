'use client';

import { useEffect, useMemo, useState } from 'react';
import { motion } from 'framer-motion';
import {
  ResponsiveContainer, BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip as ReTooltip, Legend,
  PieChart, Pie, Cell,
} from 'recharts';
import { RefreshCw, Activity, Circle, Server, Timer } from 'lucide-react';
import { api } from '@/lib/api';

type GenreDatum = { genre: string; plays: number };
type Health = { status: 'ok' | 'degraded' | 'down'; lastIngestUnix?: number };

function fmtTimeAgo(unix?: number) {
  if (!unix) return '–';
  const delta = Date.now() / 1000 - unix;
  if (delta < 60) return `${Math.floor(delta)}s ago`;
  if (delta < 3600) return `${Math.floor(delta / 60)}m ago`;
  return `${Math.floor(delta / 3600)}h ago`;
}

export default function BeatlyticsDashboard() {
  const [genres, setGenres] = useState<GenreDatum[]>([]);
  const [health, setHealth] = useState<Health>({ status: 'ok' });
  const [loading, setLoading] = useState(false);

  const load = async () => {
    setLoading(true);
    const [g, h] = await Promise.all([api.genres(), api.health()]);
    setGenres(g);
    setHealth(h);
    setLoading(false);
  };

  useEffect(() => {
    load();
    const id = setInterval(load, 30_000);
    return () => clearInterval(id);
  }, []);

  const pieData = useMemo(
    () => genres.map((g) => ({ name: g.genre, value: g.plays })),
    [genres]
  );
  const topGenres = useMemo(
    () => [...genres].sort((a, b) => b.plays - a.plays).slice(0, 8),
    [genres]
  );

  const statusColor =
    health.status === 'ok'
      ? 'text-green-600'
      : health.status === 'degraded'
      ? 'text-amber-600'
      : 'text-red-600';
  const statusLabel =
    health.status === 'ok'
      ? 'Healthy'
      : health.status === 'degraded'
      ? 'Degraded'
      : 'Down';

  return (
    <div className="min-h-screen bg-black-500 p-6">
      <div className="mx-auto max-w-6xl space-y-6">
        {/* Header */}
        <div className="flex items-center justify-between">
          <motion.h1 layout className="text-3xl font-bold tracking-tight text-emerald-600">
            Beatlytics Dashboard
          </motion.h1>
          <button
            onClick={load}
            disabled={loading}
            className="inline-flex items-center rounded-2xl bg-gray-900 px-4 py-2 text-sm font-medium text-green disabled:opacity-60"
          >
            <RefreshCw className={`mr-2 h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
            Refresh
          </button>
        </div>

        {/* Status Cards */}
        <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
          <div className="rounded-2xl bg-black p-5 shadow-sm">
            <div className="flex items-center gap-3">
              <Server className="h-5 w-5 text-gray-700" />
              <div className="text-sm text-emerald-500">Service Health</div>
            </div>
            <div className={`mt-2 flex items-center gap-2 font-semibold ${statusColor}`}>
              <Circle className="h-3 w-3" />
              {statusLabel}
            </div>
          </div>

          <div className="rounded-2xl bg-black p-5 shadow-sm">
            <div className="flex items-center gap-3">
              <Timer className="h-5 w-5 text-gray-700" />
              <div className="text-sm text-emerald-500">Last Ingest</div>
            </div>
            <div className="mt-2 text-lg font-semibold">{fmtTimeAgo(health.lastIngestUnix)}</div>
          </div>

          <div className="rounded-2xl bg-black p-5 shadow-sm">
            <div className="flex items-center gap-3">
              <Activity className="h-5 w-5 text-gray-700" />
              <div className="text-sm text-emerald-500">Tracked Genres</div>
            </div>
            <div className="mt-2 text-lg font-semibold">{genres.length || 0}</div>
          </div>
        </div>

        {/* Charts */}
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-2 border-white">
          <div className="rounded-2xl bg-black-500 p-5 shadow-sm">
            <div className="mb-3 text-sm font-medium text-emerald-600">Plays by Genre (Top 8)</div>
            <div className="h-72 w-full">
              <ResponsiveContainer>
                <BarChart data={topGenres}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis dataKey="genre" />
                  <YAxis allowDecimals={false} />
                  <ReTooltip />
                  <Legend />
                  <Bar dataKey="plays" name="Plays" radius={[6, 6, 0, 0]} fill="#059669"/>
                </BarChart>
              </ResponsiveContainer>
            </div>
          </div>

          <div className="rounded-2xl p-5 shadow-sm border-white bg-black-500">
            <div className="mb-3 text-sm font-medium text-emerald-600">Distribution</div>
            <div className="h-72 w-full">
              <ResponsiveContainer>
                <PieChart>
                  <Pie data={pieData} dataKey="value" nameKey="name" outerRadius={100}>
                    {pieData.map((_, idx) => (
                      <Cell key={idx} fill="#059669" />
                    ))}
                  </Pie>
                  <ReTooltip />
                  <Legend />
                </PieChart>
              </ResponsiveContainer>
            </div>
          </div>
        </div>

        {/* Table */}
        <div className="rounded-2xl bg-black shadow-sm">
          <div className="overflow-x-auto">
            <table className="min-w-full border border-white-500 divide-y divide-black-200">
              <thead className="bg-black-500">
                <tr>
                  <th className="px-6 py-3 text-left text-xs font-large uppercase tracking-wider text-emerald-500">
                    Genre
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-large uppercase tracking-wider text-emerald-500">
                    Plays
                  </th>
                </tr>
              </thead>    
              <tbody className="divide-y divide-gray-200 bg-black">
                {genres.map((g) => (
                  <tr key={g.genre}>
                    <td className="whitespace-nowrap px-6 py-4 text-sm font-medium text-emerald-500">
                      {g.genre}
                    </td>
                    <td className="whitespace-nowrap px-6 py-4 text-sm text-emerald-500">
                      {g.plays}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        <div className="pt-2 text-center text-xs text-gray-500">
          Tip: Run your Go server on <code>localhost:8081</code>. The dev proxy will forward <code>/api/*</code>.
        </div>
      </div>
    </div>
  );
}
