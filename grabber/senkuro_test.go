// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSenkuroTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
		slug string
	}{
		{"https://senkuro.com/manga/spy-x-family", true, "spy-x-family"},
		{"https://senkuro.com/manga/spy-x-family/chapters", true, "spy-x-family"},
		// a reader URL loads its series
		{"https://senkuro.com/manga/spy-x-family/chapters/104666807805174145/pages/1", true, "spy-x-family"},
		{"https://www.senkuro.com/manga/illegsideu?tab=chapters", true, "illegsideu"},
		{"https://senkuro.com/", false, ""},
		{"https://senkuro.com/manga/", false, ""},
		{"https://notsenkuro.com/manga/spy-x-family", false, ""},
	}

	for _, c := range cases {
		s := &Senkuro{Grabber: &Grabber{URL: c.url}}
		got, err := s.Test()
		if err != nil || got != c.want {
			t.Errorf("Test(%q) = %v, %v, want %v", c.url, got, err, c.want)
			continue
		}
		if !c.want {
			continue
		}
		if slug, err := s.slug(); err != nil || slug != c.slug {
			t.Errorf("slug(%q) = %q, %v, want %q", c.url, slug, err, c.slug)
		}
	}
}

// TestSenkuroIdentified checks a senkuro.com URL picks this grabber, not the
// generic reader that used to take it (and stopped at the first 100 chapters)
func TestSenkuroIdentified(t *testing.T) {
	site, errs := NewSite("https://senkuro.com/manga/spy-x-family", &Settings{})
	if _, ok := site.(*Senkuro); !ok {
		t.Fatalf("NewSite() = %T, %v, want *Senkuro", site, errs)
	}
}

