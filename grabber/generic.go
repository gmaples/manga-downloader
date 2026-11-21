// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/browser"
	"github.com/elboletaire/manga-downloader/http"
	"github.com/elboletaire/manga-downloader/notice"
)

// Generic is the last resort for a site no grabber knows: it reads the series
// page with no site-specific code, guessing the chapter list from the page's
// links and a chapter's pages from its images.
//
// It reads the plain page first. When that has no chapter list (a JavaScript
// app renders it), it renders the page in a headless browser, scrolls it so
// lazy lists and images mount, and reads the result, along with the JSON the
// page's own scripts fetched (some readers draw only the images in view, and
// the full list sits in that JSON). It never opens a browser window by
// itself: a site behind a bot check, or one known to need a window, is
// reported as such instead.
//
// What it tried and found is kept in report, which IdentifySite returns in a
// NotRecognisedError when the guess fails, so the user sees why.
type Generic struct {
	*Grabber
	doc      *goquery.Document
	chapters []genericLink
	report   []string
}

// NewGeneric returns the generic reader
func NewGeneric(g *Grabber) *Generic {
	return &Generic{Grabber: g}
}

// GenericChapter is a chapter the generic reader found, by its link
type GenericChapter struct {
	Chapter
	URL string
}

// genericLink is a link of the guessed chapter list
type genericLink struct {
	URL    string
	Number float64
	// NumberText is the number as the link or URL writes it ("05", "62.5")
	NumberText string
	Title      string
}

// genericBrowser says whether the generic reader may render pages in a
// headless browser; off in the tests, which can't start one
var genericBrowser = true

// genericCapture renders a page for the generic reader, running prepare
// (see browser.Capture) before it's read. A var so tests can stand in for the
// browser.
var genericCapture = func(pageURL, prepare string) (browser.Snapshot, error) {
	return browser.Capture(pageURL, prepare, 45*time.Second)
}

// genericMinChapters and genericMinPages are the fewest links and images that
// count as a chapter list and a chapter's pages. One link or one image is
// too likely to be something else (a "start reading" button, a cover).
const (
	genericMinChapters = 2
	genericMinPages    = 2
)

// genericListWordRe is the name of a chapter list, as a tab, a button or a
// link to its own page says it, a count after it allowed ("Розділи 10")
var genericListWordRe = regexp.MustCompile(`(?i)^\s*(?:all\s+)?(?:chapters|chapter\s+list|episodes|главы|глави|список\s+глав|оглавление|розділи|список\s+розділів|capítulos|capitulos|chapitres|bölümler|rozdziały)\s*[(\[]?\s*\d*\s*[)\]]?\s*$`)

// genericListPathRe is the last segment of a chapter list's own page
var genericListPathRe = regexp.MustCompile(`(?i)^(?:chapters?|chapter-list|chapterlist|episodes|glavy|rozdily|capitulos|chapitres)$`)

// genericRevealJS clicks the tab or button that shows a series' chapters
// (the smallest element whose own text names the list, see
// genericListWordRe), for pages that draw the list only once it's opened.
// It returns whether it clicked.
var genericRevealJS = `(function(){
	var re = new RegExp(` + strconv.Quote(genericListWordRe.String()[4:]) + `, "i");
	var els = document.querySelectorAll("button, a, [role=tab], li, span, div, label, h2, h3");
	var best = null;
	for (var i = 0; i < els.length; i++) {
		var el = els[i];
		if (el.closest("header, nav, footer")) continue;
		var text = (el.textContent || "").trim();
		if (text.length > 40 || !re.test(text)) continue;
		if (!best || best.contains(el)) best = el;
	}
	if (!best) return false;
	best.scrollIntoView({block: "center"});
	best.click();
	return true;
})()`

