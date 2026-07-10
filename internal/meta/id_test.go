package meta

import "testing"

func TestParseID(t *testing.T) {
	tests := []struct {
		name       string
		typ, id    string
		wantSource string
		wantID     string
		wantKind   Kind
		wantSeason int
		wantEp     int
		wantErr    bool
	}{
		{name: "imdb movie", typ: "movie", id: "tt1375666", wantSource: "tt", wantID: "tt1375666", wantKind: KindMovie},
		{name: "imdb series", typ: "series", id: "tt0944947:2:9", wantSource: "tt", wantID: "tt0944947", wantKind: KindSeries, wantSeason: 2, wantEp: 9},
		{name: "imdb bare series type", typ: "series", id: "tt0944947", wantSource: "tt", wantID: "tt0944947", wantKind: KindSeries},
		{name: "kitsu movie", typ: "movie", id: "kitsu:1376", wantSource: "kitsu", wantID: "1376", wantKind: KindMovie},
		{name: "kitsu series episode", typ: "series", id: "kitsu:44081:5", wantSource: "kitsu", wantID: "44081", wantKind: KindSeries, wantEp: 5},
		{name: "mal episode", typ: "series", id: "mal:20:12", wantSource: "mal", wantID: "20", wantKind: KindSeries, wantEp: 12},
		{name: "empty", id: "", wantErr: true},
		{name: "unsupported", id: "foo:1:2", wantErr: true},
		{name: "kitsu missing id", id: "kitsu:", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := ParseID(tt.typ, tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", q)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Source != tt.wantSource || q.ID != tt.wantID || q.Kind != tt.wantKind ||
				q.Season != tt.wantSeason || q.Episode != tt.wantEp {
				t.Errorf("ParseID(%q,%q) = %+v", tt.typ, tt.id, q)
			}
		})
	}
}
