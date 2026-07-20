package stremio

import (
	"regexp"
	"strings"
)

// Quality holds the release attributes parsed from a raw torrent/release
// title. Every field is optional; the zero value means nothing was
// recognized. Fields hold the normalized display token (e.g. "2160p",
// "WEB-DL", "HEVC"), not the raw substring that matched.
type Quality struct {
	Resolution string   // "2160p" | "1080p" | "720p" | "480p" | "SD" | ""
	Source     string   // "REMUX" | "BluRay" | "WEB-DL" | "WEBRip" | "HDTV" | "DVDRip" | "CAM" | ""
	Codec      string   // "HEVC" | "AVC" | "AV1" | ""
	HDR        []string // ordered subset of {"DV","HDR10+","HDR10","HDR"}; nil if none
	Audio      string   // "Atmos" | "DTS-HD" | "DTS" | "DDP" | "AC3" | ""
	TenBit     bool     // "10bit"/"10-bit" present
}

// Precompiled, case-insensitive, word-boundary-aware matchers. RE2 treats any
// non-word character (space, dot, dash) as a boundary, which is exactly how
// scene release names delimit tokens.
var (
	// Resolution — 2160p family first, then descending. All require the "p"
	// suffix or an explicit alias so bare years/numbers never false-match.
	reRes2160 = regexp.MustCompile(`(?i)\b(2160p|4k|uhd)\b`)
	reRes1080 = regexp.MustCompile(`(?i)\b(1080p|fhd)\b`)
	reRes720  = regexp.MustCompile(`(?i)\b720p\b`)
	reRes480  = regexp.MustCompile(`(?i)\b480p\b`)

	// Source — REMUX beats BluRay; WEB-DL and WEBRip are distinct and tried
	// before a generic bare "web".
	reRemux  = regexp.MustCompile(`(?i)\bremux\b`)
	reBluray = regexp.MustCompile(`(?i)\b(blu[-. ]?ray|bdrip|brrip|bd25|bd50)\b`)
	reWebDL  = regexp.MustCompile(`(?i)\bweb[-. ]?dl\b`)
	reWebRip = regexp.MustCompile(`(?i)\bweb[-. ]?rip\b`)
	reWeb    = regexp.MustCompile(`(?i)\bweb\b`)
	reHDTV   = regexp.MustCompile(`(?i)\bhdtv\b`)
	reDVDRip = regexp.MustCompile(`(?i)\b(dvdrip|xvid)\b`)
	reCAM    = regexp.MustCompile(`(?i)\b(cam|camrip|telesync|hdcam|ts)\b`)

	// Codec — HEVC/x265, AVC/x264, AV1 (separator tolerant).
	reHEVC = regexp.MustCompile(`(?i)\b(hevc|[xh][-. ]?265)\b`)
	reAVC  = regexp.MustCompile(`(?i)\b(avc|[xh][-. ]?264)\b`)
	reAV1  = regexp.MustCompile(`(?i)\bav1\b`)

	// HDR flags. \bdv\b will not match inside "DVDRip"/"DVD" (the char after
	// "dv" is a word char there, so there is no boundary).
	reDV     = regexp.MustCompile(`(?i)\b(dv|dovi|dolby[-. ]?vision)\b`)
	reHDR10p = regexp.MustCompile(`(?i)\b(hdr10\+|hdr10plus)`)
	reHDR10  = regexp.MustCompile(`(?i)\bhdr10\b`)
	reHDR    = regexp.MustCompile(`(?i)\bhdr\b`)

	reTenBit = regexp.MustCompile(`(?i)\b10[-. ]?bit\b`)

	// Audio (rendered in the summary only, never in the name badge).
	reAtmos = regexp.MustCompile(`(?i)\batmos\b`)
	reDTSHD = regexp.MustCompile(`(?i)\bdts[-. ]?hd\b`)
	reDTS   = regexp.MustCompile(`(?i)\bdts\b`)
	reDDP   = regexp.MustCompile(`(?i)\b(ddp|dd\+|eac3|e-ac-3)`)
	reAC3   = regexp.MustCompile(`(?i)\bac3`)
)

