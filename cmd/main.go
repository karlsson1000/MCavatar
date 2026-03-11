package main

import (
	"log"
	"net/http"
	"os"

	"mcavatar/internal/handlers"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/avatar/", handlers.RateLimit(handlers.ServeAvatar))
	mux.HandleFunc("/skin/", handlers.RateLimit(handlers.ServeSkin))
	mux.HandleFunc("/body/", handlers.RateLimit(handlers.ServeBody))
	mux.HandleFunc("/bust/", handlers.RateLimit(handlers.ServeBust))
	mux.HandleFunc("/uuid/", handlers.RateLimit(handlers.ServeUUID))
	mux.HandleFunc("/", handlers.ServeHealth)

	log.Printf("listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
