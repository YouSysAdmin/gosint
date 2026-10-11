// Gosint, Copyright (c) 2026 YouSysAdmin

package types

import (
	"errors"
	"net/mail"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// maxURLLength is pydantic's HttpUrl limit.
const maxURLLength = 2083

// normalizeURL validates an http or https URL and returns it as
// pydantic's HttpUrl stores it: scheme and host lowercased, default
// port dropped, an empty path made "/".
func normalizeURL(raw string) (string, error) {
	if len(raw) > maxURLLength {
		return "", errors.New("URL is too long")
	}

	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", errors.New("not a valid URL")
	}

	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("URL scheme must be http or https")
	}

	if u.Hostname() == "" {
		return "", errors.New("URL has no host")
	}

	host, port := strings.ToLower(u.Hostname()), u.Port()
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}

	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		port = ""
	}

	u.Host = host
	if port != "" {
		u.Host += ":" + port
	}

	if u.Path == "" {
		u.Path = "/"
	}

	return u.String(), nil
}

// specialUseTLDs are refused in an email domain, as email-validator does.
var specialUseTLDs = []string{"arpa", "invalid", "local", "localhost", "onion", "test"}

var (
	emailLabelRe = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)
	emailLocalRe = regexp.MustCompile(`^[a-zA-Z0-9!#$%&'*+/=?^_` + "`" + `{|}~-]+(?:\.[a-zA-Z0-9!#$%&'*+/=?^_` + "`" + `{|}~-]+)*$`)
)

// normalizeEmail validates an address the way email-validator does
// without a deliverability check, accepts "Name <a@b.c>", and returns
// the bare address with its domain lowercased.
func normalizeEmail(raw string) (string, error) {
	if len(raw) > 2048 {
		return "", errors.New("email is too long")
	}

	addr, err := mail.ParseAddress(strings.TrimSpace(raw))
	if err != nil {
		return "", errors.New("not a valid email address")
	}

	local, domain, ok := strings.Cut(addr.Address, "@")
	if !ok || !emailLocalRe.MatchString(local) || len(local) > 64 {
		return "", errors.New("the part before @ is not valid")
	}

	domain = strings.ToLower(domain)
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return "", errors.New("the domain must contain a dot")
	}

	for _, l := range labels {
		if !emailLabelRe.MatchString(l) {
			return "", errors.New("the domain is not valid")
		}
	}

	tld := labels[len(labels)-1]
	if _, err := strconv.Atoi(tld); err == nil || slices.Contains(specialUseTLDs, tld) {
		return "", errors.New("the domain is not a deliverable one")
	}

	return local + "@" + domain, nil
}

// urlHostname is Python's urlparse(...).hostname for the netloc of s:
// userinfo and port dropped, brackets of an IPv6 host removed,
// lowercased. "" when s has no netloc.
func urlHostname(s string) string {
	_, rest, ok := strings.Cut(s, "//")
	if !ok {
		return ""
	}

	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		rest = rest[:i]
	}

	if i := strings.LastIndex(rest, "@"); i >= 0 {
		rest = rest[i+1:]
	}

	if strings.HasPrefix(rest, "[") {
		if end := strings.Index(rest, "]"); end > 0 {
			rest = rest[1:end]
		}
	} else if host, _, found := strings.Cut(rest, ":"); found {
		rest = host
	}

	return strings.ToLower(rest)
}

// parseCIDR reads a network the way ipaddress.ip_network(strict=False)
// does: host bits allowed and masked off, a bare address taken as a
// single host, an IPv4 prefix also given as a dotted netmask.
func parseCIDR(s string) (netip.Prefix, error) {
	addrPart, maskPart, hasMask := strings.Cut(s, "/")
	if !hasMask {
		a, err := netip.ParseAddr(s)
		if err != nil {
			return netip.Prefix{}, err
		}

		return netip.PrefixFrom(a, a.BitLen()), nil
	}

	if strings.Contains(maskPart, ".") {
		bits, err := netmaskBits(maskPart)
		if err != nil {
			return netip.Prefix{}, err
		}

		maskPart = strconv.Itoa(bits)
	}

	p, err := netip.ParsePrefix(addrPart + "/" + maskPart)
	if err != nil {
		return netip.Prefix{}, err
	}

	return p.Masked(), nil
}

// netmaskBits turns a dotted IPv4 netmask into its prefix length.
func netmaskBits(mask string) (int, error) {
	m, err := netip.ParseAddr(mask)
	if err != nil || !m.Is4() {
		return 0, errors.New("not a netmask")
	}

	b := m.As4()
	v := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	bits := 0
	for v&0x80000000 != 0 {
		bits++
		v <<= 1
	}

	if v != 0 {
		return 0, errors.New("netmask is not contiguous")
	}

	return bits, nil
}

// luhn is the card number checksum.
func luhn(digits string) bool {
	sum := 0
	for i := range len(digits) {
		d := int(digits[len(digits)-1-i] - '0')
		if i%2 == 1 {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}

		sum += d
	}

	return sum%10 == 0
}

// allDigits reports whether s is non-empty and ASCII digits only.
func allDigits(s string) bool {
	if s == "" {
		return false
	}

	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}

	return true
}
