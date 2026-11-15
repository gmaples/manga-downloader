// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"bytes"
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// The bridge serves the window's frontend over HTTP on the loopback, for the
// Android app, whose WebView has no Wails runtime. It stands in for both
// halves of Wails: bridge.js gives the page the window.go and window.runtime
// the generated bindings call, and the bridge is the App's sink, sending the
// App's events to the page over server-sent events.
//
// /call and /events need the token the app was started with, so other apps
// on the device can't drive it through the port. The frontend's files don't:
// they hold nothing private.

//go:embed bridge.js
var bridgeJS []byte

// bridgeScript is put first in the page's head, so window.go and
// window.runtime exist before the frontend's module runs
const bridgeScript = `<script src="/bridge.js"></script>`

// bridgeEventBuffer is how many events a slow page may fall behind before
// more are dropped for it. Every event is a full snapshot (the state, the
// selection, the log), so a dropped one is made up for by the next.
const bridgeEventBuffer = 64

// bridgeEvent is what the page gets for an Emit
type bridgeEvent struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
}

// callResult is /call's answer: Result when the method succeeded, Error (as
// the window's ErrorFormatter writes it) when it returned an error
type callResult struct {
	Result any     `json:"result,omitempty"`
	Error  *string `json:"error,omitempty"`
}

// Bridge is the sink for the Android app; see the top of this file
type Bridge struct {
	token string
	files fs.FS
	index []byte
	app   *App

	mu     sync.Mutex
	subs   map[chan []byte]struct{}
	closed bool
	// picks are the folder pickers the page has open, by id
	picks  map[int]chan string
	pickID int
}

// NewBridge serves dist, the built frontend (its index.html at the root),
// guarded by token, which must not be empty
func NewBridge(token string, dist fs.FS) (*Bridge, error) {
	if token == "" {
		return nil, errors.New("the bridge needs a token")
	}
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		return nil, fmt.Errorf("the frontend has no index.html: %w", err)
	}
	head := []byte("<head>")
	i := bytes.Index(index, head)
	if i < 0 {
		return nil, errors.New("the frontend's index.html has no <head>")
	}
	i += len(head)
	injected := make([]byte, 0, len(index)+len(bridgeScript))
	injected = append(injected, index[:i]...)
	injected = append(injected, bridgeScript...)
	injected = append(injected, index[i:]...)

	return &Bridge{
		token: token,
		files: dist,
		index: injected,
		subs:  map[chan []byte]struct{}{},
		picks: map[int]chan string{},
	}, nil
}

// Bind sets the App whose exported methods /call runs. Call it before serving.
func (b *Bridge) Bind(app *App) {
	b.app = app
}

// Emit implements sink: the event goes to every page listening
func (b *Bridge) Emit(event string, data any) {
	raw, err := json.Marshal(bridgeEvent{Event: event, Data: data})
	if err != nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	for ch := range b.subs {
		select {
		case ch <- raw:
		default:
		}
	}
}

// ChooseDirectory implements sink. The page shows the picker: it gets a
// "bridge:chooseDirectory" event and answers through /picked. Closing the
// bridge cancels a picker still open.
func (b *Bridge) ChooseDirectory(title, defaultDir string) (string, error) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return "", uiErr("errors.notReady", nil, nil)
	}
	b.pickID++
	id := b.pickID
	ch := make(chan string, 1)
	b.picks[id] = ch
	b.mu.Unlock()

	b.Emit("bridge:chooseDirectory", map[string]any{"id": id, "title": title, "defaultDir": defaultDir})

	return <-ch, nil
}

// OpenFolder asks the page to show dir; it's openInFileManager on Android
func (b *Bridge) OpenFolder(dir string) error {
	b.Emit("bridge:openFolder", map[string]any{"path": dir})
	return nil
}

// Close implements sink: later events are dropped, the pages' streams end and
// any open picker answers "" (cancelled)
func (b *Bridge) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for ch := range b.subs {
		close(ch)
	}
	b.subs = map[chan []byte]struct{}{}
	for id, ch := range b.picks {
		ch <- ""
		delete(b.picks, id)
	}
}

