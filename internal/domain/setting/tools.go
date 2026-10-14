// Gosint, Copyright (c) 2026 YouSysAdmin

package setting

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/sandbox"
	"github.com/yousysadmin/gosint/internal/core/tool"
	"github.com/yousysadmin/gosint/internal/core/validation"
)

// readTools is the stored tool settings, none when never saved.
func readTools(ctx context.Context, rt *env.Runtime) (tool.Settings, error) {
	s := tool.Settings{}
	_, err := rt.Store.Setting.Get(ctx, Tools, &s)

	return s, err
}

func (h Handler) toolViews(s tool.Settings) toolsResponse {
	out := toolsResponse{Tools: []ToolView{}}
	for _, t := range h.Runtime.Tools.All() {
		e := s.Get(t.Name())
		v := ToolView{Name: t.Name(), Title: t.Title(), Description: t.Description(), Binary: t.Binary(),
			Path: e.Path, AllowPrivate: e.AllowPrivate, PrivateSwitch: t.Name() != tool.ScriptsName}
		if t.Binary() != "" {
			v.Found, _ = s.Locate(t)
		}

		if t.Name() == tool.ScriptsName {
			v.Sandbox = &SandboxView{Enabled: e.Confined(sandbox.Supported()), Supported: sandbox.Supported(),
				ReadPaths: nonNil(e.ReadPaths), WritePaths: nonNil(e.WritePaths)}
		}

		out.Tools = append(out.Tools, v)
	}

	return out
}

// GetTools answers every tool with its settings and where this node
// finds its program.
func (h Handler) GetTools(c fiber.Ctx) error {
	s, err := readTools(c, h.Runtime)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, h.toolViews(s))
}

// PutTools saves the tools named in the body, the others left as they
// are. A path must be absolute, so it cannot depend on where the
// server was started.
func (h Handler) PutTools(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[toolsInput](c)
	if !ok {
		return resp
	}

	s, err := readTools(c, h.Runtime)
	if err != nil {
		return response.Internal(c, err)
	}

	for name, e := range in.Tools {
		t, known := h.Runtime.Tools.Get(name)
		if !known {
			return refuse(c, "tools."+name, "unknown", "no tool named "+name)
		}

		e.Path = strings.TrimSpace(e.Path)
		if t.Binary() == "" {
			e.Path = ""
		}

		if e.Path != "" && !filepath.IsAbs(e.Path) {
			return refuse(c, "tools."+name+".path", "abs", "the path of "+t.Title()+" must be absolute")
		}

		if name != tool.ScriptsName {
			e.Sandbox, e.ReadPaths, e.WritePaths = nil, nil, nil
		} else {
			e.AllowPrivate = false

			var field string

			e.ReadPaths, field = cleanPaths(e.ReadPaths, "read_paths")
			if field == "" {
				e.WritePaths, field = cleanPaths(e.WritePaths, "write_paths")
			}

			if field != "" {
				return refuse(c, "tools."+name+"."+field, "abs", "every path a script may reach must be absolute")
			}

			if own := h.ownFileUnder(e.ReadPaths); own != "" {
				return refuse(c, "tools."+name+".read_paths", "own", "a script may not reach the server's own file "+own)
			}

			if own := h.ownFileUnder(e.WritePaths); own != "" {
				return refuse(c, "tools."+name+".write_paths", "own", "a script may not reach the server's own file "+own)
			}
		}

		s[name] = e
	}

	if err := h.Runtime.Store.Setting.Put(c, Tools, s, access.User(c).ID); err != nil {
		return response.Internal(c, err)
	}

	h.Runtime.ToolSettings.Set(s)

	return response.Success(c, h.toolViews(s))
}

// ownFileUnder names the first of the server's own files the grants
// reach, on this node.
func (h Handler) ownFileUnder(grants []string) string {
	for _, own := range h.Runtime.Config.OwnFiles() {
		if sandbox.Reaches(grants, own) {
			return own
		}
	}

	return ""
}

// maxPaths bounds the extra paths a script may be given.
const maxPaths = 64

// cleanPaths trims and drops empty lines, and names field when a path
// is not absolute or there are too many.
func cleanPaths(in []string, field string) ([]string, string) {
	var out []string

	for _, p := range in {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		if !filepath.IsAbs(p) || len(p) > 1024 {
			return nil, field
		}

		out = append(out, filepath.Clean(p))
	}

	if len(out) > maxPaths {
		return nil, field
	}

	return out, ""
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}

	return s
}
