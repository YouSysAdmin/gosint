// Gosint, Copyright (c) 2026 YouSysAdmin

package transformtemplate

import (
	"strings"
	"testing"
)

// Documents split on a dash line however it is written, and a dash line
// inside a block scalar stays where it is.
func TestDocumentsSplitOnTheirMarker(t *testing.T) {
	data := "name: a\r\n---\r\nname: b\nbody: |\n  ---\n  kept\n--- # next\nname: c\n---"

	var names []string
	for _, d := range splitDocuments(data) {
		if d = strings.TrimSpace(d); d != "" {
			names = append(names, d[:strings.IndexAny(d+"\r\n", "\r\n")])
		}
	}

	if strings.Join(names, ",") != "name: a,name: b,name: c" {
		t.Errorf("documents = %q", names)
	}

	if docs := splitDocuments(data); !strings.Contains(docs[1], "  ---\n  kept") {
		t.Errorf("the block scalar was cut: %q", docs[1])
	}
}
