// Gosint, Copyright (c) 2026 YouSysAdmin

package transform

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"syscall"
	"time"
)

// ErrPrivateTarget is a fetch of a site that resolves to this host or
// the internal network, refused so a website node cannot be used to
// read services the server can reach and the caller cannot.
var ErrPrivateTarget = errors.New("transform: refusing to fetch an internal address")

// TargetHTTPClient is the client for fetching a site an entity names,
// as opposed to a fixed API. It refuses loopback, private, link-local
// and other non-public destinations at connect time, which also covers
// a redirect or a DNS answer pointing inward. allowPrivate, asked on
// every connection so an administrator's change applies at once, lifts
// that for a lab whose targets are all on private addresses.
func TargetHTTPClient(userAgent string, allowPrivate func() bool) *http.Client {
	guarded, open := TargetDialer(false), TargetDialer(true)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if allowPrivate() {
			return open.DialContext(ctx, network, addr)
		}

		return guarded.DialContext(ctx, network, addr)
	}

	return &http.Client{
		Timeout:   time.Minute,
		Transport: userAgentTransport{agent: userAgent, next: transport},
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("transform: too many redirects")
			}

			return nil
		},
	}
}

// Never and Always are the fixed answers TargetHTTPClient may ask.
func Never() bool { return false }

// Always lets every connection reach private addresses.
func Always() bool { return true }

// TargetDialer connects only to public addresses, checked on the
// address actually dialled, unless allowPrivate. It is what a transform
// uses for a raw connection to a host an entity names. Even a lab's
// dialer never reaches a cloud's metadata service, which hands out the
// host's own credentials.
func TargetDialer(allowPrivate bool) *net.Dialer {
	return &net.Dialer{Timeout: 15 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}

		ip, err := netip.ParseAddr(host)
		if err != nil || Metadata(ip) || (!allowPrivate && !Public(ip)) {
			return fmt.Errorf("%w: %s", ErrPrivateTarget, host)
		}

		return nil
	}}
}

// metadataAddrs are the cloud metadata services: AWS, GCP, Azure and
// OpenStack on the link-local address, AWS over IPv6, Alibaba Cloud.
var metadataAddrs = []netip.Addr{
	netip.MustParseAddr("169.254.169.254"),
	netip.MustParseAddr("fd00:ec2::254"),
	netip.MustParseAddr("100.100.100.200"),
}

// Metadata reports whether ip is a cloud metadata service.
func Metadata(ip netip.Addr) bool {
	ip = ip.Unmap()

	return slices.Contains(metadataAddrs, ip)
}

// notPublic are the ranges IsGlobalUnicast lets through that no public
// host sits in: CGNAT, "this network", benchmarking, IETF protocol
// assignments, class E, IPv6 documentation, unique local and deprecated
// site local, discard, and the IPv6 forms that wrap or translate an IPv4
// address (IPv4 compatible, NAT64 well known and local use, 6to4,
// Teredo), which a host on such a network routes inward.
var notPublic = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fec0::/10"),
}

// Public reports whether ip is a routable, public unicast address, one
// a transform may contact on an entity's behalf.
func Public(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}

	for _, p := range notPublic {
		if p.Contains(ip) {
			return false
		}
	}

	return true
}

// RedactURLError strips the URL out of a transport error. net/http
// reports the whole URL, query included, and a transform's query often
// carries an API key, while its warning lands in a log every member of
// the sketch reads.
func RedactURLError(err error) error {
	ue, ok := errors.AsType[*url.Error](err)
	if !ok {
		return err
	}

	// A URL that does not parse (a control character pasted in a key)
	// is not shown at all, its query may hold the key.
	host := "a URL"
	if u, perr := url.Parse(ue.URL); perr == nil && u.Host != "" {
		host = u.Host
	}

	return fmt.Errorf("%s %s: %w", ue.Op, host, ue.Err)
}
