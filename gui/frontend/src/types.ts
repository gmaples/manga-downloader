// Types of the data the Go App sends to the window. They mirror the JSON of
// the DTOs in gui/app.go and gui/store.go; keep the two in step.

import type { LangPref } from './i18n';

/** todo is a chapter of a group from the last session that wasn't saved */
export type JobState = 'fetching' | 'downloading' | 'archiving' | 'done' | 'failed' | 'skipped' | 'todo';

export type LogLevel = 'info' | 'warn' | 'error';

/** How the chapters are packed: one file each, one per chapter with its parts merged, one per volume, or one for everything */
export type PackAs = 'chapters' | 'parts' | 'volumes' | 'bundle';

export interface Options {
  outputDir: string;
  format: 'cbz' | 'cbr' | 'raw' | 'epub';
  /** The way an EPUB's pages turn: manga reads right to left. Only EPUB output reads it */
  readingDirection: 'rtl' | 'ltr';
  packAs: PackAs;
  language: string;
  scanlator: string;
  /** Comma-separated: "avif", "avif,webp", "webp" or "none". See convertImages.ts. */
  convertImages: string;
  /** The quality (1-100) pages re-encoded to JPEG are written at, by convertImages and compress */
  jpegQuality: number;
  /** Re-encodes PNG pages to JPEG when that's smaller */
  compress: boolean;
  skipPages: string;
  /** Leaves out the chapters, volumes and bundles whose file is already in the folder */
  skipExisting: boolean;
  filenameTemplate: string;
  browserVisible: boolean;
  /** Saves into a folder named after the series, inside outputDir. App-only: the CLI has no flag */
  seriesFolders: boolean;
  /** Adds the run's language tag to that folder's name ("Title [en]"); only read with seriesFolders */
  guessLanguage: boolean;
  /** Refuses titles a site rates above age; off, every title loads */
  ageCheck: boolean;
  /** The user's age for ageCheck: 16 or 18, kept while the check is off */
  age: number;
}

export interface Run {
  chapters: number;
  bundle: boolean;
  /** The run packs one file per volume */
  volumes: boolean;
  /** How many progress rows the run ends up with: chapters, volumes plus the chapters outside them, or 1 bundle */
  rows: number;
}

/** A text the Go side sent with its key and args: translated by say() (see i18n), its text in English */
export interface Message {
  key: string;
  args?: Record<string, unknown>;
  text: string;
}

export interface Job {
  title: string;
  /** The title as a message, when the GUI wrote it (a bundle's or a volume's) */
  titleMsg?: Message;
  /** The release's language and group (" [en] [Alpha]"), shown apart from the title; empty when none */
  tags: string;
  number: number;
  /** Each page's download and archive steps are counted (core's DownloadWeight and ArchiveWeight, 4 + 1); a volume's row is a fixed scale instead */
  total: number;
  done: number;
  state: JobState;
  reason: string;
  reasonMsg?: Message;
  /** The bundle's or volume's current phase; empty for single chapters */
  status: string;
  statusMsg?: Message;
  /** The row of a volume rather than a chapter */
  volume: boolean;
  /** Names the row's chapter for restart(); unset on a volume's or bundle's row */
  key?: string;
  /** restart(group, key) downloads its chapter again: it failed, or is a restored one to do */
  restart?: boolean;
}

/** A line of the run's failures: what failed, and why */
export interface Failure {
  /** The release or volume the line is about; "" when it names none */
  label: string;
  /** The reason in English, shown when the window has no message for it */
  reason: string;
  /** The reason as the window words it, when it has window words of its own */
  reasonMsg?: Message;
}

export interface Summary {
  dir: string;
  cancelled: boolean;
  saved: number;
  /** Volume files written */
  volumes: number;
  /** Those files are chapters with their parts merged */
  parts: boolean;
  bundled: boolean;
  bundle: string;
  skipped: number;
  /** Chapters left out because their file was already in the folder */
  existing: number;
  failures: Failure[];
  /** Set when a bundle couldn't be written */
  error: string;
  /** error as a message, when the window translates it */
  errorMsg?: Message;
}

