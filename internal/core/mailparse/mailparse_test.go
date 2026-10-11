// Gosint, Copyright (c) 2026 YouSysAdmin

package mailparse

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// message assembles wire-format mail.
//
// Fixtures here were chains of quoted strings joined with +, where the
// CRLFs are load-bearing and invisible: one missing pair and the headers
// run into the body, producing a test that passes for the wrong reason.
// Written this way a fixture reads as a message.
func message(headers []string, body string) []byte {
	var b strings.Builder
	for _, h := range headers {
		b.WriteString(h)
		b.WriteString("\r\n")
	}

	b.WriteString("\r\n")
	b.WriteString(body)

	return []byte(b.String())
}

// What a message says, as far as any one case cares. A zero field is not
// checked, so a case states only what it is about.
type expect struct {
	subject   string
	from      string
	to        []string
	cc        []string
	messageID string
	text      string
	html      string
	files     []Attachment
}

func (w expect) check(t *testing.T, got *Email) {
	t.Helper()

	if w.subject != "" && got.Subject != w.subject {
		t.Errorf("Subject = %q, want %q", got.Subject, w.subject)
	}

	if w.from != "" && got.From != w.from {
		t.Errorf("From = %q, want %q", got.From, w.from)
	}

	if w.messageID != "" && got.MessageID != w.messageID {
		t.Errorf("MessageID = %q, want %q", got.MessageID, w.messageID)
	}

	if w.text != "" && got.TextBody != w.text {
		t.Errorf("TextBody = %q, want %q", got.TextBody, w.text)
	}

	if w.html != "" && got.HTMLBody != w.html {
		t.Errorf("HTMLBody = %q, want %q", got.HTMLBody, w.html)
	}

	sameAddresses(t, "To", got.To, w.to)
	sameAddresses(t, "Cc", got.Cc, w.cc)

	if w.files == nil {
		return
	}

	if len(got.Attachments) != len(w.files) {
		t.Fatalf("attachments = %d, want %d: %+v", len(got.Attachments), len(w.files), got.Attachments)
	}

	for i, want := range w.files {
		has := got.Attachments[i]
		if has.Filename != want.Filename {
			t.Errorf("attachment %d filename = %q, want %q", i, has.Filename, want.Filename)
		}

		if want.ContentType != "" && has.ContentType != want.ContentType {
			t.Errorf("attachment %d type = %q, want %q", i, has.ContentType, want.ContentType)
		}

		if has.ContentID != want.ContentID {
			t.Errorf("attachment %d content id = %q, want %q", i, has.ContentID, want.ContentID)
		}

		if string(has.Content) != string(want.Content) {
			t.Errorf("attachment %d content = %q, want %q", i, has.Content, want.Content)
		}

		if has.Size != int64(len(want.Content)) {
			t.Errorf("attachment %d size = %d, want %d", i, has.Size, len(want.Content))
		}
	}
}

func sameAddresses(t *testing.T, field string, got, want []string) {
	t.Helper()

	if want == nil {
		return
	}

	if len(got) != len(want) {
		t.Errorf("%s = %v, want %v", field, got, want)

		return
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s = %v, want %v", field, got, want)

			return
		}
	}
}

