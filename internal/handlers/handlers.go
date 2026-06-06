package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"mcavatar/internal/mc"
	"mcavatar/internal/stats"
)

const renderCacheTTL = 12 * time.Hour

type renderEntry struct {
	data    []byte
	expires time.Time
}

var (
	renderCache   = map[string]*renderEntry{}
	renderCacheMu sync.Mutex
)

func init() {
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		for range ticker.C {
			now := time.Now()
			renderCacheMu.Lock()
			for k, v := range renderCache {
				if now.After(v.expires) {
					delete(renderCache, k)
				}
			}
			renderCacheMu.Unlock()
		}
	}()
}

func cachedRender(key string, fn func() ([]byte, error)) ([]byte, error) {
	renderCacheMu.Lock()
	if e, ok := renderCache[key]; ok && time.Now().Before(e.expires) {
		renderCacheMu.Unlock()
		return e.data, nil
	}
	renderCacheMu.Unlock()

	data, err := fn()
	if err != nil {
		return nil, err
	}

	renderCacheMu.Lock()
	renderCache[key] = &renderEntry{data: data, expires: time.Now().Add(renderCacheTTL)}
	renderCacheMu.Unlock()

	return data, nil
}

func writePNG(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=43200")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(data)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func TrackRequests(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats.IncrementRequests()
		next(w, r)
	}
}

func ServeHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	writeJSON(w, 200, map[string]any{
		"api":            "mcavatar",
		"status":         "ok",
		"total_requests": stats.GetTotalRequests(),
	})
}

func ServeNotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, 404, "not found")
}

func ServeAvatar(w http.ResponseWriter, r *http.Request) {
	identifier, size := mc.ParseRoute(r.URL.Path, "/avatar/")
	if identifier == "" {
		writeError(w, 400, "username or UUID required")
		return
	}

	skin, _, err := mc.GetSkin(identifier)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}

	out, err := cachedRender(fmt.Sprintf("avatar:%s:%d", identifier, size), func() ([]byte, error) {
		return mc.EncodePNG(mc.ResizeNearest(mc.ExtractHelm(skin), size, size))
	})
	if err != nil {
		writeError(w, 500, "image encoding failed")
		return
	}
	writePNG(w, out)
}

func ServeSkin(w http.ResponseWriter, r *http.Request) {
	identifier, _ := mc.ParseRoute(r.URL.Path, "/skin/")
	if identifier == "" {
		writeError(w, 400, "username or UUID required")
		return
	}

	_, raw, err := mc.GetSkin(identifier)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	writePNG(w, raw)
}

func ServeBody(w http.ResponseWriter, r *http.Request) {
	identifier, size := mc.ParseRoute(r.URL.Path, "/body/")
	if identifier == "" {
		writeError(w, 400, "username or UUID required")
		return
	}

	skin, _, err := mc.GetSkin(identifier)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}

	out, err := cachedRender(fmt.Sprintf("body:%s:%d", identifier, size), func() ([]byte, error) {
		return mc.EncodePNG(mc.ResizeNearest(mc.ExtractBody(skin), size/2, size))
	})
	if err != nil {
		writeError(w, 500, "image encoding failed")
		return
	}
	writePNG(w, out)
}

func ServeBust(w http.ResponseWriter, r *http.Request) {
	identifier, size := mc.ParseRoute(r.URL.Path, "/bust/")
	if identifier == "" {
		writeError(w, 400, "username or UUID required")
		return
	}

	skin, _, err := mc.GetSkin(identifier)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}

	out, err := cachedRender(fmt.Sprintf("bust:%s:%d", identifier, size), func() ([]byte, error) {
		return mc.EncodePNG(mc.ResizeNearest(mc.ExtractBust(skin), size, size))
	})
	if err != nil {
		writeError(w, 500, "image encoding failed")
		return
	}
	writePNG(w, out)
}

func ServeUUID(w http.ResponseWriter, r *http.Request) {
	identifier, _ := mc.ParseRoute(r.URL.Path, "/uuid/")
	if identifier == "" {
		writeError(w, 400, "username or UUID required")
		return
	}

	profile, err := mc.LookupUUID(identifier)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}

	writeJSON(w, 200, struct {
		Username    string `json:"username"`
		UUID        string `json:"uuid"`
		TrimmedUUID string `json:"trimmed_uuid"`
	}{
		Username:    profile.Name,
		UUID:        mc.FormatUUID(profile.ID),
		TrimmedUUID: profile.ID,
	})
}