/**
 * What a language or scanlator selector offers, for a series the site lists
 * them for. Values are what --language and --scanlator take.
 */
export interface FilterChoice {
  values: string[];
  /** The site also takes "all": every value, mixed */
  all: boolean;
  /** The value the loaded list shows; "" when the site shows no single one */
  selected: string;
}

/** One press of Download: its own rows, cancel and summary */
/**
 * The progress of a site that's downloading: the first chapter in flight of
 * its running groups (title "" between chapters, total 0 while its page list
 * is fetched), until, the end of its pause after a 429 in Unix ms, 0 when
 * it isn't paused, and parallel, how many of its requests may be under way at
 * once as learned from its speed (0 before its first request)
 */
export interface SiteBar {
  site: string;
  title: string;
  tags: string;
  done: number;
  total: number;
  until: number;
  parallel: number;
  /** Why a VPN might help with the site ("region", "dns"), "" when nothing says so */
  vpn: string;
}

export interface Group {
  id: number;
  /** The series' title */
  title: string;
  running: boolean;
  cancelling: boolean;
  run: Run;
  jobs: Job[];
  /** null while it runs */
  summary: Summary | null;
  /** A group of the last session, read back from history.txt: its rows are done or to do, and resume() loads it */
  restored: boolean;
  /** The series' URL */
  url: string;
  /** The site rates the series 18+: its title is masked until clicked */
  adult: boolean;
  /** The chapters restart(id, '') downloads again: failed, cancelled or still to do */
  unfinished?: number;
  /** The duplicate files its download moved to the Recycle Bin when it started; null when that didn't run */
  cleanup?: Cleanup | null;
  /** What it is doing that no row shows (looking for duplicates, moving them); null when nothing */
  status?: Message | null;
}

/** What Delete series would take from the loaded series' folder, for the confirmation */
export interface DeleteInfo {
  title: string;
  dir: string;
  /** The series' own folder, taken whole */
  whole: boolean;
  files: number;
  /** The files' size, written to read (1.4 GB) */
  size: string;
}

/** A Delete series' count, after each batch */
export interface DeleteProgress {
  done: number;
  total: number;
  failed: number;
}

/** What the correlation of a group's folder did when its download started */
export interface Cleanup {
  removed: number;
  /** Their size in bytes */
  bytes: number;
  /** Duplicates that couldn't be moved */
  failed: number;
}

/** One series of the history pop-up, newest first */
export interface HistoryItem {
  title: string;
  url: string;
  /** The site's home page; "" when the URL has none */
  site: string;
  /** The site rates it 18+: masked until clicked */
  adult: boolean;
  /** When it was last loaded or downloaded, in Unix ms; 0 when unknown */
  when: number;
  /** Its chapters saved and still to do */
  done: number;
  todo: number;
}

