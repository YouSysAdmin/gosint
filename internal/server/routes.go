// Gosint, Copyright (c) 2026 YouSysAdmin

package server

import (
	"io/fs"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/gofiber/fiber/v3/middleware/static"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/domain/analysis"
	"github.com/yousysadmin/gosint/internal/domain/attachment"
	"github.com/yousysadmin/gosint/internal/domain/auth"
	"github.com/yousysadmin/gosint/internal/domain/chat"
	"github.com/yousysadmin/gosint/internal/domain/console"
	"github.com/yousysadmin/gosint/internal/domain/customtype"
	"github.com/yousysadmin/gosint/internal/domain/entitytype"
	"github.com/yousysadmin/gosint/internal/domain/eventlog"
	"github.com/yousysadmin/gosint/internal/domain/flow"
	"github.com/yousysadmin/gosint/internal/domain/graph"
	"github.com/yousysadmin/gosint/internal/domain/investigation"
	"github.com/yousysadmin/gosint/internal/domain/llmmodel"
	"github.com/yousysadmin/gosint/internal/domain/maintenance"
	"github.com/yousysadmin/gosint/internal/domain/oauthprovider"
	"github.com/yousysadmin/gosint/internal/domain/scan"
	"github.com/yousysadmin/gosint/internal/domain/schedule"
	"github.com/yousysadmin/gosint/internal/domain/setting"
	"github.com/yousysadmin/gosint/internal/domain/sketch"
	"github.com/yousysadmin/gosint/internal/domain/sys"
	"github.com/yousysadmin/gosint/internal/domain/transform"
	"github.com/yousysadmin/gosint/internal/domain/transformtemplate"
	"github.com/yousysadmin/gosint/internal/domain/user"
	"github.com/yousysadmin/gosint/internal/domain/vaultkey"
	invmodel "github.com/yousysadmin/gosint/internal/models/investigation"
)

// healthResponse is what /health answers.
type healthResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
}

func registerRoutes(app *fiber.App, rt *env.Runtime, console fs.FS) {
	app.Get("/health", func(c fiber.Ctx) error {
		if err := rt.DB.DB().PingContext(c); err != nil {
			rt.Log.Error("health: database ping failed", "err", err)

			return c.Status(fiber.StatusServiceUnavailable).
				JSON(healthResponse{Status: "unavailable", Database: string(rt.DB.Dialect())})
		}

		return response.Success(c, healthResponse{Status: "ok", Database: string(rt.DB.Dialect())})
	})

	api := app.Group("/api", refuseCrossSite(rt), requireUnsealed(rt))
	registerAPI(api, rt)

	// An unknown API path answers the JSON envelope. Registered after
	// every API route, since a Use matches everything below it.
	api.Use(func(c fiber.Ctx) error {
		return response.NotFound(c, "not found")
	})

	if console != nil {
		mountConsole(app, console)
		app.Get("/", func(c fiber.Ctx) error {
			return c.Redirect().Status(fiber.StatusFound).To(env.ConsolePath + "/")
		})
	}
}

