// Gosint, Copyright (c) 2026 YouSysAdmin

// Package tlsconf turns server.tls into the *tls.Config the listener
// wraps itself in.
package tlsconf

import (
	"crypto/tls"
	"fmt"
	"net"
	"slices"

	tlsutils "github.com/yousysadmin/go-tlsutils"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/pkg"
)

// Build resolves the configuration, nil when TLS is off.
func Build(cfg *env.Config) (*tls.Config, error) {
	opts := cfg.TLSOptions()
	if !opts.Enabled() {
		return nil, nil
	}

	if opts.Mode == env.TLSSelf {
		return selfSigned(cfg, opts)
	}

	return tlsutils.Build(opts)
}

// selfSigned is the library's self-signed mode with a Subject naming
// gosint and the loopback names a local install is reached by.
func selfSigned(cfg *env.Config, opts tlsutils.Config) (*tls.Config, error) {
	hosts := []string{opts.FQDN}
	for _, h := range append([]string{"localhost", "127.0.0.1", "::1"}, bindHost(cfg.Server.Addr)) {
		if h != "" && !slices.Contains(hosts, h) {
			hosts = append(hosts, h)
		}
	}

	pair, err := tlsutils.SelfSignedCertificate(tlsutils.SelfSignedOptions{
		Hosts:        hosts,
		Algorithm:    opts.Alg,
		CacheDir:     opts.CacheDir,
		CommonName:   opts.FQDN,
		Organization: []string{pkg.AppName},
	})
	if err != nil {
		return nil, fmt.Errorf("self-signed certificate: %w", err)
	}

	return &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}, nil
}

// bindHost is the address the listener binds when it names one, so a
// LAN install bound to its own address is reached by it.
func bindHost(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}

	if ip := net.ParseIP(host); ip == nil || ip.IsUnspecified() {
		return ""
	}

	return host
}
