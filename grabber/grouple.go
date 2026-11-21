// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// GroupLe is a grabber for the GroupLe platform: ReadManga (a.zazaza.me),
// MintManga, SelfManga, Usagi and their mirrors, which move to new domains
// often. It's matched by the markup of its pages, never by a domain list.
// The whole chapter list is in the series page, and a chapter's images are
// in the reader's inline rm_h.readerInit call.
type GroupLe struct {
	*Grabber
	doc *goquery.Document
}

// NewGroupLe returns a GroupLe grabber, matched by the markup of its pages
func NewGroupLe(g *Grabber) *GroupLe {
	return &GroupLe{Grabber: g}
}

// GroupLeChapter is a chapter of the list, its ID being its reader path
type GroupLeChapter struct {
	Chapter
	URL string
}

// groupleMarkers are on every series and reader page of the platform: the
// reader script object, the chapter links of the list and the platform's
// shared login host
var groupleMarkers = []string{"rm_h.", "chapter-link", "grouple.co"}

// groupleSeriesPathRe is a series path (/{slug}) and groupleReaderPathRe a
// reader path (/{slug}/vol{V}/{N}), the only shapes the markup check fetches
var (
	groupleSeriesPathRe = regexp.MustCompile(`^/[^/]+/?$`)
	groupleReaderPathRe = regexp.MustCompile(`^(/[^/]+)/vol\d+/[^/]+/?$`)
)

// groupleLinkTextRe splits a chapter link's text, "15 - 147 Земля без героя",
// into its volume and number prefix and its title
var groupleLinkTextRe = regexp.MustCompile(`^\d+\s*-\s*[\d.]+\s*(.*)$`)

// grouplePagesRe matches one page of the readerInit array:
// ['https://host/','',"path?query",width,height,”]
var grouplePagesRe = regexp.MustCompile(`\[['"](.*?)['"],['"](.*?)['"],['"](.*?)['"].*?\]`)

// groupleTokenlessHosts serve their images without the URL's token, which
// expires; the query is dropped for them (as the keiyoushi extension does)
var groupleTokenlessHosts = []string{"one-way.work"}

// Test matches a series or reader page of the platform by its markup
func (g *GroupLe) Test() (bool, error) {
	u, err := url.Parse(g.URL)
	if err != nil {
		return false, nil
	}
	if !groupleSeriesPathRe.MatchString(u.Path) && !groupleReaderPathRe.MatchString(u.Path) {
		return false, nil
	}
	return pageHasMarkup(g.Grabber, g.URL, groupleMarkers...), nil
}

// seriesURL returns the series page of the URL: a reader URL gives its
// series, so a pasted chapter link loads the whole series
func (g *GroupLe) seriesURL() (string, error) {
	u, err := url.Parse(g.URL)
	if err != nil {
		return "", err
	}
	path := u.Path
	if m := groupleReaderPathRe.FindStringSubmatch(path); m != nil {
		path = m[1]
	}
	if !groupleSeriesPathRe.MatchString(path) {
		return "", errSeriesNotFound(g.URL)
	}
	return u.Scheme + "://" + u.Host + strings.TrimSuffix(path, "/"), nil
}

// fetchSeries fetches the series page once
func (g *GroupLe) fetchSeries() (*goquery.Document, error) {
	if g.doc != nil {
		return g.doc, nil
	}
	series, err := g.seriesURL()
	if err != nil {
		return nil, err
	}
	body, err := http.Get(http.RequestParams{URL: series})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}
	g.doc = doc
	return doc, nil
}

// FetchTitle returns the series title (its Russian name)
func (g *GroupLe) FetchTitle() (string, error) {
	doc, err := g.fetchSeries()
	if err != nil {
		return "", err
	}
	title := sanitizeTitle(doc.Find("h1.cr-hero-names__main").First().Text())
	if title == "" {
		title = sanitizeTitle(doc.Find(`meta[itemprop="name"]`).First().AttrOr("content", ""))
	}
	if title == "" {
		return "", fmt.Errorf("no title found in %s", g.URL)
	}
	return title, nil
}

