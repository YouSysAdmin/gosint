// Gosint, Copyright (c) 2026 YouSysAdmin

package console

// settingsResponse is what the console is told to load.
type settingsResponse struct {
	MapStyleLight string `json:"map_style_light"`
	MapStyleDark  string `json:"map_style_dark"`
}
