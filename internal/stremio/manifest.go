package stremio

import (
	"regexp"
	"strings"
)

// Manifest is the Stremio addon manifest. seedstrem is a stream-only
// addon: it declares no catalogs and relies on Cinemeta for metadata.
type Manifest struct {
	ID            string         `json:"id"`
	Version       string         `json:"version"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Resources     []string       `json:"resources"`
	Types         []string       `json:"types"`
	Catalogs      []any          `json:"catalogs"`
	IDPrefixes    []string       `json:"idPrefixes"`
	BehaviorHints map[string]any `json:"behaviorHints,omitempty"`
}

const manifestID = "com.seedstrem.stremio"

// fallbackVersion is served when the build-injected version isn't a valid
// semantic version (e.g. "dev", "docker", a branch name, or a git sha).
// Stremio rejects a manifest whose "version" isn't semver.
const fallbackVersion = "0.0.0"

// semverRE matches a semantic version with an optional leading "v" and
// optional prerelease/build metadata. This mirrors what Stremio accepts.
var semverRE = regexp.MustCompile(`^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

// manifestVersion coerces the build version into something Stremio can
// parse, falling back to fallbackVersion when it isn't semver.
func manifestVersion(version string) string {
	v := strings.TrimPrefix(strings.TrimSpace(version), "v")
	if semverRE.MatchString(v) {
		return v
	}
	return fallbackVersion
}

// BuildManifest assembles the manifest given the enabled content types.
func BuildManifest(version string, addon AddonSettings) Manifest {
	types := make([]string, 0, 3)
	idPrefixes := []string{}
	if addon.EnableMovies {
		types = append(types, "movie")
	}
	if addon.EnableSeries {
		types = append(types, "series")
	}
	if addon.EnableMovies || addon.EnableSeries {
		idPrefixes = append(idPrefixes, "tt")
	}
	if addon.EnableAnime {
		types = append(types, "anime")
		idPrefixes = append(idPrefixes, "kitsu", "mal", "anilist", "anidb")
	}

	return Manifest{
		ID:          manifestID,
		Version:     version,
		Name:        "seedstrem",
		Description: "Self-hosted Stremio addon: searches Prowlarr indexers and streams torrents through qBittorrent while they download.",
		Resources:   []string{"stream"},
		Types:       types,
		Catalogs:    []any{},
		IDPrefixes:  idPrefixes,
		BehaviorHints: map[string]any{
			"configurable": false,
		},
	}
}
