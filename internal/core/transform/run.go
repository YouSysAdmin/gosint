// Gosint, Copyright (c) 2026 YouSysAdmin

package transform

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/yousysadmin/gosint/internal/core/tool"
	"github.com/yousysadmin/gosint/internal/core/types"
	"github.com/yousysadmin/gosint/internal/core/worker"
	"github.com/yousysadmin/gosint/internal/domain/store"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	scanmodel "github.com/yousysadmin/gosint/internal/models/scan"
)

// ScanKind is the kind of scan that runs one transform.
const ScanKind = "transform"

// Input is what a transform scan carries in scans.input.
type Input struct {
	NodeIDs []string          `json:"node_ids"`
	Params  map[string]string `json:"params"`
}

// InputMap is in as the scan's input object.
func (in Input) InputMap() map[string]any {
	ids := make([]any, 0, len(in.NodeIDs))
	for _, id := range in.NodeIDs {
		ids = append(ids, id)
	}

	params := map[string]any{}
	for k, v := range in.Params {
		params[k] = v
	}

	return map[string]any{"node_ids": ids, "params": params}
}

// inputOf reads a scan's input back. Params are strings, as stored.
func inputOf(m map[string]any) Input {
	var in Input
	ids, _ := m["node_ids"].([]any)
	for _, v := range ids {
		if s, ok := v.(string); ok {
			in.NodeIDs = append(in.NodeIDs, s)
		}
	}

	params, _ := m["params"].(map[string]any)
	in.Params = map[string]string{}
	for k, v := range params {
		if s, ok := v.(string); ok {
			in.Params[k] = s
		}
	}

	return in
}

// RunnerConfig is what the transform scan handler needs.
type RunnerConfig struct {
	// Lookup finds the transform a scan runs. ByName serves the built-in
	// registry, the template kind looks templates up instead.
	Lookup func(ctx context.Context, sc *scanmodel.Scan) (Transform, error)
	Types  *types.Registry
	Graph  store.GraphStore

	// Secrets opens an owner's secret by name or id.
	Secrets  func(ctx context.Context, ownerID, ref string) (string, bool, error)
	HTTP     *http.Client
	Web      *http.Client
	Resolver *net.Resolver

	// CacheDir reads where downloaded reference data is kept. May be nil.
	CacheDir func() string

	// Tools reads the administrator's tool settings, handed to every
	// run as they are when it starts. May be nil.
	Tools func() tool.Settings

	// Files is the store of the files attached to entities. May be nil.
	Files store.AttachmentStore
}

// CacheDirNow is the cache directory now, none without a source.
func (cfg RunnerConfig) CacheDirNow() string {
	if cfg.CacheDir == nil {
		return ""
	}

	return cfg.CacheDir()
}

// ToolSettings is the tool settings now, none without a source.
func (cfg RunnerConfig) ToolSettings() tool.Settings {
	if cfg.Tools == nil {
		return tool.Settings{}
	}

	return cfg.Tools()
}

// ByName looks a scan's transform up in reg by the scan's name.
func ByName(reg *Registry) func(context.Context, *scanmodel.Scan) (Transform, error) {
	return func(_ context.Context, sc *scanmodel.Scan) (Transform, error) {
		e, ok := reg.Get(sc.Name)
		if !ok {
			return nil, fmt.Errorf("no transform %q in this build", sc.Name)
		}

		return e, nil
	}
}

// Handler is the worker handler running one transform per scan.
func Handler(cfg RunnerConfig) worker.Handler {
	return func(ctx context.Context, run *worker.Run) (map[string]any, error) {
		sc := run.Scan
		e, err := cfg.Lookup(ctx, sc)
		if err != nil {
			return nil, err
		}

		if sc.SketchID == "" {
			return nil, errors.New("a transform scan needs a sketch")
		}

		in := inputOf(sc.Input)
		meta := e.Meta()

		params, err := ResolveParams(meta.Params, in.Params, func(ref string) (string, bool, error) {
			return cfg.Secrets(ctx, sc.OwnerID, ref)
		})
		if err != nil {
			return nil, err
		}

		inputs, err := LoadInputs(ctx, cfg.Graph, sc.SketchID, meta.Input, in.NodeIDs)
		if err != nil {
			return nil, err
		}

		x := NewExec(ExecConfig{
			SketchID: sc.SketchID, Graph: cfg.Graph, Types: cfg.Types, Log: run,
			Params: params, HTTP: cfg.HTTP, Web: cfg.Web, Resolver: cfg.Resolver, CacheDir: cfg.CacheDirNow(),
			Tools: cfg.ToolSettings(), ScanID: sc.ID, Transform: meta.Name, Files: cfg.Files,
		})

		if len(inputs) == 0 {
			x.Warn("no %s node among the %d selected, %s has nothing to do", meta.Input, len(in.NodeIDs), meta.Name)

			return map[string]any{"inputs": 0, "nodes": 0, "edges": 0}, nil
		}

		x.Info("%s %s", meta.Name, Describe(meta, inputs, params, in.Params))

		if err := e.Run(ctx, x, inputs); err != nil {
			return nil, err
		}

		nodes, edges := x.Counts()

		// written_ids lets a reader (the assistant) see WHAT was written,
		// the counts say only how much.
		return map[string]any{"inputs": len(inputs), "nodes": nodes, "edges": edges, "written_ids": x.WrittenIDs()}, nil
	}
}

// LoadInputs reads the selected nodes of the sketch that are of the
// transform's input type. Others are skipped.
func LoadInputs(ctx context.Context, g store.GraphStore, sketchID, inputType string, ids []string) ([]graphmodel.Node, error) {
	var out []graphmodel.Node
	for _, id := range ids {
		n, err := g.Node(ctx, sketchID, id)
		if err != nil {
			return nil, err
		}

		if n != nil && (inputType == "" || n.Type == inputType) {
			out = append(out, *n)
		}
	}

	return out, nil
}
