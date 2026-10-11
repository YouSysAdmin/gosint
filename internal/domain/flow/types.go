// Gosint, Copyright (c) 2026 YouSysAdmin

package flow

import (
	flowmodel "github.com/yousysadmin/gosint/internal/models/flow"
	scanmodel "github.com/yousysadmin/gosint/internal/models/scan"
)

// saveInput is a flow to store.
type saveInput struct {
	Name        string               `json:"name"        validate:"required,notblank,max=200" normalize:"trim"`
	Description string               `json:"description" validate:"max=10000" normalize:"trim"`
	Definition  flowmodel.Definition `json:"definition"`
}

// checkInput is a definition to check without storing it.
type checkInput struct {
	Definition flowmodel.Definition `json:"definition"`
}

// launchInput runs a flow on selected nodes.
type launchInput struct {
	NodeIDs []string `json:"node_ids" validate:"required,min=1,max=1000,dive,uuid"`
}

// plannedStep is one step in run order, with its types resolved.
type plannedStep struct {
	ID        string `json:"id"`
	After     string `json:"after"`
	Transform string `json:"transform"`
	Input     string `json:"input"`
	Output    string `json:"output"`
}

// checkResponse answers a check with the steps in run order.
type checkResponse struct {
	Steps []plannedStep `json:"steps"`
}

// flowResponse answers one flow.
type flowResponse struct {
	Flow flowmodel.Flow `json:"flow"`
}

// flowsResponse answers the list.
type flowsResponse struct {
	Flows []flowmodel.Flow `json:"flows"`
}

// launchResponse answers a launch with the queued scan.
type launchResponse struct {
	Scan scanmodel.Scan `json:"scan"`
}
