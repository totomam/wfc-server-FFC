package common

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// NET-19: deploy/SHA256SUMS is the published hash operators check before running a deploy script; it must match
// the scripts in the same commit (regenerate: cd deploy && sha256sum update.sh vultr_setup.sh > SHA256SUMS).
func TestDeploySHA256SUMS(t *testing.T) {
	f, err := os.Open("../deploy/SHA256SUMS")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		sum, name, ok := strings.Cut(sc.Text(), "  ")
		if !ok {
			t.Fatalf("bad line %q", sc.Text())
		}
		b, err := os.ReadFile(filepath.Join("../deploy", name))
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(b)
		if hex.EncodeToString(h[:]) != sum {
			t.Errorf("%s: SHA256SUMS is stale (cd deploy && sha256sum update.sh vultr_setup.sh > SHA256SUMS)", name)
		}
		seen[name] = true
	}
	for _, n := range []string{"update.sh", "vultr_setup.sh"} {
		if !seen[n] {
			t.Errorf("%s missing from SHA256SUMS", n)
		}
	}
}
