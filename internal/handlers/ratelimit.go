package handlers

import (
	"net"
	"net/http"
	"os"
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
	buckets       = map[string]*bucket{}
	bucketsMu     sync.Mutex
	trustedHeader = os.Getenv("TRUSTED_IP_HEADER")
)

func init() {
	go func() {
		ticker := time.NewTicker(rateWindow)
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
	if trustedHeader != "" {
		if ip := r.Header.Get(trustedHeader); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
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
