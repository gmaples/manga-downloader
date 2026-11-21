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

## v25.2.0-series-folders.24 (2026-11-21)

Commit `90a8cf0`, on top of build 22 (build 23 is a separate branch). One click downloads
whatever of a series is missing, and the Activity tab can restart chapters.

- **Download N missing**: a button above the chapter list, shown once
  anything of the series was saved or tried. It selects every chapter that
  isn't saved yet (no file in the folder, failed or cancelled) and starts
  them at once. Chapters already saved and those downloading now are left
  alone; while the series downloads, the missing ones join its queue. It
  replaces build 21's *Pick N unfinished*.
- **Restart N unfinished** on each download's header in the Activity tab:
  its failed and cancelled chapters (including the ones a cancel never
  started), or a last-session download's chapters to do. A failed or to-do
  row has its own restart button. A download of another series loads that
  series first, then starts.
- In a running download, a restarted failed chapter goes back in its queue,
  and its old failed row and failure are dropped from the summary.
- A restart downloads the whole chapter again: nothing of a failed try is
  kept.

## v25.2.0-series-folders.22 (2026-11-21)

Commit `5ad636c`, on top of build 21. Adds ReadManga (a.zazaza.me) and the
GroupLe platform it runs on.

- **a.zazaza.me** (ReadManga, Russian): series links and reader links both
  work, for example `https://a.zazaza.me/provojaiuchaia_v_poslednii_put_friren/vol1/1`
  (the whole series loads). Plain HTTP, no browser: the chapter list is in
  the series page, and the images come from the reader page's own script.
  The release's group is the chapter's translators.
- **Found by its pages, not its address**: ReadManga keeps moving to new
  domains, and its sister sites (MintManga, SelfManga, Usagi) run the same
  GroupLe engine, so any page with GroupLe's markup is taken. Only ReadManga
  has been looked at.
- Checked in a browser on a Windows PC (logged out): the Frieren series page
  lists all 147 chapters, chapter pages carry their image lists, and an
  image loads with no cookies or Referer. Not yet checked: this build's own
  download from the site, the sister sites, chapters that need an account.

## v25.2.0-series-folders.21 (2026-11-21)

Commit `8c48712`, on top of build 20. The chapter list shows what became of
each chapter.

- **A mark beside each checkbox**: a green tick when the chapter's file is
  already in the folder (the same name *Skip existing files* looks for) or it
  was saved this session; a spinner while it downloads; a grey dot while it
  waits its turn; an amber (!) when it failed; an amber circled dash when a
  cancel left it unsaved. No mark means no file and no download. Hover a mark
  for its meaning. The marks update live as chapters finish.
- **Pick N unfinished**: a button above the list, shown when chapters failed
  or were cancelled, selects just those, so Download tries them again. It's
  hidden while the series is still downloading.
- The folder is checked for the whole list's series folder, chapter by
  chapter; a volume or one-file run's chapters are marked from this session
  only. The last session's restored download doesn't mark the list.

## v25.2.0-series-folders.20 (2026-11-21)

Commit `8d294e6`, on top of build 19. Each site learns how many pages to
download at once from how fast it answers.

- **Learned level**: a site seen for the first time starts at 4 pages at once
  and doubles while each doubling is at least 25% faster, then settles at the
  fastest level. Every few windows it tries one more and keeps it when that's
  at least 5% faster. Speed is bytes per second, measured only while pages
  are queued.
- **Trouble cuts it**: a 429, a 502/503/504, a Cloudflare 520-524, a timeout or
  a dropped connection halves the level at once, holds off any rise for 30s,
  and keeps the site below the level that had trouble for the rest of the
  run. `site-rate-limits.txt` and the 429 pause still apply on top.
- **Carried over**: `site-parallel.txt`, next to `site-rate-limits.txt`, holds
  one `site: number` line per site, written when downloads end and right after
  a cut. The next run starts there. Edit a number to set a site's start.
