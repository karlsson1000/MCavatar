package handlers

import (
	"encoding/json"
	"net/http"

	"mcavatar/internal/mc"
)

func writePNG(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=43200")
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

func ServeHealth(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeError(w, 404, "not found")
		return
	}
	writeJSON(w, 200, map[string]any{
		"api":    "mcavatar",
		"status": "ok",
	})
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

	out, err := mc.EncodePNG(mc.ResizeNearest(mc.ExtractHelm(skin), size, size))
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

	out, err := mc.EncodePNG(mc.ResizeNearest(mc.ExtractBody(skin), size/2, size))
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

	out, err := mc.EncodePNG(mc.ResizeNearest(mc.ExtractBust(skin), size, size))
	if err != nil {
		writeError(w, 500, "image encoding failed")
		return
	}
	writePNG(w, out)
}

func ServeUUID(w http.ResponseWriter, r *http.Request) {
	identifier, _ := mc.ParseRoute(r.URL.Path, "/uuid/")
	if identifier == "" {
		writeError(w, 400, "username required")
		return
	}

	profile, err := mc.LookupUUID(identifier)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}

	writeJSON(w, 200, map[string]string{
		"username":    profile.Name,
		"uuid":        profile.ID,
		"uuid_dashed": mc.FormatUUID(profile.ID),
	})
}
