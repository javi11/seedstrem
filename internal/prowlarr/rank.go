package prowlarr

import (
	"sort"
	"strings"
)

// Filters constrains search results before ranking.
type Filters struct {
	MinSeeders   int
	MinSizeBytes int64
	MaxSizeBytes int64 // 0 = unbounded
}

// Dedup collapses results sharing an infohash, keeping the one with the
// most seeders. Input order is otherwise preserved for stable output.
func Dedup(results []Result) []Result {
	best := make(map[string]int, len(results)) // infohash -> index in out
	out := make([]Result, 0, len(results))
	for _, r := range results {
		key := strings.ToLower(r.InfoHash)
		if key == "" {
			out = append(out, r)
			continue
		}
		if i, ok := best[key]; ok {
			// Keep the freeleech flag if any duplicate carries it.
			freeleech := out[i].Freeleech || r.Freeleech
			if r.Seeders > out[i].Seeders {
				out[i] = r
			}
			out[i].Freeleech = freeleech
			continue
		}
		best[key] = len(out)
		out = append(out, r)
	}
	return out
}

// Sort orders results by freeleech (preferred, to protect ratio), then
// seeders (desc), then size (desc). It sorts in place and returns the
// slice for chaining.
func Sort(results []Result) []Result {
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Freeleech != results[j].Freeleech {
			return results[i].Freeleech // freeleech first
		}
		if results[i].Seeders != results[j].Seeders {
			return results[i].Seeders > results[j].Seeders
		}
		return results[i].Size > results[j].Size
	})
	return results
}

// Filter drops results failing the seeder/size constraints. ISO disc-image
// releases are always dropped: they are not streamable and only bloat the
// download disk.
func Filter(results []Result, f Filters) []Result {
	out := make([]Result, 0, len(results))
	for _, r := range results {
		if isISO(r.Title) {
			continue
		}
		if r.Seeders < f.MinSeeders {
			continue
		}
		if f.MinSizeBytes > 0 && r.Size < f.MinSizeBytes {
			continue
		}
		if f.MaxSizeBytes > 0 && r.Size > f.MaxSizeBytes {
			continue
		}
		out = append(out, r)
	}
	return out
}

// isISO reports whether a release title denotes a disc-image (ISO) release.
// "ISO" is matched as a standalone token (bounded by non-alphanumeric
// separators such as ".", " ", "[", "-", or a ".iso" extension) so that
// titles merely containing the substring — "Poison", "Prison Break" — are
// not dropped.
func isISO(title string) bool {
	for _, tok := range strings.FieldsFunc(title, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))
	}) {
		if strings.EqualFold(tok, "iso") {
			return true
		}
	}
	return false
}
