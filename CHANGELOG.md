# Changelog

This fork's builds, newest first. Each version is a git tag on the commit the
build was made from. Builds are numbered `v25.2.0-series-folders.N`: the
upstream base (v25.2.0), this fork's name, and the build number. In Windows,
an exe's Properties > Details show the same version as `25.2.0.N`; on Android
the build number is the app's version code. Add an entry here with each
new build.

This file, the README's "About this fork" section, `BUILD_GUIDELINES.md`,
`handoff/` and `.github/workflows/fork-macos.yaml` (a manual macOS build, run
from the Actions tab) belong to this fork only. Keep fork notes out of upstream's
files (AGENTS.md, the workflows), so the feature commits can be offered to
[elboletaire/manga-downloader](https://github.com/elboletaire/manga-downloader)
as they are.

What the fork changes, and why, is described in the README's
"About this fork" section. Android details are in [docs/android.md](docs/android.md).

## v25.2.0-series-folders.10 (2026-11-15)

Commit `d745ffe`. An Android app; the desktop app and command line are the same
as build 8.

- **Android app** (`android/`): shows the desktop app's own window. The Go
  backend runs on the device and serves the window to a WebView inside the app.
  - Runs on 64-bit ARM phones and tablets (arm64-v8a) and on x86_64
    (emulators). Needs Android 7.0 or later.
  - Saves to `Download/Manga` until another folder is picked with Browse….
    Android 11 and later ask once to allow access to all files.
  - Downloads keep going in the background. A notification shows while the app
    runs, and its Quit button stops it. Back puts the app in the background
    instead of closing it.
  - Open folder opens Android's Files app on the folder.
  - Signed with the fork's own key, so later builds install over this one.
- Fixed while testing: the window was blank on Android 11's own WebView
  (Chrome 83), so the Android build compiles the window for Chrome 83 and adds
  `Object.hasOwn`. The backend now uses Android's proxy setting.
- Known limits: sites that need a browser window don't work on Android; the
  update button links to the release instead of installing; the layout is the
  desktop one, which fits a tablet but wasn't adapted for phones.
- Not yet checked: picking a folder in Browse…, the Paste button, Quit in the
  notification, and a real device. The app was checked in an Android 11
  x86_64 emulator (docs/android.md, "What was tested"). To try it on a
  Windows PC, see docs/android.md, "Trying it on a Windows PC".

File: `manga-downloader-v25.2.0-series-folders.10-android.apk` (19.1 MB),
SHA-256 `e9dd72d8ec9cfc0c923c504b8e83476ba69604726183bb4b0f525285ed865493`.

## v25.2.0-series-folders.9 (2026-11-14)

Commit `7448db6`, the same code as build 8.

- **Command line on Android**: `manga-downloader` built for Android arm64
  (Android 7.0 and up) with the Android NDK r28c, to run in Termux. It uses
  Android's own C library, so it can look up site names; a plain Go build
  can't, because Android has no `/etc/resolv.conf`.

## v25.2.0-series-folders.8 (2026-11-14)

Commit `7448db6`.

- **A progress bar per site**: while downloads run, each site that's
  downloading gets a bar above the overall one, reading "site · chapter" with
  that chapter's percentage. Two series from the same site share a bar. While a
  site sits out a 429, its bar counts down.

## v25.2.0-series-folders.7 (2026-11-14)

Commit `113d9b5`.

- **Add chapters to a running series**: selecting more of a downloading
  series' chapters turns Download into "Add N chapters to the queue". They're
  added to that series' group, use the options it started with, and share its
  summary.
- Download is greyed out when the selection has nothing new for the running
  series, and after a run until the selection changes.

## v25.2.0-series-folders.6 (2026-11-14)

Commit `c0b4098`. The first numbered build. It collects everything from the
earlier unnumbered test builds:

- **Series folders** and **Guess language** switches in Options > Save, both
  off by default. Each series is saved in its own `<Series Title>` folder; Guess
  language adds a tag such as ` [ru]` only when the language is known for sure.
- **Portable file and folder names**, valid on Windows, macOS, Linux and
  Android.
- **MangaLib**: `mangalib.org` links work, and the API host that answers is
  used.
- **Faster CBZ packing**: pages are stored as they are instead of being
  compressed again.
- **Per-site rate limits** from `site-rate-limits.txt`, with pauses after a
  429.
- **Download groups**: another series can be loaded and started while
  downloads run; groups from the same site share that site's pace.
