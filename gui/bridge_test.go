// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

const testToken = "s3cret"

var testDist = fstest.MapFS{
	"index.html":     {Data: []byte("<!doctype html><html><head><title>x</title></head><body></body></html>")},
	"assets/app.js":  {Data: []byte("console.log(1)")},
	"assets/app.css": {Data: []byte("body{}")},
}

// newBridgeApp is newTestApp with a Bridge as its sink, served by a test server
func newBridgeApp(t *testing.T, n int) (*App, *Bridge, *httptest.Server, string) {
	t.Helper()
	c, _, _ := newTestController(t, false, nil, chapters(n, 2)...)
	b, err := NewBridge(testToken, testDist)
	if err != nil {
		t.Fatal(err)
	}
	prefs := filepath.Join(t.TempDir(), "gui.json")
	a := newApp(c, b, prefs, "", "v1.2.3")
	a.interval = time.Millisecond
	c.notify = a.notify
	b.Bind(a)
	srv := httptest.NewServer(b)
	t.Cleanup(func() {
		b.Close()
		srv.Close()
	})

	return a, b, srv, prefs
}

// call POSTs args to /call/method with the token, returning the status and body
func call(t *testing.T, srv *httptest.Server, method string, token string, body string) (int, callResult) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/call/"+method, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("X-MD-Token", token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out callResult
	if res.StatusCode == http.StatusOK {
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
	}

	return res.StatusCode, out
}

// openEvents opens /events and returns a function that reads the next event
func openEvents(t *testing.T, srv *httptest.Server) func() bridgeEvent {
	t.Helper()
	res, err := http.Get(srv.URL + "/events?t=" + testToken)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	if res.StatusCode != http.StatusOK {
		t.Fatalf("/events: %s", res.Status)
	}
	lines := bufio.NewScanner(res.Body)
	lines.Buffer(make([]byte, 1<<20), 1<<20)
	// the stream opens with a comment, read here so the subscription exists
	if !lines.Scan() || lines.Text() != ": ready" {
		t.Fatalf("the stream didn't open with its comment: %q", lines.Text())
	}

	return func() bridgeEvent {
		t.Helper()
		for lines.Scan() {
			data, ok := strings.CutPrefix(lines.Text(), "data: ")
			if !ok {
				continue
			}
			var e bridgeEvent
			if err := json.Unmarshal([]byte(data), &e); err != nil {
				t.Fatal(err)
			}
			return e
		}
		t.Fatalf("the stream ended: %v", lines.Err())
		return bridgeEvent{}
	}
}

func TestBridgeNeedsATokenAndAHead(t *testing.T) {
	if _, err := NewBridge("", testDist); err == nil {
		t.Error("NewBridge took an empty token")
	}
	noHead := fstest.MapFS{"index.html": {Data: []byte("<html><body></body></html>")}}
	if _, err := NewBridge(testToken, noHead); err == nil {
		t.Error("NewBridge took an index.html without <head>")
	}
	if _, err := NewBridge(testToken, fstest.MapFS{}); err == nil {
		t.Error("NewBridge took a frontend without index.html")
	}
}

func TestBridgeServesThePageWithItsScriptFirst(t *testing.T) {
	_, _, srv, _ := newBridgeApp(t, 1)
	res, err := http.Get(srv.URL + "/?t=" + testToken)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.HasPrefix(string(body), "<!doctype html><html><head>"+bridgeScript+"<title>") {
		t.Errorf("the bridge script isn't first in the head: %s", body)
	}
	if strings.Contains(string(body), testToken) {
		t.Error("the page carries the token")
	}

	for path, want := range map[string]string{"/bridge.js": string(bridgeJS), "/assets/app.js": "console.log(1)"} {
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != http.StatusOK || string(got) != want {
			t.Errorf("%s = %s %q", path, res.Status, got)
		}
	}
}

