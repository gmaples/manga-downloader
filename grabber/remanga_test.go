// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRemangaTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
		dir  string
	}{
		{"https://remanga.org/manga/murim-login_", true, "murim-login_"},
		{"https://remanga.org/manga/murim-login_/main", true, "murim-login_"},
		// a reader URL (the number is a chapter id) loads its series
		{"https://remanga.org/manga/murim-login_/320521?page=1", true, "murim-login_"},
		{"https://www.remanga.org/manga/murim-login_#x", true, "murim-login_"},
		{"https://remanga.org/", false, ""},
		{"https://oremanga.net/manga/x", false, ""},
		{"https://mangalib.org/ru/manga/1--x", false, ""},
	}

	for _, c := range cases {
		r := &Remanga{Grabber: &Grabber{URL: c.url}}
		got, err := r.Test()
		if err != nil || got != c.want {
			t.Errorf("Test(%q) = %v, %v, want %v", c.url, got, err, c.want)
			continue
		}
		if !c.want {
			continue
		}
		if dir, err := r.seriesDir(); err != nil || dir != c.dir {
			t.Errorf("seriesDir(%q) = %q, %v, want %q", c.url, dir, err, c.dir)
		}
	}
}

// remangaServer serves a fake api in the shapes the real one answered with on
// 2026-11-20: one branch, a list of two pages, and a chapter whose pages are
// groups of strips
func remangaServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	withReferer := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Referer") != "https://remanga.org/" {
				t.Errorf("%s: Referer %q", r.URL, r.Header.Get("Referer"))
			}
			next(w, r)
		}
	}
	mux.HandleFunc("/api/v2/titles/murim-login_/", withReferer(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":11004,"secondary_name":"Murim Login","main_name":"Логин Мурим","dir":"murim-login_",`+
			`"branches":[{"id":10824,"count_chapters":3,"publishers":[{"id":3169,"name":"Newlate.WTF"}]}]}`)
	}))
	mux.HandleFunc("/api/v2/titles/chapters/", withReferer(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("branch_id") != "10824" {
			t.Errorf("branch_id %q", q.Get("branch_id"))
		}
		pub := `"publishers":[{"id":3169,"name":"Newlate.WTF"}]`
		switch q.Get("page") {
		case "1":
			fmt.Fprint(w, `{"next":2,"previous":null,"results":[`+
				`{"id":2818816,"is_bought":false,`+pub+`,"index":3,"tome":4,"chapter":"206.5","name":"","price":"17.00","is_published":true,"is_paid":false},`+
				`{"id":1937253,"is_bought":false,`+pub+`,"index":2,"tome":3,"chapter":"250","name":"Paid","price":"15.00","is_published":true,"is_paid":true}]}`)
		case "2":
			fmt.Fprint(w, `{"next":null,"previous":1,"results":[`+
				`{"id":320521,"is_bought":false,"publishers":[],"index":1,"tome":1,"chapter":"1","name":"Start","price":null,"is_published":true,"is_paid":false}]}`)
		default:
			t.Errorf("unexpected page %q", q.Get("page"))
			fmt.Fprint(w, `{"next":null,"results":[]}`)
		}
	}))
	mux.HandleFunc("/api/v2/titles/chapters/320521/", withReferer(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":320521,"tome":1,"chapter":"1","is_paid":false,`+
			`"server":{"id":5,"name":"img3","link":"https://img3.reimg.org/","fallback_link":"https://img3.reimg2.org/"},`+
			`"pages":[[{"id":1,"link":"https://img3.reimg2.org/images/11004/320521/a.jpeg","height":1472,"width":1440},`+
			`{"id":2,"link":"https://img3.reimg2.org/images/11004/320521/b.jpeg"}],`+
			`[{"id":3,"link":"https://img3.reimg2.org/images/11004/320521/c.jpeg"}]]}`)
	}))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestRemangaSeries(t *testing.T) {
	server := remangaServer(t)
	old := remangaApi
	remangaApi = server.URL + "/api/v2"
	defer func() { remangaApi = old }()

	r := NewRemanga(&Grabber{URL: "https://remanga.org/manga/murim-login_/320521?page=1", Settings: &Settings{}})

	title, err := r.FetchTitle()
	if err != nil || title != "Логин Мурим" {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}

	chapters, errs := r.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters() errors = %v", errs)
	}
	want := []struct {
		id     string
		number float64
		volume int
		locked bool
	}{
		{"2818816", 206.5, 4, false},
		{"1937253", 250, 3, true},
		{"320521", 1, 1, false},
	}
	if len(chapters) != len(want) {
		t.Fatalf("FetchChapters() = %d chapters, want %d", len(chapters), len(want))
	}
	for i, w := range want {
		c := chapters[i].(*RemangaChapter)
		if c.ID != w.id || c.Number != w.number || c.Volume != w.volume || c.Locked != w.locked {
			t.Errorf("chapter %d = (%q, %v, vol %d, locked %v), want %+v", i, c.ID, c.Number, c.Volume, c.Locked, w)
		}
		// a chapter listing no publisher takes its branch's
		if c.Group != "Newlate.WTF" || c.Language != "ru" {
			t.Errorf("chapter %d group %q, language %q", i, c.Group, c.Language)
		}
	}

	chapter, err := r.FetchChapter(chapters[2])
	if err != nil {
		t.Fatalf("FetchChapter() error = %v", err)
	}
	if chapter.ID != "320521" || chapter.Title != "Start" || chapter.Group != "Newlate.WTF" || chapter.PagesCount != 3 {
		t.Errorf("FetchChapter() = ID %q, Title %q, Group %q, %d pages", chapter.ID, chapter.Title, chapter.Group, chapter.PagesCount)
	}
	wantPages := []string{"a", "b", "c"}
	for i, p := range chapter.Pages {
		if want := "https://img3.reimg2.org/images/11004/320521/" + wantPages[i] + ".jpeg"; p.URL != want || p.Number != int64(i+1) {
			t.Errorf("page %d = %d %q, want %q", i, p.Number, p.URL, want)
		}
	}
}

func TestRemangaFlattenFlatPages(t *testing.T) {
	pages, err := remangaFlattenPages([]byte(`[{"link":"https://x/1.jpg"},{"link":""},{"link":"https://x/2.jpg"}]`))
	if err != nil || len(pages) != 2 || pages[1].Link != "https://x/2.jpg" {
		t.Errorf("remangaFlattenPages() = %+v, %v", pages, err)
	}
}
