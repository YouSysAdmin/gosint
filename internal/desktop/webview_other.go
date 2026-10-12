// Gosint, Copyright (c) 2026 YouSysAdmin

//go:build !windows

package desktop

// requireWebView has nothing to check where the web view is part of the
// system (WebKit on macOS, a package dependency on Linux).
func requireWebView() error { return nil }
