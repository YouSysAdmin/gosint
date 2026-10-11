// Gosint, Copyright (c) 2026 YouSysAdmin

package eventlog

import logmodel "github.com/yousysadmin/gosint/internal/models/logentry"

// logsResponse answers a sketch's log.
type logsResponse struct {
	Logs []logmodel.Entry `json:"logs"`
}

// clearedResponse answers clearing the log.
type clearedResponse struct {
	Count int64 `json:"count"`
}

// connected opens a stream.
type connected struct {
	SketchID string `json:"sketch_id"`
}

// reconnect closes a recycled stream.
type reconnect struct {
	Reason string `json:"reason"`
}
