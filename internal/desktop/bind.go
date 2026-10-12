// Gosint, Copyright (c) 2026 YouSysAdmin

package desktop

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ncruces/zenity"
)

// openTimeout bounds the call handing a URL to the system.
const openTimeout = 10 * time.Second

// OpenExternal hands a web address to the default browser. Anything
// but http and https is refused, a page must not start programs.
func OpenExternal(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("not a web address: %q", raw)
	}

	ctx, cancel := context.WithTimeout(context.Background(), openTimeout)
	defer cancel()

	return opener(ctx, u.String()).Run()
}

// opener is the system's own way of opening a web address.
func opener(ctx context.Context, u string) *exec.Cmd {
	switch runtime.GOOS {
	case "darwin":
		return exec.CommandContext(ctx, "open", u) //nolint:gosec // the scheme is checked by the caller
	case "windows":
		// Not cmd /c start, which reads & and ^ in a URL as its own.
		return exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", u) //nolint:gosec // the scheme is checked by the caller
	default:
		return exec.CommandContext(ctx, "xdg-open", u) //nolint:gosec // the scheme is checked by the caller
	}
}

// Save asks where to keep a file the console made and writes it there.
// It answers false when the person cancelled.
func Save(name, data string) (bool, error) {
	body, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return false, fmt.Errorf("file content: %w", err)
	}

	path, err := zenity.SelectFileSave(
		zenity.Title("Save"),
		zenity.Filename(suggestedPath(name)),
		zenity.ConfirmOverwrite(),
	)
	if errors.Is(err, zenity.ErrCanceled) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	if err := os.WriteFile(path, body, 0o600); err != nil {
		return false, err
	}

	return true, nil
}

// suggestedPath puts the console's file name in the Downloads folder,
// keeping only its base so a name cannot point elsewhere.
func suggestedPath(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == "/" || name == ".." {
		name = "gosint-file"
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return name
	}

	return filepath.Join(home, "Downloads", name)
}