// Test reads the page, plain and then rendered, and returns true when it
// finds a chapter list. A list on its own page ("/chapters"), or behind a
// tab, is followed. It never fails: what it found goes into the report.
func (g *Generic) Test() (bool, error) {
	u, err := url.Parse(g.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false, nil
	}

	body, err := g.identifyPage(g.URL)
	plainReached := false
	listPage := ""
	if err != nil {
		g.note("the plain-page reader couldn't read the page: %s", genericFetchError(err))
		// a server answered, unless the page isn't there: a JavaScript app
		// answers 200 with an empty page, a bot check 403, 429 or 503
		var se *http.StatusError
		plainReached = errors.As(err, &se) && se.Code != 404 && se.Code != 410
	} else {
		plainReached = true
		doc, perr := goquery.NewDocumentFromReader(strings.NewReader(body))
		if perr == nil {
			if kind := genericBlock(doc); kind != "" {
				g.note("the plain-page reader got %s instead of the page", kind)
			} else if g.doc = doc; g.take(doc, "the plain page") {
				return true, nil
			} else {
				g.note("the plain-page reader found no chapter list in the page")
				listPage = genericListPage(doc, g.URL)
			}
		}
	}
	if listPage != "" {
		if lbody, lerr := g.identifyPage(listPage); lerr != nil {
			g.note("the plain-page reader couldn't read the chapters page %s: %s", listPage, genericFetchError(lerr))
		} else if ldoc, perr := goquery.NewDocumentFromReader(strings.NewReader(lbody)); perr == nil {
			if g.take(ldoc, "the plain chapters page") {
				return true, nil
			}
			g.note("the plain-page reader found no chapter list in the chapters page %s", listPage)
		}
	}

	switch {
	case !genericBrowser:
		return false, nil
	case !plainReached:
		// no server answered, or the page isn't there: a browser won't do better
		return false, nil
	case browser.NeedsVisible(g.URL):
		g.note("the hidden-browser reader skipped it: this site needs a visible browser window, which the generic reader doesn't open")
		return false, nil
	}

	doc, ok := g.render(g.URL)
	if ok || doc == nil {
		return ok, nil
	}
	if next := genericListPage(doc, g.URL); next != "" && next != listPage {
		_, ok = g.render(next)
	}
	return ok, nil
}

// render reads pageURL in the hidden browser, opening its chapter list's tab
// if it has one, and takes the chapters it finds. It returns the rendered
// page when it was read, nil when the browser failed or was turned away.
func (g *Generic) render(pageURL string) (*goquery.Document, bool) {
	snap, err := genericCapture(pageURL, genericRevealJS)
	if err != nil {
		g.note("the hidden-browser reader couldn't render %s: %s", pageURL, err)
		return nil, false
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(snap.HTML))
	if err != nil {
		g.note("the hidden-browser reader couldn't parse %s: %s", pageURL, err)
		return nil, false
	}
	if kind := genericBlock(doc); kind != "" {
		g.note("the hidden-browser reader got %s (HTTP %d) instead of %s", kind, snap.Status, pageURL)
		return nil, false
	}
	if snap.Status >= 400 {
		g.note("the hidden-browser reader got HTTP %d for %s: %s", snap.Status, pageURL, genericPageText(doc))
		return nil, false
	}
	if g.doc == nil || genericTitle(g.doc) == "" {
		g.doc = doc
	}
	if g.take(doc, "a hidden browser") {
		return doc, true
	}
	links := genericChapterLinks(doc, g.URL)
	g.note("the hidden-browser reader found %d chapter link(s) in the rendered page %s: %s", len(links), pageURL, genericPageText(doc))
	return doc, false
}

// take keeps doc's chapter list when it has one, read against the series URL
func (g *Generic) take(doc *goquery.Document, how string) bool {
	links := genericChapterLinks(doc, g.URL)
	if len(links) < genericMinChapters {
		return false
	}
	// the series page names the series best: a list on its own page or tab
	// is read for its links only
	if g.doc == nil || genericTitle(g.doc) == "" {
		g.doc = doc
	}
	g.chapters = links
	g.announce(how)
	return true
}

