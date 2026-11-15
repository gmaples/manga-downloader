// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build !android && !bridgedev

package main

import (
	"context"
	_ "embed"
	"log"

	"github.com/elboletaire/manga-downloader/browser"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

// appIcon is the window icon on Linux. It's the free-shape mark rather than
// the macOS tile, which would look like a box on a Linux taskbar. Windows and
// macOS take theirs from build/ at packaging time.
//
//go:embed build/linux/icon.png
var appIcon []byte

func main() {
	rt := &wailsRuntime{}
	var app *App
	// notify is called from the download goroutines too, so it reads app
	// through the pointer set before the window opens. Warnings raised while
	// the options load come before that, hence the nil check.
	ctrl, prefsPath := newSession(func() {
		if app != nil {
			app.notify()
		}
	})
	app = newApp(ctrl, rt, prefsPath, Tag, versionString())

	err := wails.Run(&options.App{
		Title:            "Manga Downloader",
		Width:            1100,
		Height:           760,
		MinWidth:         900,
		MinHeight:        600,
		BackgroundColour: startupBackground(),
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup: func(ctx context.Context) {
			app.startup(ctx, rt)
		},
		OnShutdown:     app.shutdown,
		ErrorFormatter: func(err error) any { return windowError(err) },
		Bind:           []interface{}{app},
		Linux:          &linux.Options{Icon: appIcon},
		Mac:            &mac.Options{}, // without it Wails disables the zoom button
	})
	// OnShutdown already closed it; this covers a Run that failed or returned
	// without calling it
	browser.Close()
	if err != nil {
		log.Fatal(err)
	}
}
