# Instructions for the ChatGPT Project or coding assistant

Work on Manga Downloader (elboletaire/manga-downloader), pinned to v25.2.0,
branch series-folders. Read PROJECT_HANDOFF.md and BUILD_GUIDELINES.md first.
Then obey repository AGENTS.md and the relevant GUI skill. User scope takes
precedence over repository defaults when explicitly narrower.

Goal: a minimal Windows x64 portable GUI build adding only two saved switches:
Series Folders and Guess Language. No application implementation has begun.
Do not restart app research, change site adapters or downloader speed, add
services or redesign the application.

Use fetched series titles and existing chapter-language metadata. Folder
naming must preserve local-reader compatibility. The selected root remains
user-controlled. Follow the guideline's exact switch defaults, language
precedence, mul/und handling and safe-directory rules.

Full source files and Git commits are mandatory. Never give the user code
fragments to splice in. Keep dependencies locked. Build through upstream's
existing Windows pipeline. Give honest test/build results, complete source,
portable executable archive and checksums. Do not publish or push anything.

The user prioritizes simple, usable software and concrete evidence. Avoid
speculative app recommendations, repeated confirmations and scope creep.
If blocked, state the exact technical blocker instead of changing the project.

Use source/ from the ZIP, or clone manga-downloader.bundle. Legacy scripts in
legacy/ are reference only and must not get mixed into this app feature.
