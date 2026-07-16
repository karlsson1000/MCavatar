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
	geyserSkinURL    = "https://api.geysermc.org/v2/skin/%s"
	minecraftTexture = "https://textures.minecraft.net/texture/%s"

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

func (s lookupStatus) getError(identifier string) error {
	switch s {
	case statusOK:
		return nil
	case statusUnknown:
		return fmt.Errorf("player not found: %s", identifier)
	default:
		return fmt.Errorf("could not look up player: %s", identifier)
	}
}

func statusFromHTTP(code int) lookupStatus {
	switch code {
	case 200:
		return statusOK
	case 204, 404:
		return statusUnknown
	case 429:
		return statusRateLimit
	default:
		return statusError
	}
}

func IsUUID(identifier string) bool {
	return len(strings.ReplaceAll(identifier, "-", "")) == 32
}

func isBedrockUUID(uuid string) bool {
	trimmed := strings.ReplaceAll(uuid, "-", "")
	if len(trimmed) != 32 {
		return false
	}
	return trimmed[:16] == "0000000000000000"
}

func xuidFromUUID(uuid string) (string, error) {
	trimmed := strings.ToLower(strings.ReplaceAll(uuid, "-", ""))
	if len(trimmed) != 32 {
		return "", fmt.Errorf("invalid uuid")
	}
	lower := trimmed[16:]
	var xuid uint64
	_, err := fmt.Sscanf(lower, "%016x", &xuid)
	if err != nil {
		return "", fmt.Errorf("parse xuid: %w", err)
	}
	return fmt.Sprintf("%d", xuid), nil
}

var (
	httpClient = &http.Client{Timeout: 10 * time.Second}

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

type geyserSkinResponse struct {
	IsSteve   bool   `json:"is_steve"`
	TextureID string `json:"texture_id"`
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

func LookupUUID(identifier string) (*Profile, error) {
	if strings.ContainsAny(identifier, "${}") {
		return nil, fmt.Errorf("invalid player identifier: %s", identifier)
	}

	key := strings.ToLower(strings.ReplaceAll(identifier, "-", ""))

	uuidCacheMu.RLock()
	c, ok := uuidCache[key]
	uuidCacheMu.RUnlock()
	if ok && time.Now().Before(c.expires) {
		return c.profile, c.status.getError(identifier)
	}

	v, err, _ := uuidGroup.Do(key, func() (any, error) {
		var url string
		if IsUUID(identifier) {
			url = fmt.Sprintf(mojangSessionURL, key)
		} else {
			url = fmt.Sprintf(mojangProfileURL, identifier)
		}

		resp, err := httpClient.Get(url)
		if err != nil {
			log.Printf("lookup failed for %q: %v", identifier, err)
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
			log.Printf("mojang API non-OK for %q: %s", identifier, resp.Status)
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
			log.Printf("decode profile for %q: %v", identifier, err)
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
	return c.profile, c.status.getError(identifier)
}

// GetSkin fetches the skin for a username or UUID
func GetSkin(identifier string) (image.Image, []byte, error) {
	if strings.ContainsAny(identifier, "${}") {
		return nil, nil, fmt.Errorf("invalid player identifier: %s", identifier)
	}

	uuid := strings.ToLower(strings.ReplaceAll(identifier, "-", ""))
	if !IsUUID(identifier) {
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

	v, _, _ := skinGroup.Do(uuid, func() (any, error) {
		var skinURL string
		var err error

		if isBedrockUUID(uuid) {
			skinURL, err = fetchGeyserSkinURL(uuid)
		} else {
			skinURL, err = fetchSkinURL(uuid)
		}

		if err != nil {
			skinCacheMu.RLock()
			stale, hasStale := skinCache[uuid]
			skinCacheMu.RUnlock()
			if hasStale && stale.status == statusOK {
				return stale, nil
			}
			status := statusError
			if strings.Contains(err.Error(), "rate limit") {
				status = statusRateLimit
			}
			cached := &cachedSkin{expires: time.Now().Add(status.uuidTTL()), status: status}
			skinCacheMu.Lock()
			skinCache[uuid] = cached
			skinCacheMu.Unlock()
			return cached, nil
		}

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

func fetchGeyserSkinURL(uuid string) (string, error) {
	xuid, err := xuidFromUUID(uuid)
	if err != nil {
		return "", fmt.Errorf("invalid bedrock uuid: %w", err)
	}

	resp, err := httpClient.Get(fmt.Sprintf(geyserSkinURL, xuid))
	if err != nil {
		log.Printf("geyser lookup failed for xuid %s: %v", xuid, err)
		return "", fmt.Errorf("could not fetch bedrock skin")
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		log.Printf("geyser API rate limited for xuid %s", xuid)
		return "", fmt.Errorf("rate limit exceeded")
	}
	if resp.StatusCode != 200 {
		log.Printf("geyser API error for xuid %s: %s", xuid, resp.Status)
		return "", fmt.Errorf("could not fetch bedrock skin")
	}

	var geyser geyserSkinResponse
	if err := json.NewDecoder(resp.Body).Decode(&geyser); err != nil {
		log.Printf("decode geyser response for xuid %s: %v", xuid, err)
		return "", fmt.Errorf("could not fetch bedrock skin")
	}

	if geyser.TextureID == "" {
		return "", fmt.Errorf("no skin found for bedrock player")
	}

	return fmt.Sprintf(minecraftTexture, geyser.TextureID), nil
}

func fetchSkinURL(uuid string) (string, error) {
	resp, err := httpClient.Get(fmt.Sprintf(mojangSessionURL, uuid))
	if err != nil {
		log.Printf("session lookup failed for %q: %v", uuid, err)
		return "", fmt.Errorf("could not fetch skin")
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		log.Printf("session API rate limited for %q", uuid)
		return "", fmt.Errorf("rate limit exceeded")
	}
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
