package handler

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// semverRe pulls a vMAJOR.MINOR.PATCH out of arbitrary tag strings, tolerating
// a leading "v" and any suffix (e.g. "-3-gdeadbee" from git describe, or
// "-dirty"). Anything that doesn't match is treated as "unknown" — which makes
// hasUpdate() conservatively return false so we never show a false red dot.
var semverRe = regexp.MustCompile(`v?(\d+)\.(\d+)\.(\d+)`)

// latestCache memoizes the resolved latest tag so refreshing the SPA doesn't
// hammer the upstream. TTL is intentionally coarse — releases are infrequent.
type latestCache struct {
	mu        sync.Mutex
	tag       string
	fetchedAt time.Time
	ttl       time.Duration
}

var verCache = &latestCache{ttl: time.Hour}

// noRedirectClient resolves the latest tag from the upstream's /latest path.
//
// GitHub answers GET <repo>/releases/latest with a 302 to
// <repo>/releases/tag/vX.Y.Z. We deliberately DON'T follow it (ErrUseLastResponse)
// and read the Location header — that's the cheapest way to learn the tag, and
// it reuses the same release-proxy upstream that already works in regions where
// github.com is otherwise reachable from this server.
var noRedirectClient = &http.Client{
	Timeout: 10 * time.Second,
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func (lc *latestCache) get(upstream string) string {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	if lc.tag != "" && time.Since(lc.fetchedAt) < lc.ttl {
		return lc.tag
	}
	if tag := resolveLatestTag(upstream); tag != "" {
		lc.tag = tag
		lc.fetchedAt = time.Now()
	}
	return lc.tag
}

func resolveLatestTag(upstream string) string {
	upstream = strings.TrimRight(upstream, "/")
	if upstream == "" {
		return ""
	}
	resp, err := noRedirectClient.Get(upstream + "/latest")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	// Prefer the redirect target; fall back to the final URL if the upstream
	// served the page directly (e.g. a mirror that doesn't 302).
	candidate := resp.Header.Get("Location")
	if candidate == "" && resp.Request != nil && resp.Request.URL != nil {
		candidate = resp.Request.URL.String()
	}
	if m := semverRe.FindString(candidate); m != "" {
		return m
	}
	return ""
}

func parseSemver(s string) (major, minor, patch int, ok bool) {
	m := semverRe.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, 0, false
	}
	major, _ = strconv.Atoi(m[1])
	minor, _ = strconv.Atoi(m[2])
	patch, _ = strconv.Atoi(m[3])
	return major, minor, patch, true
}

// hasUpdate reports whether latest is strictly newer than current. If either
// side can't be parsed as semver we return false rather than guess.
func hasUpdate(current, latest string) bool {
	cMa, cMi, cPa, ok1 := parseSemver(current)
	lMa, lMi, lPa, ok2 := parseSemver(latest)
	if !ok1 || !ok2 {
		return false
	}
	switch {
	case lMa != cMa:
		return lMa > cMa
	case lMi != cMi:
		return lMi > cMi
	default:
		return lPa > cPa
	}
}

// VersionInfo reports the running version and the latest available release.
//
//	GET /api/version → { current, latest, has_update, release_url, self_node }
//
// `release_url` is a path on THIS server (/releases/tag/...), so the SPA can
// open the notes through the built-in release proxy even where github.com is
// blocked. It's empty when the latest tag couldn't be resolved.
//
// `self_node` echoes server.self_node from config (may be empty) so the SPA can
// name the right node in the upgrade prompt.
func VersionInfo(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		current := deps.Version
		latest := verCache.get(deps.Config.Server.ReleaseProxyUpstream)

		releaseURL := ""
		if latest != "" {
			releaseURL = "/releases/tag/" + latest
		}

		c.JSON(http.StatusOK, gin.H{
			"current":     current,
			"latest":      latest,
			"has_update":  hasUpdate(current, latest),
			"release_url": releaseURL,
			"self_node":   deps.Config.Server.SelfNode,
		})
	}
}
