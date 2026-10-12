// Gosint, Copyright (c) 2026 YouSysAdmin

//go:build cgo && !darwin

package desktop

import "unsafe"

// prepareWindow has nothing to do where the toolkit handles the editing
// keys and the file chooser itself.
func prepareWindow(unsafe.Pointer) {}
