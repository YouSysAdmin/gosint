// Gosint, Copyright (c) 2026 YouSysAdmin

package username

import "testing"

// A name goes in as a query value after the question mark and as a path
// segment before it, so neither can add a parameter or a segment.
func TestAProfileURLEscapesByPosition(t *testing.T) {
	for _, c := range []struct{ tmpl, want string }{
		{"https://gitlab.com/api/v4/users?username={}", "https://gitlab.com/api/v4/users?username=a%26b%3Dc%2Fd"},
		{"https://github.com/{}", "https://github.com/a&b=c%2Fd"},
	} {
		if got := profileURL(c.tmpl, "a&b=c/d"); got != c.want {
			t.Errorf("%s = %q, want %q", c.tmpl, got, c.want)
		}
	}
}