// registerAPI wires every API route. Fiber runs a route's handlers in
// the order given, so the guards come first and the handler last.
func registerAPI(api fiber.Router, rt *env.Runtime) {
	authH := auth.Handler{Runtime: rt}
	userH := user.Handler{Runtime: rt}
	providerH := oauthprovider.Handler{Runtime: rt}
	invH := investigation.Handler{Runtime: rt}
	sketchH := sketch.Handler{Runtime: rt}
	graphH := graph.Handler{Runtime: rt}
	fileH := attachment.Handler{Runtime: rt}
	keyH := vaultkey.Handler{Runtime: rt}
	scanH := scan.Handler{Runtime: rt}
	logH := eventlog.Handler{Runtime: rt}
	typeH := entitytype.Handler{Runtime: rt}
	transformH := transform.Handler{Runtime: rt}
	templateH := transformtemplate.Handler{Runtime: rt}
	flowH := flow.Handler{Runtime: rt}
	consoleH := console.Handler{Runtime: rt}
	jobsH := maintenance.Handler{Runtime: rt}
	scheduleH := schedule.Handler{Runtime: rt}
	typesH := customtype.Handler{Runtime: rt}
	chatH := chat.Handler{Runtime: rt}
	analysisH := analysis.Handler{Runtime: rt}
	sysH := sys.Handler{Runtime: rt}
	modelH := llmmodel.Handler{Runtime: rt}
	settingH := setting.Handler{Runtime: rt}

	credentials := limiter.New(limiter.Config{
		Max:          rt.Config.Auth.RateLimitPerMinute,
		Expiration:   time.Minute,
		KeyGenerator: func(c fiber.Ctx) string { return auth.AddressKey(c.IP()) },
		LimitReached: func(c fiber.Ctx) error {
			return response.TooManyRequests(c, "too many attempts, try again in a minute")
		},
	})

	api.Get("/sys/seal-status", sysH.Status)
	api.Post("/sys/init", credentials, sysH.Init)
	api.Post("/sys/unseal", credentials, sysH.Unseal)

	api.Post("/auth/register", credentials, authH.Register)
	api.Post("/auth/login", credentials, authH.Login)
	api.Post("/auth/login/2fa", credentials, authH.LoginSecondFactor)
	api.Post("/auth/logout", authH.Logout)
	api.Get("/auth/info", authH.Info)
	api.Post("/auth/password-reset/request", credentials, authH.PasswordResetRequest)
	api.Post("/auth/password-reset/confirm", credentials, authH.PasswordResetConfirm)
	api.Post("/auth/passkeys/login/begin", credentials, authH.PasskeyLoginBegin)
	api.Post("/auth/passkeys/login/finish", credentials, authH.PasskeyLoginFinish)

	session := requireSession(rt)
	api.Get("/auth/me", session, userH.Me)
	api.Post("/auth/password", session, authH.ChangePassword)
	api.Post("/auth/2fa/setup", session, authH.TOTPSetup)
	api.Post("/auth/2fa/enable", session, authH.TOTPEnable)
	api.Post("/auth/2fa/disable", session, authH.TOTPDisable)
	api.Get("/auth/2fa/recovery-codes", session, authH.RecoveryCodesStatus)
	api.Post("/auth/2fa/recovery-codes", session, authH.RecoveryCodesRegenerate)
	api.Get("/auth/passkeys", session, authH.PasskeyList)
	api.Post("/auth/passkeys/register/begin", session, authH.PasskeyRegisterBegin)
	api.Post("/auth/passkeys/register/finish", session, authH.PasskeyRegisterFinish)
	api.Put("/auth/passkeys/:id", session, authH.PasskeyRename)
	api.Post("/auth/passkeys/:id/delete", session, authH.PasskeyDelete)
	api.Get("/auth/sessions", session, authH.ListSessions)
	api.Delete("/auth/sessions/:id", session, authH.RevokeSession)
	api.Post("/auth/sessions/revoke-others", session, authH.RevokeOtherSessions)
	api.Put("/auth/me", session, userH.UpdateMe)
	api.Get("/users/search", session, userH.Search)

	api.Get("/console", session, consoleH.Settings)
	api.Get("/models", session, modelH.Pick)

	api.Get("/types", session, typeH.List)
	api.Post("/types/detect", session, typeH.Detect)

	api.Get("/transforms", session, transformH.List)
	api.Get("/search", session, graphH.Search)
	api.Get("/transform-templates", session, templateH.List)
	api.Post("/transform-templates", session, templateH.Create)
	api.Post("/transform-templates/generate", session, templateH.Generate)
	api.Get("/transform-templates/export", session, templateH.ExportAll)
	api.Post("/transform-templates/import", session, templateH.Import)
	api.Get("/transform-templates/:id", session, templateH.Get)
	api.Get("/transform-templates/:id/export", session, templateH.Export)
	api.Put("/transform-templates/:id", session, templateH.Update)
	api.Delete("/transform-templates/:id", session, templateH.Delete)
	api.Post("/transform-templates/:id/test", session, templateH.Test)

	api.Get("/flows", session, flowH.List)
	api.Post("/flows", session, flowH.Create)
	api.Post("/flows/check", session, flowH.Check)
	api.Get("/flows/export", session, flowH.ExportAll)
	api.Post("/flows/import", session, flowH.Import)
	api.Get("/flows/:id", session, flowH.Get)
	api.Get("/flows/:id/export", session, flowH.Export)
	api.Put("/flows/:id", session, flowH.Update)
	api.Delete("/flows/:id", session, flowH.Delete)

	api.Get("/secrets", session, keyH.List)
	api.Post("/secrets", session, keyH.Put)
	api.Delete("/secrets/:id", session, keyH.Delete)

	inv := func(a invmodel.Action) fiber.Handler { return requireInvestigation(rt, a) }
	api.Get("/investigations", session, invH.List)
	api.Post("/investigations", session, invH.Create)
	api.Get("/investigations/:id", session, inv(invmodel.ActionRead), invH.Get)
	api.Put("/investigations/:id", session, inv(invmodel.ActionUpdate), invH.Update)
	api.Delete("/investigations/:id", session, inv(invmodel.ActionDelete), invH.Delete)
	api.Get("/investigations/:id/sketches", session, inv(invmodel.ActionRead), sketchH.ListByInvestigation)
	api.Post("/investigations/:id/sketches", session, inv(invmodel.ActionCreate), sketchH.Create)
	api.Post("/investigations/:id/sketches/import", session, inv(invmodel.ActionCreate), graphH.ImportSketch)
	api.Post("/investigations/:id/export", session, inv(invmodel.ActionRead), graphH.ExportInvestigation)
	api.Post("/investigations/import", session, graphH.ImportInvestigation)
	api.Get("/investigations/:id/entities", session, inv(invmodel.ActionRead), graphH.Entities)
	api.Get("/investigations/:id/analyses", session, inv(invmodel.ActionRead), analysisH.List)
	api.Post("/investigations/:id/analyses", session, inv(invmodel.ActionCreate), analysisH.Create)
	api.Get("/investigations/:id/analyses/:analysis_id", session, inv(invmodel.ActionRead), analysisH.Get)
	api.Put("/investigations/:id/analyses/:analysis_id", session, inv(invmodel.ActionUpdate), analysisH.Update)
	api.Delete("/investigations/:id/analyses/:analysis_id", session, inv(invmodel.ActionManage), analysisH.Delete)
	api.Put("/investigations/:id/owner", session, inv(invmodel.ActionDelete), invH.TransferOwner)
	api.Get("/investigations/:id/invitations", session, inv(invmodel.ActionManage), invH.ListInvitations)
	api.Post("/investigations/:id/invitations", session, inv(invmodel.ActionManage), invH.CreateInvitation)
	api.Delete("/investigations/:id/invitations/:inv_id", session, inv(invmodel.ActionManage), invH.DeleteInvitation)
	api.Get("/invitations/:token", credentials, optionalSession(rt), invH.LookupInvitation)
	api.Post("/invitations/:token/accept", session, invH.AcceptInvitation)
	api.Get("/auth/oauth/:slug/start", credentials, authH.OAuthStart)
	api.Get("/auth/oauth/:slug/callback", credentials, authH.OAuthCallback)
	api.Post("/auth/oauth/:slug/link", session, authH.OAuthLink)
	api.Get("/auth/identities", session, authH.Identities)
	api.Delete("/auth/identities/:id", session, authH.Unlink)
	api.Get("/admin/oauth-providers", session, requireAdmin, providerH.List)
	api.Post("/admin/oauth-providers", session, requireAdmin, providerH.Create)
	api.Get("/admin/oauth-providers/:id", session, requireAdmin, providerH.Get)
	api.Patch("/admin/oauth-providers/:id", session, requireAdmin, providerH.Update)
	api.Delete("/admin/oauth-providers/:id", session, requireAdmin, providerH.Delete)
	api.Post("/admin/oauth-providers/:id/test", session, requireAdmin, providerH.Test)
	api.Get("/admin/users", session, requireAdmin, userH.AdminList)
	api.Post("/admin/users", session, requireAdmin, userH.AdminCreate)
	api.Get("/admin/users/:id", session, requireAdmin, userH.AdminGet)
	api.Patch("/admin/users/:id", session, requireAdmin, userH.AdminUpdate)
	api.Delete("/admin/users/:id", session, requireAdmin, userH.AdminDelete)
	api.Post("/admin/users/:id/reset-link", session, requireAdmin, userH.AdminResetLink)
	api.Delete("/admin/users/:id/2fa", session, requireAdmin, userH.AdminResetTOTP)
	api.Delete("/admin/users/:id/passkeys", session, requireAdmin, userH.AdminResetPasskeys)
	api.Post("/admin/users/:id/revoke-sessions", session, requireAdmin, userH.AdminRevokeSessions)
	api.Get("/admin/types", session, requireAdmin, typesH.List)
	api.Post("/admin/types", session, requireAdmin, typesH.Create)
	api.Put("/admin/types/:id", session, requireAdmin, typesH.Update)
	api.Delete("/admin/types/:id", session, requireAdmin, typesH.Delete)
	api.Get("/admin/models", session, requireAdmin, modelH.List)
	api.Post("/admin/models", session, requireAdmin, modelH.Create)
	api.Patch("/admin/models/:id", session, requireAdmin, modelH.Update)
	api.Delete("/admin/models/:id", session, requireAdmin, modelH.Delete)
	api.Get("/admin/settings/mail", session, requireAdmin, settingH.GetMail)
	api.Put("/admin/settings/mail", session, requireAdmin, settingH.PutMail)
	api.Post("/admin/settings/mail/test", session, requireAdmin, settingH.TestMail)
	api.Get("/admin/settings/worker", session, requireAdmin, settingH.GetWorker)
	api.Put("/admin/settings/worker", session, requireAdmin, settingH.PutWorker)
	api.Get("/admin/settings/tools", session, requireAdmin, settingH.GetTools)
	api.Put("/admin/settings/tools", session, requireAdmin, settingH.PutTools)
	api.Get("/admin/settings/files", session, requireAdmin, settingH.GetFiles)
	api.Put("/admin/settings/files", session, requireAdmin, settingH.PutFiles)
	api.Post("/admin/seal", session, requireAdmin, sysH.Seal)
	api.Get("/admin/release", session, requireAdmin, consoleH.Release)
	api.Get("/admin/jobs", session, requireAdmin, jobsH.List)
	api.Post("/admin/jobs/:name/run", session, requireAdmin, jobsH.Run)
	api.Get("/investigations/:id/members", session, inv(invmodel.ActionRead), invH.Members)
	api.Post("/investigations/:id/members", session, inv(invmodel.ActionManage), invH.AddMember)
	api.Put("/investigations/:id/members/:user_id", session, inv(invmodel.ActionManage), invH.SetMemberRole)
	api.Delete("/investigations/:id/members/:user_id", session, inv(invmodel.ActionManage), invH.RemoveMember)

	sk := func(a invmodel.Action) fiber.Handler { return requireSketch(rt, a) }
	read, write := sk(invmodel.ActionRead), sk(invmodel.ActionUpdate)
	api.Get("/sketches", session, sketchH.List)
	api.Get("/sketches/:id", session, read, sketchH.Get)
	api.Put("/sketches/:id", session, write, sketchH.Update)
	api.Delete("/sketches/:id", session, sk(invmodel.ActionDelete), sketchH.Delete)
	api.Get("/sketches/:id/graph", session, read, graphH.Graph)
	api.Post("/sketches/:id/export", session, read, graphH.Export)
	api.Post("/sketches/:id/import/file", session, write, graphH.ImportFile)
	api.Post("/sketches/:id/nodes", session, write, graphH.AddNode)
	api.Post("/sketches/:id/import", session, write, graphH.Import)
	api.Post("/sketches/:id/import/nmap", session, write, graphH.ImportNmap)
	api.Post("/sketches/:id/import/nuclei", session, write, graphH.ImportNuclei)
	api.Post("/sketches/:id/import/table/preview", session, write, graphH.ImportTablePreview)
	api.Post("/sketches/:id/import/table", session, write, graphH.ImportTable)
	api.Get("/sketches/:id/nodes/:node_id/files", session, read, fileH.List)
	api.Post("/sketches/:id/nodes/:node_id/files", session, write, fileH.Upload)
	api.Get("/sketches/:id/files/:file_id", session, read, fileH.Serve)
	api.Get("/sketches/:id/files/:file_id/mail", session, read, fileH.Mail)
	api.Get("/sketches/:id/files/:file_id/mail/parts/:index", session, read, fileH.MailPart)
	api.Delete("/sketches/:id/files/:file_id", session, write, fileH.Delete)
	api.Put("/sketches/:id/nodes/positions", session, write, graphH.SetPositions)
	api.Post("/sketches/:id/nodes/delete", session, write, graphH.DeleteNodes)
	api.Post("/sketches/:id/nodes/dependents", session, read, graphH.Dependents)
	api.Post("/sketches/:id/clear", session, write, graphH.Clear)
	api.Get("/sketches/:id/changes", session, read, graphH.Changes)
	api.Post("/sketches/:id/undo", session, write, graphH.Undo)
	api.Post("/sketches/:id/redo", session, write, graphH.Redo)
	api.Post("/sketches/:id/nodes/merge", session, write, graphH.MergeNodes)
	api.Patch("/sketches/:id/nodes/:node_id", session, write, graphH.UpdateNode)
	api.Get("/sketches/:id/nodes/:node_id/neighbors", session, read, graphH.Neighbors)
	api.Post("/sketches/:id/edges", session, write, graphH.AddEdge)
	api.Post("/sketches/:id/edges/delete", session, write, graphH.DeleteEdges)
	api.Patch("/sketches/:id/edges/:edge_id", session, write, graphH.UpdateEdge)
	api.Post("/sketches/:id/transforms/:name/launch", session, write, transformH.Launch)
	api.Post("/sketches/:id/templates/:template_id/launch", session, write, templateH.Launch)
	api.Post("/sketches/:id/flows/:flow_id/launch", session, write, flowH.Launch)
	api.Get("/sketches/:id/schedules", session, read, scheduleH.List)
	api.Post("/sketches/:id/schedules", session, write, scheduleH.Create)
	api.Patch("/sketches/:id/schedules/:schedule_id", session, write, scheduleH.Update)
	api.Delete("/sketches/:id/schedules/:schedule_id", session, write, scheduleH.Delete)
	api.Post("/sketches/:id/schedules/:schedule_id/run", session, write, scheduleH.Run)
	api.Get("/sketches/:id/scans", session, read, scanH.List)
	api.Get("/sketches/:id/scans/:scan_id", session, read, scanH.Get)
	api.Post("/sketches/:id/scans/:scan_id/cancel", session, write, scanH.Cancel)
	api.Delete("/sketches/:id/scans/:scan_id", session, write, scanH.Delete)
	api.Get("/sketches/:id/logs", session, read, logH.List)
	api.Delete("/sketches/:id/logs", session, write, logH.Clear)
	api.Get("/sketches/:id/events", session, read, logH.Stream)
	api.Get("/sketches/:id/chats", session, read, chatH.List)
	api.Post("/sketches/:id/chats", session, read, chatH.Create)
	api.Delete("/sketches/:id/chats/:chat_id", session, read, chatH.Delete)
	api.Get("/sketches/:id/chats/:chat_id/messages", session, read, chatH.Messages)
	api.Post("/sketches/:id/chats/:chat_id/messages", session, read, chatH.Send)
	api.Post("/sketches/:id/chats/:chat_id/apply", session, write, chatH.Apply)
}

