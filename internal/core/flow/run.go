// Gosint, Copyright (c) 2026 YouSysAdmin

package flow

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"

	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/core/worker"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	logmodel "github.com/yousysadmin/gosint/internal/models/logentry"
	scanmodel "github.com/yousysadmin/gosint/internal/models/scan"
)

// ScanKind is the kind of scan that runs a flow.
const ScanKind = "flow"

// maxStepInputs bounds how many nodes one step is fed, so a step that
// found a thousand subdomains does not run its child a thousand times
// over without saying so.
const maxStepInputs = 500

// RunnerConfig is what the flow handler needs: the transform runner's
// config plus how to read a scan's flow.
type RunnerConfig struct {
	transform.RunnerConfig

	// Registry has the transforms steps name.
	Registry *transform.Registry

	// Definition loads the flow a scan runs, for its owner.
	Definition func(ctx context.Context, flowID, ownerID string) (*Definition, error)
}

// Handler runs a flow scan, step by step, parents first.
//
// A failing step is logged and its subtree skipped, the other branches
// run on. Nothing is retried within the scan.
func Handler(cfg RunnerConfig) worker.Handler {
	return func(ctx context.Context, run *worker.Run) (map[string]any, error) {
		sc := run.Scan
		if sc.SketchID == "" {
			return nil, errors.New("a flow scan needs a sketch")
		}

		def, err := definitionOf(ctx, cfg, sc)
		if err != nil {
			return nil, err
		}

		steps, err := Order(*def)
		if err != nil {
			return nil, err
		}

		ids, _ := sc.Input["node_ids"].([]any)
		var nodeIDs []string
		for _, v := range ids {
			if s, ok := v.(string); ok {
				nodeIDs = append(nodeIDs, s)
			}
		}

		start, err := transform.LoadInputs(ctx, cfg.Graph, sc.SketchID, def.Input, nodeIDs)
		if err != nil {
			return nil, err
		}

		produced := map[string][]graphmodel.Node{Root: start}
		summary := map[string]any{}
		nodes, edges := 0, 0
		writtenIDs := []string{}
		seen := map[string]bool{}
		for _, s := range steps {
			if err := ctx.Err(); err != nil {
				return nil, err
			}

			inputs, ok := produced[s.After]
			if !ok || len(inputs) == 0 {
				summary[s.ID] = "skipped"

				continue
			}

			written, n, e, err := runStep(ctx, cfg, run, s, inputs)
			if err != nil {
				run.Log(logmodel.LevelFailed, fmt.Sprintf("step %s (%s) failed: %v", s.ID, s.Transform, err), nil)
				summary[s.ID] = "failed: " + err.Error()

				continue
			}

			produced[s.ID] = written
			nodes, edges = nodes+n, edges+e
			// passed is what the step hands on: what it wrote, or for a
			// filter what it kept without writing.
			summary[s.ID] = map[string]any{"inputs": len(inputs), "nodes": n, "edges": e, "passed": len(written)}
			for _, w := range written {
				if !seen[w.ID] && len(writtenIDs) < transform.MaxWrittenIDs {
					seen[w.ID] = true
					writtenIDs = append(writtenIDs, w.ID)
				}
			}
		}

		return map[string]any{"steps": summary, "nodes": nodes, "edges": edges, "written_ids": writtenIDs}, nil
	}
}

// definitionOf is the flow a scan runs: one carried inline in its input,
// composed for this run and never stored (the chat's), else the saved
// flow the input names. An inline one is checked again here, since the
// worker trusts no input.
func definitionOf(ctx context.Context, cfg RunnerConfig, sc *scanmodel.Scan) (*Definition, error) {
	raw, inline := sc.Input["definition"]
	if !inline {
		flowID, _ := sc.Input["flow_id"].(string)

		return cfg.Definition(ctx, flowID, sc.OwnerID)
	}

	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}

	var def Definition
	if err := json.Unmarshal(b, &def); err != nil {
		return nil, fmt.Errorf("inline flow: %w", err)
	}

	if err := Check(def, cfg.Registry); err != nil {
		return nil, err
	}

	return &def, nil
}

// runStep runs one step's transform on the nodes of its input type.
func runStep(ctx context.Context, cfg RunnerConfig, run *worker.Run, s Step, inputs []graphmodel.Node) ([]graphmodel.Node, int, int, error) {
	e, ok := cfg.Registry.Get(s.Transform)
	if !ok {
		return nil, 0, 0, fmt.Errorf("no transform %q in this build", s.Transform)
	}

	meta := e.Meta()
	var fit []graphmodel.Node
	for _, n := range inputs {
		if meta.Input == "" || n.Type == meta.Input {
			fit = append(fit, n)
		}
	}

	if len(fit) > maxStepInputs {
		run.Log(logmodel.LevelWarning, fmt.Sprintf("step %s: %d inputs, running on the first %d", s.ID, len(fit), maxStepInputs), nil)
		fit = fit[:maxStepInputs]
	}

	if len(fit) == 0 {
		return nil, 0, 0, nil
	}

	params, err := transform.ResolveParams(meta.Params, s.Params, func(ref string) (string, bool, error) {
		return cfg.Secrets(ctx, run.Scan.OwnerID, ref)
	})
	if err != nil {
		return nil, 0, 0, err
	}

	x := transform.NewExec(transform.ExecConfig{
		SketchID: run.Scan.SketchID, Graph: cfg.Graph, Types: cfg.Types, Log: run,
		Params: params, HTTP: cfg.HTTP, Web: cfg.Web, Resolver: cfg.Resolver, CacheDir: cfg.CacheDirNow(),
		Tools: cfg.ToolSettings(), ScanID: run.Scan.ID, Transform: s.Transform, Step: s.ID, Files: cfg.Files,
	})

	run.Log(logmodel.LevelRunning, fmt.Sprintf("step %s: %s %s", s.ID, s.Transform, transform.Describe(meta, fit, params, s.Params)), nil)
	if err := e.Run(ctx, x, fit); err != nil {
		return nil, 0, 0, err
	}

	n, ed := x.Counts()

	return x.Written(), n, ed, nil
}