/** The window's state, without the chapter list, selection and log */
export interface State {
  url: string;
  loading: boolean;
  loaded: boolean;
  loadError: string;
  /** loadError as a message, when the GUI wrote it */
  loadErrorMsg?: Message;
  /** Why a VPN might help with the site of url ("region", "dns"), "" when nothing says so */
  vpn: string;
  title: string;
  /** The site rates the loaded series 18+: its title is masked until clicked */
  adult: boolean;
  chapterCount: number;
  hasDuplicates: boolean;
  /** One number is listed more than once in a language, and the groups tell the copies apart */
  hasGroupDuplicates: boolean;
  /** The language or scanlator changed since the list was fetched */
  stale: boolean;
  /** The language the list was fetched with (what the filter applies to) */
  loadedLanguage: string;
  /** The scanlator the list was fetched with */
  loadedScanlator: string;
  selectedCount: number;
  /**
   * The running group of the loaded series, 0 when none runs: Download then
   * adds the fresh selected chapters (those not in its run yet) to it
   */
  addTo: number;
  fresh: number;
  /** Set, when no group of the series runs, while the selection is the one Download was last pressed with */
  unchanged: boolean;
  /** Any group is downloading */
  running: boolean;
  /** Every running group has been asked to stop */
  cancelling: boolean;
  /** The downloads started, running or finished, in the order they started */
  groups: Group[];
  /** A bar for each site the running groups download from, in the order they started */
  sites: SiteBar[];
  /** The outcome of the group that ended last; null while any group runs or before one ended */
  summary: Summary | null;
  options: Options;
  volumes: Volumes;
  /** null when the site doesn't list languages (or nothing is identified yet) */
  languages: FilterChoice | null;
  /** null when the site doesn't list scanlation groups */
  scanlators: FilterChoice | null;
  version: string;
  /** The GitHub release of this build; "" for a dev build, which has none */
  releaseURL: string;
  /** null when there's no newer release (or the check hasn't found one) */
  update: Update | null;
  /** The loaded site opens the browser window by itself, whatever options.browserVisible says */
  browserVisibleAuto: boolean;
  /** What to tell the user while the browser window waits for them; "" when it isn't */
  browserAlert: string;
  /** browserAlert as a message */
  browserAlertMsg?: Message;
  /** The filename template a blank option stands for */
  filenameTemplateDefault: string;
  /** What is wrong with options.filenameTemplate; "" when nothing */
  filenameTemplateWarning: string;
  /** The folder a run of the selection would save into (the series folder when that's on); "" before a load */
  destination: string;
  prefs: Prefs;
}

/** App-only preferences: what the CLI has no flag for, so they aren't Options */
export interface Prefs {
  /** The window's language; '' follows the system. Go omits it when empty, so it can be absent */
  uiLanguage?: LangPref;
  /** Check GitHub for a newer release at startup */
  checkUpdates: boolean;
  /** Open on the simple view (logo and URL box) until a load starts */
  simpleStart: boolean;
  /** The side panel's width the user dragged it to, in CSS pixels; absent (Go omits 0) keeps the default */
  sideWidth?: number;
  /** The log drawer's height the user dragged it to, in CSS pixels; absent keeps the default */
  logHeight?: number;
}

/** The Volumes box, used when the options pack as volumes */
export interface Volumes {
  /** The volumes a run packs, as chapter ranges per volume ("1-7/8-16"); "" for the site's own */
  text: string;
  /** The site's volumes in the same syntax; "" when the site doesn't list them or chapter ranges can't group them as it does */
  site: string;
  /** How many volumes the site files chapters under, 0 when none does */
  listed: number;
}

/** A release newer than the running build, from the update check */
export interface Update {
  /** The newest release's tag, as GitHub names it */
  latest: string;
  /** The page the update is linked to: the latest release */
  url: string;
  /** Update() can install it: an installed copy, and its package and checksums on the release */
  oneClick: boolean;
  /** Open the update prompt on load; only the mock sets it (the 12-update-prompt screenshot) */
  prompt?: boolean;
}

export interface Chapter {
  index: number;
  number: number;
  title: string;
  /** Only set when hasDuplicates: the copies of one number differ by language */
  language: string;
  /** Only set when hasGroupDuplicates: the scanlation group that made the release */
  group: string;
  /** The volume the site files it under; 0 when the site doesn't say */
  volume: number;
  /** The listing marks it locked: it can't be selected, so no run takes it */
  locked: boolean;
  /** The part as its label writes it ("Part 2", "前編"); unset without one */
  part?: string;
  /** With a part: the title without it, '' when the site gives none */
  wholeTitle?: string;
  /** The site lists it without a number: a parts run packs it alone */
  unnumbered?: boolean;
}

/**
 * What became of a chapter of the list, shown beside its checkbox: '' is no
 * file in the folder and no download; done is saved this session or already
 * in the folder; downloading has begun and queued waits in a running
 * download; failed and cancelled were left unsaved by a download
 */
export type ChapterMark = '' | 'done' | 'downloading' | 'queued' | 'failed' | 'cancelled';

export interface LogLine {
  level: LogLevel;
  /** The English text: what copying the log gives, and what shows when the window can't translate msg */
  text: string;
  /** The line as a translatable message, when the Go side sent one (see say() in i18n) */
  msg?: Message;
}