// The shapes real mail arrives in.
func TestParseReadsAMessage(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  []byte
		want expect
	}{
		{
			name: "a plain note",
			raw: message([]string{
				"From: Alice <alice@example.com>",
				"To: bob@example.com, carol@example.com",
				"Cc: dave@example.com",
				"Subject: Hello",
				"Message-Id: <abc@example.com>",
				"Content-Type: text/plain; charset=UTF-8",
			}, "Hi Bob.\r\n"),
			want: expect{
				// The display name is dropped and the brackets with it:
				// downstream compares bare addresses and bare ids.
				from:      "alice@example.com",
				to:        []string{"bob@example.com", "carol@example.com"},
				cc:        []string{"dave@example.com"},
				subject:   "Hello",
				messageID: "abc@example.com",
				text:      "Hi Bob.\r\n",
			},
		},
		{
			name: "text and html alternatives",
			raw: message([]string{
				"From: a@x.com",
				"Content-Type: multipart/alternative; boundary=\"xyz\"",
			}, "--xyz\r\n"+
				"Content-Type: text/plain\r\n\r\nplain body\r\n"+
				"--xyz\r\n"+
				"Content-Type: text/html\r\n\r\n<p>html body</p>\r\n"+
				"--xyz--\r\n"),
			want: expect{text: "plain body", html: "<p>html body</p>"},
		},
		{
			name: "a container inside a container",
			raw: message([]string{
				"From: a@x.com",
				"Content-Type: multipart/mixed; boundary=\"outer\"",
			}, "--outer\r\n"+
				"Content-Type: multipart/alternative; boundary=\"inner\"\r\n\r\n"+
				"--inner\r\nContent-Type: text/plain\r\n\r\nnested plain\r\n"+
				"--inner\r\nContent-Type: text/html\r\n\r\n<b>nested html</b>\r\n"+
				"--inner--\r\n"+
				"--outer--\r\n"),
			want: expect{text: "nested plain", html: "<b>nested html</b>"},
		},
		{
			name: "a base64 attachment",
			raw: message([]string{
				"From: a@x.com",
				"Content-Type: multipart/mixed; boundary=\"B\"",
			}, "--B\r\nContent-Type: text/plain\r\n\r\nhi\r\n"+
				"--B\r\n"+
				"Content-Type: application/octet-stream; name=\"x.bin\"\r\n"+
				"Content-Disposition: attachment; filename=\"x.bin\"\r\n"+
				"Content-Transfer-Encoding: base64\r\n\r\n"+
				"aGVsbG8=\r\n"+
				"--B--\r\n"),
			want: expect{
				text: "hi",
				files: []Attachment{{
					Filename: "x.bin",
					// The bare type: the name parameter belongs to the
					// part, not to the file.
					ContentType: "application/octet-stream",
					Content:     []byte("hello"),
				}},
			},
		},
		{
			// An embedded image arrives exactly like this, and treating
			// it as a body would put base64 in somebody's inbox.
			name: "an inline part with a filename is still a file",
			raw: message([]string{
				"From: a@x.com",
				"Content-Type: multipart/related; boundary=\"R\"",
			}, "--R\r\nContent-Type: text/html\r\n\r\n<img src=cid:logo>\r\n"+
				"--R\r\n"+
				"Content-Type: image/png\r\n"+
				"Content-Disposition: inline; filename=\"logo.png\"\r\n"+
				"Content-ID: <logo>\r\n\r\n"+
				"PNGBYTES\r\n"+
				"--R--\r\n"),
			want: expect{
				html: "<img src=cid:logo>",
				files: []Attachment{{
					Filename: "logo.png", ContentType: "image/png", ContentID: "logo", Content: []byte("PNGBYTES"),
				}},
			},
		},
		{
			// The Content-ID alone is what makes it an embedded image.
			name: "a non-text part with a content id and no filename is a file",
			raw: message([]string{
				"From: a@x.com",
				"Content-Type: multipart/related; boundary=\"R\"",
			}, "--R\r\nContent-Type: text/html\r\n\r\n<img src=cid:ii_1>\r\n"+
				"--R\r\n"+
				"Content-Type: image/png\r\n"+
				"Content-ID: <ii_1>\r\n\r\n"+
				"PNGBYTES\r\n"+
				"--R--\r\n"),
			want: expect{
				html:  "<img src=cid:ii_1>",
				files: []Attachment{{ContentType: "image/png", ContentID: "ii_1", Content: []byte("PNGBYTES")}},
			},
		},
		{
			// Older senders skip the disposition entirely.
			name: "a name on the content type is enough",
			raw: message([]string{
				"From: a@x.com",
				"Content-Type: multipart/mixed; boundary=\"N\"",
			}, "--N\r\nContent-Type: text/csv; name=\"rows.csv\"\r\n\r\nx,y\r\n--N--\r\n"),
			want: expect{files: []Attachment{{Filename: "rows.csv", ContentType: "text/csv", Content: []byte("x,y")}}},
		},
		{
			// Legal, and it happens. Neither may be dropped and neither
			// may overwrite the other.
			name: "two parts of the same type are joined",
			raw: message([]string{
				"From: a@x.com",
				"Content-Type: multipart/mixed; boundary=\"J\"",
			}, "--J\r\nContent-Type: text/plain\r\n\r\nfirst\r\n"+
				"--J\r\nContent-Type: text/plain\r\n\r\nsecond\r\n"+
				"--J--\r\n"),
			want: expect{text: "first\nsecond"},
		},
		{
			name: "quoted-printable, in the header and the body",
			raw: message([]string{
				"From: a@x.com",
				"Subject: =?UTF-8?Q?Caf=C3=A9?=",
				"Content-Type: text/plain; charset=UTF-8",
				"Content-Transfer-Encoding: quoted-printable",
			}, "Caf=C3=A9\r\n"),
			want: expect{subject: "Café", text: "Café\r\n"},
		},
		{
			name: "an encoded word in a legacy charset",
			raw: message([]string{
				"From: a@x.com",
				"Subject: =?iso-8859-1?Q?Gr=FC=DFe?=",
				"Content-Type: text/plain; charset=UTF-8",
			}, "hi\r\n"),
			want: expect{subject: "Grüße"},
		},
		{
			// An unparseable From is still evidence of who sent this.
			name: "an address that will not parse is kept as sent",
			raw: message([]string{
				"From: not an address",
				"To: also not an address",
				"Content-Type: text/plain",
			}, "hi\r\n"),
			want: expect{from: "not an address"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.raw)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}

			tc.want.check(t, got)
		})
	}
}

