// Gosint, Copyright (c) 2026 YouSysAdmin

package desktop

import (
	"path/filepath"

	"github.com/ncruces/zenity"

	"github.com/yousysadmin/gosint/internal/core/env"
)

// Defaults put everything the node keeps into the data directory, the
// directories beside the database follow it.
func Defaults(dir string) map[string]any {
	return map[string]any{
		"database.driver": env.DriverSQLite,
		"database.dsn":    filepath.Join(dir, "gosint.db"),
		"logging.format":  "text",
		"logging.output":  filepath.Join(dir, "gosint.log"),
		"logging.color":   false,
	}
}

// fail shows err in a dialog, since a desktop app has no terminal to
// print it to, and hands it back for the exit code.
func fail(err error) error {
	_ = zenity.Error(err.Error(), zenity.Title("gosint"))

	return err
}
