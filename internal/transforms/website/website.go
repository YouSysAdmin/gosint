// Gosint, Copyright (c) 2026 YouSysAdmin

// Package website holds the transforms that start from a website URL.
package website

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	wappalyzer "github.com/projectdiscovery/wappalyzergo"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/httpx"
	"github.com/yousysadmin/gosint/internal/transforms/internal/page"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/crtsh"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/subdomains"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/subfinder"
)

// C99URL is c99.nl's subdomain finder.
const C99URL = "https://api.c99.nl/subdomainfinder"

// The crawl's settings, at launch: how many pages and how long to wait
// between two requests. A big crawl without a pause is a load on
// somebody else's server, so the default waits a second as reconcrawl
// did.
var crawlParams = []transform.Param{
	{Name: "max_pages", Type: transform.ParamNumber, Default: "25",
		Description: "Most pages of the site to read, at most " + strconv.Itoa(page.MaxCrawl)},
	{Name: "delay_ms", Type: transform.ParamNumber, Default: "1000",
		Description: "Milliseconds to wait between two requests to the site, 0 for none, at most " +
			strconv.Itoa(int(page.MaxDelay.Milliseconds()))},
}

// crawlOptions reads the settings, an empty one taking its default.
func crawlOptions(x *transform.Exec) page.CrawlOptions {
	n, err := strconv.Atoi(x.Param("max_pages"))
	if err != nil {
		n = 25
	}

	ms, err := strconv.Atoi(x.Param("delay_ms"))
	if err != nil {
		ms = 1000
	}

	delay := min(max(time.Duration(ms)*time.Millisecond, 0), page.MaxDelay)

	return page.CrawlOptions{Limit: min(max(n, 1), page.MaxCrawl), Delay: delay}
}

// WebsiteToDomain links a website to the domain it is served from.
type WebsiteToDomain struct{}

// Meta describes the transform.
func (WebsiteToDomain) Meta() transform.Meta {
	return transform.Meta{Name: "website_to_domain", Category: "Website", Input: "website", Output: "domain",
		Description: "Extract the domain of a website, without www."}
}

// Run enriches each website.
func (WebsiteToDomain) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		u, err := url.Parse(props.Str(in.Properties, "url"))
		if err != nil || u.Hostname() == "" {
			x.Warn("%s: no host in the URL", in.Label)

			continue
		}

		host := strings.TrimPrefix(u.Hostname(), "www.")
		if _, err := x.NodeAndLink(ctx, in, "HAS_DOMAIN", "domain", map[string]any{"domain": host}); err != nil {
			x.Warn("%s: %v", in.Label, err)
		}
	}

	return nil
}

// WebsiteToText keeps the visible text of a page.
type WebsiteToText struct{}

// Meta describes the transform.
func (WebsiteToText) Meta() transform.Meta {
	return transform.Meta{Name: "website_to_text", Category: "Website", Input: "website", Output: "phrase",
		Description: "Extract the visible text of a website."}
}

// Run enriches each website.
func (WebsiteToText) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		got, err := httpx.Fetch(ctx, x, props.Str(in.Properties, "url"))
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		text := page.Parse(got.Body).Text
		if text == "" {
			continue
		}

		if _, err := x.NodeAndLink(ctx, in, "HAS_INNER_TEXT", "phrase", map[string]any{"text": text}); err != nil {
			x.Warn("%s: %v", in.Label, err)
		}
	}

	return nil
}

// fingerprints is the wappalyzer database, loaded on first use.
var fingerprints = sync.OnceValues(wappalyzer.New)

// TechDetect finds the technologies a website runs, by the wappalyzer
// fingerprints as a library.
type TechDetect struct{}

// Meta describes the transform.
func (TechDetect) Meta() transform.Meta {
	return transform.Meta{Name: "tech_detect", Category: "Website", Input: "website", Output: "technology",
		Description: "Detect the technologies a website runs, from its headers and HTML."}
}

// Run enriches each website.
func (TechDetect) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	w, err := fingerprints()
	if err != nil {
		return fmt.Errorf("load fingerprints: %w", err)
	}

	for _, in := range inputs {
		got, err := httpx.Fetch(ctx, x, props.Str(in.Properties, "url"))
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		found := w.FingerprintWithInfo(got.Header, got.Body)
		names := make([]string, 0, len(found))
		for app := range found {
			names = append(names, app)
		}

		slices.Sort(names)
		for _, app := range names {
			name, version, _ := strings.Cut(app, ":")
			tech := map[string]any{"name": name, "source": "wappalyzer"}
			if version != "" {
				tech["version"] = version
			}

			if cats := found[app].Categories; len(cats) > 0 {
				tech["category"] = strings.Join(cats, ", ")
			}

			if _, err := x.NodeAndLink(ctx, in, "USES_TECHNOLOGY", "technology", tech); err != nil {
				x.Warn("%s: %s: %v", in.Label, name, err)
			}
		}
	}

	return nil
}

// WebsiteToLinks links a website to the pages and sites it links to.
type WebsiteToLinks struct{}

