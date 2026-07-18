module github.com/javib/seedstrem

go 1.25.0

require (
	github.com/autobrr/go-deluge v0.0.0-00010101000000-000000000000
	github.com/go-chi/chi/v5 v5.3.0
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/sqlite v1.53.0
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/gdm85/go-rencode v0.1.8 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sys v0.44.0 // indirect
	modernc.org/libc v1.73.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

// Local fork adding PieceStates/SetFilePriorities RPC calls upstream is
// missing. See third_party/go-deluge/FORK_NOTES.md.
replace github.com/autobrr/go-deluge => ./third_party/go-deluge
