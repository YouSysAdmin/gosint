// Gosint, Copyright (c) 2026 YouSysAdmin

// Package server builds the HTTP application: middleware, the API
// routes and the embedded console.
package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	slogfiber "github.com/samber/slog-fiber"
	"github.com/valyala/fasthttp"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/domain/auth"
	"github.com/yousysadmin/gosint/internal/domain/eventlog"
	"github.com/yousysadmin/gosint/internal/domain/graph"
)

// Options are what New needs.
type Options struct {
	Runtime *env.Runtime

	// Console is the built SPA, nil when none was embedded.
	Console fs.FS

	// TLS is the resolved server certificate, nil for plain HTTP.
	TLS *tls.Config
}

// Server is the HTTP application plus what it was built from.
type Server struct {
	app    *fiber.App
	rt     *env.Runtime
	tlsCfg *tls.Config
}

// New builds the application and registers every route.
func New(opts Options) (*Server, error) {
	cfg := fiber.Config{
		BodyLimit:    anonymousBodyLimit,
		ReadTimeout:  2 * time.Minute,
		WriteTimeout: 2 * time.Minute,
		IdleTimeout:  75 * time.Second,

		// Every error answered in the JSON envelope, never as text/plain.
		ErrorHandler: errorHandler,
		JSONEncoder:  func(v any) ([]byte, error) { return response.Marshal(v) },
		JSONDecoder:  func(data []byte, v any) error { return response.Unmarshal(data, v) },
	}

	// Forwarding headers are read only from listed peers. Without the
	// header name fiber trusts the peer for the scheme alone and c.IP()
	// stays the proxy's, which would give the whole install one
	// rate-limit bucket. The chain is walked from the right past the
	// listed proxies: a proxy appends to what the client sent, so the
	// raw header would let a client pick its own rate-limit bucket.
	if proxies := opts.Runtime.Config.Server.TrustedProxies; len(proxies) > 0 {
		cfg.TrustProxy = true
		cfg.TrustProxyConfig = fiber.TrustProxyConfig{Proxies: proxies}
		cfg.ProxyHeader = fiber.HeaderXForwardedFor
		cfg.EnableIPValidation = true
	}

	app := fiber.New(cfg)
	app.Server().HeaderReceived = perRequestLimits(opts.Runtime)
	app.Use(safeRecover)
	app.Use(securityHeaders(opts.Runtime.Config))
	app.Use(accessLog(slogfiber.New(opts.Runtime.Log)))

	registerRoutes(app, opts.Runtime, opts.Console)

	return &Server{app: app, rt: opts.Runtime, tlsCfg: opts.TLS}, nil
}

// App exposes the fiber application, for tests.
func (s *Server) App() *fiber.App { return s.app }

// Start listens on server.addr until Shutdown.
func (s *Server) Start() error {
	addr := s.rt.Config.Server.Addr

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	return s.Serve(ln)
}

// Serve answers on ln until Shutdown, wrapped in TLS when configured.
func (s *Server) Serve(ln net.Listener) error {
	if s.tlsCfg != nil {
		ln = tls.NewListener(ln, s.tlsCfg)
	}

	slog.Info("server start", "addr", ln.Addr().String(), "tls", s.tlsCfg != nil)

	return s.app.Listener(ln, fiber.ListenConfig{DisableStartupMessage: true})
}

// Shutdown stops accepting connections and waits for in-flight
// requests up to timeout.
func (s *Server) Shutdown(timeout time.Duration) error {
	err := s.app.ShutdownWithTimeout(timeout)
	if errors.Is(err, context.DeadlineExceeded) {
		slog.Warn("server: shutdown timed out with requests still in flight", "timeout", timeout)

		return nil
	}

	return err
}

// streamWriteTimeout is the write deadline of the streaming routes.
// fasthttp sets one deadline for a whole streamed body, so the app-wide
// WriteTimeout would cut an event stream after two minutes. It only has
// to sit above the stream's own recycle time.
const streamWriteTimeout = eventlog.MaxStreamLife + 5*time.Minute

// bodyLimit is the largest request body: 16 MB, or an upload of the
// file store's largest file with room for the multipart framing.
func bodyLimit(maxFile int64) int {
	return max(16<<20, int(maxFile)+(1<<20))
}

// anonymousBodyLimit is the largest body a request without a live
// session may send: a sign-in, a registration, an unseal.
const anonymousBodyLimit = 1 << 20

// sessionCheckTimeout bounds the session lookup taken before a body is
// read.
const sessionCheckTimeout = 2 * time.Second

