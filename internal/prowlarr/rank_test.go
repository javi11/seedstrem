package prowlarr

import "testing"

func TestFilterDropsISO(t *testing.T) {
	gb := int64(1 << 30)
	in := []Result{
		{Title: "Movie.2020.1080p.BluRay.ISO", Seeders: 50, Size: 8 * gb},
		{Title: "Show S01 [ISO]", Seeders: 50, Size: 8 * gb},
		{Title: "movie.iso", Seeders: 50, Size: 8 * gb},
		{Title: "Poison 2020 1080p WEB", Seeders: 20, Size: 5 * gb},     // substring "iso", must be kept
		{Title: "Prison Break 1080p BluRay", Seeders: 20, Size: 5 * gb}, // must be kept
	}
	out := Filter(in, Filters{MinSeeders: 1})
	if len(out) != 2 {
		t.Fatalf("want 2 kept (non-ISO), got %d: %+v", len(out), out)
	}
	for _, r := range out {
		if r.Title != "Poison 2020 1080p WEB" && r.Title != "Prison Break 1080p BluRay" {
			t.Errorf("filter dropped a legitimate release or kept an ISO: %q", r.Title)
		}
	}
}

func TestDedup(t *testing.T) {
	in := []Result{
		{Title: "A", InfoHash: "aaa", Seeders: 10},
		{Title: "A dup lower", InfoHash: "AAA", Seeders: 25},
		{Title: "B", InfoHash: "bbb", Seeders: 5},
		{Title: "A dup higher-then-lower", InfoHash: "aaa", Seeders: 3},
	}
	out := Dedup(in)
	if len(out) != 2 {
		t.Fatalf("want 2 deduped, got %d: %+v", len(out), out)
	}
	if out[0].Seeders != 25 {
		t.Errorf("dedup should keep max seeders, got %d", out[0].Seeders)
	}
}

func TestSort(t *testing.T) {
	in := []Result{
		{Title: "low", Seeders: 1, Size: 100},
		{Title: "high", Seeders: 50, Size: 10},
		{Title: "mid-big", Seeders: 10, Size: 900},
		{Title: "mid-small", Seeders: 10, Size: 100},
	}
	out := Sort(in)
	if out[0].Title != "high" {
		t.Errorf("want highest seeders first, got %q", out[0].Title)
	}
	if out[1].Title != "mid-big" || out[2].Title != "mid-small" {
		t.Errorf("tie should break on size desc, got %q then %q", out[1].Title, out[2].Title)
	}
}

func TestSortPrefersFreeleech(t *testing.T) {
	in := []Result{
		{Title: "popular", Seeders: 100, Size: 100},
		{Title: "freeleech-fewer-seeds", Seeders: 5, Size: 100, Freeleech: true},
	}
	out := Sort(in)
	if out[0].Title != "freeleech-fewer-seeds" {
		t.Errorf("want freeleech first regardless of seeders, got %q", out[0].Title)
	}
}

func TestFilter(t *testing.T) {
	gb := int64(1 << 30)
	in := []Result{
		{Title: "Movie 1080p BluRay", Seeders: 20, Size: 8 * gb},
		{Title: "Movie 720p WEB", Seeders: 0, Size: 2 * gb},      // too few seeders
		{Title: "Movie 2160p REMUX", Seeders: 30, Size: 60 * gb}, // too big
		{Title: "Movie 1080p WEB", Seeders: 15, Size: 5 * gb},
	}
	f := Filters{MinSeeders: 1, MaxSizeBytes: 40 * gb}
	out := Filter(in, f)
	if len(out) != 2 {
		t.Fatalf("want 2 kept, got %d: %+v", len(out), out)
	}
	for _, r := range out {
		if r.Seeders < 1 || r.Size > 40*gb {
			t.Errorf("filter let through bad result: %+v", r)
		}
	}
}
