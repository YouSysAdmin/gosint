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
			Path: e.Path, AllowPrivate: e.AllowPrivate}
		if t.Binary() != "" {
			v.Found, _ = s.Locate(t)
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

		s[name] = e
	}

	if err := h.Runtime.Store.Setting.Put(c, Tools, s, access.User(c).ID); err != nil {
		return response.Internal(c, err)
	}

	h.Runtime.ToolSettings.Set(s)

	return response.Success(c, h.toolViews(s))
}
