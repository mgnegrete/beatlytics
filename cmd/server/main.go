package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	genreCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "spotify_genre_plays_total",
			Help: "Number of times a genre appears in listening history",
		},
		[]string{"genre"},
	)
)

func init() {
	prometheus.MustRegister(genreCount)
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found")
	}

	http.Handle("/metrics", promhttp.Handler())

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	fmt.Printf("Starting Beatlytics on port %s\n", port)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Beatlytics is up\n"))
	})
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