// genericListPage returns the address of the series' chapter list when the
// page links to it on its own page (".../chapters", or a link named
// "Chapters"), "" when it doesn't
func genericListPage(doc *goquery.Document, pageURL string) string {
	base, err := url.Parse(pageURL)
	if err != nil {
		return ""
	}
	host := normalHost(base.Host)
	seriesPath := strings.TrimSuffix(base.Path, "/")
	found := ""
	doc.Find("a[href]").EachWithBreak(func(_ int, a *goquery.Selection) bool {
		if a.Closest(genericSkip).Length() > 0 {
			return true
		}
		ref, err := url.Parse(strings.TrimSpace(a.AttrOr("href", "")))
		if err != nil {
			return true
		}
		u := base.ResolveReference(ref)
		u.Fragment = ""
		if normalHost(u.Host) != host || strings.TrimSuffix(u.Path, "/") == seriesPath {
			return true
		}
		path := strings.TrimSuffix(u.Path, "/")
		last := path[strings.LastIndex(path, "/")+1:]
		underSeries := seriesPath != "" && strings.HasPrefix(path, seriesPath+"/")
		if (underSeries && genericListPathRe.MatchString(last)) || genericListWordRe.MatchString(a.Text()) {
			found = u.String()
			return false
		}
		return true
	})
	return found
}

// note adds a line to the report of what was tried
func (g *Generic) note(format string, a ...any) {
	g.report = append(g.report, fmt.Sprintf(format, a...))
}

// announce tells the user the series is being read with guesses
func (g *Generic) announce(how string) {
	notice.Warn("%s has no built-in support: the generic reader found %d chapters in %s. "+
		"Its chapter numbers and pages are guesses, so check them.", hostOf(g.URL), len(g.chapters), how)
}

// Report is what the generic reader tried and found, one line per step
func (g *Generic) Report() []string {
	return g.report
}

// FetchTitle returns the series title from the page it was found on
func (g *Generic) FetchTitle() (string, error) {
	if g.doc == nil {
		return "", fmt.Errorf("no page read for %s", g.URL)
	}
	if t := genericTitle(g.doc); t != "" {
		return t, nil
	}
	return "", fmt.Errorf("no title found on %s", g.URL)
}

// FetchChapters returns the chapter list Test found
func (g *Generic) FetchChapters() (Filterables, []error) {
	lang := ""
	if g.doc != nil {
		lang = pageLanguage(g.doc)
	}
	chapters := make(Filterables, 0, len(g.chapters))
	for _, l := range g.chapters {
		u, _ := url.Parse(l.URL)
		id := u.Path
		if u.RawQuery != "" {
			id += "?" + u.RawQuery
		}
		chapters = append(chapters, &GenericChapter{
			Chapter: Chapter{
				Number:   l.Number,
				Title:    l.Title,
				Language: lang,
				ID:       id,
				Label:    Label{Style: LabelNumbered, Number: l.NumberText},
			},
			URL: l.URL,
		})
	}
	return chapters, nil
}

// FetchChapter reads a chapter's pages: from the plain page when its images
// are there, else from the page rendered in a headless browser
func (g *Generic) FetchChapter(f Filterable) (*Chapter, error) {
	gc := f.(*GenericChapter)

	var pages []string
	body, err := http.GetText(http.RequestParams{URL: gc.URL, Referer: g.BaseUrl()})
	if err == nil {
		if doc, perr := goquery.NewDocumentFromReader(strings.NewReader(body)); perr == nil && genericBlock(doc) == "" {
			pages = genericPageImages(doc, body, gc.URL, nil)
		}
	}
	if len(pages) < genericMinPages && genericBrowser && !browser.NeedsVisible(gc.URL) {
		snap, cerr := genericCapture(gc.URL, "")
		if cerr != nil {
			return nil, fmt.Errorf("the generic reader couldn't render %s: %w", gc.URL, cerr)
		}
		doc, perr := goquery.NewDocumentFromReader(strings.NewReader(snap.HTML))
		if perr != nil {
			return nil, perr
		}
		if kind := genericBlock(doc); kind != "" {
			return nil, fmt.Errorf("the generic reader got %s instead of %s", kind, gc.URL)
		}
		pages = genericPageImages(doc, snap.HTML, gc.URL, snap.JSON)
	}
	if len(pages) < genericMinPages {
		return nil, fmt.Errorf("the generic reader found no page images on %s", gc.URL)
	}

	chapter := &Chapter{
		Title:      gc.Title,
		Number:     gc.Number,
		Language:   gc.Language,
		ID:         gc.ID,
		Label:      gc.Label,
		PagesCount: int64(len(pages)),
	}
	for i, p := range pages {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: p})
	}
	return chapter, nil
}

