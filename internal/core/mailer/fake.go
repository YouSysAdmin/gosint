// Gosint, Copyright (c) 2026 YouSysAdmin

package mailer

import (
	"context"
	"sync"
)

// Sent is one message a Fake was asked to send.
type Sent struct {
	To      []string
	Subject string
	HTML    string
	Text    string
}

// Fake is the Sender of a test: enabled, keeps what it was given.
type Fake struct {
	mu   sync.Mutex
	sent []Sent
}

// Enabled implements Sender.
func (*Fake) Enabled() bool { return true }

// Send implements Sender.
func (f *Fake) Send(_ context.Context, to []string, subject, html, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.sent = append(f.sent, Sent{To: append([]string(nil), to...), Subject: subject, HTML: html, Text: text})

	return nil
}

// Messages is everything sent so far.
func (f *Fake) Messages() []Sent {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]Sent(nil), f.sent...)
}