// A To that will not parse answers nil rather than the raw header. The
// field is a LIST, and one string containing commas would be read
// downstream as a single recipient.
func TestParseDropsAnUnparseableRecipientList(t *testing.T) {
	got, err := Parse(message([]string{
		"From: a@x.com",
		"To: also not an address",
		"Content-Type: text/plain",
	}, "hi\r\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if got.To != nil {
		t.Errorf("To = %v, want nil", got.To)
	}
}

// Anything that reaches a JSON response has to be valid UTF-8, whatever
// the sender claimed. Both paths matter: a charset that is declared and
// correct, and no charset at all.
func TestParseAlwaysProducesValidUTF8(t *testing.T) {
	for _, tc := range []struct {
		name    string
		charset string
		body    []byte
		want    string
	}{
		{
			name:    "a declared legacy charset is converted",
			charset: "; charset=iso-8859-1",
			body:    []byte{'G', 'r', 0xfc, 0xdf, 'e'},
			want:    "Grüße",
		},
		{
			// Nothing says how to read these bytes, so they are read as
			// ISO-8859-1 - the one assumption that cannot fail.
			name: "undeclared bytes are repaired rather than refused",
			body: []byte{0xdf, 0x65},
			want: "ße",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := append(message([]string{
				"From: a@x.com",
				"Content-Type: text/plain" + tc.charset,
			}, ""), tc.body...)

			got, err := Parse(raw)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}

			if !utf8.ValidString(got.TextBody) {
				t.Fatalf("TextBody is not valid UTF-8: %q", got.TextBody)
			}

			if got.TextBody != tc.want {
				t.Errorf("TextBody = %q, want %q", got.TextBody, tc.want)
			}
		})
	}
}

