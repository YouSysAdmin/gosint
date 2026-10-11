// Gosint, Copyright (c) 2026 YouSysAdmin

package transforms_test

import (
	"context"
	"net"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/dns/dnsmessage"

	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/domain"
)

// answer writes the records of one question into a reply.
type answer func(b *dnsmessage.Builder, q dnsmessage.Question)

// fakeDNS serves a fixed zone over UDP on the loopback and returns a
// resolver pointed at it, so a DNS test reaches no real server.
func fakeDNS(t *testing.T, zone map[string]answer) *net.Resolver {
	t.Helper()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = conn.Close() })

	go func() {
		buf := make([]byte, 1500)
		for {
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}

			var p dnsmessage.Parser
			hdr, err := p.Start(buf[:n])
			if err != nil {
				continue
			}

			qs, _ := p.AllQuestions()
			b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: hdr.ID, Response: true, RecursionAvailable: true})
			b.EnableCompression()
			_ = b.StartQuestions()
			for _, q := range qs {
				_ = b.Question(q)
			}

			_ = b.StartAnswers()
			for _, q := range qs {
				if a := zone[strings.ToLower(q.Name.String())+" "+q.Type.String()]; a != nil {
					a(&b, q)
				}
			}

			out, err := b.Finish()
			if err == nil {
				_, _ = conn.WriteTo(out, addr)
			}
		}
	}()

	return &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer

		return d.DialContext(ctx, "udp", conn.LocalAddr().String())
	}}
}

func name(s string) dnsmessage.Name { return dnsmessage.MustNewName(s) }

func header(q dnsmessage.Question) dnsmessage.ResourceHeader {
	return dnsmessage.ResourceHeader{Name: q.Name, Class: dnsmessage.ClassINET, TTL: 60}
}

// Every record kind becomes its own kind of node, an alias is read from
// the CNAME chain, and an MX edge carries its priority.
func TestDomainToDNSReadsEveryRecordKind(t *testing.T) {
	h := newHarness(t)
	h.resolver = fakeDNS(t, map[string]answer{
		"acme.test. TypeA": func(b *dnsmessage.Builder, q dnsmessage.Question) {
			_ = b.AResource(header(q), dnsmessage.AResource{A: [4]byte{192, 0, 2, 10}})
		},
		"acme.test. TypeMX": func(b *dnsmessage.Builder, q dnsmessage.Question) {
			_ = b.MXResource(header(q), dnsmessage.MXResource{Pref: 10, MX: name("mx1.mail.test.")})
			_ = b.MXResource(header(q), dnsmessage.MXResource{Pref: 20, MX: name("mx2.mail.test.")})
		},
		"acme.test. TypeNS": func(b *dnsmessage.Builder, q dnsmessage.Question) {
			_ = b.NSResource(header(q), dnsmessage.NSResource{NS: name("ns1.dns.test.")})
		},
		"acme.test. TypeTXT": func(b *dnsmessage.Builder, q dnsmessage.Question) {
			_ = b.TXTResource(header(q), dnsmessage.TXTResource{TXT: []string{"v=spf1 include:_spf.mail.test ~all"}})
		},
		"www.acme.test. TypeA": func(b *dnsmessage.Builder, q dnsmessage.Question) {
			_ = b.CNAMEResource(header(q), dnsmessage.CNAMEResource{CNAME: name("acme.test.")})
			hdr := header(q)
			hdr.Name = name("acme.test.")
			_ = b.AResource(hdr, dnsmessage.AResource{A: [4]byte{192, 0, 2, 10}})
		},
	})

	h.run(domain.DomainToDNS{}, map[string]string{"ipv6": "false"},
		h.node("domain", map[string]any{"domain": "acme.test"}),
		h.node("domain", map[string]any{"domain": "www.acme.test"}))

	h.expectEdges(
		"acme.test -RESOLVES_TO-> 192.0.2.10",
		"acme.test -HAS_MX-> mx1.mail.test",
		"acme.test -HAS_MX-> mx2.mail.test",
		"acme.test -HAS_NS-> ns1.dns.test",
		"acme.test -HAS_DNS_RECORD-> v=spf1 include:_spf.mail.test ~all",
		"www.acme.test -ALIAS_OF-> acme.test",
		"www.acme.test -RESOLVES_TO-> 192.0.2.10",
	)

	_, edges, err := h.st.Graph.Graph(t.Context(), h.sketch)
	if err != nil {
		t.Fatal(err)
	}

	i := slices.IndexFunc(edges, func(e graphmodel.Edge) bool { return e.Label == "HAS_MX" && e.Properties["priority"] == float64(10) })
	if i < 0 {
		t.Errorf("no HAS_MX edge carries priority 10: %v", edges)
	}
}
