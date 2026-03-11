package mc

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	mojangProfileURL = "https://api.mojang.com/users/profiles/minecraft/%s"
	mojangSessionURL = "https://sessionserver.mojang.com/session/minecraft/profile/%s"

	ttlSuccess   = 12 * time.Hour
	ttlUnknown   = 7 * 24 * time.Hour
	ttlRateLimit = 2 * time.Hour
	ttlError     = 1 * time.Hour
	cleanupEvery = 30 * time.Minute
)

type lookupStatus uint8

const (
	statusOK lookupStatus = iota
	statusUnknown
	statusRateLimit
	statusError
)

func (s lookupStatus) uuidTTL() time.Duration {
	switch s {
	case statusOK:
		return ttlSuccess
	case statusUnknown:
		return ttlUnknown
	case statusRateLimit:
		return ttlRateLimit
	default:
		return ttlError
	}
}

func (s lookupStatus) skinTTL() time.Duration {
	switch s {
	case statusOK:
		return ttlSuccess
	case statusRateLimit:
		return ttlRateLimit
	default:
		return ttlError
	}
}

func (s lookupStatus) GetError(username string) error {
	switch s {
	case statusOK:
		return nil
	case statusUnknown:
		return fmt.Errorf("player not found: %s", username)
	default:
		return fmt.Errorf("could not look up player: %s", username)
	}
}

func statusFromHTTP(code int) lookupStatus {
	switch code {
	case 200:
		return statusOK
	case 404:
		return statusUnknown
	case 429:
		return statusRateLimit
	default:
		return statusError
	}
}

var (
	httpClient = &http.Client{Timeout: 5 * time.Second}

	skinCache   = map[string]*cachedSkin{}
	skinCacheMu sync.RWMutex
	skinGroup   singleflight.Group

	uuidCache   = map[string]*cachedUUID{}
	uuidCacheMu sync.RWMutex
	uuidGroup   singleflight.Group
)

type Profile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type sessionProfile struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Properties []property `json:"properties"`
}

type property struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type texturePayload struct {
	Textures struct {
		SKIN struct {
			URL string `json:"url"`
		} `json:"SKIN"`
	} `json:"textures"`
}

type cachedSkin struct {
	img     image.Image
	raw     []byte
	expires time.Time
	status  lookupStatus
}

type cachedUUID struct {
	profile *Profile
	expires time.Time
	status  lookupStatus
}

func init() {
	go func() {
		ticker := time.NewTicker(cleanupEvery)
		for range ticker.C {
			now := time.Now()

			skinCacheMu.Lock()
			for k, v := range skinCache {
				if now.After(v.expires) {
					delete(skinCache, k)
				}
			}
			skinCacheMu.Unlock()

			uuidCacheMu.Lock()
			for k, v := range uuidCache {
				if now.After(v.expires) {
					delete(uuidCache, k)
				}
			}
			uuidCacheMu.Unlock()
		}
	}()
}

func LookupUUID(username string) (*Profile, error) {
	key := strings.ToLower(username)

	uuidCacheMu.RLock()
	c, ok := uuidCache[key]
	uuidCacheMu.RUnlock()
	if ok && time.Now().Before(c.expires) {
		return c.profile, c.status.GetError(username)
	}

	v, err, _ := uuidGroup.Do(key, func() (any, error) {
		resp, err := httpClient.Get(fmt.Sprintf(mojangProfileURL, username))
		if err != nil {
			log.Printf("mojang lookup failed for %q: %v", username, err)
			uuidCacheMu.RLock()
			stale, hasStale := uuidCache[key]
			uuidCacheMu.RUnlock()
			if hasStale && stale.status == statusOK {
				return stale, nil
			}
			cached := &cachedUUID{expires: time.Now().Add(ttlError), status: statusError}
			uuidCacheMu.Lock()
			uuidCache[key] = cached
			uuidCacheMu.Unlock()
			return cached, nil
		}
		defer resp.Body.Close()

		s := statusFromHTTP(resp.StatusCode)
		if s != statusOK {
			log.Printf("mojang API non-OK for %q: %s", username, resp.Status)
			uuidCacheMu.RLock()
			stale, hasStale := uuidCache[key]
			uuidCacheMu.RUnlock()
			if hasStale && stale.status == statusOK {
				return stale, nil
			}
			cached := &cachedUUID{expires: time.Now().Add(s.uuidTTL()), status: s}
			uuidCacheMu.Lock()
			uuidCache[key] = cached
			uuidCacheMu.Unlock()
			return cached, nil
		}

		var profile Profile
		if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
			log.Printf("decode profile for %q: %v", username, err)
			cached := &cachedUUID{expires: time.Now().Add(ttlError), status: statusError}
			uuidCacheMu.Lock()
			uuidCache[key] = cached
			uuidCacheMu.Unlock()
			return cached, nil
		}

		cached := &cachedUUID{profile: &profile, expires: time.Now().Add(ttlSuccess), status: statusOK}
		uuidCacheMu.Lock()
		uuidCache[key] = cached
		uuidCacheMu.Unlock()
		return cached, nil
	})
	if err != nil {
		return nil, err
	}

	c = v.(*cachedUUID)
	return c.profile, c.status.GetError(username)
}

