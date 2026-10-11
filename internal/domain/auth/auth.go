// Gosint, Copyright (c) 2026 YouSysAdmin

// Package auth signs people in and out: registration, password login,
// the session cookie and the middleware that resolves it.
//
// A session is an opaque random token in an HttpOnly cookie. Only its
// sha256 is stored, so a leaked database holds no usable cookie, and
// signing out or expiry is a row going away. No signing secret is
// needed, which keeps a local install free of configuration.
package auth
