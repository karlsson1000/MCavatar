package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"mcavatar/internal/handlers"
	"mcavatar/internal/stats"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbPath := os.Getenv("STATS_DB")
	if dbPath == "" {
		dbPath = "/data/stats.db"
	}

	stats.Init(dbPath)

	mux := http.NewServeMux()
	mux.HandleFunc("/avatar/", handlers.RateLimit(handlers.TrackRequests(handlers.ServeAvatar)))
	mux.HandleFunc("/skin/", handlers.RateLimit(handlers.TrackRequests(handlers.ServeSkin)))
	mux.HandleFunc("/body/", handlers.RateLimit(handlers.TrackRequests(handlers.ServeBody)))
	mux.HandleFunc("/bust/", handlers.RateLimit(handlers.TrackRequests(handlers.ServeBust)))
	mux.HandleFunc("/uuid/", handlers.RateLimit(handlers.TrackRequests(handlers.ServeUUID)))
	mux.HandleFunc("/health", handlers.ServeHealth)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, "index.html")
			return
		}
		handlers.ServeNotFound(w, r)
	})

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("listening on :%s", port)
	log.Fatal(srv.ListenAndServe())
}
