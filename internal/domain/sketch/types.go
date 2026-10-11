// Gosint, Copyright (c) 2026 YouSysAdmin

package sketch

import sketchmodel "github.com/yousysadmin/gosint/internal/models/sketch"

// createInput is a new sketch in the investigation of the path.
type createInput struct {
	Title       string `json:"title"       validate:"required,notblank,max=200" normalize:"trim"`
	Description string `json:"description" validate:"max=10000" normalize:"trim"`
}

// updateInput is decoded over the stored sketch.
type updateInput struct {
	Title       string `json:"title"       validate:"required,notblank,max=200" normalize:"trim"`
	Description string `json:"description" validate:"max=10000" normalize:"trim"`
	Status      string `json:"status"      validate:"oneof=active archived"`
}

// sketchResponse answers a single sketch.
type sketchResponse struct {
	Sketch sketchmodel.Sketch `json:"sketch"`
}

// sketchesResponse answers a list.
type sketchesResponse struct {
	Sketches []sketchmodel.Sketch `json:"sketches"`
}
