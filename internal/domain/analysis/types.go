// Gosint, Copyright (c) 2026 YouSysAdmin

package analysis

import (
	"encoding/json/jsontext"

	amodel "github.com/yousysadmin/gosint/internal/models/analysis"
)

// maxContent bounds one document, as JSON.
const maxContent = 2 << 20

// saveInput is a title and the editor's document.
type saveInput struct {
	Title   string         `json:"title"   validate:"required,notblank,max=200" normalize:"trim"`
	Content jsontext.Value `json:"content"`
}

// analysisResponse answers one analysis.
type analysisResponse struct {
	Analysis amodel.Analysis `json:"analysis"`
}

// analysesResponse lists analyses, without their content.
type analysesResponse struct {
	Analyses []amodel.Analysis `json:"analyses"`
}
