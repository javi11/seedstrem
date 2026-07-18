package deluge

import "testing"

func TestRawPieceStateToPieceState(t *testing.T) {
	tests := map[rawPieceState]PieceState{
		rawPieceMissing:     PieceMissing,
		rawPieceAvailable:   PieceMissing,
		rawPieceDownloading: PieceDownloading,
		rawPieceCompleted:   PieceHave,
		rawPieceState(99):   PieceMissing, // unknown value defaults safe
	}
	for raw, want := range tests {
		if got := raw.toPieceState(); got != want {
			t.Errorf("rawPieceState(%d).toPieceState() = %v, want %v", raw, got, want)
		}
	}
}

func TestToInt64(t *testing.T) {
	tests := []struct {
		in      any
		want    int64
		wantErr bool
	}{
		{int64(42), 42, false},
		{int32(42), 42, false},
		{int16(42), 42, false},
		{int8(42), 42, false},
		{int(42), 42, false},
		{uint64(42), 42, false},
		{uint32(42), 42, false},
		{"not a number", 0, true},
		{nil, 0, true},
	}
	for _, tt := range tests {
		got, err := toInt64(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("toInt64(%v) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if err == nil && got != tt.want {
			t.Errorf("toInt64(%v) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
