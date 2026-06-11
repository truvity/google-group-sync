package config

import (
	"testing"
)

func TestLoad_MissingRequired(t *testing.T) {
	t.Setenv("GOOGLE_ADMIN_EMAIL", "")
	t.Setenv("GOOGLE_SA_KEY_JSON", "")
	t.Setenv("GOOGLE_SA_KEY_FILE", "")

	defaults := DefaultConfig()
	_, err := Load(&defaults, Options{})
	if err == nil {
		t.Fatal("expected error for missing required env vars")
	}
}

func TestLoad_ValidMinimal(t *testing.T) {
	t.Setenv("GOOGLE_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("GOOGLE_SA_KEY_JSON", `{"type":"service_account"}`)

	defaults := DefaultConfig()
	cfg, err := Load(&defaults, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.GoogleAdminEmail != "admin@example.com" {
		t.Errorf("got GoogleAdminEmail=%q", cfg.GoogleAdminEmail)
	}

	if cfg.Port != 8080 {
		t.Errorf("got Port=%d, want 8080", cfg.Port)
	}

	if cfg.HealthPort != 7070 {
		t.Errorf("got HealthPort=%d, want 7070", cfg.HealthPort)
	}
}

func TestLoad_WithPrefix(t *testing.T) {
	t.Setenv("GGS_GOOGLE_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("GGS_GOOGLE_SA_KEY_JSON", `{"type":"service_account"}`)
	t.Setenv("GGS_PORT", "9999")

	// Ensure unprefixed vars don't interfere.
	t.Setenv("PORT", "8080")
	t.Setenv("GOOGLE_ADMIN_EMAIL", "wrong@example.com")

	defaults := ExtensionDefaults()
	cfg, err := Load(&defaults, Options{Prefix: "GGS_"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.GoogleAdminEmail != "admin@example.com" {
		t.Errorf("got GoogleAdminEmail=%q, want admin@example.com", cfg.GoogleAdminEmail)
	}

	if cfg.Port != 9999 {
		t.Errorf("got Port=%d, want 9999 (from GGS_PORT)", cfg.Port)
	}

	if cfg.HealthPort != 0 {
		t.Errorf("got HealthPort=%d, want 0 (extension default)", cfg.HealthPort)
	}
}

func TestLoad_ExtensionDefaults(t *testing.T) {
	t.Setenv("GGS_GOOGLE_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("GGS_GOOGLE_SA_KEY_JSON", `{"type":"service_account"}`)

	defaults := ExtensionDefaults()
	cfg, err := Load(&defaults, Options{Prefix: "GGS_"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != 9090 {
		t.Errorf("got Port=%d, want 9090 (extension default)", cfg.Port)
	}

	if cfg.HealthPort != 0 {
		t.Errorf("got HealthPort=%d, want 0 (extension default)", cfg.HealthPort)
	}
}

func TestLoad_MutuallyExclusive(t *testing.T) {
	t.Setenv("GOOGLE_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("GOOGLE_SA_KEY_JSON", `{"type":"service_account"}`)
	t.Setenv("GOOGLE_SA_KEY_FILE", "/some/path")

	defaults := DefaultConfig()
	_, err := Load(&defaults, Options{})
	if err == nil {
		t.Fatal("expected error for mutually exclusive SA key options")
	}
}

func TestLoad_PortOverride(t *testing.T) {
	t.Setenv("GOOGLE_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("GOOGLE_SA_KEY_JSON", `{"type":"service_account"}`)
	t.Setenv("PORT", "3000")

	defaults := DefaultConfig()
	cfg, err := Load(&defaults, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != 3000 {
		t.Errorf("got Port=%d, want 3000", cfg.Port)
	}
}