// genericFetchError is a fetch error short enough for the report
func genericFetchError(err error) string {
	var se *http.StatusError
	if errors.As(err, &se) {
		return fmt.Sprintf("HTTP %d", se.Code)
	}
	return err.Error()
}

// genericBlockMarkers are texts of the bot checks and access walls a page can
// be swapped for, matched in its title and visible text, with the name the
// report gives them
var genericBlockMarkers = []struct{ marker, kind string }{
	{"just a moment", "a Cloudflare bot check"},
	{"attention required! | cloudflare", "a Cloudflare block page"},
	{"cf-chl-", "a Cloudflare bot check"},
	{"ddos-guard", "a DDoS-Guard bot check"},
	{"vercel security checkpoint", "a Vercel bot check"},
	{"checking your browser", "a bot check"},
	{"verify you are human", "a bot check"},
	{"captcha", "a captcha"},
}

// genericBlock names the bot check or block page doc is, "" when it's none.
// Only short pages count: a real page may mention a captcha in passing.
func genericBlock(doc *goquery.Document) string {
	text := strings.ToLower(doc.Find("title").First().Text() + " " + doc.Find("body").Text())
	if len(text) > 20000 {
		return ""
	}
	for _, m := range genericBlockMarkers {
		if strings.Contains(text, m.marker) {
			return m.kind
		}
	}
	return ""
}

// genericPageText is the start of a page's visible text, quoted for the
// report, so an age gate or a "not available in your country" page explains
// itself
func genericPageText(doc *goquery.Document) string {
	main := doc.Find("main").First()
	if main.Length() == 0 {
		main = doc.Find("body").First()
	}
	main = main.Clone()
	main.Find("script, style, noscript, template").Remove()
	text := strings.Join(strings.Fields(main.Text()), " ")
	if text == "" {
		return "the page is empty"
	}
	r := []rune(text)
	if len(r) > 160 {
		text = string(r[:160]) + "…"
	}
	return "the page says " + strconv.Quote(text)
}

// genericSkip are the parts of a page whose links and images aren't content
const genericSkip = "header, nav, footer, aside"

// genericDigitsRe finds the numbers of a URL; a decimal point stays with its
// number ("c62.5")
var genericDigitsRe = regexp.MustCompile(`\d+(?:\.\d+)?`)

// genericChapterWordRe is a URL that names a chapter or a reader
var genericChapterWordRe = regexp.MustCompile(`(?i)chapter|chap|/ch[-_/.\d]|/c\d|glava|capitulo|cap[-_]|episode|/ep[-_\d]|/read|reader|viewer|/vol|/v\d|/tom`)

