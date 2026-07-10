// Package meta resolves Stremio content ids into a searchable title and
// season/episode, using Cinemeta for IMDB ids and Kitsu for anime ids.
package meta

import (
	"fmt"
	"strconv"
	"strings"
)

// Kind distinguishes a one-shot movie from an episodic series.
type Kind int

const (
	KindMovie Kind = iota
	KindSeries
)

// Query is a parsed Stremio stream id.
type Query struct {
	Source  string // "tt" (imdb), "kitsu", "mal", "anilist", "anidb"
	ID      string // the id portion, e.g. "tt1375666" or "42"
	Kind    Kind
	Season  int // 0 when unknown (anime absolute numbering)
	Episode int
}

// IsSeries reports whether the query targets an episodic release.
func (q Query) IsSeries() bool { return q.Kind == KindSeries }

// IsAnime reports whether the id came from an anime id source.
func (q Query) IsAnime() bool {
	switch q.Source {
	case "kitsu", "mal", "anilist", "anidb":
		return true
	default:
		return false
	}
}

var animeSources = map[string]bool{"kitsu": true, "mal": true, "anilist": true, "anidb": true}

// ParseID parses a Stremio stream id for the given content type.
//
//	tt1375666            -> movie
//	tt0944947:1:5        -> series S1E5
//	kitsu:12             -> anime movie
//	kitsu:44081:5        -> anime series, episode 5 (absolute numbering)
//	mal:20:5             -> anime series, episode 5
func ParseID(typ, id string) (Query, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Query{}, fmt.Errorf("empty id")
	}
	parts := strings.Split(id, ":")

	if strings.HasPrefix(id, "tt") {
		q := Query{Source: "tt", ID: parts[0]}
		if len(parts) >= 3 {
			q.Kind = KindSeries
			q.Season, _ = strconv.Atoi(parts[1])
			q.Episode, _ = strconv.Atoi(parts[2])
		} else if typ == "series" {
			q.Kind = KindSeries
		} else {
			q.Kind = KindMovie
		}
		return q, nil
	}

	if animeSources[parts[0]] {
		if len(parts) < 2 || parts[1] == "" {
			return Query{}, fmt.Errorf("anime id %q missing numeric id", id)
		}
		q := Query{Source: parts[0], ID: parts[1]}
		if len(parts) >= 3 {
			q.Kind = KindSeries
			q.Episode, _ = strconv.Atoi(parts[2])
		} else {
			q.Kind = KindMovie
		}
		return q, nil
	}

	return Query{}, fmt.Errorf("unsupported id %q", id)
}
