package handlers

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	rateLimitMax = 600
	rateWindow   = time.Minute
)

type bucket struct {
	tokens  int
	resetAt time.Time
}

var (
	buckets   = map[string]*bucket{}
	bucketsMu sync.Mutex
)

func init() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		for range ticker.C {
			bucketsMu.Lock()
			now := time.Now()
			for ip, b := range buckets {
				if now.After(b.resetAt) {
					delete(buckets, ip)
				}
			}
			bucketsMu.Unlock()
		}
	}()
}

func getIP(r *http.Request) string {
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}
	return strings.Split(r.RemoteAddr, ":")[0]
}

func RateLimit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := getIP(r)

		bucketsMu.Lock()
		b, ok := buckets[ip]
		if !ok || time.Now().After(b.resetAt) {
			b = &bucket{tokens: rateLimitMax - 1, resetAt: time.Now().Add(rateWindow)}
			buckets[ip] = b
		} else if b.tokens > 0 {
			b.tokens--
		} else {
			bucketsMu.Unlock()
			writeError(w, 429, "rate limit exceeded")
			return
		}
		bucketsMu.Unlock()

		next(w, r)
	}
}