- **Shared**: groups of one site running at once share one level.
- **Shown**: each site's bar in the app says how many run at once (*8
  parallel*); the CLI prints a line when a site settles or is cut back.
- Verified by tests only, including a mocked slow site (settles at the 8 it
  can serve) and a mocked site that answers 429 past 6 at once (3 429s in
  1,000 pages, ends at 6). Not yet run against a live site.

## v25.2.0-series-folders.19 (2026-11-21)

Commit `5ef0851`, on top of build 18. The Windows desktop app remembers its
downloads between launches, lists past series, and masks adult titles.

- **Last session**: `history.txt`, next to `gui.json`, keeps the last 100
  series loaded or downloaded, as plain `key: value` lines with a `done` or
  `todo` line per chapter. At the next launch the Activity tab shows the
  series that were in it, under *From last session*, and the newest with
  chapters to do is loaded with them selected, so Download carries on.
  Nothing downloads by itself. *Load to resume* does the same for another one.
- **Past series**: the clock button beside the gear lists them. A series
  opens its page in the browser, its site the site's home page, and Clear
  empties the list.
- **Adult titles**: a title the site rates 18+ (zenko.online) shows as
  *[Adult Title]* until clicked, in the window and the list; the log always
  masks it.
- Tested by `make test`, `make test/demo`, `make test/gui` and in the browser
  mock; not yet run as the Windows app.

## v25.2.0-series-folders.18 (2026-11-20)

Commits `4a90f43` and `2e50e27`, on top of build 17. Adds an age check, on the
Windows desktop app and command line.

- **Age check**: Options > Save has *Age check* and *I am over* 16 or 18
  (saved in `gui.json` as `ageCheck` and `age`); the command line has
  `--age 16|18`. Off, the default, every title loads as before. On, a title
  the site rates above that age is refused before anything is downloaded,
  with a message saying why.
- zenko.online is the first site checked: its API gives each title an age
  rating (`ageLimit`). Its own age gate is only on the website, so without
  the check its 18+ titles download like any other.
- Tested on a Windows PC with this build's command line, title 2779 (rated
  18): refused with `--age 16` (exit 1, no file), downloaded with `--age 18`
  and with no `--age` (7 JPEG pages). Title 1225 (rated 16) downloaded with
  `--age 16`; `--age 17` is refused as invalid. The desktop app's switch is
  covered by tests, not yet by a run on the PC.

## v25.2.0-series-folders.17 (2026-11-21)

Commit `b534d91`. Resizable panes and mouse-wheel scrolling everywhere, in the
Windows desktop app. Includes everything in build 16.

- **Drag to resize**: the gap between the chapter list and the side panel
  makes the side panel wider or narrower, and the log's top edge makes it
  taller or shorter. Each also takes the arrow keys once focused. The sizes
  are remembered for the next launch (`sideWidth` and `logHeight` in
  `gui.json`). The side panel stays at least 280px wide and leaves the
  chapter list 372px.
- **The wheel works anywhere in a pane**: over the series' title or the list
  toolbar it scrolls the chapter list, over a run's summary or the Download
  button it scrolls the Activity tab's chapters, and a run's list that has
  reached its end passes the wheel on to the Activity tab instead of
  stopping it.
- Tested in the browser mock of the app (Chromium, the engine WebView2 uses)
  and by unit tests; not yet run as the Windows app.

## v25.2.0-series-folders.16 (2026-11-20)

Commit `8a64451`. Adds zenko.online, on the Windows desktop app and command
line.

- **zenko.online** (Zenko, Ukrainian): series links and reader links both
  work, for example `https://zenko.online/titles/1225/20377` (the second
  number is a chapter id; the whole series loads). Title, chapters and pages
  come from the site's own open API (`api.zenko.online`), with images from
  `storage.zenko.online`, so no browser opens. The release's group is the
  site's publisher. Untitled chapters are named "Chapter N".