// perRequestLimits lifts the write deadline for event streams and the
// body limit for a caller holding a live session: an upload, and the
// imports of an export, which carry its files. It runs once the headers
// are in and before the body is read, so without a session nobody gets
// the server to hold more than anonymousBodyLimit.
func perRequestLimits(rt *env.Runtime) func(h *fasthttp.RequestHeader) fasthttp.RequestConfig {
	return func(h *fasthttp.RequestHeader) fasthttp.RequestConfig {
		path, _, _ := strings.Cut(string(h.RequestURI()), "?")
		if isStreamingPath(path) {
			return fasthttp.RequestConfig{WriteTimeout: streamWriteTimeout}
		}

		method := string(h.Method())
		if method == fiber.MethodGet || method == fiber.MethodHead || method == fiber.MethodOptions {
			return fasthttp.RequestConfig{}
		}

		ctx, cancel := context.WithTimeout(context.Background(), sessionCheckTimeout)
		defer cancel()

		if !auth.HasLiveSession(ctx, rt, string(h.Cookie(auth.CookieName))) {
			return fasthttp.RequestConfig{}
		}

		if method == fiber.MethodPost && isExportImportPath(path) {
			return fasthttp.RequestConfig{MaxRequestBodySize: graph.ImportBodyLimit}
		}

		return fasthttp.RequestConfig{MaxRequestBodySize: bodyLimit(rt.FileLimits.Get().MaxSize)}
	}
}

// isExportImportPath is one of the three routes taking an export file:
// a new investigation, a new sketch in one, a merge into a sketch.
func isExportImportPath(path string) bool {
	if path == "/api/investigations/import" {
		return true
	}

	if rest, ok := strings.CutPrefix(path, "/api/investigations/"); ok {
		id, tail, _ := strings.Cut(rest, "/")

		return id != "" && tail == "sketches/import"
	}

	if rest, ok := strings.CutPrefix(path, "/api/sketches/"); ok {
		id, tail, _ := strings.Cut(rest, "/")

		return id != "" && tail == "import/file"
	}

	return false
}

// accessLog skips the event streams. slog-fiber reads the response body
// after the handler to log its length, and a stream's body never ends,
// so the request would hang before its first byte left.
func accessLog(log fiber.Handler) fiber.Handler {
	return func(c fiber.Ctx) error {
		if isStreamingPath(c.Path()) {
			return c.Next()
		}

		return log(c)
	}
}

// isStreamingPath matches /api/sketches/:id/events and the chat
// answers, /api/sketches/:id/chats/:chat_id/messages.
func isStreamingPath(path string) bool {
	rest, ok := strings.CutPrefix(path, "/api/sketches/")
	if !ok {
		return false
	}

	id, tail, _ := strings.Cut(rest, "/")
	if id == "" {
		return false
	}

	return tail == "events" || (strings.HasPrefix(tail, "chats/") && strings.HasSuffix(tail, "/messages"))
}

func errorHandler(c fiber.Ctx, err error) error {
	if fe, ok := errors.AsType[*fiber.Error](err); ok {
		return response.Coded(c, fe.Code, strings.ToLower(fe.Message))
	}

	return response.Internal(c, err)
}

// safeRecover turns a handler panic into a logged 500.
func safeRecover(c fiber.Ctx) (err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic recovered",
				"reason", fmt.Sprintf("%v", r),
				"path", c.Path(),
				"method", c.Method(),
				"stack", string(debug.Stack()),
			)
			err = response.Internal(c, nil)
		}
	}()

	return c.Next()
}

// securityHeaders sets the headers every response carries: HSTS once
// the install is public over https, and no-store on the API, whose
// answers hold one-time secrets (a TOTP seed, recovery codes, a reset
// link). A handler that may be cached sets its own Cache-Control.
// permissionsPolicy turns off the browser features the console never
// uses, so nothing injected into it can reach them.
const permissionsPolicy = "camera=(), microphone=(), geolocation=(), payment=(), usb=(), serial=(), bluetooth=(), " +
	"display-capture=(), interest-cohort=()"

func securityHeaders(cfg *env.Config) fiber.Handler {
	https := cfg != nil && strings.HasPrefix(strings.ToLower(cfg.Server.PublicURL), "https://")

	return func(c fiber.Ctx) error {
		c.Set(fiber.HeaderXContentTypeOptions, "nosniff")
		c.Set(fiber.HeaderXFrameOptions, "DENY")
		c.Set(fiber.HeaderReferrerPolicy, "same-origin")
		c.Set("Permissions-Policy", permissionsPolicy)
		c.Set("Cross-Origin-Opener-Policy", "same-origin")
		if https {
			c.Set(fiber.HeaderStrictTransportSecurity, "max-age=31536000")
		}

		// No other origin embeds or reads an answer, a file included.
		if strings.HasPrefix(c.Path(), "/api/") {
			c.Set(fiber.HeaderCacheControl, "no-store")
			c.Set("Cross-Origin-Resource-Policy", "same-origin")
		}

		return c.Next()
	}
}
