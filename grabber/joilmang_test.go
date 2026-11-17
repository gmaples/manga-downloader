// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// joilmangSeriesHTML is a trimmed copy of a joilmang.com series page: the
// visible chapter panel is a ul.divide-y with only the newest rows (which
// mangavault's `ul.divide-y a` would take), and the full list sits in
// <details><ol> blocks with the number in <b>.
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
// must not be read as a page, a breadcrumb nav labelled in Russian, and the
// link back to the series.
const joilmangReaderHTML = `<html><head><title>Перерождение Дяди — том 1 глава 1 читать онлайн | Joilmang</title></head><body>
<nav aria-label="Хлебные крошки"><ol><li><a tabindex="-1" href="/">Главная</a></li><li><a tabindex="-1" href="/manga/pererozhdenie-dyadi">Перерождение Дяди</a></li></ol></nav>
<a aria-label="К странице тайтла «Перерождение Дяди»" title="К странице тайтла «Перерождение Дяди»" class="hidden h-11 md:flex" href="/manga/pererozhdenie-dyadi">back</a>
<img src="/voice/art/joli-rec-240.webp" class="jm-art__img">
<picture data-reader-page="" class="reader-page-flow"><img src="https://jm-img.lexa86218.workers.dev/p/64644/0.avif" alt="Перерождение Дяди — Том 1 · Глава 1, страница 1" width="1150" height="1618" class="h-auto w-full"></picture>
<picture data-reader-page="" class="reader-page-flow"><img src="https://jm-img.lexa86218.workers.dev/p/64644/1.avif" alt="Перерождение Дяди — Том 1 · Глава 1, страница 2" width="1138" height="1616" class="h-auto w-full"></picture>
</body></html>`

func joilmangServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/manga/pererozhdenie-dyadi", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(joilmangSeriesHTML))
	})
	mux.HandleFunc("/manga/pererozhdenie-dyadi/1/1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(joilmangReaderHTML))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv
}

func TestJoilmang(t *testing.T) {
	srv := joilmangServer(t)

	m := NewPlainHTML(&Grabber{URL: srv.URL + "/manga/pererozhdenie-dyadi", Settings: &Settings{}})
	ok, err := m.Test()
	if err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want the series to match", ok, err)
	}
	if m.site.Rows != `details > ol > li > a[href^="/manga/"]` {
		t.Errorf("Test() matched with selector %q, want the joilmang one (an earlier entry shadows it)", m.site.Rows)
	}

	title, err := m.FetchTitle()
	if err != nil {
		t.Fatalf("FetchTitle() error: %v", err)
	}
	if title != "Перерождение Дяди" {
		t.Errorf("FetchTitle() = %q, want %q", title, "Перерождение Дяди")
	}

	chapters, errs := m.FetchChapters()
	if len(errs) != 0 {
		t.Fatalf("FetchChapters() errors: %v", errs)
	}
	if len(chapters) != 4 {
		t.Fatalf("FetchChapters() returned %d chapters, want 4", len(chapters))
	}
	for i, want := range []float64{80.1, 80, 1, 0} {
		if got := chapters[i].GetNumber(); got != want {
			t.Errorf("chapter %d number = %v, want %v", i, got, want)
		}
		if got := chapters[i].GetTitle(); got != "" {
			t.Errorf("chapter %d title = %q, want none (the number is the label)", i, got)
		}
	}

	chapter, err := m.FetchChapter(chapters[2])
	if err != nil {
		t.Fatalf("FetchChapter() error: %v", err)
	}
	if len(chapter.Pages) != 2 {
		t.Fatalf("FetchChapter() returned %d pages, want 2", len(chapter.Pages))
	}
	for i, want := range []string{
		"https://jm-img.lexa86218.workers.dev/p/64644/0.avif",
		"https://jm-img.lexa86218.workers.dev/p/64644/1.avif",
	} {
		if chapter.Pages[i].URL != want {
			t.Errorf("page %d URL = %q, want %q", i, chapter.Pages[i].URL, want)
		}
	}
}

// TestJoilmangChapterURL checks that a pasted reader URL loads the series
// through the reader's link back to it.
func TestJoilmangChapterURL(t *testing.T) {
	srv := joilmangServer(t)

	m := NewPlainHTML(&Grabber{URL: srv.URL + "/manga/pererozhdenie-dyadi/1/1", Settings: &Settings{}})
	ok, err := m.Test()
	if err != nil {
		t.Fatalf("Test() error: %v", err)
	}
	if ok {
		t.Fatalf("Test() matched the reader page itself; want it to decline so the series is resolved")
	}
	if !m.resolveSeriesURL() {
		t.Fatalf("resolveSeriesURL() = false; want the reader to resolve to its series")
	}
	if want := srv.URL + "/manga/pererozhdenie-dyadi"; m.URL != want {
		t.Errorf("resolved URL = %q, want %q", m.URL, want)
	}
	if m.site.Rows != `details > ol > li > a[href^="/manga/"]` {
		t.Errorf("resolved series matched with selector %q, want the joilmang one", m.site.Rows)
	}
}