// GetSkin fetches the skin for a username or UUID
func GetSkin(identifier string) (image.Image, []byte, error) {
	uuid := identifier
	if len(strings.ReplaceAll(identifier, "-", "")) != 32 {
		profile, err := LookupUUID(identifier)
		if err != nil {
			return nil, nil, err
		}
		uuid = profile.ID
	}

	skinCacheMu.RLock()
	c, ok := skinCache[uuid]
	skinCacheMu.RUnlock()
	if ok && time.Now().Before(c.expires) {
		if c.img == nil {
			return nil, nil, fmt.Errorf("could not fetch skin")
		}
		return c.img, c.raw, nil
	}

	skinURL, err := fetchSkinURL(uuid)
	if err != nil {
		return nil, nil, err
	}

	v, err, _ := skinGroup.Do(uuid, func() (any, error) {
		img, raw, err := fetchPNG(skinURL)
		if err != nil {
			skinCacheMu.RLock()
			stale, hasStale := skinCache[uuid]
			skinCacheMu.RUnlock()
			if hasStale && stale.status == statusOK {
				return stale, nil
			}
			cached := &cachedSkin{expires: time.Now().Add(ttlError), status: statusError}
			skinCacheMu.Lock()
			skinCache[uuid] = cached
			skinCacheMu.Unlock()
			return cached, nil
		}

		cached := &cachedSkin{img: img, raw: raw, expires: time.Now().Add(ttlSuccess), status: statusOK}
		skinCacheMu.Lock()
		skinCache[uuid] = cached
		skinCacheMu.Unlock()
		return cached, nil
	})
	if err != nil {
		return nil, nil, err
	}

	result := v.(*cachedSkin)
	if result.img == nil {
		return nil, nil, fmt.Errorf("could not fetch skin")
	}
	return result.img, result.raw, nil
}

func FormatUUID(id string) string {
	if len(id) != 32 {
		return id
	}
	return fmt.Sprintf("%s-%s-%s-%s-%s", id[0:8], id[8:12], id[12:16], id[16:20], id[20:])
}

func fetchSkinURL(uuid string) (string, error) {
	resp, err := httpClient.Get(fmt.Sprintf(mojangSessionURL, uuid))
	if err != nil {
		log.Printf("session lookup failed for %q: %v", uuid, err)
		return "", fmt.Errorf("could not fetch skin")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("session API error for %q: %s", uuid, resp.Status)
		return "", fmt.Errorf("could not fetch skin")
	}

	var session sessionProfile
	if err = json.NewDecoder(resp.Body).Decode(&session); err != nil {
		log.Printf("decode session for %q: %v", uuid, err)
		return "", fmt.Errorf("could not fetch skin")
	}

	for _, p := range session.Properties {
		if p.Name != "textures" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(p.Value)
		if err != nil {
			log.Printf("decode textures for %q: %v", uuid, err)
			return "", fmt.Errorf("could not fetch skin")
		}
		var payload texturePayload
		if err := json.Unmarshal(data, &payload); err != nil {
			log.Printf("parse textures for %q: %v", uuid, err)
			return "", fmt.Errorf("could not fetch skin")
		}
		if payload.Textures.SKIN.URL == "" {
			return "", fmt.Errorf("no skin found for player")
		}
		return payload.Textures.SKIN.URL, nil
	}
	return "", fmt.Errorf("no skin found for player")
}

func fetchPNG(url string) (image.Image, []byte, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		log.Printf("skin download failed: %v", err)
		return nil, nil, fmt.Errorf("could not fetch skin")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("skin server error: %s", resp.Status)
		return nil, nil, fmt.Errorf("could not fetch skin")
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("read skin: %v", err)
		return nil, nil, fmt.Errorf("could not fetch skin")
	}

	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		log.Printf("decode PNG: %v", err)
		return nil, nil, fmt.Errorf("could not fetch skin")
	}
	return img, raw, nil
}
