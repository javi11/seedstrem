package stremio

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
