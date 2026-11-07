package http

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewRouter(h *Handlers) *http.ServeMux {
	mux := http.NewServeMux()

	// Public
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/auth/login", h.LoginHandler)
	mux.HandleFunc("/auth/callback", h.CallbackHandler)
	mux.HandleFunc("/auth/logout", h.LogoutHandler)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Beatlytics is up\n"))
	})

	// Protected — require valid session cookie
	authMW := RequireAuth(h.Sessions)
	mux.Handle("/api/me", authMW(http.HandlerFunc(h.MeHandler)))
	mux.Handle("/api/genres", authMW(http.HandlerFunc(h.MoodsHandler)))
	mux.Handle("/api/artists", authMW(http.HandlerFunc(h.ArtistsHandler)))
	mux.Handle("/api/tracks", authMW(http.HandlerFunc(h.TracksHandler)))
	mux.Handle("/api/health", authMW(http.HandlerFunc(h.HealthHandler)))

	return mux
}
