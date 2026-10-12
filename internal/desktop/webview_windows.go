// Gosint, Copyright (c) 2026 YouSysAdmin

//go:build windows

package desktop

import (
	"errors"

	"github.com/ncruces/zenity"
	"golang.org/x/sys/windows/registry"
)

// webView2Download is where Microsoft hands out the runtime.
const webView2Download = "https://developer.microsoft.com/microsoft-edge/webview2/"

// webView2Client is the runtime's EdgeUpdate key, its pv value the
// installed version.
const webView2Client = `Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`

// errNoWebView says the window has nothing to draw with.
var errNoWebView = errors.New("the Microsoft Edge WebView2 Runtime is not installed")

// requireWebView checks for the WebView2 runtime the window is drawn
// by, where Microsoft says to look, and offers its download page when
// it is missing. Windows 10 and 11 normally ship it.
func requireWebView() error {
	places := []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\` + webView2Client},
		{registry.LOCAL_MACHINE, `SOFTWARE\` + webView2Client},
		{registry.CURRENT_USER, `SOFTWARE\` + webView2Client},
	}

	for _, p := range places {
		if installed(p.root, p.path) {
			return nil
		}
	}

	err := zenity.Question("gosint needs the Microsoft Edge WebView2 Runtime, which is not installed. Open its download page?",
		zenity.Title("gosint"), zenity.OKLabel("Open"), zenity.CancelLabel("Close"))
	if err == nil {
		_ = OpenExternal(webView2Download)
	}

	return errNoWebView
}

func installed(root registry.Key, path string) bool {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return false
	}

	defer k.Close() //nolint:errcheck // read only

	v, _, err := k.GetStringValue("pv")

	return err == nil && v != "" && v != "0.0.0.0"
}