// groupleGroups returns the translators named by a chapter link's title,
// "Кофейный домик Переводчик, Световой Раскат Переводчик"
func groupleGroups(title string) []string {
	var groups []string
	for _, name := range strings.Split(title, ",") {
		name = strings.TrimSpace(name)
		name = strings.TrimSuffix(name, "(Переводчик)")
		name = strings.TrimSuffix(name, "Переводчик")
		name = strings.TrimSpace(name)
		if name != "" {
			groups = append(groups, name)
		}
	}
	return groups
}

// FetchChapters returns the chapters of the series page's list
func (g *GroupLe) FetchChapters() (Filterables, []error) {
	doc, err := g.fetchSeries()
	if err != nil {
		return nil, []error{err}
	}
	base := g.BaseUrl()

	var chapters Filterables
	var errs []error
	doc.Find("tr.item-row").Each(func(_ int, row *goquery.Selection) {
		// the keiyoushi extension leaves out rows whose date cell is
		// text-info, as not readable yet
		if row.Find("td.date.text-info").Length() > 0 {
			return
		}
		link := row.Find("a.chapter-link").First()
		href, ok := link.Attr("href")
		if !ok {
			return
		}
		cell := row.Find("td.item-title").First()
		// data-num is the number times ten: 10 is chapter 1, 1475 is 147.5
		num10, err := strconv.ParseFloat(cell.AttrOr("data-num", ""), 64)
		if err != nil {
			errs = append(errs, fmt.Errorf("chapter %s: no number", href))
			return
		}
		number := num10 / 10
		vol, _ := strconv.Atoi(cell.AttrOr("data-vol", ""))

		title := sanitizeTitle(link.Text())
		if m := groupleLinkTextRe.FindStringSubmatch(title); m != nil {
			title = m[1]
		}

		chapter := &GroupLeChapter{
			Chapter: Chapter{
				Number:   number,
				Title:    title,
				Volume:   vol,
				Language: "ru",
				ID:       href,
				Label:    Label{Style: LabelNumbered, Number: strconv.FormatFloat(number, 'f', -1, 64)},
			},
			// mtr=true is the site's own "I'm an adult" answer for
			// age-limited titles, sent by its reader links
			URL: base + href + "?mtr=true",
		}
		chapter.SetGroups(groupleGroups(link.AttrOr("title", ""))...)
		chapters = append(chapters, chapter)
	})

	return chapters, errs
}

// grouplePages returns the page URLs of a reader page's readerInit call
func grouplePages(page, base string) ([]string, error) {
	start := strings.Index(page, "rm_h.readerInit(")
	if start < 0 {
		start = strings.Index(page, "rm_h.readerDoInit(")
	}
	if start < 0 {
		return nil, fmt.Errorf("no reader pages found")
	}
	end := strings.Index(page[start:], ");")
	if end < 0 {
		return nil, fmt.Errorf("no reader pages found")
	}
	call := page[start : start+end]

	var urls []string
	for _, m := range grouplePagesRe.FindAllStringSubmatch(call, -1) {
		host, middle, path := m[1], m[2], m[3]
		var u string
		switch {
		case middle == "" && strings.HasPrefix(path, "/static/"):
			u = base + path
		case strings.HasSuffix(middle, "/manga/"):
			u = host + path
		default:
			u = middle + host + path
		}
		if !strings.Contains(u, "://") {
			u = "https:" + u
		}
		for _, h := range groupleTokenlessHosts {
			if strings.Contains(u, h) {
				u, _, _ = strings.Cut(u, "?")
			}
		}
		urls = append(urls, strings.Replace(u, "//resh", "//h", 1))
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("no reader pages found")
	}
	return urls, nil
}

// FetchChapter fetches a chapter's pages from its reader page
func (g *GroupLe) FetchChapter(f Filterable) (*Chapter, error) {
	gchap := f.(*GroupLeChapter)

	page, err := http.GetText(http.RequestParams{URL: gchap.URL})
	if err != nil {
		return nil, err
	}
	urls, err := grouplePages(page, g.BaseUrl())
	if err != nil {
		return nil, fmt.Errorf("chapter %s: %w", gchap.ID, err)
	}

	chapter := &Chapter{
		Title:      gchap.GetTitle(),
		Number:     gchap.GetNumber(),
		Volume:     gchap.Volume,
		Language:   "ru",
		ID:         gchap.ID,
		Label:      gchap.Label,
		PagesCount: int64(len(urls)),
	}
	chapter.SetGroups(gchap.GetGroups()...)
	for i, u := range urls {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: u})
	}
	return chapter, nil
}
