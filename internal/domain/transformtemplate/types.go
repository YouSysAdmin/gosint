// Gosint, Copyright (c) 2026 YouSysAdmin

package transformtemplate

import (
	"github.com/yousysadmin/gosint/internal/core/template"
	scanmodel "github.com/yousysadmin/gosint/internal/models/scan"
	tplmodel "github.com/yousysadmin/gosint/internal/models/transformtemplate"
)

// saveInput is a template's YAML and whether it is shared.
type saveInput struct {
	Source string `json:"source"    validate:"required,max=65536"`
	Public bool   `json:"is_public"`
}

// testInput is one value to run the template on, of its input type.
type testInput struct {
	Value  string            `json:"value"  validate:"required,notblank,max=4096" normalize:"trim"`
	Params map[string]string `json:"params" validate:"max=64"`
}

// launchInput runs the template on selected nodes.
type launchInput struct {
	NodeIDs []string          `json:"node_ids" validate:"required,min=1,max=1000,dive,uuid"`
	Params  map[string]string `json:"params"   validate:"max=64"`
}

// templateResponse answers one template.
type templateResponse struct {
	Template tplmodel.Template `json:"template"`
}

// templatesResponse answers the list.
type templatesResponse struct {
	Templates []tplmodel.Template `json:"templates"`
}

// testOutput is one mapped output and whether its type accepts it.
type testOutput struct {
	Label      string         `json:"label"`
	Properties map[string]any `json:"properties"`
	Error      string         `json:"error"`
}

// testResponse answers a test run: the raw answer and what it maps to.
type testResponse struct {
	Answer  *template.Answer `json:"answer"`
	Outputs []testOutput     `json:"outputs"`
	Error   string           `json:"error"`
}

// launchResponse answers a launch with the queued scan.
type launchResponse struct {
	Scan scanmodel.Scan `json:"scan"`
}
