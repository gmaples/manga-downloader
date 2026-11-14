# Simple custom build: Series Folders and Guess Language

## Purpose and source

Add exactly two switches to Manga Downloader's desktop Options panel:
**Series Folders** and **Guess Language**. This document is the scope contract.

Base repository: https://github.com/elboletaire/manga-downloader
Base tag: v25.2.0
Base commit: e4ff1389e48c92bd890cd0731b85185f507eecc1
Work branch: series-folders
Target: Windows x64 portable desktop GUI. No installer or Android build.

## User-visible behaviour

- Place both switches in Options > Save, beside the existing destination controls.
- Both default to OFF when absent from saved settings. Remember both between runs.
- Series Folders OFF: preserve the original destination and behaviour exactly.
- Series Folders ON: automatically create/reuse one subfolder named from the
  fetched series title inside the chosen destination. Never derive the title
  from a URL when the app already has the series title.
- Guess Language OFF: use `<destination>/<Series Title>/`.
- Guess Language ON: use `<destination>/<Series Title> [<language>]/`.
- Guess Language is disabled and has no effect while Series Folders is OFF;
  retain its saved value so re-enabling Series Folders restores that choice.
- Display the resolved destination before download. The Folder picker still
  selects the parent destination; do not append nested title folders on reruns.
- Disable destination changes during an active run, matching existing controls.

## Language hints and limits

Resolve ONE folder-language tag for the selected run, in this order:

1. Explicit chapter/release language values already provided by the site.
   One known language: use it. Multiple distinct known languages: use `mul`.
2. Explicit selected language filter, excluding values such as any/all/blank.
3. A single-language source hint only if already exposed by existing metadata.
4. Lightweight local script hints from chapter titles, then the series title:
   Ukrainian-specific letters -> uk; Japanese kana -> ja; Korean Hangul -> ko;
   otherwise Cyrillic -> ru; otherwise Han characters -> zh.
5. If none applies, add no tag (user decision 2026-11-14: tag only a
 language known with confidence; no `und`/`unk`). Step 4 keeps only
 scripts of one language: Ukrainian-only letters, kana, Hangul.
 Latin script alone does not establish English.

Normalize known language names/codes to conventional tags such as en, ru,
uk, ja, ko, zh, pt-BR or zh-Hant using existing facilities where practical.
Unknown/unusable site codes do not become arbitrary directory strings.
Script guesses are fallbacks and may be wrong. No OCR, online detection,
AI services, new detection library, or giant source-specific language table.

To keep this build small, mixed-language selections are labelled [mul], not
silently split into multiple download jobs. Existing chapter/volume/bundle
packing semantics remain intact. Users can select one language for separate
language folders. This supersedes the earlier proposed automatic splitting.

## Folder safety and reader compatibility

- Replace Windows-invalid filename characters/control characters in the new
  directory component; strip trailing spaces/dots; protect reserved device
  names; use Untitled if no usable title remains. Keep Unicode text.
- The new component must not be absolute or introduce parent traversal.
- Use the SAME resolved directory for saving, existing-file checks, progress
  and completion paths, and Open folder. Repeated runs reuse that directory.
- Preserve existing collision handling; never overwrite existing archives or
  move, rename, delete or reorganize previously downloaded files.
- User decision 2026-11-14: the shared filename sanitizer also replaces
  `< > | *` and control characters with `_`, so names holding them change.
- Preserve current chapter filenames, number formatting, translation-group
  labels, archive metadata, image conversion and page order.
- No new ComicInfo.xml generation or metadata changes in this build.
- All current output/packing choices keep working; this feature only adds an
  outer directory. Examples with the existing chapter filenames preserved:

  Manga/Series Title/<existing chapter filename>.cbz
  Manga/Series Title [ru]/<existing chapter filename>.cbz

- This is a portable local-library layout, not an application's private
  downloaded-source database. Readers may require adding the root as a library;
  Mihon local files must be under its configured local directory.
- The user's chosen parent path is authoritative. No hardcoded J: drive.

## Implementation boundaries

Use the existing GUI, switches, settings persistence and architecture.
Keep logic testable outside the window; change shared destination plumbing only
where required. This is deliberately a GUI-only feature: no new CLI flags or
CLI behaviour. Document that exception to upstream's usual GUI/CLI parity.

No site adapters, API addresses, authentication, browser automation, downloader
concurrency/retries, networking, updater, branding, redesign, library manager,
server, synchronization, filename-template redesign or unrelated bug fixes.
No dependency upgrades, replacement frameworks, dependency/lockfile edits or
new runtime dependencies. Required generated bindings are allowed. Preserve
existing licenses and attribution. Update only the relevant manual and GUI
translations needed for these two controls.

## Build and verification

Use upstream's Windows build path and pinned dependencies: Go 1.26.x,
Wails v2.17.0, pnpm 12.15.0 and the existing frontend lockfile. Install missing
build tools locally as needed; do not modify the user's computer or settings.
If a pinned dependency/toolchain cannot be obtained, report the exact blocker;
do not silently downgrade, upgrade, substitute a mock or omit the real build.

Run the required Go/core and GUI checks, frontend tests/typecheck/build, and
focused tests for both switches, persistence, language precedence/mul/no tag,
safe directory names, reruns/skip-existing, and all existing packing modes.
Use local fixtures, not the user's account, token, library or downloads.
Verify a generated sample CBZ's integrity and image order. Cross-compile the
Windows x64 GUI. Mark native Windows execution as untested unless actually run
on Windows; cross-compilation alone is not a Windows UI test.

## Deliverables and stopping rule

Deliver a portable Windows x64 executable/ZIP, complete modified source archive,
this guidelines file, a short usage/build note, test results and SHA-256 hashes.
Commit guidelines and implementation to local Git, with the upstream base and
custom commit recorded. No snippets as a substitute for full source.

Do not publish releases, push to a remote, open upstream requests, install over
the user's existing copy or migrate user data. Stop when the two switches work,
required checks pass and the real build/source artifacts are prepared. Any
additional feature requires a new explicit user request.
