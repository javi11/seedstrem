package metainfo

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func TestFromMagnet(t *testing.T) {
	hexHash := "0123456789abcdef0123456789abcdef01234567"

	tests := []struct {
		name     string
		magnet   string
		wantHash string
		wantName string
		wantErr  bool
	}{
		{
			name:     "hex hash with name",
			magnet:   "magnet:?xt=urn:btih:" + hexHash + "&dn=My+Movie",
			wantHash: hexHash,
			wantName: "My Movie",
		},
		{
			name:     "uppercase hex normalized",
			magnet:   "magnet:?xt=urn:btih:" + strings.ToUpper(hexHash),
			wantHash: hexHash,
		},
		{
			name:     "base32 hash",
			magnet:   "magnet:?xt=urn:btih:AEBAGBAFAYDQQCIKBMGA2DQPCAIREEYU",
			wantHash: "0102030405060708090a0b0c0d0e0f1011121314",
		},
		{name: "not a magnet", magnet: "http://example.com", wantErr: true},
		{name: "no xt param", magnet: "magnet:?dn=test", wantErr: true},
		{name: "bad hex", magnet: "magnet:?xt=urn:btih:" + strings.Repeat("z", 40), wantErr: true},
		{name: "wrong length", magnet: "magnet:?xt=urn:btih:abcdef", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, name, err := FromMagnet(tt.magnet)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalid) {
					t.Errorf("want ErrInvalid, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if hash != tt.wantHash {
				t.Errorf("hash = %q; want %q", hash, tt.wantHash)
			}
			if tt.wantName != "" && name != tt.wantName {
				t.Errorf("name = %q; want %q", name, tt.wantName)
			}
		})
	}
}

func TestFromTorrent(t *testing.T) {
	// Hand-built minimal torrent: the infohash is the SHA-1 of the raw
	// info dict bytes.
	infoDict := "d6:lengthi1024e4:name8:test.mkv12:piece lengthi16384e6:pieces20:aaaaaaaaaaaaaaaaaaaae"
	torrent := "d8:announce18:http://tracker/ann4:info" + infoDict + "e"

	wantHash := sha1.Sum([]byte(infoDict))

	hash, name, err := FromTorrent([]byte(torrent))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash != hex.EncodeToString(wantHash[:]) {
		t.Errorf("hash = %q; want %q", hash, hex.EncodeToString(wantHash[:]))
	}
	if name != "test.mkv" {
		t.Errorf("name = %q; want test.mkv", name)
	}
}

func TestFromTorrentMultiFile(t *testing.T) {
	infoDict := "d5:filesld6:lengthi100e4:pathl5:a.mkveed6:lengthi200e4:pathl5:b.mkveee4:name6:folder12:piece lengthi16384e6:pieces20:bbbbbbbbbbbbbbbbbbbbe"
	torrent := "d4:info" + infoDict + "e"

	wantHash := sha1.Sum([]byte(infoDict))
	hash, name, err := FromTorrent([]byte(torrent))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash != hex.EncodeToString(wantHash[:]) {
		t.Error("multi-file infohash mismatch")
	}
	if name != "folder" {
		t.Errorf("name = %q; want folder", name)
	}
}

func TestFromTorrentInvalid(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"empty", ""},
		{"garbage", "not bencode at all"},
		{"no info dict", "d8:announce4:abcde"},
		{"info not a dict", "d4:info4:abcde"},
		{"truncated", "d4:infod4:name3:ab"},
		{"unterminated int", "d4:infod6:lengthi123"},
		{"string too long", "d4:info999:xe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := FromTorrent([]byte(tt.data))
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("want ErrInvalid, got %v", err)
			}
		})
	}
}

func FuzzFromTorrent(f *testing.F) {
	f.Add([]byte("d4:infod6:lengthi1024e4:name8:test.mkvee"))
	f.Add([]byte("d4:infod5:filesld6:lengthi100e4:pathl5:a.mkveee4:name6:folderee"))
	f.Add([]byte("i42e"))
	f.Add([]byte(""))
	f.Fuzz(func(t *testing.T, data []byte) {
		// Must never panic; errors are fine.
		hash, _, err := FromTorrent(data)
		if err == nil && len(hash) != 40 {
			t.Errorf("valid parse returned bad hash length: %q", hash)
		}
	})
}

func TestDecoderIntEdgeCases(t *testing.T) {
	// Negative integers are legal bencode.
	d := &decoder{data: []byte("i-42e")}
	v, err := d.decode()
	if err != nil {
		t.Fatal(err)
	}
	if v.(int64) != -42 {
		t.Errorf("got %v; want -42", v)
	}
}

func TestBase32Vector(t *testing.T) {
	// The base32 form must decode to the same bytes as its hex form.
	raw, _ := hex.DecodeString("0102030405060708090a0b0c0d0e0f1011121314")
	got, _, err := FromMagnet("magnet:?xt=urn:btih:AEBAGBAFAYDQQCIKBMGA2DQPCAIREEYU")
	if err != nil {
		t.Fatal(err)
	}
	if got != hex.EncodeToString(raw) {
		t.Errorf("base32 decode mismatch: %s", got)
	}
}
