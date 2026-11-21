// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"

	"github.com/elboletaire/manga-downloader/http"
)

// remangaApi is ReManga's JSON api. The site (remanga.org) is a Next.js app
// behind DDoS-Guard, but the series page is served without a challenge and
// this api answers anonymously. Checked 2026-11-20 from a home connection:
// /api/v2/titles/{dir}/ gives the title and its translation branches,
// /api/v2/titles/chapters/?branch_id= a paginated chapter list ("next" is
// the next page number) and /api/v2/titles/chapters/{id}/ the pages; the old
// /api/titles/{dir}/ answers 404. A var so tests can point it at a server.
var remangaApi = "https://api.remanga.org/api/v2"

// remangaSite is sent as the Referer of api calls, as the site itself does
const remangaSite = "https://remanga.org/"

// remangaPageSize is the chapters asked for per page of the list
const remangaPageSize = 40

// remangaMaxPages bounds the chapter list walk, in case "next" never ends
const remangaMaxPages = 1000

// Remanga is a grabber for remanga.org
type Remanga struct {
	*Grabber
	title    string
	branches []remangaBranch
}

func NewRemanga(g *Grabber) *Remanga {
	return &Remanga{Grabber: g}
}

// RemangaChapter is a chapter of one translation branch, its ID being the
// api's chapter id
type RemangaChapter struct {
	Chapter
}

var remangaURLRe = regexp.MustCompile(`^https?://(?:www\.)?remanga\.org/manga/([^/?#]+)`)

// Test returns true for a remanga.org series or reader URL
func (r *Remanga) Test() (bool, error) {
	return remangaURLRe.MatchString(r.URL), nil
}

// seriesDir returns the series' "dir" from the URL. A reader URL
// (/manga/{dir}/{chapter id}) gives the same dir as its series, so a pasted
// chapter link loads the whole series.
func (r Remanga) seriesDir() (string, error) {
	m := remangaURLRe.FindStringSubmatch(r.URL)
	if len(m) != 2 {
		return "", fmt.Errorf("could not find the series in url %s", r.URL)
	}
	return m[1], nil
}

func remangaParams(u string) http.RequestParams {
	return http.RequestParams{URL: u, Referer: remangaSite}
}

// remangaBranch is one translation branch of a title, and its publishers
// the groups that translate it
type remangaBranch struct {
	ID         int                `json:"id"`
	Publishers []remangaPublisher `json:"publishers"`
}

type remangaPublisher struct {
	Name string `json:"name"`
}

// fetchTitle reads the title and its branches, once
func (r *Remanga) fetchTitle() error {
	if r.title != "" {
		return nil
	}
	dir, err := r.seriesDir()
	if err != nil {
		return err
	}

	body, err := http.GetText(remangaParams(remangaApi + "/titles/" + dir + "/"))
	if err != nil {
		return err
	}

	feed := struct {
		MainName      string          `json:"main_name"`
		SecondaryName string          `json:"secondary_name"`
		Branches      []remangaBranch `json:"branches"`
	}{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return err
	}

	title := feed.MainName
	if title == "" {
		title = feed.SecondaryName
	}
	if title == "" {
		return fmt.Errorf("no title found for %s", dir)
	}
	r.title = sanitizeTitle(title)
	r.branches = feed.Branches

	return nil
}

// FetchTitle fetches and returns the series' title (its Russian main name)
func (r *Remanga) FetchTitle() (string, error) {
	if err := r.fetchTitle(); err != nil {
		return "", err
	}
	return r.title, nil
}

// remangaChapterEntry is a chapter of the list
type remangaChapterEntry struct {
	ID         int                `json:"id"`
	Tome       int                `json:"tome"`
	Chapter    string             `json:"chapter"`
	Name       string             `json:"name"`
	IsPaid     bool               `json:"is_paid"`
	IsBought   bool               `json:"is_bought"`
	Publishers []remangaPublisher `json:"publishers"`
}