// genericIDRe is an id in a URL: a UUID, or a long run of hex digits
var genericIDRe = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|\b[0-9a-f]{16,}\b`)

// genericLinkNumberRe is a chapter number written in a link's text
var genericLinkNumberRe = regexp.MustCompile(`(?i)(?:chapter|chap\.?|ch\.?|глава|гл\.?|розділ|capítulo|capitulo|cap\.?|chapitre|episode|ep\.?|第|#)\s*(\d+(?:[.,]\d+)?)`)

// genericChapterLinks guesses the chapter list of a series page: the largest
// set of links on the site's host that share a URL shape and differ only in
// their numbers, under the series' own path or with a chapter word in it
// ("/chapter-12", "/read/v01/c01"). Navigation, header and footer links are
// left out. Each link's number comes from its text ("Chapter 12", "Глава
// 12") or else from the last number that varies across the set.
func genericChapterLinks(doc *goquery.Document, pageURL string) []genericLink {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}
	host := normalHost(base.Host)
	seriesPath := strings.TrimSuffix(base.Path, "/")

	type candidate struct {
		url  *url.URL
		text string
	}
	groups := map[string][]candidate{}
	var order []string
	seen := map[string]bool{}
	doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		if a.Closest(genericSkip).Length() > 0 {
			return
		}
		href, _ := a.Attr("href")
		ref, err := url.Parse(strings.TrimSpace(href))
		if err != nil {
			return
		}
		u := base.ResolveReference(ref)
		u.Fragment = ""
		if (u.Scheme != "http" && u.Scheme != "https") || normalHost(u.Host) != host {
			return
		}
		key := u.Path + "?" + u.RawQuery
		if strings.TrimSuffix(u.Path, "/") == seriesPath && u.RawQuery == base.RawQuery {
			return
		}
		// ids ("97628fda-5c9a-…", a 24-digit hex) are one token, not numbers
		shape := genericDigitsRe.ReplaceAllString(genericIDRe.ReplaceAllString(key, "{id}"), "#")
		if !strings.ContainsAny(shape, "#{") {
			return // no number and no id: not a chapter
		}
		underSeries := seriesPath != "" && strings.HasPrefix(u.Path, seriesPath+"/")
		if !underSeries && !genericChapterWordRe.MatchString(u.Path) && !genericLinkNumberRe.MatchString(a.Text()) {
			return
		}
		if !seen[key] {
			seen[key] = true
			if _, ok := groups[shape]; !ok {
				order = append(order, shape)
			}
			groups[shape] = append(groups[shape], candidate{u, strings.Join(strings.Fields(a.Text()), " ")})
			return
		}
		// the same chapter linked twice: keep the first text that says something
		for i, c := range groups[shape] {
			if c.url.String() == u.String() && c.text == "" {
				groups[shape][i].text = strings.Join(strings.Fields(a.Text()), " ")
			}
		}
	})

	best := ""
	for _, shape := range order {
		if len(groups[shape]) > len(groups[best]) {
			best = shape
		}
	}
	list := groups[best]
	if len(list) < genericMinChapters {
		return nil
	}

	// the number is the last one that differs between the links
	numbers := make([][]string, len(list))
	for i, c := range list {
		numbers[i] = genericDigitsRe.FindAllString(genericIDRe.ReplaceAllString(c.url.Path+"?"+c.url.RawQuery, "{id}"), -1)
	}
	at := -1
	for j := range numbers[0] {
		for i := range numbers {
			if j < len(numbers[i]) && numbers[i][j] != numbers[0][j] {
				at = j
				break
			}
		}
	}
	// at < 0: the URLs differ only by ids, so only the link texts number them

	links := make([]genericLink, 0, len(list))
	for i, c := range list {
		numText := ""
		if m := genericLinkNumberRe.FindStringSubmatch(c.text); m != nil {
			numText = strings.ReplaceAll(m[1], ",", ".")
		} else if at >= 0 && at < len(numbers[i]) {
			numText = numbers[i][at]
		}
		num, err := strconv.ParseFloat(numText, 64)
		if err != nil {
			continue
		}
		// a row often ends in its date and counters ("… 27 груд 2023 р., 22:46|211")
		title := c.text
		if loc := genericDateRe.FindStringIndex(title); loc != nil {
			title = strings.TrimRight(title[:loc[0]], " -–—|·•,")
		}
		// a text that's only the number ("Chapter 12", "12") is no title:
		// the label says it already
		if !genericLetterRe.MatchString(genericLinkNumberRe.ReplaceAllString(title, "")) {
			title = ""
		}
		if r := []rune(title); len(r) > 120 {
			title = string(r[:120])
		}
		links = append(links, genericLink{URL: c.url.String(), Number: num, NumberText: numText, Title: title})
	}
	if len(links) < genericMinChapters {
		return nil
	}
	return links
}

