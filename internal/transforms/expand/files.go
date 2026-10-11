// Gosint, Copyright (c) 2026 YouSysAdmin

package expand

import (
	"context"
	"fmt"
	"strings"

	"github.com/yousysadmin/gosint/internal/core/filemeta"
	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
)

// FilesToEntities draws what the files attached to an entity say about
// themselves: where a photo was taken as a location, who wrote a
// document as an individual, the camera as a device. Nothing is looked
// up, the metadata was read at upload.
type FilesToEntities struct{}

// Meta describes the transform. Input "" takes any type.
func (FilesToEntities) Meta() transform.Meta {
	return transform.Meta{Name: "files_to_entities", Category: "Graph", Output: "",
		Description: "Draw what the entity's attached files say about themselves: the place a photo was taken, " +
			"the person who wrote a document, the camera it was shot with. Nothing is looked up."}
}

// Run unfolds each input's files.
func (FilesToEntities) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		files, err := x.Files(ctx, in)
		if err != nil {
			return err
		}

		for _, f := range files {
			m := f.Metadata
			if len(m) == 0 {
				continue
			}

			if lat, lon, ok := coordinates(m); ok {
				place := map[string]any{"address": fmt.Sprintf("%.5f, %.5f", lat, lon), "latitude": lat, "longitude": lon}
				if _, err := x.NodeAndLink(ctx, in, "TAKEN_AT", "location", place); err != nil {
					x.Warn("%s: %s: %v", in.Label, f.Name, err)
				}
			}

			for _, key := range []string{filemeta.Author, filemeta.LastEditor} {
				if name, _ := m[key].(string); strings.TrimSpace(name) != "" && !strings.Contains(name, "@") {
					if _, err := x.NodeAndLink(ctx, in, "AUTHORED_BY", "individual", map[string]any{"full_name": strings.TrimSpace(name)}); err != nil {
						x.Warn("%s: %s: %v", in.Label, f.Name, err)
					}
				}
			}

			make, _ := m[filemeta.CameraMake].(string)
			model, _ := m[filemeta.CameraModel].(string)
			if camera := strings.TrimSpace(make + " " + model); camera != "" {
				device := map[string]any{"device_id": camera, "type": "camera", "manufacturer": make, "model": model}
				if _, err := x.NodeAndLink(ctx, in, "TAKEN_WITH", "device", device); err != nil {
					x.Warn("%s: %s: %v", in.Label, f.Name, err)
				}
			}

			if company, _ := m[filemeta.Company].(string); strings.TrimSpace(company) != "" {
				if _, err := x.NodeAndLink(ctx, in, "AUTHORED_AT", "organization", map[string]any{"name": strings.TrimSpace(company)}); err != nil {
					x.Warn("%s: %s: %v", in.Label, f.Name, err)
				}
			}
		}
	}

	return nil
}

// coordinates reads a position out of the metadata, numbers as the
// JSON column gives them back.
func coordinates(m map[string]any) (lat, lon float64, ok bool) {
	la, okA := m[filemeta.Latitude].(float64)
	lo, okB := m[filemeta.Longitude].(float64)

	return la, lo, okA && okB
}