// ServeHTTP serves the frontend, bridge.js, /call/{Method}, /events and /picked
func (b *Bridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/" || r.URL.Path == "/index.html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(b.index)
	case r.URL.Path == "/bridge.js":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(bridgeJS)
	case strings.HasPrefix(r.URL.Path, "/call/"):
		if b.authorized(w, r) {
			b.serveCall(w, r, strings.TrimPrefix(r.URL.Path, "/call/"))
		}
	case r.URL.Path == "/events":
		if b.authorized(w, r) {
			b.serveEvents(w, r)
		}
	case r.URL.Path == "/picked":
		if b.authorized(w, r) {
			b.servePicked(w, r)
		}
	default:
		http.FileServerFS(b.files).ServeHTTP(w, r)
	}
}

// authorized checks the request's token (the X-MD-Token header, or the t
// query parameter, which is all an EventSource can send), answering 403 when
// it's wrong
func (b *Bridge) authorized(w http.ResponseWriter, r *http.Request) bool {
	got := r.Header.Get("X-MD-Token")
	if got == "" {
		got = r.URL.Query().Get("t")
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(b.token)) != 1 {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}

	return true
}

var errorType = reflect.TypeFor[error]()

// serveCall runs the App's exported method name with the JSON array in the
// body as its arguments, as Wails does for a bound method
func (b *Bridge) serveCall(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if b.app == nil {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	m := reflect.ValueOf(b.app).MethodByName(name)
	if !m.IsValid() || name == "" || !isExported(name) {
		http.Error(w, "no such method", http.StatusNotFound)
		return
	}
	var raw []json.RawMessage
	body, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, &raw); err != nil {
			http.Error(w, "the body must be a JSON array of the arguments", http.StatusBadRequest)
			return
		}
	}
	t := m.Type()
	if len(raw) != t.NumIn() {
		http.Error(w, fmt.Sprintf("%s takes %d arguments, got %d", name, t.NumIn(), len(raw)), http.StatusBadRequest)
		return
	}
	args := make([]reflect.Value, len(raw))
	for i, a := range raw {
		v := reflect.New(t.In(i))
		if err := json.Unmarshal(a, v.Interface()); err != nil {
			http.Error(w, fmt.Sprintf("argument %d of %s: %v", i+1, name, err), http.StatusBadRequest)
			return
		}
		args[i] = v.Elem()
	}

	var res callResult
	for _, out := range m.Call(args) {
		if out.Type() == errorType {
			if !out.IsNil() {
				msg := windowError(out.Interface().(error))
				res.Error = &msg
			}
			continue
		}
		res.Result = out.Interface()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

// isExported reports whether name starts with an upper-case letter
func isExported(name string) bool {
	return name[0] >= 'A' && name[0] <= 'Z'
}

// serveEvents streams the App's events to the page until it goes away or the
// bridge closes
func (b *Bridge) serveEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ch := make(chan []byte, bridgeEventBuffer)
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		http.Error(w, "closed", http.StatusServiceUnavailable)
		return
	}
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
		}
		b.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	// a comment first, so the page's EventSource opens at once
	_, _ = io.WriteString(w, ": ready\n\n")
	flusher.Flush()
	for {
		select {
		case raw, ok := <-ch:
			if !ok {
				return
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", raw); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// servePicked takes the page's answer to a "bridge:chooseDirectory" event:
// POST /picked?id=N with the folder's path as the body, empty when cancelled
func (b *Bridge) servePicked(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	path, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	b.mu.Lock()
	ch, ok := b.picks[id]
	delete(b.picks, id)
	b.mu.Unlock()
	if !ok {
		http.Error(w, "no such picker", http.StatusNotFound)
		return
	}
	ch <- string(path)
	w.WriteHeader(http.StatusNoContent)
}

// firstRunOutputDir makes dir the output folder when nothing is saved yet (no
// gui.json), creating it. The desktop's default, the home folder's Downloads,
// is the app's private storage on Android, which no other app can open.
func firstRunOutputDir(app *App, prefsPath, dir string) error {
	if dir == "" || prefsPath == "" {
		return nil
	}
	if _, err := os.Stat(prefsPath); !errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	o := app.Options()
	o.OutputDir = dir

	return app.SetOptions(o)
}
