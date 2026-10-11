// Gosint, Copyright (c) 2026 YouSysAdmin

package types

import (
	"net/netip"
	"regexp"
	"strings"
)

// Phone numbers are normalized to E.164.
//
// No libphonenumber, the package stays on the stdlib. These rules
// cover ten common regions by their national number plans, and accept
// any well formed international
// number. They are coarser than libphonenumber: a number in a valid
// shape for its plan is taken without checking it is allocated.

// phoneSeparators are dropped before a number is read.
var phoneSeparators = strings.NewReplacer(" ", "", "-", "", ".", "", "(", "", ")", "", "/", "")

// internationalRe is "+", a country code and a subscriber number, 8 to
// 15 digits in all as E.164 allows.
var internationalRe = regexp.MustCompile(`^\+[1-9]\d{7,14}$`)

// nationalPlan reads a number dialled within one region.
type nationalPlan struct {
	region  string
	pattern *regexp.Regexp

	// country is the calling code, trunk the national prefix dropped
	// before it.
	country string
	trunk   string
}

// nationalPlans are tried in order, first match wins.
var nationalPlans = []nationalPlan{
	{"US", regexp.MustCompile(`^1?[2-9]\d{2}[2-9]\d{6}$`), "1", "1"},
	{"GB", regexp.MustCompile(`^0[1-9]\d{9}$`), "44", "0"},
	{"FR", regexp.MustCompile(`^0[1-9]\d{8}$`), "33", "0"},
	{"DE", regexp.MustCompile(`^0[1-9]\d{5,12}$`), "49", "0"},
	{"ES", regexp.MustCompile(`^[6-9]\d{8}$`), "34", ""},
	{"IT", regexp.MustCompile(`^(?:3\d{8,9}|0\d{5,10})$`), "39", ""},
	{"CA", regexp.MustCompile(`^1?[2-9]\d{2}[2-9]\d{6}$`), "1", "1"},
	{"AU", regexp.MustCompile(`^0[2-478]\d{8}$`), "61", "0"},
	{"JP", regexp.MustCompile(`^0[1-9]\d{8,9}$`), "81", "0"},
	{"CN", regexp.MustCompile(`^1[3-9]\d{9}$`), "86", ""},
}

// e164International reads a "+" number.
func e164International(raw string) (string, bool) {
	s := phoneSeparators.Replace(raw)
	if !internationalRe.MatchString(s) {
		return "", false
	}

	return s, true
}

// e164National reads a number in one of the national plans.
func e164National(raw string) (string, bool) {
	s := phoneSeparators.Replace(raw)
	if !allDigits(s) {
		return "", false
	}

	for _, p := range nationalPlans {
		if !p.pattern.MatchString(s) {
			continue
		}

		national := s
		if p.trunk != "" && strings.HasPrefix(s, p.trunk) && (p.trunk != "1" || len(s) == 11) {
			national = s[len(p.trunk):]
		}

		return "+" + p.country + national, true
	}

	return "", false
}

// normalizePhone is the Phone.number validator: an international form
// first, then the national plans.
func normalizePhone(raw string) (string, bool) {
	if strings.HasPrefix(raw, "+") {
		if n, ok := e164International(raw); ok {
			return n, true
		}
	}

	return e164National(raw)
}

// detectPhone is Phone.detect: never an IP address, an international
// form decides on its own validity, otherwise the national plans.
func detectPhone(s string) bool {
	if _, err := netip.ParseAddr(s); err == nil {
		return false
	}

	if strings.HasPrefix(s, "+") {
		_, ok := e164International(s)

		return ok
	}

	_, ok := e164National(s)

	return ok
}
