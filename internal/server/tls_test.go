// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/tlsconf"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
	"github.com/yousysadmin/gosint/internal/server"
	"github.com/yousysadmin/gosint/pkg"
)

// The block becomes a tls.Config, the listener wraps itself in it, and
// a client completes a handshake against the certificate's names.
func TestTheServerTerminatesTLS(t *testing.T) {
	cfg := &env.Config{}
	cfg.Server.PublicURL = "https://osint.example.com"
	cfg.Server.TLS = env.TLSConfig{Mode: env.TLSSelf, CacheDir: filepath.Join(t.TempDir(), "tls")}

	tlsCfg, err := tlsconf.Build(cfg)
	if err != nil {
		t.Fatal(err)
	}

	srv, err := server.New(server.Options{Runtime: testRuntime(t, dbtest.SQLite(t), cfg), TLS: tlsCfg})
	if err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	go func() { _ = srv.Serve(ln) }()

	t.Cleanup(func() { _ = srv.Shutdown(time.Second) })

	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}, //nolint:gosec // self-signed on purpose
	}}

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+ln.Addr().String()+"/health", nil)

	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	defer res.Body.Close() //nolint:errcheck // test

	if res.StatusCode != http.StatusOK || res.TLS == nil {
		t.Fatalf("health over tls = %d, tls %v", res.StatusCode, res.TLS != nil)
	}

	leaf := res.TLS.PeerCertificates[0]
	for _, name := range []string{"osint.example.com", "localhost", "127.0.0.1"} {
		if err := leaf.VerifyHostname(name); err != nil {
			t.Errorf("the certificate does not answer for %s: %v", name, err)
		}
	}

	if leaf.Subject.CommonName != "osint.example.com" || !slices.Contains(leaf.Subject.Organization, pkg.AppName) {
		t.Errorf("subject = %v", leaf.Subject)
	}
}

// The generated pair is kept, so a restart serves the same certificate.
func TestTheSelfSignedPairSurvivesARestart(t *testing.T) {
	cfg := &env.Config{}
	cfg.Database.DSN = filepath.Join(t.TempDir(), "gosint.db")
	cfg.Server.TLS = env.TLSConfig{Mode: env.TLSSelf, Alg: "ecdsa"}

	first, err := tlsconf.Build(cfg)
	if err != nil {
		t.Fatal(err)
	}

	second, err := tlsconf.Build(cfg)
	if err != nil {
		t.Fatal(err)
	}

	if string(first.Certificates[0].Certificate[0]) != string(second.Certificates[0].Certificate[0]) {
		t.Error("a second build produced a different certificate")
	}

	leaf, err := x509.ParseCertificate(first.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}

	if err := leaf.VerifyHostname("localhost"); err != nil {
		t.Errorf("no localhost: %v", err)
	}
}

// Plain HTTP is the default.
func TestNoTLSByDefault(t *testing.T) {
	cfg, err := tlsconf.Build(&env.Config{})
	if err != nil || cfg != nil {
		t.Errorf("Build with no block = %v, %v, want nil, nil", cfg, err)
	}
}
