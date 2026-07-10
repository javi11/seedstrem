package meta

import (
	"context"
	"fmt"
	"net/url"
)

// kitsuExternalSite maps an anime id source to Kitsu's mapping site key.
var kitsuExternalSite = map[string]string{
	"mal":     "myanimelist/anime",
	"anilist": "anilist/anime",
	"anidb":   "anidb",
}

// AnimeTitle resolves a human title for an anime id. Kitsu ids are looked
// up directly; other sources (mal/anilist/anidb) go through Kitsu's
// mappings endpoint.
func (c *Client) AnimeTitle(ctx context.Context, source, id string) (string, error) {
	key := "anime:" + source + ":" + id
	if info, ok := c.cacheGet(key); ok {
		return info.Name, nil
	}

	var title string
	var err error
	switch source {
	case "kitsu":
		title, err = c.kitsuTitleByID(ctx, id)
	case "mal", "anilist", "anidb":
		title, err = c.kitsuTitleByMapping(ctx, kitsuExternalSite[source], id)
	default:
		return "", fmt.Errorf("meta: unsupported anime source %q", source)
	}
	if err != nil {
		return "", err
	}
	if title == "" {
		return "", fmt.Errorf("meta: no title for %s:%s", source, id)
	}
	c.cachePut(key, Info{Name: title})
	return title, nil
}

type kitsuAttributes struct {
	CanonicalTitle string            `json:"canonicalTitle"`
	Titles         map[string]string `json:"titles"`
}

func (a kitsuAttributes) title() string {
	if a.CanonicalTitle != "" {
		return a.CanonicalTitle
	}
	for _, k := range []string{"en", "en_jp", "en_us"} {
		if v := a.Titles[k]; v != "" {
			return v
		}
	}
	for _, v := range a.Titles {
		if v != "" {
			return v
		}
	}
	return ""
}

func (c *Client) kitsuTitleByID(ctx context.Context, id string) (string, error) {
	var body struct {
		Data struct {
			Attributes kitsuAttributes `json:"attributes"`
		} `json:"data"`
	}
	if err := c.getJSON(ctx, c.kitsuURL+"/anime/"+url.PathEscape(id), &body); err != nil {
		return "", err
	}
	return body.Data.Attributes.title(), nil
}

func (c *Client) kitsuTitleByMapping(ctx context.Context, site, id string) (string, error) {
	q := url.Values{}
	q.Set("filter[externalSite]", site)
	q.Set("filter[externalId]", id)
	q.Set("include", "item")
	u := c.kitsuURL + "/mappings?" + q.Encode()

	var body struct {
		Included []struct {
			Type       string          `json:"type"`
			Attributes kitsuAttributes `json:"attributes"`
		} `json:"included"`
	}
	if err := c.getJSON(ctx, u, &body); err != nil {
		return "", err
	}
	for _, inc := range body.Included {
		if inc.Type == "anime" {
			if t := inc.Attributes.title(); t != "" {
				return t, nil
			}
		}
	}
	return "", fmt.Errorf("meta: no kitsu mapping for %s:%s", site, id)
}
