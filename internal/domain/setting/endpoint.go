// Gosint, Copyright (c) 2026 YouSysAdmin

package setting

import (
	"context"
	"net/mail"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/mailer"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/validation"
)

// Handler serves the settings to the admin panel.
type Handler struct {
	Runtime *env.Runtime
}

// relay is what can take a new mail relay while running, the server's
// mailer.Switch. A test's fake cannot.
type relay interface {
	Set(cfg mailer.Config)
}

// readMail is the stored relay, the default when none was saved.
func readMail(ctx context.Context, rt *env.Runtime) (mailer.Config, error) {
	cfg := mailer.DefaultConfig()
	_, err := rt.Store.Setting.Get(ctx, Mail, &cfg)

	return cfg, err
}

// Load hands the stored settings to the running server: at boot, at
// the unseal and by the refresh job, so a change on one node reaches
// the others.
func Load(ctx context.Context, rt *env.Runtime) error {
	cfg, err := readMail(ctx, rt)
	if err != nil {
		return err
	}

	if r, ok := rt.Mail.(relay); ok {
		r.Set(cfg)
	}

	tuning, err := readWorker(ctx, rt)
	if err != nil {
		return err
	}

	applyWorker(rt, tuning)

	tools, err := readTools(ctx, rt)
	if err != nil {
		return err
	}

	rt.ToolSettings.Set(tools)

	files, err := readFiles(ctx, rt)
	if err != nil {
		return err
	}

	return applyFiles(ctx, rt, files)
}

func mailView(cfg mailer.Config) mailResponse {
	return mailResponse{Mail: MailView{Host: cfg.Host, Port: cfg.Port, Username: cfg.Username,
		HasPassword: cfg.Password != "", From: cfg.From, StartTLS: cfg.StartTLS, Enabled: cfg.Host != ""}}
}

// GetMail answers the relay, the password as has_password.
func (h Handler) GetMail(c fiber.Ctx) error {
	cfg, err := readMail(c, h.Runtime)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, mailView(cfg))
}

// PutMail saves the relay and puts it to use on this node at once. An
// empty host turns mail off.
func (h Handler) PutMail(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[mailInput](c)
	if !ok {
		return resp
	}

	cfg, err := readMail(c, h.Runtime)
	if err != nil {
		return response.Internal(c, err)
	}

	cfg.Host, cfg.Port, cfg.Username = in.Host, in.Port, in.Username
	cfg.From, cfg.StartTLS = in.From, in.StartTLS
	if in.Password != nil {
		cfg.Password = *in.Password
	}

	if cfg.Username == "" {
		cfg.Password = ""
	}

	if cfg.Host != "" {
		if cfg.Port < 1 || cfg.Port > 65535 {
			return refuse(c, "port", "range", "the port must be between 1 and 65535")
		}

		if _, err := mail.ParseAddress(cfg.From); err != nil {
			return refuse(c, "from", "email", "the sender must be an email address, a display name allowed")
		}

		if cfg.Username != "" && cfg.Password == "" {
			return refuse(c, "password", "required", "a username needs its password")
		}
	}

	if err := h.Runtime.Store.Setting.Put(c, Mail, cfg, access.User(c).ID); err != nil {
		return response.Internal(c, err)
	}

	if r, ok := h.Runtime.Mail.(relay); ok {
		r.Set(cfg)
	}

	return response.Success(c, mailView(cfg))
}

// TestMail sends a message to the administrator through the saved
// relay and says what the relay answered.
func (h Handler) TestMail(c fiber.Ctx) error {
	cfg, err := readMail(c, h.Runtime)
	if err != nil {
		return response.Internal(c, err)
	}

	if cfg.Host == "" {
		return response.BadRequest(c, "save a relay first")
	}

	subject, htmlBody, textBody := mailer.Test(strings.TrimRight(h.Runtime.Config.Server.PublicURL, "/") + "/app/")
	if err := mailer.New(cfg, h.Runtime.Log).Send(c, []string{access.User(c).Email}, subject, htmlBody, textBody); err != nil {
		return response.BadRequest(c, err.Error())
	}

	return response.Success(c, testResponse{SentTo: access.User(c).Email})
}

func refuse(c fiber.Ctx, field, rule, msg string) error {
	return response.BadRequestFields(c, msg, []validation.FieldError{{Field: field, Rule: rule, Message: msg}})
}
