// Gosint, Copyright (c) 2026 YouSysAdmin

package blob_test

import (
	"testing"

	"github.com/yousysadmin/gosint/internal/blob"
)

func TestValidName(t *testing.T) {
	for _, ok := range []string{"notes.txt", "report 2026.pdf", "console.log", "com10.txt", "lpt0", "auxiliary.md"} {
		if err := blob.ValidName(ok); err != nil {
			t.Errorf("%q refused: %v", ok, err)
		}
	}

	for _, bad := range []string{"", ".env", "..", "a/b", `a\b`, "a:b", "tab\tname", " lead", "trail ",
		"trail.", "CON", "con.txt", "Nul", "com1.log", "LPT9.tar.gz", "aux .txt"} {
		if err := blob.ValidName(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
