# Manga Downloader: Series Folders + Guess Language

Project handoff date: 2026-11-14 (America/Los_Angeles).

## Start here

This is a continuation package for ChatGPT Desktop, Codex, or another coding
assistant. It preserves the source, Git history, agreed scope, working legacy
code and material findings from a long troubleshooting/research conversation.
It is a curated project record, not a verbatim chat export.

Read BUILD_GUIDELINES.md at the source root first. That is the detailed scope
contract. Read AGENTS.md and .agents/skills/gui/SKILL.md before GUI changes.
The current request is to save a transferable project; implementation has NOT
started. Present the saved scope to the user when continuing; do not assume
that every historical idea remains authorized or requested.

## Current objective

Make a SIMPLE custom Windows x64 portable build of the existing open-source
Manga Downloader by elboletaire. Add exactly two Save-panel switches:

1. Series Folders
2. Guess Language

User's exact scope clarification: "'Series Folders' and 'Guess Language'".
The user explicitly requested a full guidelines file before the build, saying:
"I want a simple build" and "so that you don't go off the rails".

Source: https://github.com/elboletaire/manga-downloader
Base: v25.2.0 / e4ff1389e48c92bd890cd0731b85185f507eecc1
Branch: series-folders
Scope-only commit: 26a3b31

The desktop app already downloads MUCH faster for the user than Mangayomi and
AnymeX. Do not replace it, change its downloader, or resume app shopping.

## Agreed functionality and last-described scope

- Both switches initially off, saved between sessions.
- Series Folders: fetched series title becomes a safe subdirectory of the
  selected root. No URL slug parsing if the actual title is available.
- Guess Language: append [ru], [en], etc. to that folder. Inactive when Series
  Folders is off. Do not change archive metadata or chapter filenames.
- Metadata first: chapter/release language, explicit language filter, then an
  already-exposed single-language source hint, then simple script guesses.
  Unknown is und. Explicit multiple languages is mul.
- Earlier discussion proposed separating every language automatically. The
  final scope deliberately SIMPLIFIED this: one folder tag per run, mixed
  selections [mul], no new language-based splitting of packing jobs. The user
  has not separately responded to that final scope detail yet; surface it if
  relevant rather than silently claiming explicit approval of every detail.
- Destination save, preview, skip-existing and Open folder must agree.
- Preserve existing chapter/volume/merged/one-file modes and output formats.
- Standard reader layout: <chosen root>/<Series Title> [language]/<existing
  chapter filenames>.cbz. There is no universal standard for language folders.
- No special J: drive in code. Previously preferred root J:\Mangayomi\Manga;
  the latest app screenshot used J:\Manga. Respect whatever root user picks.

## Hard limits and preferences

- Full source, never replacement fragments. User explicitly insisted:
  "Always give me full source, and check it in for git".
- Commit all delivered source and scope to local Git.
- No unrelated redesign, framework replacement, dependencies/upgrades,
  website adapters, API/login fixes, concurrency changes, servers, sync,
  Android builds, installers, upstream PRs or remote publication.
- Existing file names, page numbering, groups, archive contents and conversion
  remain unchanged. No new ComicInfo generation in this feature.
- Be direct about failures and verification. Do not call source inspection or
  a successful cross-compile an actual Windows end-to-end test.
- Avoid another chain of speculative installs, toggles or screenshots.
- Full source ZIP and Windows executable/portable ZIP are the eventual build
  deliverables, with build notes and SHA-256. There is NO custom binary yet.

## Implementation state and toolchain

No implementation code modified. Only BUILD_GUIDELINES.md and handoff docs
were added to the pinned checkout. The legacy MangaLib code is historical and
belongs outside the new feature's implementation.

Upstream build stack:
- Go >=1.26.0, use 1.26.x.
- Wails v2.17.0.
- React/TypeScript/Vite frontend, pnpm 12.15.0 and frozen existing lockfile.
- Windows cross-build: make build/gui/windows GOARCH=amd64.
- Windows webview cross-build does not require cgo, per upstream workflow.
- Upstream .github/workflows/release-gui.yaml builds Windows from Linux.
- Test guidance: make test (root/core), make test/gui, frontend typecheck/build,
  and focused directory/language/persistence/packing tests.
- /usr/bin/go in the original cloud workspace was NOT a usable Go compiler
  (`go version` returned "Unknown option: version"). Node v24.22.1 was present;
  npm 11.9.0; a pnpm command wrapper existed. Wails was not located. Required
  toolchain installation remains to be done. Do not change dependencies to
  compensate for missing tools.

## Where to work

The archive contains the complete pinned source in source/ plus a Git bundle.
To restore the main Git working copy with local history:

    git clone manga-downloader.bundle manga-downloader
    cd manga-downloader
    git switch series-folders

If not using Git, open source/ directly. The source includes AGENTS.md, the
GUI skill, BUILD_GUIDELINES.md and this handoff. Temporary build work belongs
outside user deliverables. Required generated Wails bindings are in scope.

## Verification already done

- Inspected v25.2.0 GUI/backend/build files. There is no built-in automatic
  folder-per-series option. Filename templates do not create parent folders
  for CBZ output.
- User reports this downloader is much faster than the prior apps.
- No feature tests, custom app builds or native Windows runs performed yet.
- Legacy MangaLib extension syntax and mocked-response tests passed; user
  confirmed catalogue/reading worked after the API runtime override.

## Risks worth preserving, without expanding scope

- Complete CBZ files and safe paths matter; tests must cover skip-existing
  and reruns with the new destination.
- No assumption that title text reliably determines language; prefer metadata.
- A portable custom build should not overwrite the user's working install.
- Upstream app/extension updates may overwrite local customizations.
- The original project's AGPL license and attribution must accompany source.

See RESEARCH_AND_HISTORY.md for the earlier app investigations and failures.
See PROJECT_INSTRUCTIONS.md for ready-to-paste ChatGPT Project instructions.
