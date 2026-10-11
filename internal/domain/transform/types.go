// Gosint, Copyright (c) 2026 YouSysAdmin

package transform

import (
	"github.com/yousysadmin/gosint/internal/core/transform"
	scanmodel "github.com/yousysadmin/gosint/internal/models/scan"
)

// launchInput runs a transform on selected nodes.
type launchInput struct {
	NodeIDs []string          `json:"node_ids" validate:"required,min=1,max=1000,dive,uuid"`
	Params  map[string]string `json:"params"   validate:"max=64"`
}

// listed is one transform of the catalog. TemplateID is set for a YAML
// template, which launches by id, "" for a built-in one. File names
// the file of a transform read from the transforms directory.
type listed struct {
	transform.Meta
	TemplateID string `json:"template_id"`
	Public     bool   `json:"is_public"`
	File       string `json:"file"`
}

// transformsResponse answers the catalog.
type transformsResponse struct {
	Transforms []listed `json:"transforms"`
}

// launchResponse answers a launch with the queued scan.
type launchResponse struct {
	Scan scanmodel.Scan `json:"scan"`
}