- Tested on a Windows PC with this build's command line: chapter 1 of
  Доктор Стоун from the reader link (45 pages) and chapters 2-3 from the
  series link (24 and 21 pages), all JPEG, no browser opened. Not yet
  checked: the desktop app on zenko, paid or unpublished chapters.

## v25.2.0-series-folders.15 (2026-11-20)

Commits `0722a60` and `845b1b8`. Adds remanga.org, on the Windows desktop app
and command line.

- **remanga.org** (ReManga, Russian): series links and reader links both work,
  for example `https://remanga.org/manga/murim-login_/320521?page=1` (the
  number is a chapter id; the whole series loads). Title, chapters and pages
  come from the site's own open API (`api.remanga.org/api/v2`), so no browser
  opens. Every translation branch is listed, with its group as the release's
  group. Chapters the site marks paid are left out; a price alone isn't paid,
  since it stays on early-access chapters that are free now.
- Untitled chapters are named "Chapter N", so files read
  `Логин Мурим 1 - Chapter 1 [Newlate.WTF].cbz`.
- Recent chapters are WebP; add `webp` to Convert images if a reader needs JPEG.
- Tested on a Windows PC with a test build of `0722a60`'s grabber: chapter 1
  of Логин Мурим (146 pages, all JPEG) and chapters 250-251 (53 and 57 pages,
  WebP), no browser opened. The "Chapter N" naming is covered by tests, not
  yet by a run on the PC.

## v25.2.0-series-folders.14 (2026-11-20)

Commits `b3f6a2a` and `aeb8f7c`. Desktop app changes, on Windows. (Build 13
was a test build of `b3f6a2a` alone; it was never released.)

- **One browser window per series**: on a site that needs a browser
  (joilmang.com, Cloudflare-gated sites), the window opened to load a series
  now stays open for its download, instead of closing and a second one
  opening. It closes when the download ends, on Clear, or when another
  series is loaded. A second download of the same loaded series still opens
  a window of its own.
- **Guess language** no longer trusts the language filter on a site that has
  no language choice: joilmang ignores it, yet the saved `en` named the
  folder `Мир Лидейл [en]`. It now also tags from the language the site's
  page declares (`<html lang="ru">`) when the title is written in that
  language's script, and from Russian letters (Ы Э Ъ); Ы and Э are
  Belarusian too and Ъ Bulgarian.
- Tested on a Windows PC with build 13 (the window change): World of Leadale
  on joilmang loaded and downloaded all 52 chapters with one Chrome window
  for the load and the download, as far as the process log reached.

## v25.2.0-series-folders.12 (2026-11-17)

Commit `a73f88b`. Adds joilmang.com, on the Windows desktop app and command line.

- **joilmang.com** (Russian): series and chapter links both work, for example
  `https://joilmang.com/manga/pererozhdenie-dyadi/1/1`. The site sits behind
  Vercel's Security Checkpoint, which turns away programs with a 429, so it
  opens in a visible browser window that passes the check as a normal visitor's
  browser does; the images then download over plain HTTP and are converted
  from AVIF to JPEG as usual.
- Tested on a Windows PC: chapter 1 (16 pages) and chapter 80 (43 pages) of
  Перерождение Дяди downloaded into CBZs holding real JPEG pages.

## v25.2.0-series-folders.11 (2026-11-15)

Commit `cd05cc5`. The first macOS build of this fork; the code is the same as
build 10.

- **macOS desktop app**: `manga-downloader-gui-v25.2.0-series-folders.11-darwin-universal.zip`,
  one `Manga Downloader.app` for Apple Silicon and Intel Macs, built by the
  fork-only `fork-macos.yaml` workflow on GitHub's macOS runner with the same
  `make build/gui/darwin` that upstream's release uses. It's kept as that
  run's artifact (run 43692104268).
  SHA-256 of the zip: `88163c3986361e0558ecc16b86fca32050d70f10eb8d2bf7789ea925987a61d4`.
- Not signed or notarized, like upstream's: allow the first launch once in
  System Settings > Privacy & Security.
- Not tested yet: it hasn't been run on a Mac.

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