// senkuroServer answers the three queries the grabber sends, in the shapes a
// live probe of api.senkuro.com saw (2026-11-22): the series, its chapters in
// pages joined by a cursor (2 chapters a page here, 100 on the site), and a
// chapter's pages
func senkuroServer(t *testing.T, rating string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("%s with Content-Type %q, want a JSON POST", r.Method, r.Header.Get("Content-Type"))
		}
		req := struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		switch {
		case strings.Contains(req.Query, "manga(slug"):
			if req.Variables["slug"] != "spy-x-family" {
				fmt.Fprint(w, `{"data":{"manga":null}}`)
				return
			}
			fmt.Fprintf(w, `{"data":{"manga":{"id":"M1","slug":"spy-x-family","rating":%q,`+
				`"titles":[{"lang":"EN","content":"Spy x Family"},{"lang":"RU","content":"Семья шпиона"}],`+
				`"originalName":{"lang":"JA","content":"Spy x Family"},`+
				`"branches":[{"id":"B1","lang":"RU","primaryBranch":true}]}}}`, rating)
		case strings.Contains(req.Query, "mangaChapters"):
			if req.Variables["branchId"] != "B1" {
				t.Errorf("branchId = %v", req.Variables["branchId"])
			}
			switch req.Variables["after"] {
			case nil:
				fmt.Fprint(w, `{"data":{"mangaChapters":{"edges":[`+
					`{"node":{"id":"C1","slug":"101","name":"Миссия 1-я","number":"1","volume":"1","teams":[{"name":"Unnamed Manga"}]}},`+
					`{"node":{"id":"C2","slug":"102","name":null,"number":"2","volume":"1","teams":[]}}],`+
					`"pageInfo":{"hasNextPage":true,"endCursor":"CUR1"}}}}`)
			case "CUR1":
				fmt.Fprint(w, `{"data":{"mangaChapters":{"edges":[`+
					`{"node":{"id":"C3","slug":"1045","name":"Специальная иллюстрация","number":"64.5","volume":"10","teams":[{"name":"Unnamed Manga"}]}},`+
					`{"node":{"id":"C4","slug":"999","name":null,"number":"","volume":null}}],`+
					`"pageInfo":{"hasNextPage":false,"endCursor":"CUR2"}}}}`)
			default:
				t.Errorf("after = %v", req.Variables["after"])
			}
		case strings.Contains(req.Query, "mangaChapter(slug"):
			switch req.Variables["slug"] {
			case "1045":
				// 64.5 is one landscape page; pages come out of order and a
				// URL can carry a stray line end
				fmt.Fprint(w, `{"data":{"mangaChapter":{"slug":"1045","pages":[`+
					`{"number":2,"image":{"original":{"url":"https://mirai.example/b.jpeg\r\n"}}},`+
					`{"number":1,"image":{"original":{"url":"https://mirai.example/a.jpeg"}}},`+
					`{"number":3,"image":null}]}}}`)
			case "102":
				fmt.Fprint(w, `{"data":{"mangaChapter":{"slug":"102","pages":[]}}}`)
			default:
				fmt.Fprint(w, `{"data":{"mangaChapter":null}}`)
			}
		default:
			fmt.Fprint(w, `{"data":null,"errors":[{"message":"Unknown field \"x\" on type \"Query\"."}]}`)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestSenkuroSeries(t *testing.T) {
	server := senkuroServer(t, "SENSITIVE")
	old := senkuroApi
	senkuroApi = server.URL
	defer func() { senkuroApi = old }()

	s := NewSenkuro(&Grabber{URL: "https://senkuro.com/manga/spy-x-family/chapters/101/pages/1", Settings: &Settings{}})

	title, err := s.FetchTitle()
	if err != nil || title != "Семья шпиона" {
		t.Fatalf("FetchTitle() = %q, %v, want the Russian title", title, err)
	}
	if s.AgeLimit() != 12 {
		t.Errorf("AgeLimit() = %d, want 12", s.AgeLimit())
	}

	chapters, errs := s.FetchChapters()
	// the chapter with no number is reported, not listed
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
		{"101", 1, 1, "Миссия 1-я", "Unnamed Manga"},
		{"102", 2, 1, "Chapter 2", ""},
		{"1045", 64.5, 10, "Специальная иллюстрация", "Unnamed Manga"},
	}
	if len(chapters) != len(want) {
		t.Fatalf("FetchChapters() = %d chapters, want %d (both pages)", len(chapters), len(want))
	}
	for i, w := range want {
		c := chapters[i].(*SenkuroChapter)
		if c.ID != w.id || c.Number != w.number || c.Volume != w.volume || ChapterLabel(c) != w.label || c.Group != w.group || c.Language != "ru" {
			t.Errorf("chapter %d = (%q, %v, vol %d, %q, %q, %q), want %+v", i, c.ID, c.Number, c.Volume, ChapterLabel(c), c.Group, c.Language, w)
		}
	}

	chapter, err := s.FetchChapter(chapters[2])
	if err != nil {
		t.Fatalf("FetchChapter() error = %v", err)
	}
	if chapter.ID != "1045" || chapter.PagesCount != 2 || ChapterLabel(chapter) != ChapterLabel(chapters[2]) || chapter.Group != "Unnamed Manga" || chapter.Volume != 10 {
		t.Errorf("FetchChapter() = %+v", chapter)
	}
	urls := []string{"https://mirai.example/a.jpeg", "https://mirai.example/b.jpeg"}
	for i, u := range urls {
		if chapter.Pages[i].URL != u || chapter.Pages[i].Number != int64(i+1) {
			t.Errorf("page %d = %+v, want %s", i, chapter.Pages[i], u)
		}
	}

	if _, err := s.FetchChapter(chapters[1]); err == nil || !strings.Contains(err.Error(), "no pages") {
		t.Errorf("FetchChapter(empty) error = %v, want no pages", err)
	}
}

func TestSenkuroUnknownSeries(t *testing.T) {
	server := senkuroServer(t, "GENERAL")
	old := senkuroApi
	senkuroApi = server.URL
	defer func() { senkuroApi = old }()

	s := NewSenkuro(&Grabber{URL: "https://senkuro.com/manga/nope", Settings: &Settings{}})
	if _, err := s.FetchTitle(); err == nil || !strings.Contains(err.Error(), "no series") {
		t.Errorf("FetchTitle() error = %v, want no series", err)
	}
}

func TestSenkuroAgeCheck(t *testing.T) {
	server := senkuroServer(t, "EXPLICIT")
	old := senkuroApi
	senkuroApi = server.URL
	defer func() { senkuroApi = old }()

	s := NewSenkuro(&Grabber{URL: "https://senkuro.com/manga/spy-x-family", Settings: &Settings{AgeCheck: true, Age: AgeTeen}})
	if _, err := s.FetchTitle(); err == nil {
		t.Error("FetchTitle() of an 18+ title with the age check at 16 succeeded, want refused")
	}
	if s.AgeLimit() != 18 {
		t.Errorf("AgeLimit() = %d, want 18", s.AgeLimit())
	}
}