// ParseQuality extracts resolution/source/codec/HDR tokens from a raw release
// title. It never errors; unrecognized tokens are simply omitted.
func ParseQuality(title string) Quality {
	var q Quality

	switch {
	case reRes2160.MatchString(title):
		q.Resolution = "2160p"
	case reRes1080.MatchString(title):
		q.Resolution = "1080p"
	case reRes720.MatchString(title):
		q.Resolution = "720p"
	case reRes480.MatchString(title):
		q.Resolution = "480p"
	}

	switch {
	case reRemux.MatchString(title):
		q.Source = "REMUX"
	case reBluray.MatchString(title):
		q.Source = "BluRay"
	case reWebDL.MatchString(title):
		q.Source = "WEB-DL"
	case reWebRip.MatchString(title):
		q.Source = "WEBRip"
	case reWeb.MatchString(title):
		q.Source = "WEB-DL"
	case reHDTV.MatchString(title):
		q.Source = "HDTV"
	case reDVDRip.MatchString(title):
		q.Source = "DVDRip"
	case reCAM.MatchString(title):
		q.Source = "CAM"
	}

	// A low-quality source with no pixel resolution implies SD.
	if q.Resolution == "" && (q.Source == "DVDRip" || q.Source == "CAM") {
		q.Resolution = "SD"
	}

	switch {
	case reHEVC.MatchString(title):
		q.Codec = "HEVC"
	case reAVC.MatchString(title):
		q.Codec = "AVC"
	case reAV1.MatchString(title):
		q.Codec = "AV1"
	}

	// HDR flags, strongest first. DV is independent of the HDR tier; the
	// ordered switch emits exactly one tier so "HDR10+" never also adds "HDR".
	if reDV.MatchString(title) {
		q.HDR = append(q.HDR, "DV")
	}
	switch {
	case reHDR10p.MatchString(title):
		q.HDR = append(q.HDR, "HDR10+")
	case reHDR10.MatchString(title):
		q.HDR = append(q.HDR, "HDR10")
	case reHDR.MatchString(title):
		q.HDR = append(q.HDR, "HDR")
	}

	q.TenBit = reTenBit.MatchString(title)

	switch {
	case reAtmos.MatchString(title):
		q.Audio = "Atmos"
	case reDTSHD.MatchString(title):
		q.Audio = "DTS-HD"
	case reDTS.MatchString(title):
		q.Audio = "DTS"
	case reDDP.MatchString(title):
		q.Audio = "DDP"
	case reAC3.MatchString(title):
		q.Audio = "AC3"
	}

	return q
}

// qualityBadge returns the compact badge for the name's second line, e.g.
// "2160p" or "2160p DV·HDR10+". It is empty when no resolution was parsed, so
// callers can omit the line entirely rather than show a misleading badge.
func qualityBadge(q Quality) string {
	if q.Resolution == "" {
		return ""
	}
	if len(q.HDR) == 0 {
		return q.Resolution
	}
	return q.Resolution + " " + strings.Join(q.HDR, "·")
}

// qualitySummary renders a one-line human summary such as
// "1080p • WEB-DL • HEVC • HDR". Empty segments are omitted; the result is ""
// when nothing was recognized.
func qualitySummary(q Quality) string {
	parts := make([]string, 0, 5)
	if q.Resolution != "" {
		parts = append(parts, q.Resolution)
	}
	if q.Source != "" {
		parts = append(parts, q.Source)
	}
	if q.Codec != "" {
		c := q.Codec
		if q.TenBit {
			c += " 10bit"
		}
		parts = append(parts, c)
	}
	if len(q.HDR) > 0 {
		parts = append(parts, strings.Join(q.HDR, " "))
	}
	if q.Audio != "" {
		parts = append(parts, q.Audio)
	}
	return strings.Join(parts, " • ")
}
