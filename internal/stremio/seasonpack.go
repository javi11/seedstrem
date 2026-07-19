package stremio

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	// SxxEyy single-episode marker (also the start of a range).
	reSeasonEpisode = regexp.MustCompile(`(?i)s0*(\d{1,3})[\s._-]*e0*(\d{1,4})`)
	// NxM single-episode marker (1x05).
	reSeasonXEpisode = regexp.MustCompile(`(?i)\b\d{1,3}x0*\d{1,4}\b`)
	// An episode range (E01-E10, S01E01-E13) — still a full-season pack.
	reEpisodeRange = regexp.MustCompile(`(?i)e0*\d{1,4}[\s._-]*-[\s._-]*e?0*\d{1,4}`)
	// "complete" anywhere in the title (complete series/season).
	reComplete = regexp.MustCompile(`(?i)\bcomplete\b`)
	// Any season token at all (SNN or "Season N") — used to tell a bare
	// "Complete Series" (contains every season) from "S03 Complete".
	reAnySeason = regexp.MustCompile(`(?i)\bs0*\d{1,3}\b|\bseason[\s._-]*0*\d{1,3}\b`)
)

// isSeasonPack reports whether a release title denotes a full-season pack
// for the requested season, as opposed to a single-episode release. It is
// used to keep only packs from a season-scoped search (which also returns
// every individual episode of the season) when the user asked for one
// specific episode.
func isSeasonPack(title string, season int) bool {
	t := strings.ToLower(title)

	// Episode-range releases (S01E01-E13, E01-E10) are packs. When the range
	// carries an explicit season, it must be the requested one.
	if reEpisodeRange.MatchString(t) {
		if m := reSeasonEpisode.FindStringSubmatch(t); m != nil {
			s, _ := strconv.Atoi(m[1])
			return s == season
		}
		return referencesSeason(t, season)
	}

	// A single specific episode (SxxEyy or NxM) is not a pack.
	if reSeasonEpisode.MatchString(t) || reSeasonXEpisode.MatchString(t) {
		return false
	}

	// A bare "Complete Series" (no season token) spans every season, so it
	// contains the requested one.
	if reComplete.MatchString(t) && !reAnySeason.MatchString(t) {
		return true
	}

	// Otherwise it's a pack iff it names the requested season.
	return referencesSeason(t, season)
}

// referencesSeason reports whether t names the given season via an SNN or
// "Season N" token. The trailing boundary keeps "s01" (season-only) from
// matching the "s01" inside a single-episode "s01e05".
func referencesSeason(t string, season int) bool {
	re := regexp.MustCompile(fmt.Sprintf(`(?i)\bs0*%d\b|\bseason[\s._-]*0*%d\b`, season, season))
	return re.MatchString(t)
}
