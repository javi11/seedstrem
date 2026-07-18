package deluge

import (
	"errors"
	"testing"

	libdeluge "github.com/autobrr/go-deluge"
)

func TestConvertTorrent(t *testing.T) {
	ts := &libdeluge.TorrentStatus{
		Hash:                "abc123",
		Name:                "Movie",
		State:               "Downloading",
		Progress:            42.5,
		TotalSize:           1000,
		DownloadPayloadRate: 500,
		NumSeeds:            7,
		SavePath:            "/downloads",
	}
	info := convertTorrent(ts)
	if info.Hash != "abc123" || info.Name != "Movie" || info.State != "Downloading" {
		t.Errorf("basic fields lost: %+v", info)
	}
	if info.Progress != 0.425 {
		t.Errorf("progress = %v, want 0.425 (converted from 0-100 scale)", info.Progress)
	}
	if info.Size != 1000 || info.DlSpeed != 500 || info.NumSeeds != 7 || info.SavePath != "/downloads" {
		t.Errorf("fields lost: %+v", info)
	}
}

func TestIsNotFound(t *testing.T) {
	if isNotFound(errors.New("some other error")) {
		t.Error("generic error should not be treated as not-found")
	}
	if isNotFound(nil) {
		t.Error("nil error should not be treated as not-found")
	}
	rpcErr := libdeluge.RPCError{ExceptionType: "InvalidTorrentError", ExceptionMessage: "unknown hash"}
	if !isNotFound(rpcErr) {
		t.Error("InvalidTorrentError should be treated as not-found")
	}
	wrapped := errors.Join(errors.New("context"), rpcErr)
	if !isNotFound(wrapped) {
		t.Error("wrapped InvalidTorrentError should still be detected via errors.As")
	}
}
