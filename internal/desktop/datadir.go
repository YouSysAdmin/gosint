// Gosint, Copyright (c) 2026 YouSysAdmin

// Package desktop runs a node on the loopback behind a native window,
// the console as an app on a workstation.
package desktop

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
)

// DataDir is where the desktop app keeps its database, files and
// settings: Application Support on macOS, the local application data on
// Windows (it must not roam), the XDG data home elsewhere.
func DataDir() (string, error) {
	if runtime.GOOS == "windows" {
		if dir := os.Getenv("LOCALAPPDATA"); filepath.IsAbs(dir) {
			return filepath.Join(dir, "gosint"), nil
		}

		dir, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}

		return filepath.Join(dir, "gosint"), nil
	}

	if runtime.GOOS == "darwin" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}

		return filepath.Join(home, "Library", "Application Support", "gosint"), nil
	}

	if dir := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(dir) {
		return filepath.Join(dir, "gosint"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, ".local", "share", "gosint"), nil
}

// configStub is written once so the settings file has a known place.
const configStub = `# gosint desktop settings, the same keys as a server's gosint.yaml.
# The database, files and logs live in this directory by default.
`

// ConfigFile is the desktop's gosint.yaml, created empty on first use.
func ConfigFile(dir string) (string, error) {
	path := filepath.Join(dir, "gosint.yaml")

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // a fixed name in the data directory
	if errors.Is(err, fs.ErrExist) {
		return path, nil
	}

	if err != nil {
		return "", err
	}

	if _, err := f.WriteString(configStub); err != nil {
		_ = f.Close()

		return "", err
	}

	return path, f.Close()
}

// state is what the app remembers between launches.
type state struct {
	Port int `json:"port"`
}

// Listen binds the loopback port of the last launch, so the console
// keeps its origin and with it what it stored in the browser. A port
// taken by now gives way to a free one, remembered for next time.
func Listen(dir string) (net.Listener, error) {
	path := filepath.Join(dir, "desktop.json")

	var st state
	if b, err := os.ReadFile(path); err == nil { //nolint:gosec // a fixed name in the data directory
		_ = json.Unmarshal(b, &st)
	}

	if st.Port > 0 && st.Port < 65536 {
		if ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(st.Port))); err == nil {
			return ln, nil
		}
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen on loopback: %w", err)
	}

	st.Port = ln.Addr().(*net.TCPAddr).Port
	if b, err := json.Marshal(st); err == nil {
		_ = os.WriteFile(path, b, 0o600)
	}

	return ln, nil
}
