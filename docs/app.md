# Desktop app guide

How to use the app, step by step, with its settings and the update check at the
end. For installing it see the [README](../README.md#installing-the-desktop-app).

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="../assets/screenshots/01-start-dark.webp">
  <img alt="The app's start screen: the logo over an empty URL box with its Paste button, Load to its right and Settings below" src="../assets/screenshots/01-start-light.webp">
</picture>

The app opens on a simple start screen: the logo, the URL box with its *Paste*
button, *Load* and *Settings*. (To open on the full window instead, turn off
*Open with the simple view* under *Settings* → *Start screen*.) From there:

1. **Paste the series' URL** into the box (or use its *Paste* button). The
   series loads on its own once you paste or stop typing; *Load* or Enter
   loads it at once. The window then turns into the full app, with the URL box
   at the top and the chapter list below it:

   <picture>
     <source media="(prefers-color-scheme: dark)" srcset="../assets/screenshots/02-loaded-dark.webp">
     <img alt="The Manga Downloader desktop app with a series loaded" src="../assets/screenshots/02-loaded-light.webp">
   </picture>

2. **Pick chapters** in the list. Type a range such as `1-10,12` and press
   Enter (or the + in the box), Shift+click to select a span, or use *All* and
   *None*. The arrow keys and Space work too. Locked chapters (paid or members-only) show
   a lock and can't be selected.
   A mark beside each checkbox says what became of the chapter: a green tick
   when its file is already in the folder (the name *Skip existing files*
   looks for, or any file of the chapter's number, see below; hovering over
   the tick names the file) or it was saved this session, a spinner while it downloads, a
   grey dot while it waits its turn, and an amber mark when it failed (!) or a
   cancel left it unsaved (a circled dash; hovering over a failed mark says
   why). No mark means no file and no
   download. Under "N of M selected", a second line counts them: **N
   missing** (no mark) and **N failed** (failed or cancelled). Clicking
   either selects exactly those chapters, highlights them with a bar at the
   left of their row and scrolls to the first one, so Download takes just
   them. Last on that line, **Delete series** takes the loaded series'
   files away, for when one series was downloaded twice under two folder
   names: it asks first, saying the folder, the file count and the size,
   then moves them to the Recycle Bin (the Trash on macOS and Linux) a batch
   of 100 at a time, counting in the dialog and the log, and logs "Deleted
   series *name* from *folder*". On a drive with no Recycle Bin (a memory
   card, a network share) the dialog says in red that the files will be
   deleted for good, and its button reads *Delete for good*. With *Series folders* on, the series' folder
   goes whole; otherwise only the series' chapter files in the output folder
   go, and other series' files stay. The series leaves the history; its rows
   in the Activity tab stay. It's refused while the series downloads. Once anything of the series was saved or tried, **Download N
   missing** above the list downloads every chapter that isn't saved yet,
   the failed and cancelled ones included, in one click (chapters already on
   their way are left to their download). In the Activity tab, each download's
   header has **Restart N unfinished** for its failed, cancelled and to-do
   chapters, and a failed or to-do row has a restart button of its own; a
   download of another series loads that series first. A restart downloads
   the whole chapter again.
3. **Choose a language and a scanlation group** when the site offers them. The
   selectors tell you when a series has none, and picking one reloads the
   chapter list. Sites that support it also offer *All*.

   <picture>
     <source media="(prefers-color-scheme: dark)" srcset="../assets/screenshots/03-language-selector-dark.webp">
     <img alt="The language selector open, with English selected" src="../assets/screenshots/03-language-selector-light.webp">
   </picture>