// A missing Date stays zero: a viewer of evidence invents no date.
func TestParseLeavesAMissingDateZero(t *testing.T) {
	got, err := Parse(message([]string{"From: a@x.com", "Content-Type: text/plain"}, "hi\r\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if !got.Date.IsZero() {
		t.Errorf("Date = %v, want zero", got.Date)
	}
}

// Headers come back in message order, repeats kept, folded lines
// unfolded, and the Received chain reads top down.
func TestParseKeepsHeaderOrderAndReceivedChain(t *testing.T) {
	got, err := Parse(message([]string{
		"Received: from relay.example (relay.example [192.0.2.2])\r\n\tby mx.example; Mon, 1 Jan 2024 10:00:00 +0000",
		"Received: from sender.example by relay.example",
		"Subject: =?UTF-8?Q?Caf=C3=A9?=",
		"X-Zebra: 1",
		"X-Apple: 2",
		"Content-Type: text/plain",
	}, "hi\r\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var names []string
	for _, h := range got.HeaderList {
		names = append(names, h.Name)
	}

	want := "Received Received Subject X-Zebra X-Apple Content-Type"
	if strings.Join(names, " ") != want {
		t.Errorf("header order = %q, want %q", strings.Join(names, " "), want)
	}

	if got.HeaderList[2].Value != "Café" {
		t.Errorf("subject in the list = %q, want decoded", got.HeaderList[2].Value)
	}

	if !strings.Contains(got.HeaderList[0].Value, "by mx.example") || strings.Contains(got.HeaderList[0].Value, "\n") {
		t.Errorf("folded header = %q, want one unfolded line", got.HeaderList[0].Value)
	}

	if len(got.Received) != 2 || !strings.HasPrefix(got.Received[0], "from relay.example") ||
		!strings.HasPrefix(got.Received[1], "from sender.example") {
		t.Errorf("Received = %q, want the chain top down", got.Received)
	}
}

// Almost nothing is refused - see the package comment. These two are.
func TestParseRefusesWhatItCannotRead(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  []byte
	}{
		{name: "not a message at all", raw: []byte("not an email at all")},
		{
			// No fallback exists: nothing says where the parts begin.
			name: "a multipart container with no boundary",
			raw:  message([]string{"From: a@x.com", "Content-Type: multipart/mixed"}, "body\r\n"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(tc.raw); err == nil {
				t.Error("want an error")
			}
		})
	}
}

// nestedMultipart builds a message nesting multipart/mixed `depth`
// levels deep - the cheapest way to buy a lot of parser work with very
// few bytes.
func nestedMultipart(depth int) []byte {
	var b strings.Builder
	b.WriteString("From: a@x.com\r\nTo: b@x.com\r\nSubject: nest\r\n")
	b.WriteString("Content-Type: multipart/mixed; boundary=\"b0\"\r\n\r\n")
	for i := range depth {
		fmt.Fprintf(&b, "--b%d\r\nContent-Type: multipart/mixed; boundary=\"b%d\"\r\n\r\n", i, i+1)
	}

	fmt.Fprintf(&b, "--b%d\r\nContent-Type: text/plain\r\n\r\nhi\r\n--b%d--\r\n", depth, depth)
	for i := depth - 1; i >= 0; i-- {
		fmt.Fprintf(&b, "--b%d--\r\n", i)
	}

	return []byte(b.String())
}

// The MX listener hands this parser mail from anyone on the internet,
// and Parse takes no context - once it starts, nothing can cancel it.
// The upstream size cap bounds the bytes of a message but not its SHAPE,
// and the shape is what costs: before the depth limit, a 3.6 MB message
// nesting 50000 levels held a goroutine for over two minutes, so a
// handful of them took the listener down. This pins that a pathological
// tree is refused promptly.
func TestParseRefusesPathologicalNesting(t *testing.T) {
	raw := nestedMultipart(50000)

	done := make(chan error, 1)
	go func() {
		_, err := Parse(raw)
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, ErrTooComplex) {
			t.Fatalf("parse err = %v, want ErrTooComplex", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Parse did not return within 10s on a 3.6 MB message - the depth limit is not holding")
	}
}

// Wide is as good an attack as deep: a flat tree with an enormous number
// of tiny parts costs a struct and an allocation each, and with no blob
// store every attachment is base64-inlined into the row.
func TestParseRefusesTooManyParts(t *testing.T) {
	var b strings.Builder
	b.WriteString("From: a@x.com\r\nContent-Type: multipart/mixed; boundary=\"z\"\r\n\r\n")
	for range maxMIMEParts + 10 {
		b.WriteString("--z\r\nContent-Type: text/plain\r\n\r\nx\r\n")
	}

	b.WriteString("--z--\r\n")

	if _, err := Parse([]byte(b.String())); !errors.Is(err, ErrTooComplex) {
		t.Fatalf("parse err = %v, want ErrTooComplex", err)
	}
}

// The limits must sit far above real mail. A normal newsletter is
// mixed > alternative > (text, html) plus a couple of attachments -
// nowhere near either ceiling - and it has to keep parsing cleanly.
func TestParseAcceptsOrdinaryNesting(t *testing.T) {
	raw := message([]string{
		"From: a@x.com",
		"To: b@x.com",
		"Subject: real",
		"Content-Type: multipart/mixed; boundary=\"outer\"",
	}, "--outer\r\nContent-Type: multipart/alternative; boundary=\"inner\"\r\n\r\n"+
		"--inner\r\nContent-Type: text/plain\r\n\r\nplain text\r\n"+
		"--inner\r\nContent-Type: text/html\r\n\r\n<p>html</p>\r\n"+
		"--inner--\r\n"+
		"--outer\r\nContent-Type: text/csv\r\nContent-Disposition: attachment; filename=\"a.csv\"\r\n\r\nx,y\r\n"+
		"--outer--\r\n")

	got, err := Parse(raw)
	if err != nil {
		t.Fatalf("ordinary mail must parse, got %v", err)
	}

	expect{
		text:  "plain text",
		html:  "<p>html</p>",
		files: []Attachment{{Filename: "a.csv", ContentType: "text/csv", Content: []byte("x,y")}},
	}.check(t, got)
}
