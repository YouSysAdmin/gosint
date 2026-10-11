// Gosint, Copyright (c) 2026 YouSysAdmin

package transform_test

import (
	"net/netip"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/transform"
)

// The IPv6 forms that wrap or translate an internal IPv4 address, and
// the old site-local range, are not public: a host on such a network
// routes them inward.
func TestInternalAddressesInIPv6FormsAreNotPublic(t *testing.T) {
	for addr, want := range map[string]bool{
		"8.8.8.8": true, "2606:4700::1111": true, "203.0.113.9": true,
		"10.0.0.1": false, "127.0.0.1": false, "169.254.169.254": false, "::ffff:127.0.0.1": false,
		"::7f00:1": false, "::a9fe:a9fe": false, "64:ff9b::a9fe:a9fe": false, "64:ff9b:1::a9fe:a9fe": false,
		"fec0::1": false, "2001::1": false, "2002:7f00:1::": false, "fd00:ec2::254": false, "100::1": false,
	} {
		if got := transform.Public(netip.MustParseAddr(addr)); got != want {
			t.Errorf("Public(%s) = %v, want %v", addr, got, want)
		}
	}

	for _, addr := range []string{"169.254.169.254", "::ffff:169.254.169.254", "fd00:ec2::254", "100.100.100.200"} {
		if !transform.Metadata(netip.MustParseAddr(addr)) {
			t.Errorf("%s is not a metadata address", addr)
		}
	}
}
