// Gosint, Copyright (c) 2026 YouSysAdmin

package template

import (
	"encoding/json/v2"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// placeholderRe is {{ name }} or {{ secrets.NAME }}.
var placeholderRe = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_.]*)\s*\}\}`)

// escapeMode says how a placeholder's value is written into the text
// around it, so an input cannot change the request's shape.
type escapeMode int

const (
	escapeNone escapeMode = iota
	escapeURL
	escapeJSON
	escapeForm
	escapeXML
)

// render fills placeholders from values. A placeholder with no value is
// an error, not an empty string.
func render(tmpl string, values map[string]string, mode escapeMode) (string, error) {
	var missing []string
	out := placeholderRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		name := placeholderRe.FindStringSubmatch(m)[1]
		v, ok := values[name]
		if !ok {
			missing = append(missing, name)

			return m
		}

		switch mode {
		case escapeURL:
			return escapeComponent(v)
		case escapeJSON:
			return escapeJSONString(v)
		case escapeForm:
			return url.QueryEscape(v)
		case escapeXML:
			return html.EscapeString(v)
		default:
			return v
		}
	})

	if len(missing) > 0 {
		return "", fmt.Errorf("template: no value for %s", strings.Join(missing, ", "))
	}

	return out, nil
}

// escapeJSONString is v as the inside of a JSON string literal, quotes
// not included, since the template carries them.
func escapeJSONString(v string) string {
	b, _ := json.Marshal(v)

	return string(b[1 : len(b)-1])
}

// escapeComponent percent-encodes everything but the unreserved
// characters, as python's quote(value, safe="-_.~") does.
func escapeComponent(s string) string {
	var b strings.Builder
	for i := range len(s) {
		c := s[i]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || strings.IndexByte("-_.~", c) >= 0 {
			b.WriteByte(c)

			continue
		}

		fmt.Fprintf(&b, "%%%02X", c)
	}

	return b.String()
}

// lookup follows a dot path through decoded JSON. A number indexes an
// array. Nil when the path leads nowhere.
func lookup(data any, path string) any {
	if path == "" {
		return data
	}

	cur := data
	for part := range strings.SplitSeq(path, ".") {
		switch v := cur.(type) {
		case map[string]any:
			cur = v[part]
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(v) {
				return nil
			}

			cur = v[i]
		default:
			return nil
		}
	}

	return cur
}

// xmlNode is a parsed XML element, enough to follow a path of names.
type xmlNode struct {
	XMLName  xml.Name
	Text     string    `xml:",chardata"`
	Children []xmlNode `xml:",any"`
}

func parseXML(body []byte) (*xmlNode, error) {
	var n xmlNode
	if err := xml.Unmarshal(body, &n); err != nil {
		return nil, fmt.Errorf("template: the answer is not XML: %w", err)
	}

	return &n, nil
}

// find follows a dot path of element names from n: a direct child
// first, otherwise the first descendant of that name.
func (n *xmlNode) find(path string) (string, bool) {
	cur := n
	for part := range strings.SplitSeq(path, ".") {
		next := cur.child(part)
		if next == nil {
			next = cur.descendant(part)
		}

		if next == nil {
			return "", false
		}

		cur = next
	}

	return strings.TrimSpace(cur.Text), true
}

func (n *xmlNode) child(name string) *xmlNode {
	for i := range n.Children {
		if n.Children[i].XMLName.Local == name {
			return &n.Children[i]
		}
	}

	return nil
}

func (n *xmlNode) descendant(name string) *xmlNode {
	for i := range n.Children {
		if n.Children[i].XMLName.Local == name {
			return &n.Children[i]
		}

		if d := n.Children[i].descendant(name); d != nil {
			return d
		}
	}

	return nil
}

var errNotArray = errors.New("template: output.is_array is set and the answer at array_path is not an array")
