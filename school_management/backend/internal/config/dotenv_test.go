
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	content := "# local settings\n" +
		"DATABASE_URL=\"postgres://user:pass#word@localhost/db?x=1\"\n" +
		"export FRONTEND_URL='http://localhost:3000' # comment\n" +
		"PORT=8080 # comment\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	values, err := readEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if values["DATABASE_URL"] != "postgres://user:pass#word@localhost/db?x=1" ||
		values["FRONTEND_URL"] != "http://localhost:3000" || values["PORT"] != "8080" {
		t.Error("quoted values, equals signs, or comments were parsed incorrectly")
	}
	t.Setenv("PORT", "9090")
	if got := envValue("PORT", values); got != "9090" {
		t.Errorf("shell variable should take precedence, got %q", got)
	}
}

func TestEnvFileErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if _, err := readEnvFile(path); err != nil {
		t.Fatalf("missing optional file should be accepted: %v", err)
	}
	if err := os.WriteFile(path, []byte("DATABASE_URL=\"unfinished\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readEnvFile(path); err == nil {
		t.Error("unterminated quoted value was accepted")
	}
}

func TestLoadUsesLocalEnvFile(t *testing.T) {
	dir := t.TempDir()
	content := "DATABASE_URL=\"postgres://localhost/school\"\n" +
		"SUPABASE_URL=\"https://example.supabase.co\"\n" +
		"FRONTEND_URL=http://localhost:3000\nPORT=9090\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"DATABASE_URL", "SUPABASE_URL", "SUPABASE_JWT_SECRET", "FRONTEND_URL", "PORT"} {
		old, present := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
		key := key
		t.Cleanup(func() {
			if present {
				_ = os.Setenv(key, old)
			} else {
				_ = os.Unsetenv(key)
			}
		})
	}
	t.Chdir(dir)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://localhost/school" || cfg.SupabaseURL != "https://example.supabase.co" ||
		cfg.FrontendURL != "http://localhost:3000" || cfg.Port != "9090" {
		t.Error("Load did not use the local .env values")
	}
}