export const API_BASE =
  process.env.NEXT_PUBLIC_API_BASE && process.env.NEXT_PUBLIC_API_BASE.length > 0
    ? process.env.NEXT_PUBLIC_API_BASE
    : ''; // same-origin or dev rewrite

async function getJSON<T>(path: string, mock: T): Promise<T> {
  try {
    const r = await fetch(`${API_BASE}${path}`, { cache: 'no-store' });
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
    return (await r.json()) as T;
  } catch {
    return mock; // graceful fallback for local dev before API is ready
  }
}

export const api = {
  genres: () =>
    getJSON<{ genre: string; plays: number }[]>(
      '/api/genres',
      [
        { genre: 'rock', plays: 42 },
        { genre: 'pop', plays: 31 },
        { genre: 'hip-hop', plays: 28 },
        { genre: 'indie', plays: 15 },
        { genre: 'electronic', plays: 12 },
        { genre: 'R&B', plays: 55}
      ]
    ),
  health: () =>
    getJSON<{ status: 'ok' | 'degraded' | 'down'; lastIngestUnix?: number }>(
      '/api/health',
      { status: 'degraded', lastIngestUnix: Math.floor(Date.now() / 1000) - 7200 }
    ),
};
