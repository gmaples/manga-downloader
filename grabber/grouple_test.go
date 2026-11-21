// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// groupleSeriesPage is a series page cut down from a.zazaza.me's (2026-11-21):
// the title, the login host and two rows of the chapter list
const groupleSeriesPage = `<!DOCTYPE html><html lang="ru"><head>
<meta itemprop="name" content="Провожающая в последний путь Фрирен">
<script>var X_URL = 'https://3.grouple.co'; rm_h.init();</script></head><body>
<h1 class="cr-hero-names__main line-clamp-2">Провожающая в последний путь Фрирен</h1>
<table><tbody>
<tr class="item-row" data-id="1076726" data-vol="15" data-num="1470">
  <td class="item-status "><i class="chapter-status item-unvisited"></i></td>
  <td class=" item-title" data-vol="15" data-num="1470">
    <a href="/frieren/vol15/147" title="Кофейный домик Переводчик, Световой Раскат Переводчик" class="chapter-link cp-l">
      15 - 147 Земля без героя
    </a>
  </td>
  <td class="d-none d-sm-table-cell date text-right " data-date="21.08.26">21.08.26</td>
</tr>
<tr class="item-row" data-id="313688" data-vol="1" data-num="15">
  <td class=" item-title" data-vol="1" data-num="15">
    <a href="/frieren/vol1/1.5" title="" class="chapter-link cp-l">1 - 1.5</a>
  </td>
  <td class="date text-info">завтра</td>
</tr>
<tr class="item-row" data-id="313687" data-vol="1" data-num="10">
  <td class=" item-title" data-vol="1" data-num="10">
    <a href="/frieren/vol1/1" title="Nippa Переводчик" class="chapter-link cp-l">
      1 - 1 Конец приключения
    </a>
  </td>
  <td class="date">15.05.20</td>
</tr>
</tbody></table></body></html>`

// groupleReaderPage is a reader page's script, in the shape a.zazaza.me serves it
const groupleReaderPage = `<html><body><a class="chapter-link" href="/frieren/vol1/1">1</a>
<script>var X_URL = 'https://3.grouple.co';
var chapterInfo = {'id': 19756, 'type': 'MANGA', 'vol': 1, 'num': 10, 'siteId': '1'};
rm_h.readerInit(chapterInfo, [['https://rm.one-way.work/','',"auto/38/40/03/01.png_res.jpg?t=1795291064&u=0&h=Dkn",836,1200, ''],['https://img.example/','',"auto/02.webp?t=1&h=x",1141,1600, ''],['','',"/static/03.png",1,1, '']], false);
</script></body></html>`

func groupleServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/frieren", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, groupleSeriesPage)
	})
	mux.HandleFunc("/frieren/vol1/1", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("mtr") != "true" && r.URL.RawQuery != "" {
			t.Errorf("reader query %q, want mtr=true", r.URL.RawQuery)
		}
		fmt.Fprint(w, groupleReaderPage)
	})
	mux.HandleFunc("/other", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><a class="chapter-link" href="/x">x</a></body></html>`)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestGroupLeIdentify(t *testing.T) {
	server := groupleServer(t)

	for _, path := range []string{"/frieren", "/frieren/vol1/1"} {
		site, errs := NewSite(server.URL+path, &Settings{})
		if _, ok := site.(*GroupLe); !ok {
			t.Errorf("NewSite(%s) = %T, %v, want *GroupLe", path, site, errs)
		}
	}

	// a page without the platform's markers isn't GroupLe's
	g := &GroupLe{Grabber: &Grabber{URL: server.URL + "/other"}}
	if ok, _ := g.Test(); ok {
		t.Error("Test matched a page without the GroupLe markers")
	}
	// paths of other shapes aren't fetched
	g = &GroupLe{Grabber: &Grabber{URL: server.URL + "/list/genres/x"}}
	if ok, _ := g.Test(); ok {
		t.Error("Test matched a path that's no series or reader")
	}
}

func TestGroupLeSeriesURL(t *testing.T) {
	cases := map[string]string{
		"https://a.zazaza.me/frieren":                 "https://a.zazaza.me/frieren",
		"https://a.zazaza.me/frieren/":                "https://a.zazaza.me/frieren",
		"https://a.zazaza.me/frieren/vol1/1":          "https://a.zazaza.me/frieren",
		"https://a.zazaza.me/frieren/vol15/147?mtr=1": "https://a.zazaza.me/frieren",
	}
	for in, want := range cases {
		g := &GroupLe{Grabber: &Grabber{URL: in}}
		if got, err := g.seriesURL(); err != nil || got != want {
			t.Errorf("seriesURL(%q) = %q, %v, want %q", in, got, err, want)
		}
	}
}

func TestGroupLeSeries(t *testing.T) {
	server := groupleServer(t)
	g := NewGroupLe(&Grabber{URL: server.URL + "/frieren/vol1/1", Settings: &Settings{}})

	title, err := g.FetchTitle()
	if err != nil || title != "Провожающая в последний путь Фрирен" {
		t.Fatalf("FetchTitle = %q, %v", title, err)
	}

	chapters, errs := g.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters errors: %v", errs)
	}
	// the text-info row is left out
	if len(chapters) != 2 {
		t.Fatalf("got %d chapters, want 2", len(chapters))
	}
	c := chapters[0].(*GroupLeChapter)
	if c.Number != 147 || c.Volume != 15 || c.Title != "Земля без героя" || c.Language != "ru" || c.ID != "/frieren/vol15/147" {
		t.Errorf("chapter 147 = %+v", c.Chapter)
	}
	if want := []string{"Кофейный домик", "Световой Раскат"}; !reflect.DeepEqual(c.GetGroups(), want) {
		t.Errorf("groups = %q, want %q", c.GetGroups(), want)
	}
	c = chapters[1].(*GroupLeChapter)
	if c.Number != 1 || c.Title != "Конец приключения" || c.URL != server.URL+"/frieren/vol1/1?mtr=true" {
		t.Errorf("chapter 1 = %+v, url %s", c.Chapter, c.URL)
	}

	chapter, err := g.FetchChapter(c)
	if err != nil {
		t.Fatalf("FetchChapter: %v", err)
	}
	var urls []string
	for _, p := range chapter.Pages {
		urls = append(urls, p.URL)
	}
	want := []string{
		// one-way.work serves images without the expiring token
		"https://rm.one-way.work/auto/38/40/03/01.png_res.jpg",
		"https://img.example/auto/02.webp?t=1&h=x",
		g.BaseUrl() + "/static/03.png",
	}
	if !reflect.DeepEqual(urls, want) {
		t.Errorf("pages = %q, want %q", urls, want)
	}
	if chapter.Number != 1 || chapter.Language != "ru" || !reflect.DeepEqual(chapter.GetGroups(), []string{"Nippa"}) {
		t.Errorf("chapter = %+v", chapter)
	}
}

func TestGroupLePagesMissing(t *testing.T) {
	if _, err := grouplePages("<html>no reader</html>", "https://x"); err == nil || !strings.Contains(err.Error(), "no reader pages") {
		t.Errorf("err = %v", err)
	}
}
