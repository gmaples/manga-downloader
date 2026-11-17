// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"reflect"
	"testing"
)

// joilmangSeriesHTML is a trimmed copy of a joilmang.com series page as the
// browser renders it: the visible chapter panel is a ul.divide-y with only
// the newest rows, and the full list sits in <details><ol> blocks with a bare
// number in <b>.
const joilmangSeriesHTML = `<html><head><title>Перерождение Дяди — читать мангу онлайн, 133 главы | Joilmang</title></head><body>
<main id="main-content">
<nav aria-label="Хлебные крошки" class="jm-title-crumbs"><ol class="flex flex-wrap"><li><a href="/">Главная</a></li><li><a href="/catalog">Каталог</a></li></ol></nav>
<h1 class="section-heading text-[22px]">Перерождение Дяди</h1>
<a href="/manga/pererozhdenie-dyadi/11/0">Начать читать</a>
<ul class="panel max-h-[440px] divide-y divide-ink-line">
<li><div class="flex items-stretch"><a class="group flex items-center" href="/manga/pererozhdenie-dyadi/16/80.1"><span class="sticker shrink-0">Том&nbsp;<!-- -->16</span><span class="min-w-0 flex-1 truncate">Глава 80.1</span><span class="shrink-0 text-xs">13.11.2026</span></a></div></li>
<li><div class="flex items-stretch"><a class="group flex items-center" href="/manga/pererozhdenie-dyadi/16/80"><span class="sticker shrink-0">Том&nbsp;<!-- -->16</span><span class="min-w-0 flex-1 truncate">Глава 80</span><span class="shrink-0 text-xs">18.10.2026</span></a></div></li>
</ul>
<div class="mt-2"><div class="panel overflow-hidden p-0">
<details><summary><span>Главы 27–80.1</span><span>2</span></summary><ol>
<li><a href="/manga/pererozhdenie-dyadi/16/80.1"><b>80.1</b><span>13.11.2026</span></a></li>
<li><a href="/manga/pererozhdenie-dyadi/16/80"><b>80</b><span>18.10.2026</span></a></li>
</ol></details>
<details><summary><span>Главы 0–26</span><span>2</span></summary><ol>
<li><a href="/manga/pererozhdenie-dyadi/1/1"><b>1</b><span>08.03.2023</span></a></li>
<li><a href="/manga/pererozhdenie-dyadi/11/0"><b>0</b><span>26.03.2024</span></a></li>
</ol></details>
</div></div>
</main>
</body></html>`

// joilmangReaderHTML is a trimmed copy of a joilmang.com reader page: every
// page is an <img> in picture[data-reader-page], next to the site's art that
// must not be read as a page.
const joilmangReaderHTML = `<html><head><title>Перерождение Дяди — том 1 глава 1 читать онлайн | Joilmang</title></head><body>
<a title="К странице тайтла «Перерождение Дяди»" href="/manga/pererozhdenie-dyadi">back</a>
<img src="/voice/art/joli-rec-240.webp" class="jm-art__img">
<picture data-reader-page="" class="reader-page-flow"><img src="https://jm-img.lexa86218.workers.dev/p/64644/0.avif" alt="Перерождение Дяди — Том 1 · Глава 1, страница 1" width="1150" height="1618" class="h-auto w-full"></picture>
<picture data-reader-page="" class="reader-page-flow"><img src="https://jm-img.lexa86218.workers.dev/p/64644/1.avif" alt="Перерождение Дяди — Том 1 · Глава 1, страница 2" width="1138" height="1616" class="h-auto w-full"></picture>
</body></html>`

func joilmangEntry(t *testing.T) BrowserSiteSelector {
	t.Helper()
	sel, ok := matchBrowserSelector("joilmang.com")
	if !ok {
		t.Fatal("no joilmang.com entry")
	}
	return sel
}

func TestJoilmangChapters(t *testing.T) {
	sel := joilmangEntry(t)
	m := PlainHTML{Grabber: &Grabber{URL: "https://joilmang.com/manga/pererozhdenie-dyadi"}, site: sel.SiteSelector}
	m.doc = docFromHTML(t, joilmangSeriesHTML)
	m.rows = m.doc.Find(m.site.Rows)

	if got := m.pageTitle(); got != "Перерождение Дяди" {
		t.Errorf("title = %q, want %q", got, "Перерождение Дяди")
	}

	chapters, errs := m.FetchChapters()
	if len(errs) != 0 || len(chapters) != 4 {
		t.Fatalf("chapters = %d, errs = %v; want 4 from the full list, not the newest-rows panel", len(chapters), errs)
	}
	for i, want := range []float64{80.1, 80, 1, 0} {
		if got := chapters[i].GetNumber(); got != want {
			t.Errorf("chapter %d number = %v, want %v", i, got, want)
		}
		if got := chapters[i].GetTitle(); got != "" {
			t.Errorf("chapter %d title = %q, want none (the number is the label)", i, got)
		}
	}
	if got := chapters[2].(*PlainHTMLChapter).URL; got != "https://joilmang.com/manga/pererozhdenie-dyadi/1/1" {
		t.Errorf("url = %q", got)
	}
}

func TestJoilmangReaderImages(t *testing.T) {
	sel := joilmangEntry(t)
	doc := docFromHTML(t, joilmangReaderHTML)
	want := []string{
		"https://jm-img.lexa86218.workers.dev/p/64644/0.avif",
		"https://jm-img.lexa86218.workers.dev/p/64644/1.avif",
	}
	if got := getPlainHTMLImageURL(sel.Image, doc); !reflect.DeepEqual(got, want) {
		t.Errorf("images = %v, want %v", got, want)
	}
}

// TestJoilmangChapterURL checks that a pasted reader URL, the form Greg
// pasted, is trimmed to its series before anything is rendered
func TestJoilmangChapterURL(t *testing.T) {
	sel := joilmangEntry(t)
	for _, c := range []struct{ url, want string }{
		{"https://joilmang.com/manga/pererozhdenie-dyadi/1/1", "https://joilmang.com/manga/pererozhdenie-dyadi"},
		{"https://www.joilmang.com/manga/pererozhdenie-dyadi/16/80.1?x=1", "https://www.joilmang.com/manga/pererozhdenie-dyadi"},
		{"https://joilmang.com/manga/pererozhdenie-dyadi", ""},
	} {
		m := &PlainHTMLBrowser{PlainHTML: NewPlainHTML(&Grabber{URL: c.url}), selector: sel}
		if got := m.pathSeriesURL(); got != c.want {
			t.Errorf("pathSeriesURL(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}
