// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"errors"
	"fmt"
	nethttp "net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/browser"
)

func TestMain(m *testing.M) {
	// no test may start a real browser through the generic reader
	genericBrowser = false
	os.Exit(m.Run())
}

func mustDoc(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// A series page whose chapters are under its own path, as joilmang lists them
// (/manga/{slug}/{volume}/{number}), with navigation links left out
func TestGenericChapterLinksUnderSeriesPath(t *testing.T) {
	html := `<html><body>
		<header><a href="/manga/other/1/1">Popular 1</a><a href="/manga/other/1/2">Popular 2</a><a href="/manga/other/1/3">Popular 3</a></header>
		<a href="/manga/slug/1/0">Start reading</a>
		<ol>
			<li><a href="/manga/slug/2/187">187</a></li>
			<li><a href="/manga/slug/2/186">186</a></li>
			<li><a href="/manga/slug/1/1">1</a></li>
			<li><a href="/manga/slug/1/0">0</a></li>
		</ol>
		<a href="/catalog?page=2">next</a>
	</body></html>`
	links := genericChapterLinks(mustDoc(t, html), "https://site.example/manga/slug")
	var got []string
	for _, l := range links {
		got = append(got, fmt.Sprintf("%s=%g", l.URL[len("https://site.example"):], l.Number))
	}
	want := "/manga/slug/1/0=0 /manga/slug/2/187=187 /manga/slug/2/186=186 /manga/slug/1/1=1"
	if strings.Join(got, " ") != want {
		t.Errorf("got %v, want %s", got, want)
	}
}

// Chapter links outside the series path count when they name a chapter, and a
// number in the link text wins over the URL's
func TestGenericChapterLinksByChapterWord(t *testing.T) {
	html := `<html><body>
		<a href="/ru/53517--slug/read/v01/c01">Том 1 Глава 1</a>
		<a href="/ru/53517--slug/read/v01/c02">Том 1 Глава 2</a>
		<a href="/ru/53517--slug/read/v15/c62.5">Том 15 Глава 62.5</a>
		<a href="/ru/user/123">profile</a>
		<a href="/ru/user/456">profile</a>
	</body></html>`
	links := genericChapterLinks(mustDoc(t, html), "https://site.example/ru/manga/53517--slug")
	if len(links) != 3 {
		t.Fatalf("got %d links: %+v", len(links), links)
	}
	if links[2].Number != 62.5 || links[2].NumberText != "62.5" {
		t.Errorf("last link = %+v, want 62.5", links[2])
	}
}

// One link, or links whose number never changes, is no chapter list
func TestGenericChapterLinksNone(t *testing.T) {
	for name, html := range map[string]string{
		"one link":    `<a href="/manga/slug/1">Chapter 1</a>`,
		"no numbers":  `<a href="/manga/slug/a">a</a><a href="/manga/slug/b">b</a>`,
		"other hosts": `<a href="https://cdn.example/manga/slug/1">1</a><a href="https://cdn.example/manga/slug/2">2</a>`,
	} {
		if links := genericChapterLinks(mustDoc(t, "<body>"+html+"</body>"), "https://site.example/manga/slug"); links != nil {
			t.Errorf("%s: got %+v", name, links)
		}
	}
}

// Reader images in one folder are the pages; site images are left out
func TestGenericPageImagesFromImg(t *testing.T) {
	html := `<html><body>
		<header><img src="/logo.png"></header>
		<img src="/voice/art/joli-rec-240.webp" loading="lazy">
		<picture data-reader-page><img src="https://img.example/p/1138863/0.avif"></picture>
		<picture data-reader-page><img src="https://img.example/p/1138863/1.avif"></picture>
		<picture data-reader-page><img data-src="https://img.example/p/1138863/2.avif" src="data:image/gif;base64,R0"></picture>
		<img src="https://img.example/avatars/me.jpg">
	</body></html>`
	got := genericPageImages(mustDoc(t, html), html, "https://site.example/manga/slug/1/1", nil)
	want := []string{"https://img.example/p/1138863/0.avif", "https://img.example/p/1138863/1.avif", "https://img.example/p/1138863/2.avif"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("got %v, want %v", got, want)
	}
}

// Image URLs written out in a script count too, escaped slashes and all
func TestGenericPageImagesFromScript(t *testing.T) {
	html := `<html><body><script>var pages = ["https:\/\/cdn.example\/ch\/1\/01.jpg","https:\/\/cdn.example\/ch\/1\/02.jpg?t=1"];</script></body></html>`
	got := genericPageImages(mustDoc(t, html), html, "https://site.example/read/1", nil)
	want := "https://cdn.example/ch/1/01.jpg https://cdn.example/ch/1/02.jpg?t=1"
	if strings.Join(got, " ") != want {
		t.Errorf("got %v", got)
	}
}

// A reader that draws only the pages in view: the rest come from the JSON the
// page fetched, whose paths are joined to the host of a drawn page (MangaLib's
// "//manga/..." paths on img3.cdnrulib.link)
func TestGenericPageImagesFromJSON(t *testing.T) {
	html := `<html><body>
		<img src="https://img3.example//manga/slug/chapters/1134099/ws001-01_R2qr.jpg">
		<img src="https://img3.example//manga/slug/chapters/1134099/ws001-02_eblG.jpg">
	</body></html>`
	jsons := []browser.APIResponse{
		{URL: "https://api.example/constants", Body: `{"data":{"imageServers":[{"url":"https://img3.example"}]}}`},
		{URL: "https://api.example/chapter", Body: `{"data":{"pages":[
			{"url":"//manga/slug/chapters/1134099/ws001-01_R2qr.jpg"},
			{"url":"//manga/slug/chapters/1134099/ws001-02_eblG.jpg"},
			{"url":"//manga/slug/chapters/1134099/ws001-03_kk1.jpg"}]}}`},
	}
	got := genericPageImages(mustDoc(t, html), html, "https://site.example/read/v1/c1", jsons)
	if len(got) != 3 || got[2] != "https://img3.example//manga/slug/chapters/1134099/ws001-03_kk1.jpg" {
		t.Errorf("got %v", got)
	}
}

func TestGenericTitle(t *testing.T) {
	for name, tc := range map[string]struct{ html, want string }{
		"json-ld": {`<head><title>Read X | Site</title><script type="application/ld+json">{"@type":"CreativeWorkSeries","name":"Доктор Стоун"}</script></head><body><h1>Other</h1></body>`, "Доктор Стоун"},
		"h1":      {`<head><title>Read X | Site</title></head><body><h1> My  Series </h1></body>`, "My Series"},
		"title":   {`<head><title>My Series | Site</title></head><body></body>`, "My Series"},
	} {
		if got := genericTitle(mustDoc(t, tc.html)); got != tc.want {
			t.Errorf("%s: got %q, want %q", name, got, tc.want)
		}
	}
}

func TestGenericBlock(t *testing.T) {
	if got := genericBlock(mustDoc(t, `<title>Vercel Security Checkpoint</title><body>We're verifying your browser</body>`)); got != "a Vercel bot check" {
		t.Errorf("got %q", got)
	}
	if got := genericBlock(mustDoc(t, `<title>Series</title><body>chapters</body>`)); got != "" {
		t.Errorf("got %q", got)
	}
}

// genericSite serves a page per path
func genericSite(t *testing.T, pages map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		body, ok := pages[r.URL.Path]
		if !ok {
			nethttp.NotFound(w, r)
			return
		}
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// An unknown site with a plain chapter list loads through the generic reader,
// and its chapters' pages come from the plain reader pages
func TestGenericLoadsPlainSite(t *testing.T) {
	srv := genericSite(t, map[string]string{
		"/series/abc": `<html lang="pl"><head><title>Abc | Site</title></head><body><h1>Abc</h1>
			<a href="/series/abc/chapter-1">Chapter 1</a><a href="/series/abc/chapter-2">Chapter 2</a></body></html>`,
		"/series/abc/chapter-2": `<html><body><img src="/img/abc/2/1.jpg"><img src="/img/abc/2/2.jpg"></body></html>`,
	})
	g := &Grabber{URL: srv.URL + "/series/abc", Settings: &Settings{}}
	site, errs := g.IdentifySite()
	gs, ok := site.(*Generic)
	if !ok {
		t.Fatalf("got %T, errs %v", site, errs)
	}
	if title, _ := gs.FetchTitle(); title != "Abc" {
		t.Errorf("title %q", title)
	}
	chapters, _ := gs.FetchChapters()
	if len(chapters) != 2 || chapters[1].GetNumber() != 2 {
		t.Fatalf("chapters %+v", chapters)
	}
	if lang := chapters[0].(*GenericChapter).Language; lang != "pl" {
		t.Errorf("language %q", lang)
	}
	ch, err := gs.FetchChapter(chapters[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(ch.Pages) != 2 || ch.Pages[1].URL != srv.URL+"/img/abc/2/2.jpg" {
		t.Errorf("pages %+v", ch.Pages)
	}
}

// A JavaScript app's empty page is rendered; the rendered chapter list loads
func TestGenericLoadsRenderedSite(t *testing.T) {
	srv := genericSite(t, map[string]string{"/t/7": `<html><body><div id="app"></div></body></html>`})
	defer func(old bool, cap func(string, string) (browser.Snapshot, error)) {
		genericBrowser, genericCapture = old, cap
	}(genericBrowser, genericCapture)
	genericBrowser = true
	genericCapture = func(string, string) (browser.Snapshot, error) {
		return browser.Snapshot{Status: 200, HTML: `<html><body><h1>Seven</h1>
			<a href="/t/7/101">Розділ 1</a><a href="/t/7/102">Розділ 2</a><a href="/t/7/103">Розділ 3</a></body></html>`}, nil
	}
	g := &Grabber{URL: srv.URL + "/t/7", Settings: &Settings{}}
	site, errs := g.IdentifySite()
	gs, ok := site.(*Generic)
	if !ok {
		t.Fatalf("got %T, errs %v", site, errs)
	}
	chapters, _ := gs.FetchChapters()
	if len(chapters) != 3 || chapters[2].GetNumber() != 3 {
		t.Errorf("chapters %+v", chapters)
	}
}

// When nothing works, IdentifySite says what the generic reader tried
func TestGenericReportsWhatItTried(t *testing.T) {
	srv := genericSite(t, map[string]string{"/t/7": `<html><body><div id="app"></div></body></html>`})
	defer func(old bool, cap func(string, string) (browser.Snapshot, error)) {
		genericBrowser, genericCapture = old, cap
	}(genericBrowser, genericCapture)
	genericBrowser = true
	genericCapture = func(string, string) (browser.Snapshot, error) {
		return browser.Snapshot{Status: 200, HTML: `<html><body><main>Confirm your age or sign in</main></body></html>`}, nil
	}
	g := &Grabber{URL: srv.URL + "/t/7", Settings: &Settings{}}
	site, errs := g.IdentifySite()
	if site != nil {
		t.Fatalf("got %T", site)
	}
	var nr *NotRecognisedError
	if len(errs) == 0 || !errors.As(errs[len(errs)-1], &nr) {
		t.Fatalf("errs %v", errs)
	}
	detail := nr.Detail()
	for _, want := range []string{"no chapter list in the page", "0 chapter link(s) in the rendered page", `"Confirm your age or sign in"`} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %q lacks %q", detail, want)
		}
	}
}

// A link text that only names the chapter's number is no title
func TestGenericChapterLinksTitles(t *testing.T) {
	html := `<body><a href="/s/x/1">Розділ 1</a><a href="/s/x/2">Chapter 2 - The Fight</a><a href="/s/x/3">3</a></body>`
	links := genericChapterLinks(mustDoc(t, html), "https://site.example/s/x")
	var got []string
	for _, l := range links {
		got = append(got, l.Title)
	}
	if strings.Join(got, "|") != "|Chapter 2 - The Fight|" {
		t.Errorf("titles %q", got)
	}
}

// A series page that links to its chapter list on a page of its own
// (senkuro's /manga/{slug}/chapters) is followed there
func TestGenericFollowsChaptersPage(t *testing.T) {
	srv := genericSite(t, map[string]string{
		"/manga/abc": `<html><body><h1>Abc</h1><a href="/manga/abc/chapters">Главы 2</a></body></html>`,
		"/manga/abc/chapters": `<html><body><a href="/manga/abc/chapters/111">Глава 1</a>
			<a href="/manga/abc/chapters/112">Глава 2</a></body></html>`,
	})
	g := &Grabber{URL: srv.URL + "/manga/abc", Settings: &Settings{}}
	site, errs := g.IdentifySite()
	gs, ok := site.(*Generic)
	if !ok {
		t.Fatalf("got %T, errs %v", site, errs)
	}
	chapters, _ := gs.FetchChapters()
	if len(chapters) != 2 || chapters[1].GetNumber() != 2 {
		t.Errorf("chapters %+v", chapters)
	}
	if title, _ := gs.FetchTitle(); title != "Abc" {
		t.Errorf("title %q", title)
	}
}

// The hidden browser gets the script that opens a chapters tab
func TestGenericRendersWithTheTabScript(t *testing.T) {
	srv := genericSite(t, map[string]string{"/book/x": `<html><body><div id="app"></div></body></html>`})
	defer func(old bool, cap func(string, string) (browser.Snapshot, error)) {
		genericBrowser, genericCapture = old, cap
	}(genericBrowser, genericCapture)
	genericBrowser = true
	var prepared string
	genericCapture = func(_, prepare string) (browser.Snapshot, error) {
		prepared = prepare
		return browser.Snapshot{Status: 200, HTML: `<html><body><h1>X</h1>
			<a href="/read/x/1">Розділ 1</a><a href="/read/x/2">Розділ 2</a></body></html>`}, nil
	}
	g := &Grabber{URL: srv.URL + "/book/x", Settings: &Settings{}}
	if site, errs := g.IdentifySite(); site == nil {
		t.Fatalf("no site, errs %v", errs)
	}
	if prepared != genericRevealJS {
		t.Errorf("the capture ran %q", prepared)
	}
}

// A chapter list's name, with a count, opens it; a single chapter's doesn't
func TestGenericListWord(t *testing.T) {
	for text, want := range map[string]bool{
		"Розділи": true, "Розділи 10": true, "Chapters (120)": true, "Главы": true, "All chapters": true,
		"Chapter 1": false, "Глава 12": false, "Read chapters of this series and more": false,
	} {
		if got := genericListWordRe.MatchString(text); got != want {
			t.Errorf("%q: got %v, want %v", text, got, want)
		}
	}
}

// Page URLs inside a script's JSON string, escaped (senkuro's window.__SENKURO__)
func TestGenericPageImagesFromEscapedJSON(t *testing.T) {
	html := `<html><body><img src="data:image/svg+xml;base64,AA"><script>window.__S__=JSON.parse("{\"image\":{\"original\":{\"url\":\"https://cdn.example/manga-chapters/1/2/a_1.jpeg\"}}},{\"image\":{\"original\":{\"url\":\"https://cdn.example/manga-chapters/1/2/b_2.jpeg\"}}}")</script></body></html>`
	got := genericPageImages(mustDoc(t, html), html, "https://site.example/manga/x/chapters/1/pages/1", nil)
	want := "https://cdn.example/manga-chapters/1/2/a_1.jpeg https://cdn.example/manga-chapters/1/2/b_2.jpeg"
	if strings.Join(got, " ") != want {
		t.Errorf("got %v", got)
	}
}

// A link outside the series path counts when its text names a chapter
func TestGenericChapterLinksByLinkText(t *testing.T) {
	html := `<body><a href="/r/9/101">Розділ 1</a><a href="/r/9/102">Розділ 2</a><a href="/u/1">x</a><a href="/u/2">y</a></body>`
	if links := genericChapterLinks(mustDoc(t, html), "https://site.example/book/abc"); len(links) != 2 || links[1].Number != 2 {
		t.Errorf("links %+v", links)
	}
}

// Chapter URLs told apart only by ids (honey-manga's /read/{uuid}/{uuid})
// are one list, numbered by their link texts
func TestGenericChapterLinksByID(t *testing.T) {
	html := `<body>
		<a href="/read/97628fda-5c9a-48a1-a926-98096a4eebac/00427aa7-c581-4f5e-9ff6-97144c505ae3"><span>Том 1 - Розділ 2</span></a>
		<a href="/read/e37467c7-0c11-4c7a-92f1-238ab8e69a41/00427aa7-c581-4f5e-9ff6-97144c505ae3"><span>Том 1 - Розділ 1</span></a>
	</body>`
	links := genericChapterLinks(mustDoc(t, html), "https://site.example/book/00427aa7-c581-4f5e-9ff6-97144c505ae3")
	if len(links) != 2 || links[0].Number != 2 || links[1].Number != 1 {
		t.Errorf("links %+v", links)
	}
}
