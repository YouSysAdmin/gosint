// Gosint, Copyright (c) 2026 YouSysAdmin

// Package page reads HTML pages: their title, text and links, and crawls
// a site through them.
package page

import (
	"bytes"
	"context"
	"net/url"
	"slices"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/core/webtext"
	"github.com/yousysadmin/gosint/internal/transforms/internal/httpx"
)

// Page is what a fetched HTML document says about itself.
type Page struct {
	Title, Description, Text string
}

// MaxText caps the text kept from a page.
const MaxText = 5000

// Parse reads the title, meta description and visible text of an HTML
// document. Script and style contents are left out.
func Parse(body []byte) Page {
	w := webtext.Parse(body)
	p := Page{Title: w.Title, Description: w.Description, Text: w.Text}
	if len(p.Text) > MaxText {
		p.Text = p.Text[:MaxText] + "..."
	}

	return p
}

// Attr reads an element's attribute, "" when absent.
func Attr(n *html.Node, name string) string {
	return webtext.Attr(n, name)
}

// Links lists the absolute http(s) links of a page, without fragments.
func Links(base *url.URL, body []byte) []string {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil || base == nil {
		return nil
	}

	var out []string
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			if u, err := base.Parse(Attr(n, "href")); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
				u.Fragment = ""
				out = append(out, u.String())
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	walk(doc)
	slices.Sort(out)

	return slices.Compact(out)
}

// The bounds of one crawl: at most MaxCrawl pages, and no more than
// MaxDelay between two requests.
const (
	MaxCrawl = 2000
	MaxDelay = time.Minute

	// crawlProgress is how often a long crawl says how far it got.
	crawlProgress = 50
)

// CrawlOptions are how far and how politely a crawl goes: Limit pages
// at most, Delay between two requests to the site.
type CrawlOptions struct {
	Limit int
	Delay time.Duration
}

// Crawled is one page read by a crawl.
type Crawled struct {
	URL   string
	Body  []byte
	Links []string
}

// Crawl reads up to opt.Limit pages of start's host, breadth first,
// waiting opt.Delay between requests, through the client refusing
// internal addresses. The replacement for reconurge's reconcrawl.
func Crawl(ctx context.Context, x *transform.Exec, start string, opt CrawlOptions) ([]Crawled, error) {
	root, err := url.Parse(start)
	if err != nil {
		return nil, err
	}

	limit := opt.Limit
	queue, seen := []string{root.String()}, map[string]bool{root.String(): true}

	var pages []Crawled
	for fetched := 0; len(queue) > 0 && len(pages) < limit; fetched++ {
		if fetched > 0 && opt.Delay > 0 {
			select {
			case <-ctx.Done():
				return pages, ctx.Err()
			case <-time.After(opt.Delay):
			}
		}

		if err := ctx.Err(); err != nil {
			return pages, err
		}

		if fetched > 0 && fetched%crawlProgress == 0 {
			x.Info("%s: %d pages read, %d queued", root.Hostname(), len(pages), len(queue))
		}

		next := queue[0]
		queue = queue[1:]

		got, err := httpx.Fetch(ctx, x, next)
		if err != nil || got.Status >= 400 || !strings.Contains(got.Header.Get("Content-Type"), "html") {
			if len(pages) == 0 && err != nil {
				return nil, err
			}

			continue
		}

		base, _ := url.Parse(got.ReachedURL)
		p := Crawled{URL: next, Body: got.Body, Links: Links(base, got.Body)}
		pages = append(pages, p)

		// No more is queued than can still be fetched, a page can carry
		// far more links than the crawl will ever read.
		for _, l := range p.Links {
			if len(pages)+len(queue) >= limit {
				break
			}

			u, err := url.Parse(l)
			if err != nil || !strings.EqualFold(u.Hostname(), root.Hostname()) || seen[l] {
				continue
			}

			seen[l] = true
			queue = append(queue, l)
		}
	}

	return pages, nil
}

// HostOf is the host of a URL, "" when it does not parse.
func HostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}

	return u.Hostname()
}

// SiteOf reduces a link to its site's root URL.
func SiteOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	return u.Scheme + "://" + u.Host
}
