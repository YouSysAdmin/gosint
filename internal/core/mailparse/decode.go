// Gosint, Copyright (c) 2026 YouSysAdmin

package mailparse

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/quotedprintable"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding/charmap"
)

// Everything in this file answers one question: what do these bytes say?
//
// A message states its own encoding three separate times and in three
// different vocabularies - Content-Transfer-Encoding wraps the octets,
// a charset parameter says how to read them as text, and RFC 2047
// encoded words do both again inside a single header value. A parse that
// skips any one of them produces a string that looks like text and is
// not, which then travels all the way to somebody's screen.

// unwrapTransfer undoes Content-Transfer-Encoding.
//
// Only base64 and quoted-printable are transformations. The rest name
// how the octets were SAFE to send, not how they were changed, so they
// come back untouched - as does an encoding this does not know, since
// handing the raw bytes on is strictly better than refusing the message.
func unwrapTransfer(raw []byte, encoding string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		// Line breaks are how base64 travels in mail, and the decoder
		// refuses them, so they come out first. Spaces and tabs go too:
		// they are not legal here, but they turn up, and rejecting a
		// whole attachment over one is not worth it.
		return base64.StdEncoding.DecodeString(string(bytes.Map(dropWhitespace, raw)))

	case "quoted-printable":
		return io.ReadAll(quotedprintable.NewReader(bytes.NewReader(raw)))

	default:
		return raw, nil
	}
}

func dropWhitespace(r rune) rune {
	switch r {
	case ' ', '\t', '\r', '\n':
		return -1
	default:
		return r
	}
}

// asText reads decoded bytes as a string, honouring a declared charset.
//
// The declaration is TRIED, not trusted: a message that says windows-1251
// and carries UTF-8 is common enough that a conversion producing invalid
// UTF-8 is taken as evidence the label was wrong, and the bytes are read
// the other way instead.
func asText(b []byte, declared string) string {
	if declared == "" {
		return repairUTF8(string(b))
	}

	enc, _ := charset.Lookup(declared)
	if enc == nil {
		return repairUTF8(string(b))
	}

	out, err := enc.NewDecoder().Bytes(b)
	if err != nil || !utf8.Valid(out) {
		return repairUTF8(string(b))
	}

	return string(out)
}

// repairUTF8 makes a string safe to put in JSON.
//
// Valid UTF-8 passes through. Anything else is read as ISO-8859-1, which
// is the one assumption that cannot fail: every byte is a code point
// there, so the result is always valid UTF-8 and no byte is lost. It may
// be the wrong text - that is what a message with no usable charset
// leaves us with - but it is text, and the alternative is a response body
// the JSON encoder refuses to produce.
func repairUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}

	out, err := charmap.ISO8859_1.NewDecoder().String(s)
	if err != nil {
		return strings.ToValidUTF8(s, "")
	}

	return out
}

// words decodes RFC 2047 encoded words in a header value.
//
// The decoder is built per call rather than kept as a package value: it
// closes over a charset lookup, and a shared one would be a mutable
// global in a parser that runs on every inbound message.
func words() *mime.WordDecoder {
	return &mime.WordDecoder{
		CharsetReader: func(label string, input io.Reader) (io.Reader, error) {
			enc, _ := charset.Lookup(label)
			if enc == nil {
				// Unknown label, so the bytes go through as they are
				// and repairUTF8 deals with whatever comes out. An
				// error here would lose the whole header.
				return input, nil
			}

			return enc.NewDecoder().Reader(input), nil
		},
	}
}

// headerText decodes one header value: encoded words first, then
// whatever is left made safe to carry.
func headerText(v string) string {
	if v == "" {
		return ""
	}

	if decoded, err := words().DecodeHeader(v); err == nil {
		return repairUTF8(decoded)
	}

	return repairUTF8(v)
}
