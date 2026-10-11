// Gosint, Copyright (c) 2026 YouSysAdmin

package mailer

import (
	"fmt"
	"html"
	"strings"
)

// The server's own messages are plain string builders: they must work
// on an install with nothing configured but the relay, and a reset
// happens before the person can sign in to fix anything.

// Invitation renders the investigation invitation message.
func Invitation(investigation, inviterEmail, acceptURL string, expiresHours int) (subject, htmlBody, textBody string) {
	subject = fmt.Sprintf("You have been invited to the %s investigation", investigation)

	intro := fmt.Sprintf("%s invited you to join the %s investigation on Gosint.",
		fallback(inviterEmail, "A member"), investigation)
	expiry := fmt.Sprintf("This invitation expires in %d hours.", expiresHours)

	htmlBody = layout("Invitation", intro, "Accept the invitation", acceptURL, expiry)
	textBody = plain(intro, acceptURL, expiry)

	return subject, htmlBody, textBody
}

// PasswordReset renders the password reset message.
func PasswordReset(resetURL string, expiresMinutes int) (subject, htmlBody, textBody string) {
	subject = "Reset your Gosint password"

	intro := "Somebody asked to reset the password for this address."
	expiry := fmt.Sprintf("The link expires in %d minutes and can be used once. "+
		"If you did not ask for this, ignore this message, your password has not changed.", expiresMinutes)

	htmlBody = layout("Password reset", intro, "Choose a new password", resetURL, expiry)
	textBody = plain(intro, resetURL, expiry)

	return subject, htmlBody, textBody
}

// layout wraps one call to action in a minimal HTML document. Inline
// styles only, no external assets, it has to render in clients that
// block everything.
func layout(heading, intro, buttonLabel, buttonURL, footer string) string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html><head><meta charset="utf-8">`)
	b.WriteString(`<meta name="viewport" content="width=device-width,initial-scale=1"></head>`)
	b.WriteString(`<body style="margin:0;padding:24px;background:#f5f6f8;`)
	b.WriteString(`font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;color:#1f2329">`)
	b.WriteString(`<div style="max-width:520px;margin:0 auto;background:#ffffff;border-radius:8px;padding:32px">`)
	b.WriteString(`<h1 style="margin:0 0 16px;font-size:20px;font-weight:600">`)
	b.WriteString(html.EscapeString(heading))
	b.WriteString(`</h1><p style="margin:0 0 24px;font-size:15px;line-height:1.5">`)
	b.WriteString(html.EscapeString(intro))
	b.WriteString(`</p><p style="margin:0 0 24px"><a href="`)
	b.WriteString(html.EscapeString(buttonURL))
	b.WriteString(`" style="display:inline-block;padding:11px 20px;background:#2563eb;color:#ffffff;`)
	b.WriteString(`text-decoration:none;border-radius:6px;font-size:15px;font-weight:500">`)
	b.WriteString(html.EscapeString(buttonLabel))
	b.WriteString(`</a></p><p style="margin:0 0 8px;font-size:13px;color:#5c6470">Or paste this link into your browser:</p>`)
	b.WriteString(`<p style="margin:0 0 24px;font-size:13px;word-break:break-all"><a href="`)
	b.WriteString(html.EscapeString(buttonURL))
	b.WriteString(`" style="color:#2563eb">`)
	b.WriteString(html.EscapeString(buttonURL))
	b.WriteString(`</a></p><p style="margin:0;font-size:13px;color:#5c6470;line-height:1.5">`)
	b.WriteString(html.EscapeString(footer))
	b.WriteString(`</p></div></body></html>`)

	return b.String()
}

// plain renders the text alternative.
func plain(intro, url, footer string) string {
	return intro + "\n\n" + url + "\n\n" + footer + "\n"
}

func fallback(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}

	return s
}

// Test renders the message an administrator sends to check the relay.
func Test(consoleURL string) (subject, htmlBody, textBody string) {
	subject = "Gosint mail test"

	intro := "The relay works: this message was sent from the mail settings of Gosint."
	footer := "Nothing else to do, password reset and invitation links will reach people now."

	htmlBody = layout("Mail test", intro, "Open Gosint", consoleURL, footer)
	textBody = plain(intro, consoleURL, footer)

	return subject, htmlBody, textBody
}
