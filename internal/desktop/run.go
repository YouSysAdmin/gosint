// Gosint, Copyright (c) 2026 YouSysAdmin

//go:build cgo

package desktop

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"

	webview "github.com/webview/webview_go"

	"github.com/yousysadmin/gosint/internal/cli"
	"github.com/yousysadmin/gosint/internal/core/env"
)

// The window's first size, the user resizes from there.
const (
	windowWidth  = 1440
	windowHeight = 900
)

// startPage shows while the node migrates and opens.
const startPage = `<!doctype html><html><body style="font:14px system-ui,sans-serif;
display:flex;align-items:center;justify-content:center;height:100vh;margin:0;
color:#888;background:#111">Starting gosint</body></html>`

// initScript tells the console it runs inside the desktop window.
const initScript = `window.gosintDesktop = true`

// Run opens the desktop window over a node on the loopback and returns
// once the window is closed and the node has stopped. It must be called
// from main, the window lives on the main thread.
func Run(ctx context.Context) error {
	if err := requireWebView(); err != nil {
		return err
	}

	dir, err := DataDir()
	if err != nil {
		return fail(fmt.Errorf("data directory: %w", err))
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fail(fmt.Errorf("data directory: %w", err))
	}

	release, err := Lock(dir)
	if err != nil {
		return fail(err)
	}

	defer release()

	cfgPath, err := ConfigFile(dir)
	if err != nil {
		return fail(fmt.Errorf("config: %w", err))
	}

	ln, err := Listen(dir)
	if err != nil {
		return fail(err)
	}

	cfg, err := env.Load(cfgPath, Defaults(dir))
	if err != nil {
		_ = ln.Close()

		return fail(err)
	}

	// The window reaches the node on the loopback only, whatever the
	// settings say.
	origin := "http://" + ln.Addr().(*net.TCPAddr).String()
	cfg.Server.Addr = ln.Addr().String()
	cfg.Server.PublicURL = origin
	cfg.Server.TLS.Mode = env.TLSNone

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	w := webview.New(false)
	defer w.Destroy()

	w.SetTitle("gosint")
	w.SetSize(windowWidth, windowHeight, webview.HintNone)
	w.Init(initScript)
	prepareWindow(w.Window())

	if err := bind(w); err != nil {
		return fail(err)
	}

	w.SetHtml(startPage)

	done := make(chan error, 1)
	go func() {
		err := cli.RunNode(ctx, cfg, cli.NodeOptions{
			Listener: ln,
			Ready: func() {
				w.Dispatch(func() { w.Navigate(origin + env.ConsolePath + "/") })
			},
		})

		// A node that stopped on its own takes the window with it.
		if ctx.Err() == nil {
			w.Dispatch(w.Terminate)
		}

		done <- err
	}()

	w.Run()
	cancel()

	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		return fail(err)
	}

	return nil
}

// bind gives the console what a window cannot do the browser way.
func bind(w webview.WebView) error {
	if err := w.Bind("gosintOpenExternal", OpenExternal); err != nil {
		return err
	}

	return w.Bind("gosintSave", Save)
}