// Meta describes the transform.
func (WebsiteToLinks) Meta() transform.Meta {
	return transform.Meta{Name: "website_to_links", Category: "Website", Input: "website", Output: "website",
		Description: "Crawl a website and link it to the sites its pages link to.",
		Params:      crawlParams}
}

// Run enriches each website. Links to other hosts are kept, a site's
// own pages are the crawl and not findings.
func (WebsiteToLinks) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		start := props.Str(in.Properties, "url")
		pages, err := page.Crawl(ctx, x, start, crawlOptions(x))
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		host := page.HostOf(start)
		external := map[string]bool{}
		for _, p := range pages {
			for _, l := range p.Links {
				if h := page.HostOf(l); h != "" && !strings.EqualFold(h, host) {
					external[page.SiteOf(l)] = true
				}
			}
		}

		for _, site := range slices.Sorted(maps.Keys(external)) {
			if _, err := x.NodeAndLink(ctx, in, "LINKS_TO", "website", map[string]any{"url": site}); err != nil {
				x.Warn("%s: %v", site, err)
			}
		}

		x.Info("%s: %d pages read, %d sites linked", in.Label, len(pages), len(external))
	}

	return nil
}

var (
	emailRe = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)

	// phoneRe is an international number as written on a page: a plus,
	// a country code and at least seven more digits.
	phoneRe = regexp.MustCompile(`\+[1-9][0-9 ().-]{7,18}[0-9]`)

	// phoneSeparators strips how a number is grouped on a page.
	phoneSeparators = strings.NewReplacer(" ", "", "(", "", ")", "", "-", "", ".", "")
)

// WebsiteToCrawler collects the email addresses and phone numbers a
// website publishes.
type WebsiteToCrawler struct{}

// Meta describes the transform.
func (WebsiteToCrawler) Meta() transform.Meta {
	return transform.Meta{Name: "website_to_crawler", Category: "Website", Input: "website", Output: "email",
		Description: "Crawl a website for the email addresses and international phone numbers it publishes.",
		Params:      crawlParams}
}

// Run enriches each website.
func (WebsiteToCrawler) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		pages, err := page.Crawl(ctx, x, props.Str(in.Properties, "url"), crawlOptions(x))
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		emails, phones := map[string]bool{}, map[string]bool{}
		for _, p := range pages {
			text := string(p.Body)
			for _, e := range emailRe.FindAllString(text, -1) {
				if !strings.HasSuffix(strings.ToLower(e), ".png") && !strings.HasSuffix(strings.ToLower(e), ".jpg") {
					emails[strings.ToLower(e)] = true
				}
			}

			for _, ph := range phoneRe.FindAllString(page.Parse(p.Body).Text, -1) {
				phones[phoneSeparators.Replace(ph)] = true
			}
		}

		for e := range emails {
			if _, err := x.NodeAndLink(ctx, in, "HAS_EMAIL", "email", map[string]any{"email": e}); err != nil {
				x.Warn("%s: %v", e, err)
			}
		}

		for ph := range phones {
			if _, err := x.NodeAndLink(ctx, in, "HAS_PHONE", "phone", map[string]any{"number": ph}); err != nil {
				x.Warn("%s: %v", ph, err)
			}
		}
	}

	return nil
}

// trackerPatterns are the analytics and advertising IDs a page embeds.
var trackerPatterns = []struct {
	name, company string
	re            *regexp.Regexp
}{
	{"Google Analytics", "Google", regexp.MustCompile(`\bUA-\d{4,10}-\d{1,4}\b`)},
	{"Google Analytics 4", "Google", regexp.MustCompile(`\bG-[A-Z0-9]{8,12}\b`)},
	{"Google Tag Manager", "Google", regexp.MustCompile(`\bGTM-[A-Z0-9]{4,9}\b`)},
	{"Google Ads", "Google", regexp.MustCompile(`\bAW-\d{6,12}\b`)},
	{"Google AdSense", "Google", regexp.MustCompile(`\bca-pub-\d{10,20}\b`)},
	{"Meta Pixel", "Meta", regexp.MustCompile(`fbq\(\s*['"]init['"]\s*,\s*['"](\d{10,20})['"]`)},
	{"Hotjar", "Hotjar", regexp.MustCompile(`hjid\s*:\s*(\d{5,10})`)},
	{"Yandex Metrica", "Yandex", regexp.MustCompile(`ym\(\s*(\d{6,10})\s*,\s*['"]init['"]`)},
}

// WebsiteToWebTrackers finds the analytics and advertising IDs of a
// website, which tie sites run by the same owner together.
type WebsiteToWebTrackers struct{}

// Meta describes the transform.
func (WebsiteToWebTrackers) Meta() transform.Meta {
	return transform.Meta{Name: "website_to_webtrackers", Category: "Website", Input: "website", Output: "webtracker",
		Description: "Find the analytics and advertising IDs a website embeds (Google, Meta, Hotjar, Yandex)."}
}

