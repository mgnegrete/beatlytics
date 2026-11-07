export default function LoginPage() {
  return (
    <main className="min-h-screen flex items-center justify-center bg-black">
      <div className="text-center space-y-6">
        <h1 className="text-4xl font-bold text-emerald-500">Beatlytics</h1>
        <p className="text-gray-400">Your Spotify listening analytics</p>
        <a
          href="/auth/login"
          className="inline-flex items-center gap-2 bg-green-500 hover:bg-green-400 text-black font-bold px-8 py-3 rounded-full transition-colors"
        >
          Sign in with Spotify
        </a>
      </div>
    </main>
  );
}