// FetchChapters returns every branch's chapters. A chapter the list marks
// is_paid (and not bought) is locked; a price alone isn't, since it stays on
// early-access chapters that are free now.
func (r *Remanga) FetchChapters() (chapters Filterables, errs []error) {
	if err := r.fetchTitle(); err != nil {
		return nil, []error{err}
	}

	for _, b := range r.branches {
		page := 1
		for n := 0; n < remangaMaxPages && page > 0; n++ {
			u := fmt.Sprintf("%s/titles/chapters/?branch_id=%d&ordering=-index&page=%d&count=%d", remangaApi, b.ID, page, remangaPageSize)
			body, err := http.GetText(remangaParams(u))
			if err != nil {
				errs = append(errs, err)
				break
			}

			feed := struct {
				Next    *int                  `json:"next"`
				Results []remangaChapterEntry `json:"results"`
			}{}
			if err = json.Unmarshal([]byte(body), &feed); err != nil {
				errs = append(errs, err)
				break
			}

			for _, c := range feed.Results {
				num, err := strconv.ParseFloat(c.Chapter, 64)
				if err != nil {
					errs = append(errs, fmt.Errorf("chapter %d: %w", c.ID, err))
					continue
				}
				chapter := &RemangaChapter{Chapter{
					Number:   num,
					Title:    c.Name,
					Language: "ru",
					Volume:   c.Tome,
					ID:       strconv.Itoa(c.ID),
					Locked:   c.IsPaid && !c.IsBought,
				}}
				publishers := c.Publishers
				if len(publishers) == 0 {
					publishers = b.Publishers
				}
				chapter.SetGroups(remangaGroups(publishers)...)
				chapters = append(chapters, chapter)
			}

			if feed.Next == nil || len(feed.Results) == 0 {
				break
			}
			page = *feed.Next
		}
	}

	return chapters, errs
}

// remangaPage is one image of a chapter
type remangaPage struct {
	Link string `json:"link"`
}

// FetchChapter fetches a chapter's pages. The api lists them as groups of
// strips (one tall page cut in pieces), so they're flattened in order. Each
// link is used as given: its host is the server's fallback, and the same
// path on the server's main host answered 403.
func (r *Remanga) FetchChapter(f Filterable) (*Chapter, error) {
	rchap := f.(*RemangaChapter)

	body, err := http.GetText(remangaParams(remangaApi + "/titles/chapters/" + rchap.ID + "/"))
	if err != nil {
		return nil, err
	}

	feed := struct {
		Pages json.RawMessage `json:"pages"`
	}{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, err
	}
	pages, err := remangaFlattenPages(feed.Pages)
	if err != nil {
		return nil, fmt.Errorf("chapter %s: %w", rchap.ID, err)
	}

	chapter := &Chapter{
		Title:      rchap.GetTitle(),
		Number:     rchap.GetNumber(),
		Volume:     rchap.Volume,
		Language:   "ru",
		ID:         rchap.ID,
		PagesCount: int64(len(pages)),
	}
	chapter.SetGroups(rchap.GetGroups()...)
	for i, p := range pages {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    p.Link,
		})
	}

	return chapter, nil
}

// remangaFlattenPages reads "pages" as a list of strip groups, or as a flat
// list, leaving out entries without a link
func remangaFlattenPages(raw json.RawMessage) ([]remangaPage, error) {
	var groups [][]remangaPage
	if err := json.Unmarshal(raw, &groups); err != nil {
		var flat []remangaPage
		if err2 := json.Unmarshal(raw, &flat); err2 != nil {
			return nil, fmt.Errorf("unexpected pages: %w", err)
		}
		groups = [][]remangaPage{flat}
	}

	var pages []remangaPage
	for _, g := range groups {
		for _, p := range g {
			if p.Link != "" {
				pages = append(pages, p)
			}
		}
	}
	return pages, nil
}

func remangaGroups(publishers []remangaPublisher) []string {
	var names []string
	for _, p := range publishers {
		if p.Name != "" {
			names = append(names, p.Name)
		}
	}
	return names
}
