// Package torrents holds the reusable "add magnet → wait for metadata →
// select a file → mint a streaming link" mechanics shared by the Stremio
// addon (and previously the RealDebrid API emulation). It is decoupled
// from any particular HTTP surface.
package torrents

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

const (
	idAlphabet    = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567" // base32-ish ids
	idLength      = 13
	tokenAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	tokenLength   = 32
)

func randomString(alphabet string, length int) (string, error) {
	out := make([]byte, length)
	max := big.NewInt(int64(len(alphabet)))
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("random string: %w", err)
		}
		out[i] = alphabet[n.Int64()]
	}
	return string(out), nil
}

// NewID returns an opaque torrent id (the torrents table primary key).
func NewID() (string, error) {
	return randomString(idAlphabet, idLength)
}

// NewLinkToken returns an opaque streaming link token.
func NewLinkToken() (string, error) {
	return randomString(tokenAlphabet, tokenLength)
}
