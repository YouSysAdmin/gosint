// Gosint, Copyright (c) 2026 YouSysAdmin

package webtext_test

import (
	"strings"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/webtext"
)

func TestParseReadsTheReadablePart(t *testing.T) {
	p := webtext.Parse([]byte(`<html><head><title> Hi </title><meta name="description" content="About"></head>
		<body><script>x()</script><p>One</p><div>Two</div></body></html>`))
	if p.Title != "Hi" || p.Description != "About" || p.Text != "One Two" {
		t.Errorf("page = %+v", p)
	}
}

// A page nested deeper than the HTML parser's limit (512 open
// elements) is refused by the parser, which bounds the walk's depth.
func TestADeeplyNestedPageReadsAsNothing(t *testing.T) {
	body := "<p>top</p>" + strings.Repeat("<div>", 20000) + "deep" + strings.Repeat("</div>", 20000)

	if p := webtext.Parse([]byte(body)); p != (webtext.Page{}) {
		t.Errorf("page = %+v", p)
	}
}
