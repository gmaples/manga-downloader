// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build !android && !bridgedev

package main

import (
	"context"
	"os"
	"sync"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// startup is called by Wails once the window exists, with the context the
// runtime calls need.
func (a *App) startup(ctx context.Context, rt *wailsRuntime) {
	rt.set(ctx)
	a.quit = func() { wruntime.Quit(ctx) }
	go sweepUpdateDirs(os.TempDir())
	a.launch()
}

// wailsRuntime is the sink on top of Wails' runtime. Its context is set in
// startup; before that (and after shutdown) it drops events instead of calling
// into a window that isn't there.
type wailsRuntime struct {
	mu  sync.Mutex
	ctx context.Context
}

func (r *wailsRuntime) set(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ctx = ctx
}

// Close implements sink: the context is dropped, after which events are discarded
func (r *wailsRuntime) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ctx = nil
}

func (r *wailsRuntime) context() context.Context {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ctx
}

// Emit implements sink
func (r *wailsRuntime) Emit(event string, data any) {
	if ctx := r.context(); ctx != nil {
		wruntime.EventsEmit(ctx, event, data)
	}
}

// ChooseDirectory implements sink
func (r *wailsRuntime) ChooseDirectory(title, defaultDir string) (string, error) {
	ctx := r.context()
	if ctx == nil {
		return "", uiErr("errors.notReady", nil, nil)
	}

	return wruntime.OpenDirectoryDialog(ctx, wruntime.OpenDialogOptions{
		Title:                title,
		DefaultDirectory:     defaultDir,
		CanCreateDirectories: true,
	})
}
