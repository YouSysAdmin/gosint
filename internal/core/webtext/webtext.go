// Gosint, Copyright (c) 2026 YouSysAdmin

// Package webtext reads what a web page says: its title, description
// and visible text, without scripts and styles. Shared by the website
// transforms and the assistant's page lookups.
package webtext

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

// Page is the readable part of a document.
type Page struct {
	Title       string
	Description string
	Text        string
}

// Parse reads body as HTML. A body that is not HTML still yields its
// text, as the parser is lenient.
func Parse(body []byte) Page {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return Page{}
	}

	var p Page
	var text strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.ElementNode:
			switch n.Data {
			case "script", "style", "noscript":
				return
			case "title":
				if n.FirstChild != nil && p.Title == "" {
					p.Title = strings.TrimSpace(n.FirstChild.Data)
				}

				return
			case "meta":
				if Attr(n, "name") == "description" {
					p.Description = strings.TrimSpace(Attr(n, "content"))
				}
			}
		case html.TextNode:
			if t := strings.TrimSpace(n.Data); t != "" {
				if text.Len() > 0 {
					text.WriteByte(' ')
				}

				text.WriteString(t)
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	walk(doc)
	p.Text = text.String()

	return p
}

// Attr reads an element's attribute, "" when absent.
func Attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, name) {
			return a.Val
		}
	}

	return ""
}
