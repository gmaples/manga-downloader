// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
)

// senkuroApi is Senkuro's GraphQL endpoint, the one its own pages call. A var
// so tests can point it at a server.
var senkuroApi = "https://api.senkuro.com/graphql"

// senkuroSite is sent as the Referer of api calls
const senkuroSite = "https://senkuro.com/"

// senkuroMaxPages caps the chapter list walk (100 chapters a page), so a
// cursor that never ends can't loop forever
const senkuroMaxPages = 200

// senkuroRatings are the site's age ratings as minimum ages
var senkuroRatings = map[string]int{
	"GENERAL":      0,
	"SENSITIVE":    12,
	"QUESTIONABLE": 16,
	"EXPLICIT":     18,
}

// Senkuro is a grabber for senkuro.com. Its pages are a Vue app whose
// chapter list shows the first 100 chapters only; the api pages through all
// of them.
type Senkuro struct {
	*Grabber
	title    string
	ageLimit int
	branches []senkuroBranch
}

type senkuroBranch struct {
	ID   string `json:"id"`
	Lang string `json:"lang"`
}

// AgeLimit is the title's age rating (0, 12, 16 or 18), once FetchTitle read it
func (s *Senkuro) AgeLimit() int {
	return s.ageLimit
}

func NewSenkuro(g *Grabber) *Senkuro {
	return &Senkuro{Grabber: g}
}

// SenkuroChapter is a chapter, its ID being the chapter's slug, which is what
// the api fetches a chapter's pages by
type SenkuroChapter struct {
	Chapter
}

// senkuroURLRe matches a series URL, its chapter list
// (/manga/{slug}/chapters) and a reader URL
// (/manga/{slug}/chapters/{chapter slug}/pages/1), so a pasted chapter link
// loads the whole series
var senkuroURLRe = regexp.MustCompile(`^https?://(?:www\.)?senkuro\.com/manga/([^/?#]+)`)

// Test returns true for a senkuro.com series or reader URL
func (s *Senkuro) Test() (bool, error) {
	return senkuroURLRe.MatchString(s.URL), nil
}

// slug returns the series slug from the URL
func (s Senkuro) slug() (string, error) {
	m := senkuroURLRe.FindStringSubmatch(s.URL)
	if len(m) != 2 {
		return "", fmt.Errorf("could not find the series in url %s", s.URL)
	}
	return m[1], nil
}

// senkuroQuery posts a query and decodes the response's data into out
func senkuroQuery(query string, variables map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return err
	}
	text, err := http.PostText(http.RequestParams{
		URL:     senkuroApi,
		Referer: senkuroSite,
		Accept:  "application/json",
		Body:    string(body),
		Headers: map[string]string{"Content-Type": "application/json"},
	})
	if err != nil {
		return err
	}

	feed := struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}{}
	if err = json.Unmarshal([]byte(text), &feed); err != nil {
		return err
	}
	if len(feed.Errors) > 0 {
		return fmt.Errorf("senkuro api: %s", feed.Errors[0].Message)
	}
	return json.Unmarshal(feed.Data, out)
}

const senkuroMangaQuery = `query($slug: String!) { manga(slug: $slug) { id slug rating ` +
	`titles { lang content } originalName { lang content } branches { id lang primaryBranch } } }`

type senkuroName struct {
	Lang    string `json:"lang"`
	Content string `json:"content"`
}

// FetchTitle fetches and returns the series' title: its Russian one, else its
// English one, else the original name
func (s *Senkuro) FetchTitle() (string, error) {
	if s.title != "" {
		return s.title, nil
	}
	slug, err := s.slug()
	if err != nil {
		return "", err
	}

	feed := struct {
		Manga *struct {
			Rating       string          `json:"rating"`
			Titles       []senkuroName   `json:"titles"`
			OriginalName *senkuroName    `json:"originalName"`
			Branches     []senkuroBranch `json:"branches"`
		} `json:"manga"`
	}{}
	if err = senkuroQuery(senkuroMangaQuery, map[string]any{"slug": slug}, &feed); err != nil {
		return "", err
	}
	if feed.Manga == nil {
		return "", fmt.Errorf("senkuro has no series %q", slug)
	}
	s.ageLimit = senkuroRatings[feed.Manga.Rating]
	if err = s.Settings.AgeAllows(s.ageLimit); err != nil {
		return "", err
	}
	s.branches = feed.Manga.Branches

	names := map[string]string{}
	for _, t := range feed.Manga.Titles {
		if t.Content != "" && names[t.Lang] == "" {
			names[t.Lang] = t.Content
		}
	}
	if o := feed.Manga.OriginalName; o != nil && names["original"] == "" {
		names["original"] = o.Content
	}
	for _, lang := range []string{"RU", "EN", "original"} {
		if t := strings.TrimSpace(names[lang]); t != "" {
			s.title = sanitizeTitle(t)
			return s.title, nil
		}
	}
	return "", fmt.Errorf("no title found for %s", slug)
}

