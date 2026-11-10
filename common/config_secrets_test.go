package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadConfigFrom runs GetConfig on the given config.xml text in a temp dir and returns what it panicked with.
func loadConfigFrom(t *testing.T, xmlText string) (panicked any) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.xml"), []byte(xmlText), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	cmutex.Lock()
	configLoaded, config = false, Config{}
	cmutex.Unlock()
	defer func() {
		panicked = recover()
		cmutex.Lock()
		configLoaded, config = false, Config{}
		cmutex.Unlock()
	}()
	GetConfig()
	return nil
}

// NET-33: the server must refuse to start with the example config's secrets.
func TestExampleConfigRefused(t *testing.T) {
	example, err := os.ReadFile("../config_example.xml")
	if err != nil {
		t.Fatal(err)
	}
	if p := loadConfigFrom(t, string(example)); p == nil {
		t.Fatal("config_example.xml accepted as config.xml")
	}

	// The secrets shipped in earlier example configs are refused too.
	old := strings.NewReplacer("CHANGE_ME_DB_PASSWORD", "password", "CHANGE_ME_API_SECRET", "hQ3f57b3tW2WnjJH3v").Replace(string(example))
	if p := loadConfigFrom(t, old); p == nil {
		t.Fatal("old example secrets accepted")
	}

	real := strings.NewReplacer("CHANGE_ME_DB_PASSWORD", "s3cret-db", "CHANGE_ME_API_SECRET", "a-real-admin-secret").Replace(string(example))
	if p := loadConfigFrom(t, real); p != nil {
		t.Fatalf("real secrets refused: %v", p)
	}
}
