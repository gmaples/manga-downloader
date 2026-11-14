# Research, earlier work and corrections

This preserves context, including mistakes, without making those earlier
approaches part of the current build. External research was checked during
November 2026, but future assistants must recheck changing claims as needed.

## Initial Brave / MangaLib cache task

User had 155 MangaLib offline chapters in Brave. Browser tools were unavailable
in this chat, so direct DevTools inspection by the assistant never occurred.
The user supplied a browser-console export script and screenshots. The script
reads Cache Storage dw-api-cache (chapter metadata) and dw-images-cache (page
images), not IndexedDB. User console confirmed 155 cached chapters, 155 chapter
records and no missing metadata. The script is title-specific:
/manga/sssgeubjasalheonteo/chapters/ . Do not confuse that limitation with the
later generic Mangayomi source extension.

The original export created separate CBZ downloads. An attempted directory
picker revision failed because showDirectoryPicker was absent. Restoring 155
anchor downloads caused 155 save dialogs despite suggested browser settings;
the user force-killed Brave. Do not repeat that approach. A later one-ZIP draft
with a selectable cached title exists in legacy/, tested with synthetic ZIP
fixtures only, never verified end-to-end in the user's browser. It is not the
current objective and was not presented as a finished solution.

## Mangayomi and the working MangaLib source

User installed Mangayomi beta 0.9.95 on Windows. Original MangaLib 0.0.1 used
api.mangalib.me and failed with Network unreachable. Community source 0.6.0
from redcatpet/mangayomi-ru-sources used api2.mangalib.me and displayed empty
catalogues because its list parser swallowed errors.

User supplied source, auth headers privately in their UI and screenshots.
No credential is stored in this project. Their working website request was
https://api.cdnrulib.link/api/auth/me (HTTP 200). Direct navigation to catalogue
API URLs produced 403/1020. Browser navigation does not send the same headers
as the actual site; those tests alone did not prove authenticated app requests
would fail. Earlier assistant explanations overstated that conclusion.

Patched COMPLETE source in legacy/mangayomi-extension/mangalib.js:
- Runtime default https://api.cdnrulib.link/api, overriding old saved metadata.
- Added editable API URL preference.
- Retained account token/image-server preferences.
- Normalizes token whitespace/optional Bearer prefix.
- Exposes catalogue/details/pages errors instead of hiding them as no result.
- Rebases saved chapter URL paths to the active API.
The user confirmed catalogue results and reading seemed to work.

This extension review found no fixed manga ID or chapter-155 cap, but inherited
first-translation-branch-only selection, limited catalogue pagination parsing,
a single chapter-list fetch, and some silently omitted invalid page records.
Cover images go through a third-party proxy; chapter images do not.

Brave/Mangayomi auth token: paste the token value only, not the word Bearer.
Keep account credentials out of files, logs, source and shared screenshots.

## Performance and attempted AnymeX migration

User reported slow Mangayomi downloads on COMPUTER, including MangaDex. Tablet
and SD card had not yet been tested. Website felt fast. Source review found
Mangayomi page concurrency defaults to 1; that is a plausible throughput factor,
not a proven root cause of slow individual transfers. Assistant initially
oversold it as an explanation. User later suspected website-specific effects.

AnymeX 3.2.0 was suggested because source uses 3 concurrent manga pages. User
installed it, but it did not provide a usable migration:
- MangaDex through the Mangayomi bridge failed with relative URL without base.
  Bridge toMSource omitted apiUrl needed by that extension.
- Legacy Keiyoushi index.min.json returned upgrade placeholders. index.pb
  loaded real sources in AnymeX's Aniyomi system.
- MangaLib through Aniyomi stalled; its settings pane was blank. No verified
  fix for that runtime problem. No claim that every AnymeX installation is
  broken; it was a dead end for this user's setup.
The user rejected more speculative integrations and asked for broad research.

## Broad offline-reading survey (not current implementation scope)

Recurring established workflows found in reader discussions/documentation:
1. Native Android app downloads directly and reads offline: Mihon, Komikku,
   Kotatsu-Redo. Original Kotatsu is archived, not the maintained Redo fork.
2. Desktop downloader produces CBZ files, copied by USB/SD to a local reader.
3. Browser-assisted or browser-driven downloader collects images and exports.
4. Komga/Kavita manage existing files and sync/download through tablet clients.
   Suwayomi additionally downloads through website-source extensions.
5. Publisher apps offer offline chapters within catalogue/account restrictions.

Reader-only examples: CDisplayEx, Mihon's Local source. CBZ under series folders
is portable, but applications' private download-database layouts are not a
universal interchange format. Mihon's local library has its own local/ root.

Candidates and source links:
- Mihon: https://mihon.app/ ; https://mihon.app/docs/guides/local-source/
- Komikku: https://github.com/komikku-app/komikku
- Kotatsu-Redo: https://github.com/Kotatsu-Redo/Kotatsu-Redo
- Original archived Kotatsu: https://github.com/KotatsuApp/Kotatsu
- FMD2: https://github.com/dazedcat19/FMD2
- Manga Downloader: https://github.com/elboletaire/manga-downloader
  and https://mangadownloader.cat/
- Selector Downloader (small, preset-driven Chrome automation project, NOT
  verified for the user's sites): https://github.com/letSmilesz/selector-downloader
- HaruNeko (canary/prerelease): https://github.com/manga-download/haruneko
- Houdoku (last release inspected was December 2024): https://github.com/xgi/houdoku
- Komga: https://komga.org/docs/guides/cdisplayex/
- Kavita: https://wiki.kavitareader.com/guides/3rdparty/cdisplayex/
- Suwayomi: https://github.com/Suwayomi/Suwayomi-Server
- CDisplayEx SD/network locations: https://www.cdisplayex.com/mobile/settings/
- Publisher examples: https://www.viz.com/apps

Maintenance snapshot inspected (not permanent endorsements): Mihon 0.21.3
2026-10-25; Komikku 1.14.1 2026-07-17; Kotatsu-Redo 9.8.7 2026-11-13;
FMD2 packaged release 2.0.34.5 2025-03-17 but repository still active;
Manga Downloader 25.2.0 2026-11-11. Source coverage claims do not prove every
mirror, account or particular chapter works.

## Why the current project is Manga Downloader

User installed the desktop GUI v25.2.0. MangaLib .org URL was not recognized;
code's site tester only accepted mangalib.me. Changing the URL to .me reached
a 403. Its adapter hardcodes api2.mangalib.me and img2.imglib.info, without a
browser-assisted MangaLib implementation. This remains OUT OF SCOPE for the
folder feature. Do not promise a browser-window toggle fixes that adapter.

Crucially, user then reported Manga Downloader is MUCH FASTER than earlier
apps on working sources. They wanted automatic per-series folders. Code review
confirmed no existing auto-folder switch; output names/templates do not create
parent directories for CBZs. Thus the requested SMALL custom build emerged.

Language discussion:
- Title language is a fallible hint. Prefer chapter/release language metadata,
  selected filter and existing source hints.
- Folder [language] is a useful human-readable convention, not a universal
  reader standard. ComicInfo LanguageISO was discussed but is NOT included in
  the final minimal feature build. Original filenames/metadata stay intact.
- Final build scope and exact last-described behaviour are in BUILD_GUIDELINES.md.
