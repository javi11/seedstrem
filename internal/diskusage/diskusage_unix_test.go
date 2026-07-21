//go:build unix

package diskusage

import "testing"

func TestStat(t *testing.T) {
	used, total, err := Stat(t.TempDir())
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if total <= 0 {
		t.Errorf("total = %d, want > 0", total)
	}
	if used < 0 || used > total {
		t.Errorf("used = %d, want in [0, %d]", used, total)
	}
}

func TestStatMissingPath(t *testing.T) {
	if _, _, err := Stat("/no/such/path/seedstrem-diskusage-test"); err == nil {
		t.Error("expected error for a nonexistent path")
	}
}
