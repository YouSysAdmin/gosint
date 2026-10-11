// Gosint, Copyright (c) 2026 YouSysAdmin

// Package automation holds the transforms that hand entities to systems
// outside gosint.
package automation

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/httpx"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
)

// N8NConnector posts the selected entities to an n8n webhook.
type N8NConnector struct{}

// Meta describes the transform. Input "" takes any type.
func (N8NConnector) Meta() transform.Meta {
	return transform.Meta{Name: "n8n_connector", Category: "Automation", Output: "phrase",
		Description: "Send the selected entities to an n8n webhook, for workflows run outside gosint.",
		Params: []transform.Param{
			{Name: "webhook_url", Type: transform.ParamURL, Required: true, Description: "The n8n webhook URL to send data to."},
			{Name: "N8N_WEBHOOK_TOKEN", Type: transform.ParamSecret,
				Description: "The secret holding the token n8n checks on the webhook, sent as a Bearer Authorization header. Leave it unset for a webhook without auth."},
			{Name: "extra_payload", Type: transform.ParamString, Description: "A JSON object merged into the payload."},
		}}
}

// Run posts once for all inputs. The webhook goes through the client
// refusing internal addresses, so it must be reachable publicly.
func (N8NConnector) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	entities := make([]map[string]any, 0, len(inputs))
	for _, in := range inputs {
		entities = append(entities, map[string]any{"id": in.ID, "type": in.Type, "label": in.Label, "properties": in.Properties})
	}

	payload := map[string]any{}
	if extra := x.Param("extra_payload"); extra != "" {
		if err := json.Unmarshal([]byte(extra), &payload); err != nil {
			return fmt.Errorf("extra_payload is not a JSON object: %w", err)
		}
	}

	payload["sketch_id"] = inputs[0].SketchID
	payload["inputs"] = entities

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, httpx.RequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, x.Param("webhook_url"), bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	if tok := x.Param("N8N_WEBHOOK_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := x.Web.Do(req)
	if errors.Is(err, transform.ErrPrivateTarget) {
		return fmt.Errorf("%w: an n8n on this network needs worker.allow_private_targets", err)
	}

	if err != nil {
		return err
	}

	defer resp.Body.Close() //nolint:errcheck // read-only

	answer, _ := io.ReadAll(io.LimitReader(resp.Body, httpx.MaxBody))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("n8n answered %s: %s", resp.Status, props.Truncate(string(answer), 300))
	}

	x.Info("sent %d entities to n8n, it answered %s", len(entities), props.Truncate(string(answer), 300))

	return nil
}

// All is the package's transforms.
func All() []transform.Transform {
	return []transform.Transform{N8NConnector{}}
}
