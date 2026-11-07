package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/mgnegrete/beatlytics/internal/auth"
	beathttp "github.com/mgnegrete/beatlytics/internal/http"
	"github.com/mgnegrete/beatlytics/internal/metrics"
	"github.com/mgnegrete/beatlytics/internal/poller"
	"github.com/mgnegrete/beatlytics/internal/spotify"
	"github.com/mgnegrete/beatlytics/internal/store"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	db, err := store.Open("beatlytics.db")
	if err != nil {
		log.Fatalf("open db: %v", err)
	}

	interval := 5 * time.Minute
	if v := os.Getenv("SPOTIFY_POLL_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			interval = d
		}
	}

	pollers := poller.NewManager(db, interval)

	// Resume polling for every known user using their stored refresh tokens.
	users, err := db.ListUsers()
	if err != nil {
		log.Fatalf("list users: %v", err)
	}
	pollers.StartAll(users, func(u store.User) poller.Source {
		c := spotify.NewClientForUser(os.Getenv("SPOTIFY_CLIENT_ID"), os.Getenv("SPOTIFY_CLIENT_SECRET"), u.RefreshToken)
		c.Observe = metrics.SpotifyRequest
		return c
	})
	log.Printf("resumed pollers for %d users", len(users))

	h := &beathttp.Handlers{
		Sessions: auth.NewSessionStore(os.Getenv("SESSION_SECRET")),
		Store:    db,
		Pollers:  pollers,
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	fmt.Printf("Starting Beatlytics on port %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, beathttp.NewRouter(h)))
}