func TestBridgeRefusesAWrongToken(t *testing.T) {
	_, _, srv, _ := newBridgeApp(t, 1)
	for _, token := range []string{"", "wrong", testToken + "x"} {
		if code, _ := call(t, srv, "Version", token, "[]"); code != http.StatusForbidden {
			t.Errorf("/call with token %q: %d, want 403", token, code)
		}
		res, err := http.Get(srv.URL + "/events?t=" + token)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("/events with token %q: %s, want 403", token, res.Status)
		}
		res, err = http.Post(srv.URL+"/picked?id=1&t="+token, "text/plain", strings.NewReader("/x"))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("/picked with token %q: %s, want 403", token, res.Status)
		}
	}
}

func TestBridgeCallRunsTheAppsMethod(t *testing.T) {
	a, _, srv, _ := newBridgeApp(t, 4)
	code, out := call(t, srv, "Version", testToken, "[]")
	if code != http.StatusOK || out.Result != "v1.2.3" || out.Error != nil {
		t.Errorf("Version = %d %+v", code, out)
	}

	if code, out := call(t, srv, "Load", testToken, `["https://example.com/manga/test"]`); code != http.StatusOK || out.Error != nil {
		t.Fatalf("Load = %d %+v", code, out)
	}
	waitUntil(t, "the load", func() bool { return !a.State().Loading })
	code, out = call(t, srv, "SelectRange", testToken, `["1-2"]`)
	if code != http.StatusOK || out.Result != float64(2) || out.Error != nil {
		t.Errorf("SelectRange = %d %+v, want 2", code, out)
	}
	if got := a.Selection(); len(got) != 2 {
		t.Errorf("selection after SelectRange: %v", got)
	}
	// a struct argument, as the window's SetOptions sends it
	o := a.Options()
	o.Format = "epub"
	raw, _ := json.Marshal([]any{o})
	if code, out := call(t, srv, "SetOptions", testToken, string(raw)); code != http.StatusOK || out.Error != nil {
		t.Fatalf("SetOptions = %d %+v", code, out)
	}
	if a.Options().Format != "epub" {
		t.Errorf("SetOptions didn't apply: %+v", a.Options())
	}
}

func TestBridgeCallReturnsTheWindowError(t *testing.T) {
	a, _, srv, _ := newBridgeApp(t, 1)
	code, out := call(t, srv, "Load", testToken, `[""]`)
	if code != http.StatusOK || out.Error == nil {
		t.Fatalf("Load(\"\") = %d %+v, want an error", code, out)
	}
	if want := windowError(a.Load("")); *out.Error != want {
		t.Errorf("error %q, want the window's %q", *out.Error, want)
	}
}