// Run enriches each website.
func (WebsiteToWebTrackers) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		got, err := httpx.Fetch(ctx, x, props.Str(in.Properties, "url"))
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		seen := map[string]bool{}
		body := string(got.Body)
		for _, p := range trackerPatterns {
			for _, m := range p.re.FindAllStringSubmatch(body, -1) {
				id := m[len(m)-1]
				if seen[id] {
					continue
				}

				seen[id] = true
				tracker := map[string]any{"tracker_id": id, "name": p.name, "company": p.company, "source": "html"}
				if _, err := x.NodeAndLink(ctx, in, "HAS_TRACKER", "webtracker", tracker); err != nil {
					x.Warn("%s: %v", id, err)
				}
			}
		}
	}

	return nil
}

// WebsiteToSubdomains lists a website's subdomains as websites. By
// default from the same free sources as domain_to_subdomains (subfinder,
// then crt.sh). The c99 source asks c99.nl instead, which
// needs a paid key and adds each subdomain's address and whether it
// sits behind Cloudflare.
type WebsiteToSubdomains struct {
	// BaseURL is c99.nl's subdomain finder.
	BaseURL string

	// Finder is the free lookup.
	Finder subdomains.Finder
}

// Meta describes the transform.
func (WebsiteToSubdomains) Meta() transform.Meta {
	return transform.Meta{Name: "website_to_subdomains", Category: "Website", Input: "website", Output: "website",
		Description: "Find the subdomains of a website, from subfinder and crt.sh, or from c99.nl with its address and Cloudflare flag.",
		Params: []transform.Param{
			{Name: "source", Type: transform.ParamSelect, Default: "free", Description: "Where the subdomains come from",
				Options: []transform.Option{{Label: "Free (subfinder, then crt.sh)", Value: "free"},
					{Label: "C99.nl (paid key, adds the address and Cloudflare)", Value: "c99"}}},
			{Name: "C99_API_KEY", Type: transform.ParamSecret,
				Description: "Your C99.nl API key, for the c99 source only."},
		}}
}

// Run enriches each website.
func (e WebsiteToSubdomains) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	useC99 := x.Param("source") == "c99"
	if useC99 && x.Param("C99_API_KEY") == "" {
		return errors.New("the c99 source needs the secret C99_API_KEY")
	}

	for _, in := range inputs {
		u, err := url.Parse(props.Str(in.Properties, "url"))
		if err != nil || u.Hostname() == "" {
			continue
		}

		host := strings.TrimPrefix(u.Hostname(), "www.")
		var found []subdomainSite
		if useC99 {
			found = e.fromC99(ctx, x, in.Label, host)
		} else {
			subs, source := e.Finder.Find(ctx, x, host)
			for _, s := range subs {
				found = append(found, subdomainSite{Name: s})
			}

			x.Info("%s: %d subdomains found with %s", host, len(subs), source)
		}

		for _, s := range found {
			if s.Name == host || s.Name == "www."+host {
				continue
			}

			site := map[string]any{"url": u.Scheme + "://" + s.Name}
			if s.IP != "" {
				site["description"] = "IP address: " + s.IP
			}

			if s.Cloudflare {
				site["technologies"] = []any{"Cloudflare"}
			}

			if _, err := x.NodeAndLink(ctx, in, "IS_SUBDOMAIN", "website", site); err != nil {
				x.Warn("%s: %v", s.Name, err)
			}
		}
	}

	return nil
}

// subdomainSite is one subdomain found, with what c99 adds.
type subdomainSite struct {
	Name       string
	IP         string
	Cloudflare bool
}

// fromC99 asks c99.nl.
func (e WebsiteToSubdomains) fromC99(ctx context.Context, x *transform.Exec, label, host string) []subdomainSite {
	var answer struct {
		Success    bool `json:"success"`
		Subdomains []struct {
			Subdomain  string `json:"subdomain"`
			IP         string `json:"ip"`
			Cloudflare bool   `json:"cloudflare"`
		} `json:"subdomains"`
	}

	q := url.Values{"key": {x.Param("C99_API_KEY")}, "domain": {host}, "json": {""}}
	if _, err := httpx.GetJSON(ctx, x, e.BaseURL+"?"+q.Encode(), nil, &answer); err != nil {
		x.Warn("%s: %v", label, err)

		return nil
	}

	out := make([]subdomainSite, 0, len(answer.Subdomains))
	for _, s := range answer.Subdomains {
		if s.Subdomain != "" {
			out = append(out, subdomainSite{Name: strings.ToLower(s.Subdomain), IP: s.IP, Cloudflare: s.Cloudflare})
		}
	}

	return out
}

// All is the package's transforms, pointed at their public sources.
func All() []transform.Transform {
	return []transform.Transform{
		WebsiteToDomain{},
		WebsiteToText{},
		TechDetect{},
		WebsiteToLinks{},
		WebsiteToCrawler{},
		WebsiteToWebTrackers{},
		WebsiteToSubdomains{BaseURL: C99URL, Finder: subdomains.Finder{CrtURL: crtsh.URL, Passive: subfinder.Enumerate}},
	}
}