const senkuroChaptersQuery = `query($branchId: ID!, $after: String) { ` +
	`mangaChapters(first: 100, branchId: $branchId, after: $after, orderBy: {field: NUMBER, direction: ASC}) { ` +
	`edges { node { id slug name number volume teams { name } } } pageInfo { hasNextPage endCursor } } }`

type senkuroChapterNode struct {
	Slug   string     `json:"slug"`
	Name   *string    `json:"name"`
	Number string     `json:"number"`
	Volume jsonVolume `json:"volume"`
	Teams  []struct {
		Name string `json:"name"`
	} `json:"teams"`
}

// FetchChapters returns every chapter of every branch (a branch is one
// language's translation), walking the api's pages of 100
func (s *Senkuro) FetchChapters() (chapters Filterables, errs []error) {
	// the title carries the age rating and the branches
	if _, err := s.FetchTitle(); err != nil {
		return nil, []error{err}
	}
	if len(s.branches) == 0 {
		return nil, []error{fmt.Errorf("senkuro lists no translation of this series")}
	}

	for _, b := range s.branches {
		lang := strings.ToLower(b.Lang)
		after := ""
		for page := 0; ; page++ {
			if page == senkuroMaxPages {
				errs = append(errs, fmt.Errorf("branch %s: stopped after %d pages of chapters", b.ID, page))
				break
			}
			feed := struct {
				MangaChapters struct {
					Edges []struct {
						Node senkuroChapterNode `json:"node"`
					} `json:"edges"`
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
				} `json:"mangaChapters"`
			}{}
			vars := map[string]any{"branchId": b.ID}
			if after != "" {
				vars["after"] = after
			}
			if err := senkuroQuery(senkuroChaptersQuery, vars, &feed); err != nil {
				errs = append(errs, err)
				break
			}

			for _, e := range feed.MangaChapters.Edges {
				c := e.Node
				num, err := strconv.ParseFloat(c.Number, 64)
				if err != nil {
					errs = append(errs, fmt.Errorf("chapter %s: no number in %q", c.Slug, c.Number))
					continue
				}
				title := ""
				if c.Name != nil {
					title = strings.TrimSpace(*c.Name)
				}
				chapter := &SenkuroChapter{Chapter{
					Number:   num,
					Title:    title,
					Language: lang,
					Volume:   int(c.Volume),
					ID:       c.Slug,
					// many chapters have no name: "Chapter 12" then
					Label: Label{Style: LabelNumbered, Number: c.Number},
				}}
				var teams []string
				for _, t := range c.Teams {
					if t.Name != "" {
						teams = append(teams, t.Name)
					}
				}
				if len(teams) > 0 {
					chapter.SetGroups(teams...)
				}
				chapters = append(chapters, chapter)
			}

			next := feed.MangaChapters.PageInfo
			if !next.HasNextPage || next.EndCursor == "" || next.EndCursor == after {
				break
			}
			after = next.EndCursor
		}
	}

	return chapters, errs
}

const senkuroPagesQuery = `query($slug: String!) { mangaChapter(slug: $slug) { slug ` +
	`pages { number image { original { url } } } } }`

// FetchChapter fetches a chapter's pages, in the site's page order, at their
// original size
func (s *Senkuro) FetchChapter(f Filterable) (*Chapter, error) {
	schap := f.(*SenkuroChapter)

	feed := struct {
		MangaChapter *struct {
			Pages []struct {
				Number int `json:"number"`
				Image  *struct {
					Original *struct {
						URL string `json:"url"`
					} `json:"original"`
				} `json:"image"`
			} `json:"pages"`
		} `json:"mangaChapter"`
	}{}
	if err := senkuroQuery(senkuroPagesQuery, map[string]any{"slug": schap.ID}, &feed); err != nil {
		return nil, err
	}
	if feed.MangaChapter == nil {
		return nil, fmt.Errorf("chapter %s: not found", schap.ID)
	}

	type page struct {
		number int
		url    string
	}
	var pages []page
	for _, p := range feed.MangaChapter.Pages {
		if p.Image == nil || p.Image.Original == nil {
			continue
		}
		if u := strings.TrimSpace(p.Image.Original.URL); u != "" {
			pages = append(pages, page{p.Number, u})
		}
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("chapter %s: no pages", schap.ID)
	}
	sort.SliceStable(pages, func(i, j int) bool { return pages[i].number < pages[j].number })

	chapter := &Chapter{
		Title:      schap.GetTitle(),
		Number:     schap.GetNumber(),
		Volume:     schap.Volume,
		Language:   schap.Language,
		ID:         schap.ID,
		Label:      schap.Label,
		PagesCount: int64(len(pages)),
	}
	chapter.SetGroups(schap.GetGroups()...)
	for i, p := range pages {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    p.url,
		})
	}

	return chapter, nil
}
