// Gosint, Copyright (c) 2026 YouSysAdmin

// Package mailer sends the server's own messages, a password reset or
// an invitation link, through one SMTP relay an administrator sets in
// the console. Without a host it is disabled and every caller degrades,
// the link is handed to an administrator instead.
package mailer

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Sender sends one message. Enabled says whether anything is sent at
// all.
type Sender interface {
	Enabled() bool
	Send(ctx context.Context, to []string, subject, html, text string) error
}

// Config is the relay.
type Config struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`

	// From is the sender address, a display name allowed.
	From string `json:"from"`

	// StartTLS upgrades a plain connection, off means implicit TLS on
	// connect (port 465).
	StartTLS bool `json:"starttls"`
}

// DefaultConfig is the relay of an install that set none: disabled,
// with the usual submission port ready for the form.
func DefaultConfig() Config { return Config{Port: 587, StartTLS: true} }

// sendTimeout bounds one delivery, dial to quit.
const sendTimeout = 30 * time.Second

// SMTP is the Sender over a relay.
type SMTP struct {
	cfg Config
	log *slog.Logger
}

// Disabled is the Sender of an install with no relay.
type Disabled struct{}

// Enabled implements Sender.
func (Disabled) Enabled() bool { return false }

// Send implements Sender.
func (Disabled) Send(context.Context, []string, string, string, string) error {
	return errors.New("mail is not configured")
}

// New builds the Sender for cfg: a relay when a host is named, else
// Disabled.
func New(cfg Config, log *slog.Logger) Sender {
	if cfg.Host == "" {
		return Disabled{}
	}

	return &SMTP{cfg: cfg, log: log}
}

// Enabled implements Sender.
func (s *SMTP) Enabled() bool { return true }

// Switch is a Sender whose relay is replaced while the server runs,
// when an administrator saves it or another node did.
type Switch struct {
	log *slog.Logger
	cur atomic.Pointer[Sender]
}

// NewSwitch starts disabled.
func NewSwitch(log *slog.Logger) *Switch {
	s := &Switch{log: log}
	s.Set(Config{})

	return s
}

// Set replaces the relay.
func (s *Switch) Set(cfg Config) {
	next := New(cfg, s.log)
	s.cur.Store(&next)
}

// Enabled implements Sender.
func (s *Switch) Enabled() bool { return (*s.cur.Load()).Enabled() }

// Send implements Sender.
func (s *Switch) Send(ctx context.Context, to []string, subject, html, text string) error {
	return (*s.cur.Load()).Send(ctx, to, subject, html, text)
}

// Send delivers one message, text and html as alternatives.
func (s *SMTP) Send(ctx context.Context, to []string, subject, html, text string) error {
	from, err := mail.ParseAddress(s.cfg.From)
	if err != nil {
		return fmt.Errorf("mail: from address: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	var conn net.Conn
	d := net.Dialer{}
	if s.cfg.StartTLS {
		conn, err = d.DialContext(ctx, "tcp", addr)
	} else {
		conn, err = (&tls.Dialer{NetDialer: &d, Config: &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}}).DialContext(ctx, "tcp", addr)
	}

	if err != nil {
		return fmt.Errorf("mail: connect %s: %w", addr, err)
	}

	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)

	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		_ = conn.Close()

		return fmt.Errorf("mail: %w", err)
	}

	defer client.Close() //nolint:errcheck // Quit below is the real end

	if s.cfg.StartTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("mail: the relay offers no STARTTLS, turn STARTTLS off for implicit TLS")
		}

		if err := client.StartTLS(&tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("mail: starttls: %w", err)
		}
	}

	if s.cfg.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return fmt.Errorf("mail: auth: %w", err)
		}
	}

	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("mail: from: %w", err)
	}

	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("mail: to %s: %w", rcpt, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("mail: data: %w", err)
	}

	if _, err := io.WriteString(w, Message(from, to, subject, html, text)); err != nil {
		return fmt.Errorf("mail: write: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("mail: send: %w", err)
	}

	return client.Quit()
}

// SendAsync delivers in the background with its own timeout, logging a
// failure with the address masked. For a request that must not wait on
// the relay.
func SendAsync(s Sender, log *slog.Logger, to []string, subject, html, text string) {
	if !s.Enabled() {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
		defer cancel()

		if err := s.Send(ctx, to, subject, html, text); err != nil {
			masked := make([]string, 0, len(to))
			for _, a := range to {
				masked = append(masked, Mask(a))
			}

			log.Warn("mail: send failed", "to", masked, "subject", subject, "err", err)
		}
	}()
}

// Message builds the wire form: a multipart/alternative with the text
// first and the html second, both quoted-printable, headers encoded.
func Message(from *mail.Address, to []string, subject, html, text string) string {
	var b strings.Builder
	boundary := "gosint-" + hex.EncodeToString(randomBytes(12))
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <%s@%s>\r\n",
		from.String(), strings.Join(to, ", "), mime.QEncoding.Encode("utf-8", subject),
		time.Now().UTC().Format(time.RFC1123Z), hex.EncodeToString(randomBytes(16)), domainOf(from.Address))
	fmt.Fprintf(&b, "MIME-Version: 1.0\r\nAuto-Submitted: auto-generated\r\n"+
		"Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)
	for _, part := range []struct{ ct, body string }{{"text/plain", text}, {"text/html", html}} {
		if part.body == "" {
			continue
		}

		fmt.Fprintf(&b, "--%s\r\nContent-Type: %s; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n",
			boundary, part.ct)
		qp := quotedprintable.NewWriter(&b)
		_, _ = io.WriteString(qp, part.body)
		_ = qp.Close()
		b.WriteString("\r\n")
	}

	fmt.Fprintf(&b, "--%s--\r\n", boundary)

	return b.String()
}

// Mask hides most of an address for a log line: j***@example.com.
func Mask(addr string) string {
	local, domain, ok := strings.Cut(addr, "@")
	if !ok || local == "" {
		return "***"
	}

	return local[:1] + "***@" + domain
}

func domainOf(addr string) string {
	if _, domain, ok := strings.Cut(addr, "@"); ok {
		return domain
	}

	return "localhost"
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)

	return b
}
