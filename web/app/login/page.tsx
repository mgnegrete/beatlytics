export default function LoginPage() {
  return (
    <main className="flex min-h-screen flex-col justify-between px-6 py-8 sm:px-12 sm:py-10">
      <span className="text-lg font-semibold tracking-tight">Beatlytics</span>

      <div className="max-w-3xl pb-16">
        <h1 className="display text-[clamp(3rem,10vw,8rem)]">
          See what you&rsquo;ve actually been playing.
        </h1>
        <p className="mt-6 max-w-md text-lg text-dust">
          Sign in with Spotify to see your most played artists and songs.
          Beatlytics only reads your listening history and never changes your
          playlists or library.
        </p>
        <a
          href="/auth/login"
          className="mt-10 inline-flex items-center rounded-full bg-signal px-8 py-3.5 font-semibold text-ink transition hover:brightness-110"
        >
          Sign in with Spotify
        </a>
      </div>

      <span aria-hidden className="h-4" />
    </main>
  );
}
