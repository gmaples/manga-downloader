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

// zenkoApi is Zenko's JSON api: /titles/{id} gives the title,
// /titles/{id}/chapters every chapter and /chapters/{id} a chapter's pages.
// A var so tests can point it at a server.
var zenkoApi = "https://api.zenko.online"

// zenkoStorage is where the api's page and cover paths are served from
var zenkoStorage = "https://storage.zenko.online"

// zenkoSite is sent as the Referer of api calls, as the site itself does
const zenkoSite = "https://zenko.online/"

// zenkoSeparator splits a chapter's "name" into volume, chapter number and
// title: "1@#%&;№%#&**#!@12@#%&;№%#&**#!@Title". Fewer parts drop the title,
// then the number.
const zenkoSeparator = "@#%&;№%#&**#!@"

// Zenko is a grabber for zenko.online
type Zenko struct {
	*Grabber
	title    string
	ageLimit int
}

// AgeLimit is the title's age rating (0, 16 or 18), once FetchTitle read it
func (z *Zenko) AgeLimit() int {
	return z.ageLimit
}

func NewZenko(g *Grabber) *Zenko {
	return &Zenko{Grabber: g}
}

// ZenkoChapter is a chapter, its ID being the api's chapter id
type ZenkoChapter struct {
	Chapter
}

var zenkoURLRe = regexp.MustCompile(`^https?://(?:www\.)?zenko\.online/titles/(\d+)`)

// Test returns true for a zenko.online series or reader URL
func (z *Zenko) Test() (bool, error) {
	return zenkoURLRe.MatchString(z.URL), nil
}

// seriesID returns the series id from the URL. A reader URL
// (/titles/{id}/{chapter id}) gives its series' id, so a pasted chapter link
// loads the whole series.
func (z Zenko) seriesID() (string, error) {
	m := zenkoURLRe.FindStringSubmatch(z.URL)
	if len(m) != 2 {
		return "", fmt.Errorf("could not find the series in url %s", z.URL)
	}
	return m[1], nil
}

func zenkoParams(u string) http.RequestParams {
	return http.RequestParams{URL: u, Referer: zenkoSite}
}

// FetchTitle fetches and returns the series' title (its Ukrainian name)
func (z *Zenko) FetchTitle() (string, error) {
	if z.title != "" {
		return z.title, nil
	}
	id, err := z.seriesID()
	if err != nil {
		return "", err
	}

	body, err := http.GetText(zenkoParams(zenkoApi + "/titles/" + id))
	if err != nil {
		return "", err
	}

	feed := struct {
		Name         string `json:"name"`
		EngName      string `json:"engName"`
		OriginalName string `json:"originalName"`
		// AgeLimit is the title's minimum age (0, 16, 18). The nested
		// author object has its own, a string, which is the uploader's
		AgeLimit int `json:"ageLimit"`
	}{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return "", err
	}
	z.ageLimit = feed.AgeLimit
	// the site's own age gate is on its pages only: the api serves every
	// title, so the user's age check is applied here
	if err = z.Settings.AgeAllows(feed.AgeLimit); err != nil {
		return "", err
	}

	for _, t := range []string{feed.Name, feed.EngName, feed.OriginalName} {
		if t != "" {
			z.title = sanitizeTitle(t)
			return z.title, nil
		}
	}
	return "", fmt.Errorf("no title found for %s", id)
}

// zenkoChapterEntry is a chapter of the list, or the chapter itself
type zenkoChapterEntry struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Publisher *struct {
		Name string `json:"name"`
	} `json:"publisher"`
	Pages []zenkoPage `json:"pages"`
}

type zenkoPage struct {
	Order   int    `json:"order"`
	Content string `json:"content"`
}

// zenkoName splits a chapter's name into its volume, number and title
func zenkoName(name string) (volume, number, title string) {
	parts := strings.Split(name, zenkoSeparator)
	switch len(parts) {
	case 3:
		return parts[0], parts[1], parts[2]
	case 2:
		return parts[0], parts[1], ""
	default:
		return "", "", name
	}
}

// FetchChapters returns the series' chapters
func (z *Zenko) FetchChapters() (chapters Filterables, errs []error) {
	// the title carries the age rating, so it's checked before the list
	if _, err := z.FetchTitle(); err != nil {
		return nil, []error{err}
	}
	id, err := z.seriesID()
	if err != nil {
		return nil, []error{err}
	}

	body, err := http.GetText(zenkoParams(zenkoApi + "/titles/" + id + "/chapters"))
	if err != nil {
		return nil, []error{err}
	}

	var feed []zenkoChapterEntry
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, []error{err}
	}

	for _, c := range feed {
		volume, number, title := zenkoName(c.Name)
		num, err := strconv.ParseFloat(number, 64)
		if err != nil {
			errs = append(errs, fmt.Errorf("chapter %d: no number in %q", c.ID, c.Name))
			continue
		}
		vol, _ := strconv.Atoi(volume)
		chapter := &ZenkoChapter{Chapter{
			Number:   num,
			Title:    title,
			Language: "uk",
			Volume:   vol,
			ID:       strconv.Itoa(c.ID),
			// most chapters have no title: "Chapter 12" then
			Label: Label{Style: LabelNumbered, Number: number},
		}}
		if c.Publisher != nil && c.Publisher.Name != "" {
			chapter.SetGroups(c.Publisher.Name)
		}
		chapters = append(chapters, chapter)
	}

	return chapters, errs
}

// FetchChapter fetches a chapter's pages, in the api's page order
func (z *Zenko) FetchChapter(f Filterable) (*Chapter, error) {
	zchap := f.(*ZenkoChapter)

	body, err := http.GetText(zenkoParams(zenkoApi + "/chapters/" + zchap.ID))
	if err != nil {
		return nil, err
	}

	var feed zenkoChapterEntry
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, err
	}

	pages := make([]zenkoPage, 0, len(feed.Pages))
	for _, p := range feed.Pages {
		if p.Content != "" {
			pages = append(pages, p)
		}
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("chapter %s: no pages", zchap.ID)
	}
	sort.SliceStable(pages, func(i, j int) bool { return pages[i].Order < pages[j].Order })

	chapter := &Chapter{
		Title:      zchap.GetTitle(),
		Number:     zchap.GetNumber(),
		Volume:     zchap.Volume,
		Language:   "uk",
		ID:         zchap.ID,
		Label:      zchap.Label,
		PagesCount: int64(len(pages)),
	}
	chapter.SetGroups(zchap.GetGroups()...)
	for i, p := range pages {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    zenkoStorage + "/" + strings.TrimPrefix(p.Content, "/"),
		})
	}

	return chapter, nil
}
