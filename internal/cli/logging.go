// Gosint, Copyright (c) 2026 YouSysAdmin

package cli

import (
	"log/slog"

	logger "github.com/yousysadmin/go-logger"

	"github.com/yousysadmin/gosint/internal/core/env"
)

// buildLogger turns the logging config into exactly one sink.
func buildLogger(cfg env.LoggingConfig) (*slog.Logger, error) {
	format := cfg.Format
	if format == "" {
		format = logger.FormatJSON
	}

	output := cfg.Output
	if output == "" {
		output = logger.OutputStdout
	}

	// Color is honored by the library only on an interactive terminal.
	return logger.New(logger.Sink{
		Level:  cfg.Level,
		Output: output,
		Format: format,
		Color:  cfg.Color && format == logger.FormatText,
	})
}