// consolePolicy is the shell's Content-Security-Policy. Scripts only
// from the build, inline styles for the components' style bindings,
// pictures and map tiles from anywhere an entity or the map style
// names. The workers carry no policy of their own (MapLibre's is a
// same-origin asset), and file serves keep theirs.
const consolePolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob: https: http:; font-src 'self' data:; connect-src 'self' https: http:; " +
	"worker-src 'self' blob:; frame-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"

// mountConsole serves the embedded build under env.ConsolePath with a
// history fallback to index.html.
//
// The fallback does NOT cover /assets: a hashed chunk is never a deep
// link, and answering a missing one with the HTML shell at 200 makes
// the browser report an opaque module import error.
func mountConsole(app *fiber.App, sub fs.FS) {
	app.Use(env.ConsolePath, func(c fiber.Ctx) error {
		if strings.HasPrefix(c.Path(), env.ConsolePath+"/assets/") {
			c.Set(fiber.HeaderCacheControl, "public, max-age=31536000, immutable")
		} else {
			c.Set(fiber.HeaderCacheControl, "no-cache")
			c.Set(fiber.HeaderContentSecurityPolicy, consolePolicy)
		}

		// Embedded files carry a zero modification time, which would
		// answer every date revalidation with 304 and pin an old build.
		c.Request().Header.Del(fiber.HeaderIfModifiedSince)
		err := c.Next()
		c.Response().Header.Del(fiber.HeaderLastModified)

		return err
	})

	app.Use(env.ConsolePath+"/assets", func(c fiber.Ctx) error {
		f, err := sub.Open(strings.TrimPrefix(c.Path(), env.ConsolePath+"/"))
		if err != nil {
			c.Set(fiber.HeaderCacheControl, "no-store")

			return c.Status(fiber.StatusNotFound).SendString("not found")
		}

		_ = f.Close()

		return c.Next()
	})

	app.Use(env.ConsolePath, static.New("", static.Config{
		FS:         sub,
		IndexNames: []string{"index.html"},
		NotFoundHandler: func(c fiber.Ctx) error {
			page, err := fs.ReadFile(sub, "index.html")
			if err != nil {
				return c.SendStatus(fiber.StatusNotFound)
			}

			c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)

			return c.Status(fiber.StatusOK).Send(page)
		},
	}))
}
