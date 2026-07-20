package stremio

import (
	"reflect"
	"testing"
)

func TestParseQuality(t *testing.T) {
	tests := []struct {
		title string
		want  Quality
	}{
		{"The Matrix 1999 1080p BluRay", Quality{Resolution: "1080p", Source: "BluRay"}},
		{"The Matrix 1999 720p", Quality{Resolution: "720p"}},
		{"Show S01 2160p WEB-DL HEVC HDR", Quality{Resolution: "2160p", Source: "WEB-DL", Codec: "HEVC", HDR: []string{"HDR"}}},
		{"Movie 2160p REMUX HEVC DV HDR10+ Atmos", Quality{Resolution: "2160p", Source: "REMUX", Codec: "HEVC", HDR: []string{"DV", "HDR10+"}, Audio: "Atmos"}},
		{"Movie 2160p HDR10+ DV", Quality{Resolution: "2160p", HDR: []string{"DV", "HDR10+"}}},
		{"Anime 1080p x265 10bit", Quality{Resolution: "1080p", Codec: "HEVC", TenBit: true}},
		{"Movie 4K UHD BDRip x264", Quality{Resolution: "2160p", Source: "BluRay", Codec: "AVC"}},
		{"Movie FHD WEBRip DDP5.1", Quality{Resolution: "1080p", Source: "WEBRip", Audio: "DDP"}},
		{"Some.Movie.DVDRip.XviD", Quality{Resolution: "SD", Source: "DVDRip"}},
		{"Movie 2019 HDCAM", Quality{Resolution: "SD", Source: "CAM"}},
		{"Just A Movie Title", Quality{}},
		{"Movie 1080p WEB-DL AV1 DTS-HD", Quality{Resolution: "1080p", Source: "WEB-DL", Codec: "AV1", Audio: "DTS-HD"}},
		{"Movie 720p HDTV h264", Quality{Resolution: "720p", Source: "HDTV", Codec: "AVC"}},
	}
	for _, tt := range tests {
		if got := ParseQuality(tt.title); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ParseQuality(%q)\n got  %+v\n want %+v", tt.title, got, tt.want)
		}
	}
}

func TestQualityBadge(t *testing.T) {
	tests := []struct {
		q    Quality
		want string
	}{
		{Quality{Resolution: "1080p", Source: "BluRay"}, "1080p"},
		{Quality{Resolution: "2160p", HDR: []string{"DV", "HDR10+"}}, "2160p DV·HDR10+"},
		{Quality{Resolution: "1080p", Source: "WEB-DL", Codec: "HEVC", HDR: []string{"HDR"}}, "1080p HDR"},
		{Quality{}, ""},
	}
	for _, tt := range tests {
		if got := qualityBadge(tt.q); got != tt.want {
			t.Errorf("qualityBadge(%+v) = %q, want %q", tt.q, got, tt.want)
		}
	}
}

func TestQualitySummary(t *testing.T) {
	tests := []struct {
		q    Quality
		want string
	}{
		{Quality{Resolution: "1080p", Source: "BluRay"}, "1080p • BluRay"},
		{Quality{Resolution: "2160p", HDR: []string{"DV", "HDR10+"}}, "2160p • DV HDR10+"},
		{Quality{Resolution: "1080p", Source: "WEB-DL", Codec: "HEVC", HDR: []string{"HDR"}}, "1080p • WEB-DL • HEVC • HDR"},
		{Quality{Resolution: "2160p", Source: "REMUX", Codec: "HEVC", HDR: []string{"DV", "HDR10+"}, TenBit: true, Audio: "Atmos"}, "2160p • REMUX • HEVC 10bit • DV HDR10+ • Atmos"},
		{Quality{}, ""},
	}
	for _, tt := range tests {
		if got := qualitySummary(tt.q); got != tt.want {
			t.Errorf("qualitySummary(%+v) = %q, want %q", tt.q, got, tt.want)
		}
	}
}