// genericDateRe is a date or a time in a chapter row's text
var genericDateRe = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b|\b\d{1,2}[./]\d{1,2}[./]\d{2,4}\b|\b\d{1,2}\s+\pL{3,}\.?\s+\d{4}\b|\b\d{1,2}:\d{2}\b`)

// genericLetterRe is a letter, in any script
var genericLetterRe = regexp.MustCompile(`\pL`)

// genericImageRe is an image URL written out in a page's scripts, escaped
// slashes included
var genericImageRe = regexp.MustCompile(`https?:(?:\\?/){2}(?:[^"'\s<>()\\]|\\/)+?\.(?:jpe?g|png|webp|avif)(?:\?[^"'\s<>()\\]*)?`)

// genericImagePathRe is a JSON string that's an image path or URL
var genericImagePathRe = regexp.MustCompile(`(?i)^(?:https?://|/)[^\s"'<>]+\.(?:jpe?g|png|webp|avif)(?:\?\S*)?$`)

// genericNotPageRe is an image that's part of the site, not of a chapter
var genericNotPageRe = regexp.MustCompile(`(?i)logo|avatar|favicon|icon|banner|emoji|sprite|/ads?/|placeholder|loading`)

// genericImageAttrs are where an <img> keeps its address, lazy loaders first
var genericImageAttrs = []string{"data-src", "data-lazy-src", "data-original", "data-url", "src"}

// genericPageImages guesses a chapter's pages: the largest set of images
// served from one folder, taken from the page's <img>s, the image URLs its
// scripts write out, and the JSON its scripts fetched (jsons, from a rendered
// page). A JSON path relative to an image host ("//manga/x/1.jpg") is joined
// to the host of an image on the page that ends with it.
func genericPageImages(doc *goquery.Document, html, pageURL string, jsons []browser.APIResponse) []string {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}
	var found []string
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.HasPrefix(raw, "data:") {
			return
		}
		ref, err := url.Parse(raw)
		if err != nil {
			return
		}
		u := base.ResolveReference(ref)
		if u.Scheme != "http" && u.Scheme != "https" {
			return
		}
		if genericNotPageRe.MatchString(u.Path) || strings.HasSuffix(strings.ToLower(u.Path), ".svg") {
			return
		}
		found = append(found, u.String())
	}

	doc.Find("img").Each(func(_ int, img *goquery.Selection) {
		if img.Closest(genericSkip).Length() > 0 {
			return
		}
		for _, attr := range genericImageAttrs {
			if v, ok := img.Attr(attr); ok && strings.TrimSpace(v) != "" && !strings.HasPrefix(v, "data:") {
				add(v)
				return
			}
		}
		if v, ok := img.Attr("srcset"); ok {
			add(strings.Fields(v + " ")[0])
		}
	})
	// scripts write URLs inside JSON strings, escaped: \/, \u002F, \"
	unescaped := strings.NewReplacer(`\u002F`, "/", `\u002f`, "/", `\/`, "/", `\"`, `"`).Replace(html)
	for _, m := range genericImageRe.FindAllString(unescaped, -1) {
		add(m)
	}

	// The JSON's image paths. A relative one is joined to the base of the
	// page's own images that end with one of them ("https://img3.example" +
	// "//manga/x/1.jpg"), so the pages a reader didn't draw get its host too.
	var paths []string
	for _, r := range jsons {
		var v any
		if json.Unmarshal([]byte(r.Body), &v) != nil {
			continue
		}
		genericJSONStrings(v, func(s string) {
			if genericImagePathRe.MatchString(s) {
				paths = append(paths, s)
			}
		})
	}
	pageImages := append([]string(nil), found...)
	imageBase := ""
	for _, s := range paths {
		for _, p := range pageImages {
			if !strings.HasPrefix(s, "http") && strings.HasSuffix(p, s) && len(p) > len(s) {
				imageBase = p[:len(p)-len(s)]
				break
			}
		}
		if imageBase != "" {
			break
		}
	}
	for _, s := range paths {
		switch {
		case strings.HasPrefix(s, "http"):
			add(s)
		case imageBase != "":
			add(imageBase + s)
		case !strings.HasPrefix(s, "//") || strings.Contains(strings.SplitN(s[2:], "/", 2)[0], "."):
			// a host-relative path ("/uploads/1.jpg"), or "//host/1.jpg"
			add(s)
		}
	}

	groups := map[string][]string{}
	var order []string
	seen := map[string]bool{}
	for _, f := range found {
		if seen[f] {
			continue
		}
		seen[f] = true
		u, _ := url.Parse(f)
		dir := u.Scheme + "://" + u.Host + u.Path[:strings.LastIndex(u.Path, "/")+1]
		if _, ok := groups[dir]; !ok {
			order = append(order, dir)
		}
		groups[dir] = append(groups[dir], f)
	}
	best := ""
	for _, dir := range order {
		if len(groups[dir]) > len(groups[best]) {
			best = dir
		}
	}
	if len(groups[best]) < genericMinPages {
		return nil
	}
	return groups[best]
}

