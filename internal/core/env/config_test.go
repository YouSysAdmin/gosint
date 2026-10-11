// Gosint, Copyright (c) 2026 YouSysAdmin

package env_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/env"
)

// A server reachable by others must not let strangers sign up unless
// the operator asked for it.
func TestRegistrationIsClosedByDefault(t *testing.T) {
	t.Chdir(t.TempDir())

	cfg, err := env.Load("")
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Auth.OpenRegistration {
		t.Error("auth.open_registration defaults to true")
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("defaults do not validate: %v", err)
	}
}

func TestEnvironmentOverridesTheDefault(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("GOSINT_AUTH_OPEN_REGISTRATION", "true")

	cfg, err := env.Load("")
	if err != nil {
		t.Fatal(err)
	}

	if !cfg.Auth.OpenRegistration {
		t.Error("GOSINT_AUTH_OPEN_REGISTRATION=true was not applied")
	}
}

// An install that must not reach a tile server empties the map styles,
// which must not fall back to the defaults.
func TestAnEmptyMapStyleStaysEmpty(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := os.WriteFile("gosint.yaml", []byte("console:\n  map_style_light: \"\"\n  map_style_dark: \"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := env.Load("")
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Console.MapStyleLight != "" || cfg.Console.MapStyleDark != "" {
		t.Errorf("map styles = %q, %q", cfg.Console.MapStyleLight, cfg.Console.MapStyleDark)
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("empty map styles do not validate: %v", err)
	}

	cfg.Console.MapStyleDark = "javascript:alert(1)"
	if err := cfg.Validate(); err == nil {
		t.Error("a non http(s) map style was accepted")
	}
}

// A public URL the links and the cross-site check cannot use is refused
// at boot rather than quietly ignored.
func TestAMalformedPublicURLIsRefused(t *testing.T) {
	t.Chdir(t.TempDir())

	cfg, err := env.Load("")
	if err != nil {
		t.Fatal(err)
	}

	for _, bad := range []string{"osint.example.com", "ftp://osint.example.com", "https://", "::"} {
		cfg.Server.PublicURL = bad
		if cfg.Validate() == nil {
			t.Errorf("public_url %q validated", bad)
		}
	}

	for _, good := range []string{"", "https://osint.example.com", "http://localhost:5001/"} {
		cfg.Server.PublicURL = good
		if err := cfg.Validate(); err != nil {
			t.Errorf("public_url %q refused: %v", good, err)
		}
	}
}

// The TLS block: what is refused, and the name a generated certificate
// carries.
func TestTLSBlock(t *testing.T) {
	t.Chdir(t.TempDir())

	cfg, err := env.Load("")
	if err != nil {
		t.Fatal(err)
	}

	if cfg.TerminatesTLS() {
		t.Error("TLS is on by default")
	}

	if got := cfg.TLSHost(); got != "localhost" {
		t.Errorf("TLSHost with nothing set = %q", got)
	}

	if got := cfg.TLSCacheDir(); got != filepath.Join("data", "tls") {
		t.Errorf("TLSCacheDir = %q, want beside the database", got)
	}

	cfg.Server.PublicURL = "https://osint.example.com"
	if got := cfg.TLSHost(); got != "osint.example.com" {
		t.Errorf("TLSHost from public_url = %q", got)
	}

	refused := map[string]env.TLSConfig{
		"unknown mode":         {Mode: "sure"},
		"acme is not offered":  {Mode: "acme"},
		"manual without files": {Mode: env.TLSManual},
		"unknown algorithm":    {Mode: env.TLSSelf, Alg: "dsa"},
	}
	for name, block := range refused {
		cfg.Server.TLS = block
		if cfg.Validate() == nil {
			t.Errorf("%s validated", name)
		}
	}

	cfg.Server.TLS = env.TLSConfig{Mode: env.TLSSelf}
	if err := cfg.Validate(); err != nil {
		t.Errorf("self with an https public_url refused: %v", err)
	}

	cfg.Server.PublicURL = "http://osint.example.com"
	if cfg.Validate() == nil {
		t.Error("TLS with an http public_url validated")
	}

	cfg.Server.PublicURL = ""
	if err := cfg.Validate(); err != nil {
		t.Errorf("self with no public_url refused: %v", err)
	}
}