func TestBridgeRefusesBadCalls(t *testing.T) {
	_, _, srv, _ := newBridgeApp(t, 1)
	for _, c := range []struct {
		method, body string
		want         int
	}{
		{"NoSuchMethod", "[]", http.StatusNotFound},
		{"notify", "[]", http.StatusNotFound},
		{"", "[]", http.StatusNotFound},
		{"Load", "[]", http.StatusBadRequest},
		{"Load", `["a","b"]`, http.StatusBadRequest},
		{"Load", `[1]`, http.StatusBadRequest},
		{"Load", `{"url":"x"}`, http.StatusBadRequest},
	} {
		if code, _ := call(t, srv, c.method, testToken, c.body); code != c.want {
			t.Errorf("%s %s: %d, want %d", c.method, c.body, code, c.want)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/call/Version", nil)
	req.Header.Set("X-MD-Token", testToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /call: %s, want 405", res.Status)
	}
}

func TestBridgeStreamsTheAppsEvents(t *testing.T) {
	a, b, srv, _ := newBridgeApp(t, 3)
	next := openEvents(t, srv)
	b.Emit("custom", map[string]int{"n": 7})
	if e := next(); e.Event != "custom" || e.Data.(map[string]any)["n"] != float64(7) {
		t.Errorf("event %+v", e)
	}

	a.launch()
	if err := a.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	for {
		e := next()
		if e.Event != "state" {
			continue
		}
		if s := e.Data.(map[string]any); s["loading"] == false && s["url"] != "" {
			break
		}
	}
}

func TestBridgeChooseFolderAsksThePage(t *testing.T) {
	a, _, srv, prefs := newBridgeApp(t, 1)
	next := openEvents(t, srv)
	dir := t.TempDir()
	type picked struct {
		dir string
		err error
	}
	done := make(chan picked, 1)
	go func() {
		d, err := a.ChooseFolder("Choose the output folder")
		done <- picked{d, err}
	}()

	e := next()
	if e.Event != "bridge:chooseDirectory" {
		t.Fatalf("event %+v, want the picker", e)
	}
	req := e.Data.(map[string]any)
	if req["title"] != "Choose the output folder" {
		t.Errorf("picker title %v", req["title"])
	}
	id := int(req["id"].(float64))
	res, err := http.Post(srv.URL+"/picked?t="+testToken+"&id="+strconv.Itoa(id), "text/plain", strings.NewReader(dir))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("/picked: %s", res.Status)
	}
	select {
	case p := <-done:
		if p.err != nil || p.dir != dir {
			t.Fatalf("ChooseFolder = %q, %v", p.dir, p.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ChooseFolder didn't return after the page answered")
	}
	if a.Options().OutputDir != dir {
		t.Errorf("output folder %q, want %q", a.Options().OutputDir, dir)
	}
	if loaded := defaultSettings(); loadOptions(prefs, loaded) != nil || loaded.OutputDir != dir {
		t.Error("the picked folder wasn't saved")
	}
	// the id is used up
	res, err = http.Post(srv.URL+"/picked?t="+testToken+"&id="+strconv.Itoa(id), "text/plain", strings.NewReader(dir))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("a second answer: %s, want 404", res.Status)
	}
}

func TestBridgeCloseCancelsAnOpenPicker(t *testing.T) {
	a, b, srv, _ := newBridgeApp(t, 1)
	next := openEvents(t, srv)
	before := a.Options().OutputDir
	done := make(chan string, 1)
	go func() {
		d, _ := a.ChooseFolder("Choose")
		done <- d
	}()
	if e := next(); e.Event != "bridge:chooseDirectory" {
		t.Fatalf("event %+v, want the picker", e)
	}
	b.Close()
	select {
	case d := <-done:
		if d != "" {
			t.Errorf("a closed picker answered %q", d)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close didn't cancel the picker")
	}
	if a.Options().OutputDir != before {
		t.Error("a cancelled picker changed the output folder")
	}
	if _, err := b.ChooseDirectory("again", ""); err == nil {
		t.Error("a closed bridge opened a picker")
	}
}

func TestBridgeOpenFolderAsksThePage(t *testing.T) {
	_, b, srv, _ := newBridgeApp(t, 1)
	next := openEvents(t, srv)
	if err := b.OpenFolder("/storage/emulated/0/Download/Manga"); err != nil {
		t.Fatal(err)
	}
	e := next()
	if e.Event != "bridge:openFolder" || e.Data.(map[string]any)["path"] != "/storage/emulated/0/Download/Manga" {
		t.Errorf("event %+v", e)
	}
}

func TestFirstRunOutputDirOnlyOnTheFirstRun(t *testing.T) {
	a, _, _, prefs := newBridgeApp(t, 1)
	dir := filepath.Join(t.TempDir(), "Download", "Manga")
	if err := firstRunOutputDir(a, prefs, dir); err != nil {
		t.Fatal(err)
	}
	if a.Options().OutputDir != dir {
		t.Errorf("first run: output folder %q, want %q", a.Options().OutputDir, dir)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Errorf("the folder wasn't made: %v", err)
	}
	// gui.json now exists, so a later run keeps what's saved
	other := filepath.Join(t.TempDir(), "other")
	if err := firstRunOutputDir(a, prefs, other); err != nil {
		t.Fatal(err)
	}
	if a.Options().OutputDir != dir {
		t.Errorf("a later run changed the output folder to %q", a.Options().OutputDir)
	}
	if _, err := os.Stat(other); err == nil {
		t.Error("a later run made its folder")
	}
}
