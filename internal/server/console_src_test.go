// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every HTML body the console renders goes through HtmlPreview, whose
// sandboxed frame and image blocking are the whole protection. A view
// writing srcdoc by hand would render a stranger's markup with none of
// it, so none may.
func TestEveryHTMLPreviewGoesThroughTheComponent(t *testing.T) {
	root := filepath.Join("..", "..", "web", "src")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".vue") && !strings.HasSuffix(path, ".ts") {
			return err
		}

		if filepath.Base(path) == "HtmlPreview.vue" {
			return nil
		}

		src, err := os.ReadFile(path) //nolint:gosec // a source file of this repository
		if err != nil {
			return err
		}

		if strings.Contains(string(src), "srcdoc") {
			t.Errorf("%s writes srcdoc, render through HtmlPreview instead", path)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
