package config

import (
	"os"
	"path/filepath"
	"testing"
)

func withTempConfig(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yml")
	t.Setenv(EnvConfig, p)
	t.Setenv(EnvURL, "")
	t.Setenv(EnvToken, "")
	t.Setenv(EnvInstance, "")
	return p
}

func TestLoadMissingFile(t *testing.T) {
	withTempConfig(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Instances) != 0 || cfg.Default != "" {
		t.Fatalf("expected empty config, got %+v", cfg)
	}
}

func TestSaveLoadRoundtripAndPerms(t *testing.T) {
	p := withTempConfig(t)
	cfg := &Config{
		Default: "work",
		Instances: map[string]Instance{
			"work": {URL: "https://support.example.com", Token: "secret"},
		},
	}
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("expected 0600 perms, got %o", perm)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Default != "work" || got.Instances["work"].Token != "secret" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
}

func TestResolveOrder(t *testing.T) {
	withTempConfig(t)
	if err := Save(&Config{
		Default: "a",
		Instances: map[string]Instance{
			"a": {URL: "https://a.example.com", Token: "ta"},
			"b": {URL: "https://b.example.com", Token: "tb"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	// Default from config file.
	name, url, token, err := Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	if name != "a" || url != "https://a.example.com" || token != "ta" {
		t.Fatalf("default resolution wrong: %s %s %s", name, url, token)
	}

	// ZAMMAD_INSTANCE beats default.
	t.Setenv(EnvInstance, "b")
	if name, _, _, _ = Resolve(""); name != "b" {
		t.Fatalf("env instance not honored, got %s", name)
	}

	// --instance flag beats env.
	if name, _, _, _ = Resolve("a"); name != "a" {
		t.Fatalf("flag instance not honored, got %s", name)
	}

	// ZAMMAD_URL + ZAMMAD_TOKEN beat everything.
	t.Setenv(EnvURL, "https://env.example.com")
	t.Setenv(EnvToken, "tenv")
	_, url, token, err = Resolve("a")
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://env.example.com" || token != "tenv" {
		t.Fatalf("env override wrong: %s %s", url, token)
	}
}

func TestResolveErrors(t *testing.T) {
	withTempConfig(t)
	if _, _, _, err := Resolve(""); err == nil {
		t.Fatal("expected error with empty config")
	}
	if err := Save(&Config{Instances: map[string]Instance{"x": {URL: "u", Token: "t"}}}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := Resolve("nope"); err == nil {
		t.Fatal("expected error for unknown instance")
	}
}
