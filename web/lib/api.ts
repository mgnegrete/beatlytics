export const API_BASE =
  process.env.NEXT_PUBLIC_API_BASE && process.env.NEXT_PUBLIC_API_BASE.length > 0
    ? process.env.NEXT_PUBLIC_API_BASE
    : '';

async function getJSON<T>(path: string, mock: T): Promise<T> {
  try {
    const r = await fetch(`${API_BASE}${path}`, { cache: 'no-store' });
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
    return (await r.json()) as T;
  } catch {
    return mock;
  }
}

export interface Me {
  id: string;
  displayName: string;
}

export async function getMe(): Promise<Me | null> {
  try {
    const r = await fetch(`${API_BASE}/api/me`, { cache: 'no-store' });
    if (!r.ok) return null;
    return r.json();
  } catch {
    return null;
  }
}

export type Window = '24h' | '7d' | '30d' | 'all';

export const api = {
  artists: (w: Window) =>
    getJSON<{ name: string; plays: number }[]>(
      `/api/artists?window=${w}`,
      [
        { name: 'Artist One', plays: 12 },
        { name: 'Artist Two', plays: 9 },
        { name: 'Artist Three', plays: 7 },
        { name: 'Artist Four', plays: 5 },
        { name: 'Artist Five', plays: 3 },
      ]
    ),
  tracks: (w: Window) =>
    getJSON<{ name: string; artist: string; plays: number }[]>(
      `/api/tracks?window=${w}`,
      [
        { name: 'Song One', artist: 'Artist One', plays: 6 },
        { name: 'Song Two', artist: 'Artist Two', plays: 5 },
        { name: 'Song Three', artist: 'Artist One', plays: 4 },
        { name: 'Song Four', artist: 'Artist Three', plays: 3 },
        { name: 'Song Five', artist: 'Artist Four', plays: 2 },
      ]
    ),
  health: () =>
    getJSON<{ status: 'ok' | 'degraded' | 'down'; lastIngestUnix?: number }>(
      '/api/health',
      { status: 'degraded', lastIngestUnix: Math.floor(Date.now() / 1000) - 7200 }
    ),
};