4. **Set the options** in the *Options* tab: the folder the files go to (the
   app creates it if it's missing),
   *Series folders* (each series saved in its own folder, named after the
   series, inside that folder; reruns reuse it), *Guess language* (adds the
   run's language to that folder's name, as `One Piece [en]`, only when it's
   known: the chapters' own language, the language filter (when the site
   offers languages), the site's only language, the language the site's page
   declares (when the title is written in its script), or letters used by one
   language alone (Ukrainian Ї Є Ґ, Russian Ы Э Ъ, kana, Hangul); mixed languages are
   `[mul]`, and otherwise no tag is added; it needs *Series folders*), *Skip existing files* (chapters already in the folder aren't
   downloaded again), *Age check* with *I am over* 16 or 18 (titles a site
   rates above that age aren't loaded; off, every title loads; for now
   zenko.online and senkuro.com publish a rating), the format (CBZ, CBR, Folder or EPUB; EPUB adds a *Reading
   direction*) and *Pack as*: *Chapters* (a file each), *Merged* (a chapter's
   parts in one file), *Volumes* (a file per volume) or *One file* (everything
   together). Under *Pages*, *Convert to JPEG* ticks *AVIF* (on by default,
   and always on for EPUB, since readers can't show it) and *WebP*, and
   *Compress pages* saves a page as JPEG when that makes it smaller. *Skip
   pages* drops positions from every chapter (`2`, or `-1` for the last page).
   Under *Advanced* are the *Filename template*, the *JPEG quality* of every
   page saved as JPEG, and *Show the browser window*, which you rarely need: a
   site that needs a window opens one by itself, off the screen, and it only
   comes on screen when its check waits for you. The browser keeps its
   profile between runs (in your cache folder under
   `manga-downloader/browser`), so a check passed once is skipped until it
   expires.
   *Series folders* and *Guess language* are app-only: the CLI has no flag
   for them.

   <picture>
     <source media="(prefers-color-scheme: dark)" srcset="../assets/screenshots/06-options-dark.webp">
     <img alt="The Options tab with EPUB chosen, showing the Skip existing files switch above the format, reading direction and Pack as options" src="../assets/screenshots/06-options-light.webp">
   </picture>

   *Pack as → Volumes* adds a *Volumes* box above the chapter list. Left empty,
   it packs the site's own volumes; a line under it offers them as chapter
   ranges when they can be written that way, and the list is grouped by volume.

   <picture>
     <source media="(prefers-color-scheme: dark)" srcset="../assets/screenshots/07-volumes-dark.webp">
     <img alt="Packing by volume: the Volumes box above the chapter list, the list grouped by volume, and the Download button naming the volumes" src="../assets/screenshots/07-volumes-light.webp">
   </picture>

   *Pack as → Merged* packs a chapter's parts (19-① and 19-②) into one file named
   after the chapter, and leaves the chapters without parts as one file each.

   <picture>
     <source media="(prefers-color-scheme: dark)" srcset="../assets/screenshots/08-parts-dark.webp">
     <img alt="Pack as Merged: the chapter list with each chapter's parts grouped under it" src="../assets/screenshots/08-parts-light.webp">
   </picture>

   When a site's browser window needs you (a captcha or a puzzle to solve: its
   page hasn't loaded after 10 seconds), the window comes on screen and an
   alert says so, with *Show the browser window* to bring it to the front and
   *Dismiss* to hide the alert.

   <picture>
     <source media="(prefers-color-scheme: dark)" srcset="../assets/screenshots/09-browser-alert-dark.webp">
     <img alt="The app's alert saying the browser needs you, with Dismiss and Show the browser window buttons" src="../assets/screenshots/09-browser-alert-light.webp">
   </picture>

5. **Press Download.** The *Activity* tab shows each chapter as it's fetched and
   packed. *Cancel* lets the chapters already downloading finish, and starts no
   new ones. A log is one click away in the status bar, and its *Clear* button
   empties it without touching a download that's running.

   While it runs you can paste another series, load it, pick its chapters and
   press Download again: each press is a group of its own, listed in the
   Activity tab under the series' title with its own *Cancel* and, once it
   ends, its own summary. The groups download at the same time. A group keeps
   the options it started with, so changing them only affects the next one. The
   *Cancel all* button under the progress stops every group. A series never
   downloads twice at once: while it does, selecting more of its chapters
   turns the button into *Add N chapters to the queue*, which adds them to its
   group, after the chapters already queued and with the group's options (with
   *One file* they make a file of their own). Download stays greyed out while
   the selection has nothing new for the running series, and after a run until
   the selection changes. *Clear* removes the groups that ended and leaves the
   running ones. Above the overall progress, each site that's downloading
   has a bar of its own showing the chapter it's on (the first one, when
   it has several going) and how many of its pages download at once, or how
   long it's still waiting after a 429. Groups from the same site share its
   bar, its pace from `site-rate-limits.txt` and its learned level from
   `site-parallel.txt` (see [Rate limits](#rate-limits)). This is an app feature: on the command line,
   each run is its own process.

   <picture>
     <source media="(prefers-color-scheme: dark)" srcset="../assets/screenshots/04-downloading-dark.webp">
     <img alt="A download in progress" src="../assets/screenshots/04-downloading-light.webp">
   </picture>

6. **When it's done**, the summary shows what was downloaded. Press
   *Open folder* to go straight to the files.

   <picture>
     <source media="(prefers-color-scheme: dark)" srcset="../assets/screenshots/05-done-dark.webp">
     <img alt="The finished download summary" src="../assets/screenshots/05-done-light.webp">
   </picture>

   If a chapter fails, the summary names it and says why; the other chapters
   are still saved.

   <picture>
     <source media="(prefers-color-scheme: dark)" srcset="../assets/screenshots/11-failed-dark.webp">
     <img alt="A finished download where one chapter failed, with the reason" src="../assets/screenshots/11-failed-light.webp">
   </picture>

   With *Skip existing files*, a chapter whose file is already in the folder is
   left out before anything is fetched, and the summary says how many were
   there. When the chapters are packed one file each, the download first tidies
   the folder: two files with the same bytes, or two files of one chapter
   number whatever their title or group, are one chapter, and the older file is
   kept while the others go to the Recycle Bin (the Trash on macOS and Linux).
   On a drive with no Recycle Bin (a memory card, a network share) they would
   be deleted for good, so the window asks first, once per download: *Keep
   them* (the default) leaves every file where it is, *Delete for good*
   deletes the duplicates. The download goes on either way, and cancelling it
   answers *Keep them*. A chapter whose number is in the folder isn't downloaded again, and of
   several releases of one missing number only the first is. The log names
   each file moved and the file it duplicates. The command line guide's
   "Skipping existing files" section has the details.

   The Activity tab shows only what is happening: a row for each chapter
   being fetched, downloaded or packed now. While the folder is tidied, the
   header says which step it's on (looking at the files, comparing them,
   moving duplicates to the Recycle Bin, with counts that move), and the log
   names each file as it goes. The log also says when each chapter starts
   downloading, and each page that needed another server or another try. Each download's header counts the
   rest (downloading, queued, saved, already in the folder, failed) and how
   many duplicates went to the Recycle Bin when it started; the chapter list's
   marks say what became of each chapter, and failed ones are downloaded again
   from the header's *Restart* or the list's *Download N missing*.

   If another copy of the app (or the command line) is downloading the same
   chapter into the same folder, the log says so and the chapter waits for it,
   then is skipped once that copy has saved it (or downloaded, if that copy
   failed). Meanwhile a file named after the chapter with `.downloading` added
   sits next to where it will be saved; it goes when the chapter is done, and
   one left by a copy that was closed mid-chapter is ignored after 2 minutes.

   <picture>
     <source media="(prefers-color-scheme: dark)" srcset="../assets/screenshots/14-skip-existing-dark.webp">
     <img alt="A download in progress where the first two chapters were already in the folder, listed as skipped, while the others download" src="../assets/screenshots/14-skip-existing-light.webp">
   </picture>

The app saves your options to `gui.json` in the `manga-downloader` folder of
your user config folder (`~/.config` on Linux, `~/Library/Application Support`
on macOS, `%AppData%` on Windows), so they're still there the next time you
open it. Files go to your Downloads folder (or your home folder, if there isn't
one) unless you choose another folder.

The app downloads up to five chapters at once, ten pages each, and retries a
failed page once, the command line's defaults; there's no setting for them.
Sites whose image servers limit the request rate get lower limits on their own.

Downloads go through the proxy in `HTTPS_PROXY` or `HTTP_PROXY` when the app is
started with one set, as on the [command line](cli.md#environment-variables).

## Pane sizes and scrolling

Drag the gap between the chapter list and the side panel to make the side
panel wider or narrower, and the top edge of the log to make it taller or
shorter. Each also takes the arrow keys once focused. The app remembers the
sizes for the next launch (`sideWidth` and `logHeight` in `gui.json`).

The mouse wheel scrolls a pane wherever the pointer is over it: over the
series' title or the toolbar it scrolls the chapter list, and over a run's
summary or the Download button it scrolls the Activity tab's chapters.

## The log file

The log at the bottom of the window is also written to `log.txt` next to
`gui.json`, one line per entry with its date and time, so it can still be read
after the app is closed (for example to see why chapters failed). When
`log.txt` has grown past 5 MB at a start, it's renamed to `log-old.txt`
(replacing an older one) and a new `log.txt` is begun. *Clear* empties the
window's log only. Adult titles are masked in the file as they are in the
window.

## History and the last session

The app keeps `history.txt` next to `gui.json`: the last 100 series you loaded
or downloaded, newest first, written as it runs. It's plain text, one
`key: value` line each (`title`, `url`, `adult`, `when`, `open`, then a `done`
or `todo` line per chapter), so it can be read or edited with the app closed. Two copies of the app open at once share it: each save first reads the file and keeps the series the other copy wrote, writing over only the series this copy changed since it last saved.

When the app starts, the Activity tab shows again the series that were in it
when it closed, under *From last session*: the chapters saved and the ones
still to do. The newest one with chapters to do is loaded with them selected,
so Download carries on with it; nothing downloads by itself. *Load to resume*
does the same for another series. Clear (the × in the URL box) takes them off
the tab, and the next launch won't show them.

The same list drops down from the URL box: click the arrow at its right end,
or press ↓ in the box. Typing then narrows it by title or address, ↑ and ↓
move through it, Enter (or a click) puts that series in the box and loads it,
and Esc closes it. A new link can still be typed or pasted as before.

The clock icon beside the gear opens the list of past series. A series opens
its page in your browser, and its site the site's home page; *Load* beside it
puts the series in the URL box and loads it, as the drop-down does. Clear there
empties the list.

A title the site rates 18+ (zenko.online and senkuro.com give a rating) shows as
*[Adult Title]* until you click it, which shows it until the app closes. The
log always writes *[Adult Title]* for it, in file and folder names too.

## Sites without built-in support

A site none of the built-in handlers knows is read by the generic reader. It looks for the series' chapter list in the page's links (the largest set of links that differ only in a number, under the series' address or with a word like "chapter" or "read" in them) and for a chapter's pages in its images (the largest set served from one folder). When the plain page has no chapter list, as on sites built as JavaScript apps, it renders the page in a hidden browser, scrolls it, and reads that, along with the data the page fetched. It never opens a browser window by itself.

The chapter numbers and pages it finds are guesses, and a warning says so: check the first chapter before downloading a whole series. When it finds nothing, the error lists what it tried and what it saw, for example a bot check, an age confirmation, or a page that says the site isn't available in your country.

## Rate limits

When a site answers 429 Too Many Requests, the downloader slows down for that site and remembers it. It writes a line such as `mangalib.org: 1s` to `site-rate-limits.txt` in the download root (the folder picked under Save, not a series folder inside it). Every later request to that site, in this run and the next ones, waits at least that long after the previous one.

After a 429, every download from that site pauses: 15s after the first one, then 1.5 times longer after each further one, up to 5m. Only the pages that got the 429 are fetched again, so the chapter's file is still complete. A site's time also grows 1.5 times on each 429, up to 5m. A page that still gets a 429 after the 5m pause fails, and so does its chapter. The time comes back down as well: after 5m of downloads from the site with no 429, it's divided by 1.5 and saved, again after each further 5m, and once it would drop below 1s the site's line is taken out of the file. A time you wrote by hand comes down the same way.

When several copies run at once on one download folder (two app windows, or the app and the command line), they share each site's pace through `site-pace.txt`, next to `site-rate-limits.txt`: the time between requests holds over all their requests together, and a 429 one copy gets pauses the others too. Each copy still learns its own level. The program rewrites that file as it runs, so leave it alone; it can be deleted when no copy is running.

The file is plain text and can be edited: one `site: time` line per site, with times such as `500ms`, `15s` or `1m30s`, and `#` for comments. A site's line replaces any pacing built into the downloader for it. Because the file sits next to your series, it travels with the folder or card to another device.

### How many downloads run at once

Each site also learns how many of its pages to download at once. Its bar shows the level, as in *8 parallel*. A site seen for the first time starts at 4 and doubles (8, 16, 32, up to 50) as long as each doubling makes it at least 25% faster, then goes back to the fastest level and stays there. Every so often it tries one more and keeps it when that's at least 5% faster. Speed is measured in bytes per second over a few seconds at a time, and only while more pages are waiting than are running, so the last pages of a run teach it nothing.

When a site struggles (a 429, a 502, 503 or 504, a Cloudflare 520-524, a timeout or a dropped connection), its level is halved at once, no rise is tried for 30s, and for the rest of the run it stays below the level that had the trouble. The 429 pause and the time in `site-rate-limits.txt` above still apply, so the level never makes requests go out faster than that file allows. The concurrency limits still apply too (at most 5 chapters at once, 10 pages each).

The level is shared by every download from the site that runs at once (several groups of one site share one level), and it's saved in `site-parallel.txt`, next to `site-rate-limits.txt`, as one `site: number` line per site (for example `mangadex.org: 8`). It's written when the downloads end and right after a cut, and only for a site whose level moved. The next run starts from it. The file is plain text: change a number to make that site start there, or remove its line to have it learned again.

### Other servers for a page (`site-mirrors.txt`)

Some sites serve their pages from more than one server, and a page can be missing from one of them (mangalib's main image server answers 400 for some old pages that its second server has). `site-mirrors.txt`, next to `site-rate-limits.txt`, names the servers to try when a page fails, one line per server:

```
img2.imglib.info: img3.cdnlibs.org, img3.cdnrulib.link
```

A page whose address is on the host before the colon, and that still fails after its retries, is fetched from each host after it in turn, with the same path, until one works. Nothing is built in: a host the file doesn't list has no fallback, and the downloader never writes the file. Lines starting with `#` are comments. A page got from another server is noted in the log.

### A site that might need a VPN

A small **VPN?** tag appears next to a site's name (on its download bar, by the loaded series, and under a load that failed) when the site answered in a way that suggests it blocks the network you're on. Hover it to see why. It only ever says a VPN *might* help:

- the site answered 451, or a 403 page saying it isn't available in your country or region;
- the site's address didn't resolve, or resolved to an address such as 127.0.0.1, which is one way networks block sites.

Ordinary failures (a 400, a plain 403, a 404, a 429, a 5xx, a timeout) never show it, since a busy or broken site gives those too. The tag goes away once a chapter of the site downloads or its series loads. The app doesn't connect through a VPN or a proxy itself. This is an app feature; the command line prints the error as before.

## Settings and the update check

The gear icon (below the URL box on the start screen, in the header of the full
app) or Ctrl+, / ⌘, opens Settings: the app's language (English, Español,
Català, 日本語 or Українська; the Ukrainian guide is [uk/app.md](uk/app.md)), whether it opens with the simple start screen, and whether it
checks for new versions at startup.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="../assets/screenshots/13-settings-dark.webp">
  <img alt="The Settings dialog, with the app's language, the start screen and the update check" src="../assets/screenshots/13-settings-light.webp">
</picture>

When a newer release is out, a yellow *!* next to the version in the status bar
links to it. If the app was installed by the Windows installer or the Linux
`install.sh`, or on macOS sits in a folder you can write to, the *!* opens a
dialog instead that updates the app in one click, after checking the download
against the release's checksums. The Homebrew cask's copy keeps the link, since
`brew upgrade` updates it.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="../assets/screenshots/10-update-dark.webp">
  <img alt="The status bar with a yellow exclamation mark next to the version, linking to a newer release" src="../assets/screenshots/10-update-light.webp">
</picture>

The check asks our update server for the latest release, telling it the
program, its version, your OS and architecture, and the app's language (plus
your system's language when the app follows it). If our server doesn't answer,
the app asks GitHub directly. The server runs on Cloudflare, which sees your IP
address like any web host; only a hash of it, salted with a key deleted monthly,
is kept, to count monthly users.
