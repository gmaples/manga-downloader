package downloader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elboletaire/manga-downloader/grabber"
)

// A chapter whose page fails as the rest finish fails with that page's
// error, every time. The pages after a failed one return at once without
// a request, so they can all be done by the time the chapter looks: the
// failure and "all done" are both ready then, and the chapter must still
// report the failure, never hand back the pages with a gap (which panicked
// sorting them).
func TestFetchChapterFailsWhenThePagesFinishWithTheFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/1" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("page"))
	}))
	t.Cleanup(server.Close)
	site := fetchChapterTestSite{Grabber: &grabber.Grabber{URL: server.URL, Settings: &grabber.Settings{
		MaxConcurrency: grabber.MaxConcurrency{Chapters: 1, Pages: 1},
		Retry:          0,
	}}}
	saved := beforeResult
	beforeResult = func(done <-chan bool) { <-done }
	t.Cleanup(func() { beforeResult = saved })

	// select picks either ready case at random, so one try isn't enough
	for try := 1; try <= 64; try++ {
		chapter := &grabber.Chapter{Number: 1, Pages: []grabber.Page{
			{Number: 1, URL: server.URL + "/1"},
			{Number: 2, URL: server.URL + "/2"},
		}}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("try %d panicked: %v", try, r)
				}
			}()
			files, err := FetchChapter(context.Background(), site, chapter, func(int, int, error) {})
			if err == nil || !strings.Contains(err.Error(), "page 1") {
				t.Fatalf("try %d: files %v, err %v; want page 1's error", try, files, err)
			}
		}()
	}
}