// genericJSONStrings calls fn with every string in v, in document order
func genericJSONStrings(v any, fn func(string)) {
	switch t := v.(type) {
	case string:
		fn(t)
	case []any:
		for _, e := range t {
			genericJSONStrings(e, fn)
		}
	case map[string]any:
		// map order is random: the page order lives in arrays, whose order
		// is kept, and a page object's own keys hold one image each
		for _, e := range t {
			genericJSONStrings(e, fn)
		}
	}
}

// genericTitleSplitRe cuts a site's name off a <title> ("Title | Site")
var genericTitleSplitRe = regexp.MustCompile(`\s+[|–—•·]\s+|\s+-\s+`)

// genericTitle guesses the series title: a JSON-LD name, the first heading,
// the og:title, or the <title> without the site's name
func genericTitle(doc *goquery.Document) string {
	title := ""
	doc.Find(`script[type="application/ld+json"]`).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		var v any
		if json.Unmarshal([]byte(s.Text()), &v) != nil {
			return true
		}
		title = genericLDName(v)
		return title == ""
	})
	if title == "" {
		title = strings.TrimSpace(doc.Find("main h1, article h1, h1").First().Text())
	}
	if title == "" {
		title = strings.TrimSpace(doc.Find(`meta[property="og:title"]`).AttrOr("content", ""))
	}
	if title == "" {
		t := strings.TrimSpace(doc.Find("title").First().Text())
		title = genericTitleSplitRe.Split(t, 2)[0]
	}
	return sanitizeTitle(title)
}

// genericLDTypes are the JSON-LD types whose name is a series' title
var genericLDTypes = []string{"CreativeWorkSeries", "ComicSeries", "Book", "BookSeries", "Series"}

// genericLDName returns the name of the first series-like JSON-LD object in v
func genericLDName(v any) string {
	switch t := v.(type) {
	case []any:
		for _, e := range t {
			if n := genericLDName(e); n != "" {
				return n
			}
		}
	case map[string]any:
		if graph, ok := t["@graph"]; ok {
			if n := genericLDName(graph); n != "" {
				return n
			}
		}
		typ := fmt.Sprint(t["@type"])
		for _, want := range genericLDTypes {
			if strings.Contains(typ, want) {
				if n, ok := t["name"].(string); ok {
					return strings.TrimSpace(n)
				}
			}
		}
	}
	return ""
}

// NotRecognisedError says no grabber matched a URL, and what the generic
// reader tried before giving up
type NotRecognisedError struct {
	Host string
	// Handlers is how many built-in site handlers declined the URL
	Handlers int
	// Tried is the generic reader's report, one line per step
	Tried []string
}

func (e *NotRecognisedError) Error() string {
	return fmt.Sprintf("site not recognised: %s", e.Detail())
}

// Detail is the error without its "site not recognised" lead
func (e *NotRecognisedError) Detail() string {
	parts := []string{fmt.Sprintf("%s has no built-in support (%d site handlers declined it)", e.Host, e.Handlers)}
	parts = append(parts, e.Tried...)
	return strings.Join(parts, "; ")
}

// hostOf is rawURL's host without "www.", or rawURL itself when it has none
func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	return normalHost(u.Host)
}

// normalHost lowercases host and drops "www."
func normalHost(host string) string {
	return strings.TrimPrefix(strings.ToLower(host), "www.")
}
