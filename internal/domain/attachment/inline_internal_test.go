// Gosint, Copyright (c) 2026 YouSysAdmin

package attachment

import "testing"

// Only media, PDF and plain text render on this origin: an xml flavour
// a sender declares, or any type a browser might run as a document,
// downloads.
func TestOnlyMediaRendersInline(t *testing.T) {
	for ct, want := range map[string]bool{
		"image/png": true, "IMAGE/JPEG; q=1": true, "video/mp4": true, "application/pdf": true, "text/plain; charset=utf-8": true,
		"image/svg+xml": false, "image/x+xml": false, "video/x+xml": false, "text/html": false, "application/xhtml+xml": false,
		"text/xml": false, "image/": false, "": false,
	} {
		if got := inlineSafe(ct); got != want {
			t.Errorf("inlineSafe(%q) = %v, want %v", ct, got, want)
		}
	}
}
