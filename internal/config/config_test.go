package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAtomicallyTightensConfigPermissions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CARRITO_CONFIG_DIR", dir)
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Default()
	cfg.Auth.Cookie = "secret-session"
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("config mode = %o, want 600", got)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("config directory mode = %o, want 700", got)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Auth.Cookie != "secret-session" {
		t.Fatalf("saved config was not readable: %+v", loaded.Auth)
	}
}
