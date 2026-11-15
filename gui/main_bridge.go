// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build android || bridgedev

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/elboletaire/manga-downloader/browser"
)

// main is the Android app's backend: the Android app starts it with a port
// and a token, and its WebView shows the window it serves on the loopback (see
// bridge.go). The bridgedev tag builds the same thing for a desktop, to try it
// in a browser.
func main() {
	addr := flag.String("addr", "127.0.0.1:0", "loopback address to serve the window on")
	token := flag.String("token", "", "token the page must send with each call (required)")
	output := flag.String("output-dir", "", "output folder for a first run, before any is saved")
	flag.Parse()
	if *token == "" {
		log.Fatal("-token is required")
	}

	dist, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}
	bridge, err := NewBridge(*token, dist)
	if err != nil {
		log.Fatal(err)
	}
	openInFileManager = bridge.OpenFolder

	var app *App
	ctrl, prefsPath := newSession(func() {
		if app != nil {
			app.notify()
		}
	})
	app = newApp(ctrl, bridge, prefsPath, Tag, versionString())
	bridge.Bind(app)
	if err := firstRunOutputDir(app, prefsPath, *output); err != nil {
		log.Printf("output folder: %v", err)
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{Handler: bridge, ReadHeaderTimeout: 10 * time.Second}
	go sweepUpdateDirs(os.TempDir())
	app.launch()
	// the Android app reads the port from this line
	fmt.Printf("READY %d\n", ln.Addr().(*net.TCPAddr).Port)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, os.Interrupt)
	go func() {
		<-stop
		app.shutdown(context.Background())
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Print(err)
	}
	browser.Close()
}
