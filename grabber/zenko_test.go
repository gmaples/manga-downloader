// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestZenkoTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
		id   string
	}{
		{"https://zenko.online/titles/1225", true, "1225"},
		// a reader URL (the second number is a chapter id) loads its series
		{"https://zenko.online/titles/1225/20377", true, "1225"},
		{"https://www.zenko.online/titles/1225?x=1", true, "1225"},
		{"https://zenko.online/", false, ""},
		{"https://zenko.online/titles/abc", false, ""},
		{"https://notzenko.online/titles/1225", false, ""},
	}

	for _, c := range cases {
		z := &Zenko{Grabber: &Grabber{URL: c.url}}
		got, err := z.Test()
		if err != nil || got != c.want {
			t.Errorf("Test(%q) = %v, %v, want %v", c.url, got, err, c.want)
			continue
		}
		if !c.want {
			continue
		}
		if id, err := z.seriesID(); err != nil || id != c.id {
			t.Errorf("seriesID(%q) = %q, %v, want %q", c.url, id, err, c.id)
		}
	}
}

const zenkoSep = "@#%&;№%#&**#!@"

// zenkoServer serves a fake api in the shapes the Zenko reader extension
// (keiyoushi) reads: a title, a flat chapter list, and a chapter's pages
func zenkoServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	withReferer := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Referer") != "https://zenko.online/" {
				t.Errorf("%s: Referer %q", r.URL, r.Header.Get("Referer"))
			}
			next(w, r)
		}
	}
	mux.HandleFunc("/titles/1225", withReferer(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":1225,"name":"Назва","engName":"Name","coverImg":"c.jpg"}`)
	}))
	mux.HandleFunc("/titles/1225/chapters", withReferer(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[`+
			`{"id":20378,"name":"2`+zenkoSep+`12.5`+zenkoSep+`Кінець","titleId":1225,"publisher":{"name":"Team"}},`+
			`{"id":20377,"name":"1`+zenkoSep+`1","titleId":1225,"publisher":null},`+
			`{"id":20376,"name":"Анонс","titleId":1225}]`)
	}))
	mux.HandleFunc("/chapters/20377", withReferer(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":20377,"name":"1`+zenkoSep+`1","titleId":1225,"pages":[`+
			`{"order":2,"content":"b.webp"},{"order":1,"content":"a.webp"},{"order":3,"content":""}]}`)
	}))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestZenkoSeries(t *testing.T) {
	server := zenkoServer(t)
	oldApi, oldStorage := zenkoApi, zenkoStorage
	zenkoApi, zenkoStorage = server.URL, "https://storage.example"
	defer func() { zenkoApi, zenkoStorage = oldApi, oldStorage }()

	z := NewZenko(&Grabber{URL: "https://zenko.online/titles/1225/20377", Settings: &Settings{}})

	title, err := z.FetchTitle()
	if err != nil || title != "Назва" {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}

	chapters, errs := z.FetchChapters()
	// the announcement has no number and is reported, not listed
	if len(errs) != 1 {
		t.Errorf("FetchChapters() errors = %v, want 1", errs)
	}
	want := []struct {
		id     string
		number float64
		volume int
		label  string
		group  string
	}{
		{"20378", 12.5, 2, "Кінець", "Team"},
		{"20377", 1, 1, "Chapter 1", ""},
	}
	if len(chapters) != len(want) {
		t.Fatalf("FetchChapters() = %d chapters, want %d", len(chapters), len(want))
	}
	for i, w := range want {
		c := chapters[i].(*ZenkoChapter)
		if c.ID != w.id || c.Number != w.number || c.Volume != w.volume || ChapterLabel(c) != w.label || c.Group != w.group || c.Language != "uk" {
			t.Errorf("chapter %d = (%q, %v, vol %d, %q, %q, %q), want %+v", i, c.ID, c.Number, c.Volume, ChapterLabel(c), c.Group, c.Language, w)
		}
	}

	chapter, err := z.FetchChapter(chapters[1])
	if err != nil {
		t.Fatalf("FetchChapter() error = %v", err)
	}
	if chapter.ID != "20377" || chapter.PagesCount != 2 || ChapterLabel(chapter) != "Chapter 1" {
		t.Errorf("FetchChapter() = ID %q, %d pages, label %q", chapter.ID, chapter.PagesCount, ChapterLabel(chapter))
	}
	for i, name := range []string{"a", "b"} {
		if p := chapter.Pages[i]; p.URL != "https://storage.example/"+name+".webp" || p.Number != int64(i+1) {
			t.Errorf("page %d = %d %q", i, p.Number, p.URL)
		}
	}
}
