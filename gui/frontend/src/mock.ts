// A stand-in for the Go App, for working on the UI in a plain browser
// (pnpm dev) and in tests. It keeps its own state, simulates a load and a
// download, and pushes the same events the Go side does.
//
// Add ?mock to the URL to use it inside the app too. ?scenario= starts it in
// a given state, so screenshots need no clicking: empty, loading, error,
// loaded, stale, running, bundle, cancelling, done, update, check, volumes
// (loaded, packing by volume, with three volumes and a half selected) or
// volumes-running (a run by volume in progress), parts (loaded, packing each
// chapter with its parts merged, as &parts lists them) or two-groups (a first
// series' download that ended, and the loaded series' one running after it,
// each under the header of its group), restored (loaded, with two groups of the
// last session back from history.txt: Vinland Saga with chapters to do, and an
// adult title, masked until clicked, all done). update is loaded too, as a
// release build (v1.17.0 with &shot, v1.12.0 without) with a newer release
// (v1.18.0 with &shot, v1.13.0 without): the status bar shows the yellow "!".
// check is loading a site that needs the browser window, with the alert asking
// the user to look at it; loading a URL with "check" in it does the same until
// the load ends. &freeze stops the simulated run from moving (stable
// screenshots), &dupes lists some numbers twice in two languages, &chapters=N
// shows the newest N chapters, &parts lists a series whose chapters come in
// two parts (a real listing, its later chapters locked), &novolumes lists no volumes (the series
// otherwise files its chapters by their real One Piece volume), &locked marks
// some chapters locked (see below), &fail makes
// the fourth chapter fail halfway and &skip makes the sixth one come out
// skipped, every page excluded by --skip-pages (both off by default; every run
// saves every chapter without them), &exists (with running or done) leaves the first two chapters
// out as already in the folder, at the top and counted apart in the summary, and
// &fetching has each chapter's row spend its
// first few ticks fetching its page list (frozen, the newest rows are still
// fetching), &epub starts the output as EPUB, &prompt
// (with update) makes the update one-click and opens its prompt on load, &shot
// uses production release versions and a macOS output path (for screenshots),
// &nosimple turns the simpleStart pref off (an empty window opens on the full view),
// and VITE_MOCK=1 at build time includes the mock in production builds (e.g.
// for the screenshot capture build; set via Wails frontend env). ?lang=en|es|ca|ja
// starts the window in that language, ?lang=system follows the browser's, and
// without it the window is English, so screenshots and dev stay English. Under
// multi the language filter shows that language and the list is in it, titled
// from OP_TITLES; every fifth chapter also appears once in another language. A list in ja
// (multi's ja, or the window's on a site without languages) is titled from OP_TITLES_JA where it has one.
//
// &site= picks what the series lists for the two filters: multi (the default:
// languages ca, en, es with All, the window's language shown by default, no groups), plus
// (Manga Plus: languages without All, the URL edition shown by default),
// groups (groups with All, every group shown by default, each chapter listed
// once per group, no languages), or none (both fields disabled: a typed value shows cleared, as
// the site ignores it, and is kept for the sites that take it). A language or
// group the series doesn't list falls back on load, as the backend does: typed
// "xx" under multi loads "all", under plus the edition the URL points at.
// Typed values match ignoring case, and take the spelling the series lists.
// &locked lists every 50th chapter as locked: the list shows its lock
// and never selects it, as the backend does.

import type { Api, Unsubscribe } from './api';
import { createT, LANGS, resolveLanguage, type Lang, type LangPref, type Vars } from './i18n';
import type {
  Chapter,
  ChapterMark,
  FilterChoice,
  HistoryItem,
  Job,
  LogLine,
  Message,
  Options,
  Prefs,
  SiteBar,
  State,
  Summary,
  Update,
} from './types';
import { hostOf } from './run';
import { groupChapters, groupParts, listedVolumes, parseVolumes, siteVolumes } from './volumes';
import { OP_TITLES, OP_TITLES_JA } from './mock-onepiece';

const TEMPLATE_DEFAULT =
  '{{.Series}} {{.Number}} - {{.Title}}{{if .Group}} [{{.Group}}]{{end}}{{if gt .Version 1}} v{{.Version}}{{end}}';
const TEMPLATE_WARNING =
  'This template has no {{.Number}}, {{.Title}}, {{.SiteTitle}} or {{.Volume}}, so files only differ by a version suffix.';

/** Mirrors packer.TemplateWarning */
export function templateWarning(templ: string): string {
  const t = templ.trim();
  return t === '' || /\{\{-?\s*[^}]*\.(Number|Title|SiteTitle|Volume)\b/.test(t) ? '' : TEMPLATE_WARNING;
}

/** Total chapters in the One Piece feed (MangaDex, ch 598-1194) */
const CHAPTERS = OP_TITLES.ca.length; // 597
const PAGES = 12;
// a page's steps while downloading and while archiving, as core's
// DownloadWeight and ArchiveWeight: a bar fills 80% in the download
const DOWNLOAD_WEIGHT = 4;
const ARCHIVE_WEIGHT = 1;
const STEPS_PER_PAGE = DOWNLOAD_WEIGHT + ARCHIVE_WEIGHT;
const TICK_MS = 160;
// the rows (0-based, in run order) that &exists leaves out as already in the folder: the
// first two, so they show at the top of the activity panel
const EXISTS_AT = [0, 1];

const SCENARIOS = [
  'empty',
  'loading',
  'error',
  'loaded',
  'stale',
  'running',
  'bundle',
  'cancelling',
  'done',
  'update',
  'check',
  'volumes',
  'volumes-running',
  'parts',
  'two-groups',
  'restored',
] as const;

export type Scenario = (typeof SCENARIOS)[number];

const SITES = ['multi', 'plus', 'groups', 'none'] as const;

const HOUR = 3600 * 1000;
// a fixed time, so screenshots don't change from day to day
const MOCK_NOW = Date.UTC(2026, 10, 21, 18, 30);

/** The history pop-up's series: one adult title among them, masked until clicked */
const MOCK_HISTORY: HistoryItem[] = [
  { title: 'One Piece', url: 'https://example.com/manga/one-piece', site: 'https://example.com/', adult: false, when: MOCK_NOW, done: 24, todo: 0 },
  { title: 'Vinland Saga', url: 'https://example.com/manga/vinland-saga', site: 'https://example.com/', adult: false, when: MOCK_NOW - 3 * HOUR, done: 3, todo: 4 },
  { title: 'Night Garden', url: 'https://example.com/manga/night-garden', site: 'https://example.com/', adult: true, when: MOCK_NOW - 30 * HOUR, done: 3, todo: 0 },
];

export type MockSite = (typeof SITES)[number];

export interface MockSetup {
  scenario: Scenario;
  /** The simulated run doesn't advance */
  freeze: boolean;
  /** Some numbers are listed twice, in two languages */
  dupes: boolean;
  chapters: number;
  site: MockSite;
  /** The series lists no volumes */
  novolumes: boolean;
  /** The series splits its chapters in parts */
  parts: boolean;
  /** The fourth chapter fails halfway through download */
  fail: boolean;
  /** The sixth chapter comes out skipped (every page excluded by --skip-pages) */
  skip: boolean;
  /** With running or done, the first two chapters come out skipped because their files are already in the folder */
  exists: boolean;
  /** Every 50th chapter is listed as locked */
  locked: boolean;
  /** Use production release versions and a macOS output path (for screenshots) */
  shot: boolean;
  /** Chapter rows start fetching their page list before they download */
  fetching: boolean;
  /** Start with EPUB as the output format */
  epub: boolean;
  /** With the update scenario: one-click update, its prompt open on load */
  prompt: boolean;
  /** The simpleStart pref: an empty window opens on the simple view (off with &nosimple) */
  simpleStart: boolean;
  /** The window's language preference the mock starts with */
  uiLanguage: LangPref;
}

/** What the mock series offers for each filter; null when the site lists none */
interface MockOffer {
  values: string[];
  all: boolean;
  /** The value shown when no filter is set, for the sites that show one */
  current: string;
}

interface MockOffers {
  language: MockOffer | null;
  scanlator: MockOffer | null;
}

const OFFERS: Record<MockSite, MockOffers> = {
  // multi's current language is the window's, see offersFor
  multi: { language: { values: ['ca', 'en', 'es', 'ja'], all: true, current: 'en' }, scanlator: null },
  plus: { language: { values: ['en', 'es'], all: false, current: 'en' }, scanlator: null },
  groups: { language: null, scanlator: { values: ['Alpha', 'Delta'], all: true, current: '' } },
  none: { language: null, scanlator: null },
};

/** A site's offers; multi shows the window's language, which its list is in */
function offersFor(site: MockSite, lang: Lang): MockOffers {
  const offers = OFFERS[site];
  if (site !== 'multi' || offers.language === null) {
    return offers;
  }
  const current = offers.language.values.includes(lang) ? lang : offers.language.current;
  return { ...offers, language: { ...offers.language, current } };
}

const en = createT('en');

/** A message as the Go side sends it: its key, its args, and the English text */
function msgOf(key: string, args?: Vars): Message {
  return args === undefined ? { key, text: en(key) } : { key, args, text: en(key, args) };
}

/** A rejection as the backend gives it: the message's JSON, which the window translates */
const reject = (key: string, args?: Vars): string => JSON.stringify(msgOf(key, args));

/** A job's phase: its status and its message, none when it has no phase */
const phaseOf = (m?: Message) => ({ status: m?.text ?? '', statusMsg: m });

/** A failed job's reason, with its message */
const failedWith = (m: Message) => ({ reason: m.text, reasonMsg: m });

/** The line for a filter that was reset, as the backend words it */
function fallbackLine(kind: 'language' | 'group', value: string, fallback: string): Message {
  const args = { value, fallback };
  if (kind === 'group') {
    if (fallback === 'all') return msgOf('log.groupFallbackAll', args);
    return fallback === '' ? msgOf('log.groupFallbackNone', args) : msgOf('log.groupFallbackValue', args);
  }
  if (fallback === 'all') return msgOf('log.languageFallbackAll', args);
  return fallback === '' ? msgOf('log.languageFallbackNone', args) : msgOf('log.languageFallbackValue', args);
}

const MOCK_ALERT = msgOf('browser.alertNamed', { browser: 'Brave', host: 'check.example.com' });

const REPO = 'https://github.com/elboletaire/manga-downloader';
const RELEASE_VERSION_DEV = 'v1.12.0';
const RELEASE_VERSION_SHOT = 'v23.0.0';
const UPDATE_VERSION_DEV = 'v1.13.0';
const UPDATE_VERSION_SHOT = 'v23.0.1';

function siteFrom(search: string): MockSite {
  const value = new URLSearchParams(search).get('site');
  return (SITES as readonly string[]).includes(value ?? '') ? (value as MockSite) : 'multi';
}

function scenarioFrom(search: string): Scenario {
  const value = new URLSearchParams(search).get('scenario');
  return (SCENARIOS as readonly string[]).includes(value ?? '') ? (value as Scenario) : 'empty';
}

/** ?lang=system is the preference that follows the system; anything else but en, es, ca or ja is English */
function uiLanguageFrom(search: string): LangPref {
  const value = new URLSearchParams(search).get('lang');
  if (value === 'system') {
    return '';
  }
  return (LANGS as readonly string[]).includes(value ?? '') ? (value as LangPref) : 'en';
}

/** The mock's whole setup from the page's query string */
export function mockSetupFrom(search: string): MockSetup {
  const params = new URLSearchParams(search);
  const n = Number(params.get('chapters'));
  return {
    scenario: scenarioFrom(search),
    freeze: params.has('freeze'),
    dupes: params.has('dupes'),
    chapters: Number.isInteger(n) && n > 0 ? n : CHAPTERS,
    site: siteFrom(search),
    novolumes: params.has('novolumes'),
    parts: params.has('parts'),
    fail: params.has('fail'),
    skip: params.has('skip'),
    exists: params.has('exists'),
    locked: params.has('locked'),
    shot: params.has('shot'),
    fetching: params.has('fetching'),
    epub: params.has('epub'),
    prompt: params.has('prompt'),
    simpleStart: !params.has('nosimple'),
    uiLanguage: uiLanguageFrom(search),
  };
}

// the extension each format's output carries; raw writes a folder without one
const EXTENSIONS: Record<Options['format'], string> = { cbz: '.cbz', cbr: '.cbr', epub: '.epub', raw: '' };

function defaultOptions(shot: boolean, epub: boolean): Options {
  return {
    outputDir: shot ? '/Users/reader/Downloads/Manga' : '/home/reader/Downloads',
    format: epub ? 'epub' : 'cbz',
    readingDirection: 'rtl',
    packAs: 'chapters',
    language: '',
    scanlator: '',
    convertImages: 'avif',
    jpegQuality: 90,
    compress: false,
    skipPages: '',
    skipExisting: false,
    filenameTemplate: TEMPLATE_DEFAULT,
    browserVisible: false,
    seriesFolders: false,
    guessLanguage: false,
    ageCheck: false,
    age: 18,
  };
}

/** The One Piece titles in a language; the dataset has no Japanese, so ja gets the English ones. */
function titlesIn(lang: Lang): (typeof OP_TITLES)['en'] {
  return Object.hasOwn(OP_TITLES, lang) ? OP_TITLES[lang as keyof typeof OP_TITLES] : OP_TITLES.en;
}

/** A chapter's title: its Japanese one under the ja window where OP_TITLES_JA has it, else as given */
function titleIn(number: number, title: string, japanese: boolean): string {
  return japanese ? (OP_TITLES_JA[number] ?? title) : title;
}

/**
 * The newest `count` chapters, titled in `lang`. With withLanguages they are tagged with it, and every
 * fifth also appears once in `other` (the two lists share their chapters, in order). Volume 0: not collected.
 */
function mockChapters(
  count: number,
  lang: Lang,
  withLanguages: boolean,
  other: Lang,
  withVolumes = true,
  withLocked = false,
  japanese = false,
): Chapter[] {
  const list: Chapter[] = [];
  const rows = titlesIn(lang);
  const otherRows = titlesIn(other);
  for (let i = Math.max(0, rows.length - count); i < rows.length; i++) {
    const [number, title, vol] = rows[i];
    const volume = withVolumes ? vol : 0;
    const locked = withLocked && isMockLocked(number);
    const shown = titleIn(number, title, japanese);
    list.push({
      index: list.length,
      number,
      title: shown,
      language: withLanguages ? lang : '',
      group: '',
      volume,
      locked,
    });
    if (withLanguages && number % 5 === 0) {
      list.push({
        index: list.length,
        number,
        title: titleIn(number, otherRows[i][1], japanese),
        language: other,
        group: '',
        volume,
        locked,
      });
    }
  }
  return list;
}

/**
 * A series published in parts: chapter 1 whole, then each chapter in two,
 * everything past chapter 4 locked but the second half of 19. The parts share
 * their chapter's One Piece title, plus their part, as the grabbers give them.
 */
function partsChapters(lang: Lang, japanese = false): Chapter[] {
  const list: Chapter[] = [];
  const withPart = (title: string, part: string) => (title === '' ? part : `${title} ${part}`);
  const add = (number: number, title: string, locked: boolean, part?: string) =>
    list.push({
      index: list.length,
      number,
      title,
      language: '',
      group: '',
      volume: 0,
      locked,
      ...(part ? { part, wholeTitle: titleOf(number) } : {}),
    });
  const newest = titlesIn(lang)
    .slice(-20)
    .map(([number, title]) => titleIn(number, title, japanese));
  const titleOf = (n: number) => newest[n - 1];
  add(1, titleOf(1), false);
  for (let n = 2; n <= 20; n++) {
    add(n, withPart(titleOf(n), 'Part 1'), n > 4, 'Part 1');
    add(n, withPart(titleOf(n), 'Part 2'), n > 4 && n !== 19, 'Part 2');
  }
  return list;
}

/** The error a locked chapter fails with, as core.ErrLocked says it */
const LOCKED_REASON = msgOf('job.reason.locked');

/** A chapter's job label: its title, or its number where the list has none */
const labelOf = (c: Chapter) => (c.title === '' ? `Chapter ${c.number}` : c.title);

/** Every 50th chapter is locked (a listing's coin lock), so a few show in dev. */
function isMockLocked(number: number): boolean {
  return number % 50 === 0;
}

/** Every chapter by Alpha, and every fifth again by Delta, so a number repeats in one language. */
function groupedChapters(list: Chapter[]): Chapter[] {
  const out: Chapter[] = [];
  for (const c of list) {
    out.push({ ...c, group: 'Alpha' });
    if (c.number % 5 === 0) {
      out.push({ ...c, group: 'Delta' });
    }
  }

  return out.map((c, index) => ({ ...c, index }));
}

type Listener<T> = (v: T) => void;

class Bus<T> {
  private listeners = new Set<Listener<T>>();
  subscribe(cb: Listener<T>): Unsubscribe {
    this.listeners.add(cb);
    return () => {
      this.listeners.delete(cb);
    };
  }
  emit(v: T): void {
    for (const cb of this.listeners) {
      cb(v);
    }
  }
}

/** One simulated download group, as the backend's: its own run, rows and outcome */
interface MockGroup {
  id: number;
  title: string;
  url: string;
  /** The folder and the file extension it saves with, as the options were when it started */
  dir: string;
  ext: string;
  running: boolean;
  cancelling: boolean;
  // a group of the last session, back from history.txt; adult masks its title
  restored: boolean;
  adult: boolean;
  runChapters: number;
  // the bundle option as the run started with it, as the backend's RunInfo
  runBundle: boolean;
  runVolumes: boolean;
  runParts: boolean;
  runRows: number;
  jobs: Job[];
  summary: Summary | null;
  // the chapters of the run that &exists left out as already in the folder
  existing: number;
  // the chapters of the run that haven't started; the backend only creates a
  // job when a chapter starts, so the mock does too
  pending: Chapter[];
  // how many chapters each volume row of the run holds, by title
  volumeChapters: Map<string, number>;
  // with &fetching, the ticks a chapter's row has left fetching its page list
  fetchLeft: Map<string, number>;
  timer?: ReturnType<typeof setInterval>;
  // orders the groups that ended
  ended: number;
  // the chapters in the run, by index, which Download doesn't add again
  keys: Set<number>;
}

export function createMockApi(setup: Scenario | Partial<MockSetup> = 'empty'): Api {
  const {
    scenario,
    freeze,
    dupes,
    chapters: chapterCount,
    site,
    novolumes,
    parts: withParts,
    fail,
    skip,
    exists,
    locked,
    shot,
    fetching,
    epub,
    prompt,
    simpleStart,
    uiLanguage,
  } = {
    scenario: 'empty' as Scenario,
    freeze: false,
    dupes: false,
    chapters: CHAPTERS,
    site: 'multi' as MockSite,
    novolumes: false,
    parts: false,
    fail: false,
    skip: false,
    exists: false,
    locked: false,
    shot: false,
    fetching: false,
    epub: false,
    prompt: false,
    simpleStart: true,
    uiLanguage: 'en' as LangPref,
    ...(typeof setup === 'string' ? { scenario: setup } : setup),
  };
  // the window's language: the one a multi site shows and lists in
  const uiLang = resolveLanguage(uiLanguage, typeof navigator === 'undefined' ? [] : navigator.languages);
  const offers = offersFor(site, uiLang);
  const options = defaultOptions(shot, epub);
  // the app preferences, kept as the backend keeps them in gui.json
  const prefs: Prefs = { uiLanguage, checkUpdates: true, simpleStart };
  let url = '';
  let loading = false;
  let loadError: Message | undefined;
  let loaded = false;
  let title = '';
  let chapters: Chapter[] = [];
  let selected = new Set<number>();
  // the Volumes box and the site's volumes, as the backend keeps them per load
  let volumesText = '';
  let volumesSite = '';
  let volumesListed = 0;
  let applied = { language: '', scanlator: '' };
  // whether a load has been attempted, so the filters are the series' own choices
  let identified = false;
  // the downloads started, in the order they started (see MockGroup)
  let runs: MockGroup[] = [];
  // the history pop-up's series, newest first, as history.txt keeps them
  let history: HistoryItem[] = MOCK_HISTORY.map((h) => ({ ...h }));
  let lastRunID = 0;
  // the selection Download was last pressed with, by URL, as the backend's started
  const started = new Map<string, Set<number>>();
  let lastEnded = 0;
  let log: LogLine[] = [];
  // the loaded site needs the browser window, and the alert about it
  let browserAuto = false;
  let browserAlert: Message | undefined;

  const stateBus = new Bus<State>();
  const chaptersBus = new Bus<Chapter[]>();
  const selectionBus = new Bus<number[]>();
  const marksBus = new Bus<ChapterMark[]>();
  const logBus = new Bus<LogLine[]>();

  const selectionOf = () => [...selected].sort((a, b) => a - b);
  const pushLog = (level: LogLine['level'], line: string | Message) => {
    const entry: LogLine = typeof line === 'string' ? { level, text: line } : { level, text: line.text, msg: line };
    log = [...log, entry].slice(-500);
    logBus.emit(log);
  };
  // what a filter's selector shows: the value the list was loaded with, else
  // the site's default (its current value, else every value where it takes all)
  const filterOf = (kind: 'language' | 'scanlator'): FilterChoice | null => {
    const offer = offers[kind];
    if (!identified || offer === null) {
      return null;
    }
    const loadedWith = applied[kind];
    let selected = loadedWith;
    if (loadedWith === '') {
      selected = offer.current !== '' ? offer.current : offer.all ? 'all' : '';
    }
    return { values: [...offer.values], all: offer.all, selected };
  };
  // the filter's language, else the site's current one (the window's where the site has no languages)
  const listedLanguage = (): Lang => {
    const offer = offers.language;
    if (offer === null) return uiLang;
    return (offer.values.includes(options.language) ? options.language : offer.current) as Lang;
  };
  // the language a list's repeated chapters are in: the site's first other one
  const otherLanguage = (lang: Lang): Lang => {
    const values: readonly string[] = offers.language?.values ?? LANGS;
    return values.find((v) => v !== lang) as Lang;
  };
  // a typed value the series doesn't offer falls back as the backend does: to
  // "all" where the site takes it, else to the site's current value, else to no
  // filter. A value that matches one ignoring case takes the listed spelling.
  const fallbackFor = (kind: 'language' | 'scanlator', line: 'language' | 'group'): void => {
    const offer = offers[kind];
    const typed = options[kind];
    if (offer === null || typed === '' || typed === 'all') {
      return;
    }
    const listed = offer.values.find((v) => v.toLowerCase() === typed.toLowerCase());
    if (listed !== undefined) {
      options[kind] = listed;
      return;
    }
    const fallback = offer.all ? 'all' : offer.current;
    options[kind] = fallback;
    pushLog('info', fallbackLine(line, typed, fallback));
  };

  const releaseVersion = shot ? RELEASE_VERSION_SHOT : RELEASE_VERSION_DEV;
  // update stands for a release build, and so does every scenario with &shot,
  // which reports a release version: the status bar links it to its release
  const isReleaseBuild = (sc: Scenario) => sc === 'update';
  const reportsRelease = shot || isReleaseBuild(scenario);

  // what Download would do, as the backend's DownloadMode
  const downloadMode = (): Pick<State, 'addTo' | 'fresh' | 'unchanged'> => {
    if (!loaded || loading) {
      return { addTo: 0, fresh: 0, unchanged: false };
    }
    const g = runs.find((r) => r.running && r.url === url);
    if (g) {
      return { addTo: g.id, fresh: [...selected].filter((i) => !g.keys.has(i)).length, unchanged: false };
    }
    const last = started.get(url);
    const unchanged =
      last !== undefined && selected.size > 0 && last.size === selected.size && [...selected].every((i) => last.has(i));
    return { addTo: 0, fresh: 0, unchanged };
  };

  // a bar for each site of the running groups, as the backend's SiteBars: the
  // first row still in flight. The mock's sites never pause.
  const siteBars = (): SiteBar[] => {
    const bars = new Map<string, SiteBar>();
    for (const g of runs.filter((r) => r.running)) {
      const site = hostOf(g.url);
      if (!bars.has(site)) {
        bars.set(site, { site, title: '', tags: '', done: 0, total: 0, until: 0, parallel: 8, vpn: '' });
      }
      const bar = bars.get(site)!;
      if (bar.title !== '') {
        continue;
      }
      if (g.runBundle) {
        // the chapter the bundle's phase names, as step does
        const j = g.jobs[0];
        const end = downloadEnd(j.total);
        if (j.done < end) {
          const share = end / g.runChapters;
          const n = Math.floor(j.done / share);
          Object.assign(bar, { title: `Chapter ${n + 1}`, done: j.done - n * share, total: share });
        }
        continue;
      }
      // a chapter's own row; a volume's row names the volume, not a chapter
      const job = g.jobs.find(
        (j) => !j.volume && (j.state === 'fetching' || j.state === 'downloading' || j.state === 'archiving'),
      );
      if (job) {
        Object.assign(bar, { title: job.title, tags: job.tags, done: job.done, total: job.total });
      }
    }
    return [...bars.values()];
  };

  const snapshot = (): State => ({
    url,
    loading,
    loaded,
    loadError: loadError?.text ?? '',
    loadErrorMsg: loadError,
    // the mock's sites never look blocked
    vpn: '',
    title,
    adult: false,
    chapterCount: chapters.length,
    hasDuplicates: chapters.some((c) => c.language !== ''),
    hasGroupDuplicates: chapters.some(
      (c) => c.group !== '' && chapters.some((d) => d !== c && d.number === c.number && d.language === c.language),
    ),
    stale: loaded && !loading && (options.language !== applied.language || options.scanlator !== applied.scanlator),
    loadedLanguage: applied.language,
    loadedScanlator: applied.scanlator,
    languages: filterOf('language'),
    scanlators: filterOf('scanlator'),
    selectedCount: selected.size,
    ...downloadMode(),
    running: runs.some((g) => g.running),
    cancelling: runs.some((g) => g.running) && runs.every((g) => !g.running || g.cancelling),
    groups: runs.map((g) => ({
      id: g.id,
      title: g.title,
      running: g.running,
      cancelling: g.cancelling,
      run: { chapters: g.runChapters, bundle: g.runBundle, volumes: g.runVolumes, rows: g.runRows },
      jobs: g.jobs.map((j) => {
        const restart = j.state === 'todo' || (j.state === 'failed' && !j.volume && !g.runBundle);
        return { ...j, ...(restart ? { key: `n:${j.number}`, restart } : {}) };
      }),
      summary: copySummary(g.summary),
      restored: g.restored,
      url: g.url,
      adult: g.adult,
      unfinished: unfinishedOf(g).length,
    })),
    sites: siteBars(),
    summary: runs.some((g) => g.running) ? null : copySummary(lastSummary()),
    options: { ...options },
    volumes: { text: volumesText, site: volumesSite, listed: volumesListed },
    version: reportsRelease ? releaseVersion : 'v0.0.0-mock',
    releaseURL: reportsRelease ? `${REPO}/releases/tag/${releaseVersion}` : '',
    // off hides the update; on brings it back
    update:
      isReleaseBuild(scenario) && prefs.checkUpdates
        ? {
            latest: shot ? UPDATE_VERSION_SHOT : UPDATE_VERSION_DEV,
            url: `${REPO}/releases/latest`,
            oneClick: !shot || prompt,
            ...(prompt && { prompt: true }),
          }
        : null,
    browserVisibleAuto: browserAuto,
    browserAlert: browserAlert?.text ?? '',
    browserAlertMsg: browserAlert,
    filenameTemplateDefault: TEMPLATE_DEFAULT,
    filenameTemplateWarning:
      options.packAs === 'chapters' || options.packAs === 'parts' ? templateWarning(options.filenameTemplate) : '',
    // the mock has no series folders: the dock falls back to outputDir
    destination: '',
    // the wire omits uiLanguage when it follows the system, and a pane size
    // left at its default (0), as gui.json does
    prefs: {
      checkUpdates: prefs.checkUpdates,
      simpleStart: prefs.simpleStart,
      ...(prefs.uiLanguage !== '' && { uiLanguage: prefs.uiLanguage }),
      ...(prefs.sideWidth ? { sideWidth: prefs.sideWidth } : {}),
      ...(prefs.logHeight ? { logHeight: prefs.logHeight } : {}),
    },
  });
  // each chapter's mark, as the backend's ChapterMarks: the newest group of
  // the series that took it says, by its row, or, without one, by whether
  // it's still waiting (a volume's or a bundle's chapter has no row of its own)
  const marksOf = (): ChapterMark[] => {
    if (!loaded) {
      return [];
    }
    return chapters.map((c, i) => {
      for (let k = runs.length - 1; k >= 0; k--) {
        const g = runs[k];
        if (g.restored || g.url !== url || !g.keys.has(i)) {
          continue;
        }
        // a bundle's one row stands for all its chapters
        const job = g.runBundle
          ? g.jobs[g.jobs.length - 1]
          : g.jobs.find((j) => !j.volume && j.number === c.number && j.title === labelOf(c));
        if (job !== undefined) {
          switch (job.state) {
            case 'done':
            case 'skipped':
              return 'done';
            case 'failed':
              return 'failed';
            default:
              return g.running ? 'downloading' : g.cancelling ? 'cancelled' : 'failed';
          }
        }
        if (g.pending.includes(c)) {
          return g.running ? 'queued' : g.cancelling ? 'cancelled' : 'failed';
        }
        return g.running ? 'downloading' : 'done';
      }
      return '';
    });
  };
  // the chapter numbers restart() downloads again of group g, as the
  // backend's restartTargets: a restored group's to do, another's failed
  // rows and, once it ended, the chapters it never started
  const unfinishedOf = (g: MockGroup): number[] => {
    if (g.restored) {
      return g.jobs.filter((j) => j.state === 'todo').map((j) => j.number);
    }
    const failed = g.runBundle ? [] : g.jobs.filter((j) => j.state === 'failed' && !j.volume).map((j) => j.number);
    return g.running ? failed : [...failed, ...g.pending.map((c) => c.number)];
  };
  const push = () => {
    stateBus.emit(snapshot());
    marksBus.emit(marksOf());
  };
  const copySummary = (s: Summary | null): Summary | null => (s ? { ...s, failures: [...s.failures] } : null);
  // the outcome of the group that ended last, as the backend's LastSummary
  const lastSummary = (): Summary | null =>
    runs.filter((g) => g.summary !== null).sort((a, b) => b.ended - a.ended)[0]?.summary ?? null;
  // a new group of the loaded series, with the options as they are
  const newRun = (): MockGroup => {
    const g: MockGroup = {
      id: ++lastRunID,
      title,
      url,
      dir: options.outputDir,
      ext: EXTENSIONS[options.format],
      running: true,
      cancelling: false,
      restored: false,
      adult: false,
      runChapters: 0,
      runBundle: false,
      runVolumes: false,
      runParts: false,
      runRows: 0,
      jobs: [],
      summary: null,
      existing: 0,
      pending: [],
      volumeChapters: new Map(),
      fetchLeft: new Map(),
      ended: 0,
      keys: new Set(),
    };
    runs = [...runs, g];
    return g;
  };

  function finishLoad(): void {
    loading = false;
    loaded = true;
    identified = true;
    fallbackFor('language', 'language');
    fallbackFor('scanlator', 'group');
    const lang = listedLanguage();
    // a ja list is titled in Japanese where OP_TITLES_JA has it, and its series is named in Latin letters
    const japanese = lang === 'ja';
    title = japanese ? 'ONE PIECE' : 'One Piece';
    chapters =
      withParts || scenario === 'parts'
        ? partsChapters(lang, japanese)
        : mockChapters(
            chapterCount,
            lang,
            dupes || url.includes('dupes') || options.language === 'all',
            otherLanguage(lang),
            !novolumes,
            locked,
            japanese,
          );
    if (offers.scanlator !== null) {
      chapters = groupedChapters(chapters);
    }
    volumesSite = siteVolumes(chapters);
    volumesListed = listedVolumes(chapters);
    volumesText = '';
    applied = { language: options.language, scanlator: options.scanlator };
    selected = new Set<number>();
    pushLog('info', msgOf('log.loaded', { title, count: chapters.length }));
    chaptersBus.emit(chapters);
    selectionBus.emit([]);
    push();
  }

  function failLoad(): void {
    loading = false;
    loaded = false;
    identified = true;
    applied = { language: options.language, scanlator: options.scanlator };
    chapters = [];
    selected = new Set<number>();
    loadError = msgOf('errors.loadChapters', { reason: 'unexpected status 403 Forbidden' });
    pushLog('error', loadError);
    chaptersBus.emit([]);
    selectionBus.emit([]);
    push();
  }

  function finishRun(g: MockGroup): void {
    clearInterval(g.timer);
    g.timer = undefined;
    g.running = false;
    g.ended = ++lastEnded;
    // a volume's chapters count as saved with it, as the backend counts them
    const saved = g.jobs
      .filter((j) => j.state === 'done')
      .reduce((n, j) => n + (j.volume ? (g.volumeChapters.get(j.title) ?? 0) : 1), 0);
    const volumes = g.jobs.filter((j) => j.volume && j.state === 'done').length;
    const failures = g.jobs
      .filter((j) => j.state === 'failed')
      .map((j) => ({ label: j.title, reason: j.reason, reasonMsg: j.reasonMsg }));
    // as the backend, the chapters left out as already in the folder are not skipped
    const skipped = g.jobs.filter((j) => j.state === 'skipped' && j.reasonMsg?.key !== 'job.reason.exists').length;
    const bundled = g.runBundle;
    g.summary = {
      dir: g.dir,
      cancelled: g.cancelling,
      saved: bundled && failures.length === 0 && !g.cancelling ? g.runChapters : saved,
      volumes,
      parts: g.runParts,
      bundled,
      bundle: bundled && !g.cancelling ? `${g.title} 1-${g.runChapters}${g.ext}` : '',
      skipped,
      existing: g.existing,
      failures,
      error: '',
    };
    pushLog('info', g.cancelling ? `Cancelled: ${saved} saved` : `Finished: ${saved} saved`);
    if (failures.length > 0) {
      pushLog('warn', `${failures.length} chapter(s) failed`);
    }
    push();
  }

  const FETCH_TICKS = 4;

  /** A tick's steps: the pace it had when a page was two steps, scaled to STEPS_PER_PAGE. */
  function ticks(n: number): number {
    return Math.round((n * STEPS_PER_PAGE) / 2);
  }

  /** The step at which a bar of total steps switches from downloading to archiving. */
  function downloadEnd(total: number): number {
    return (total * DOWNLOAD_WEIGHT) / STEPS_PER_PAGE;
  }

  /** One step of the simulated run. Returns false once it's over. */
  function step(g: MockGroup): boolean {
    if (g.runBundle) {
      const j = g.jobs[0];
      const done = Math.min(j.total, j.done + ticks(4));
      const fetched = done < downloadEnd(j.total);
      g.jobs = [
        {
          ...j,
          done,
          state: done >= j.total ? 'done' : fetched ? 'downloading' : 'archiving',
          ...phaseOf(
            done >= j.total
              ? undefined
              : fetched
                ? msgOf('job.phase.downloadingLabel', {
                    label: `Chapter ${Math.floor((done / downloadEnd(j.total)) * g.runChapters) + 1}`,
                  })
                : msgOf('job.phase.creatingBundle', { range: `1-${g.runChapters}` }),
          ),
        },
      ];
      return done < j.total;
    }

    // the volumes all have rows from the start; two move at a time, as the
    // backend downloads chapters in order
    let movingVolumes = 0;
    g.jobs = g.jobs.map((j) => {
      if (!j.volume || (j.state !== 'downloading' && j.state !== 'archiving') || movingVolumes >= 2) {
        return j;
      }
      movingVolumes++;
      const done = Math.min(j.total, j.done + ticks(5));
      if (done >= j.total) {
        return { ...j, done, state: 'done', ...phaseOf() };
      }
      const packing = done >= downloadEnd(j.total);
      return {
        ...j,
        done,
        state: packing ? 'archiving' : 'downloading',
        ...phaseOf(packing ? msgOf('job.phase.packing') : undefined),
      };
    });

    g.jobs = g.jobs.map((j) => {
      const left = g.fetchLeft.get(j.title);
      if (j.state !== 'fetching' || left === undefined) {
        return j;
      }
      if (left > 1) {
        g.fetchLeft.set(j.title, left - 1);
        return j;
      }
      g.fetchLeft.delete(j.title);
      return { ...j, state: 'downloading', total: PAGES * STEPS_PER_PAGE, ...phaseOf() };
    });

    g.jobs = g.jobs.map((j, idx) => {
      if (j.volume || (j.state !== 'downloading' && j.state !== 'archiving')) {
        return j;
      }
      if (fail && idx === 3 && j.done >= downloadEnd(j.total) / 2) {
        return { ...j, state: 'failed', ...failedWith(msgOf('job.reason.download', { reason: 'page 5 returned 404 Not Found' })) };
      }
      // uneven speeds, so the rows don't move in lockstep
      const done = Math.min(j.total, j.done + ticks(1 + ((j.number * 7) % 4)));
      const state = done >= j.total ? 'done' : done >= downloadEnd(j.total) ? 'archiving' : 'downloading';
      return { ...j, done, state };
    });

    const active = g.jobs.filter(
      (j) => !j.volume && (j.state === 'fetching' || j.state === 'downloading' || j.state === 'archiving'),
    ).length;
    if (!g.cancelling && active < 3 && g.pending.length > 0) {
      const c = g.pending.shift()!;
      if (c.locked) {
        // as the backend's FetchFailed: a listed lock fails before any fetch,
        // with a row unless the run is a bundle
        if (!g.runBundle) {
          g.jobs = [
            ...g.jobs,
            {
              title: labelOf(c),
              tags: '',
              number: c.number,
              total: 0,
              done: 0,
              state: 'failed',
              reason: LOCKED_REASON.text,
              reasonMsg: LOCKED_REASON,
              status: '',
              volume: false,
            },
          ];
        }
        pushLog('error', msgOf('log.errorFetching', { label: labelOf(c), reason: LOCKED_REASON.text }));
        return true;
      }
      if (skip && g.jobs.filter((j) => !j.volume).length === 5) {
        // as the backend's Skipped event: a row with nothing to download
        g.jobs = [
          ...g.jobs,
          {
            title: labelOf(c),
            tags: '',
            number: c.number,
            total: 0,
            done: 0,
            state: 'skipped',
            reason: 'every page excluded by --skip-pages',
            reasonMsg: msgOf('job.reason.skipped'),
            status: '',
            volume: false,
          },
        ];
        pushLog('warn', msgOf('log.skipping', { label: labelOf(c) }));
        return true;
      }
      if (fetching) {
        g.fetchLeft.set(labelOf(c), FETCH_TICKS);
      }
      g.jobs = [
        ...g.jobs,
        {
          title: labelOf(c),
          tags: '',
          number: c.number,
          total: fetching ? 0 : PAGES * STEPS_PER_PAGE,
          done: 0,
          state: fetching ? 'fetching' : 'downloading',
          reason: '',
          ...phaseOf(fetching ? msgOf('job.status.fetching') : undefined),
          volume: false,
        },
      ];
    }

    return (
      g.jobs.some((j) => j.state === 'fetching' || j.state === 'downloading' || j.state === 'archiving') ||
      (!g.cancelling && g.pending.length > 0)
    );
  }

  /** &exists: the chapters at EXISTS_AT come out as already in the folder, as rows before any fetch */
  function leaveOutExisting(g: MockGroup, list: Chapter[]): void {
    for (const at of EXISTS_AT) {
      const c = list[at];
      if (c === undefined) {
        continue;
      }
      const name = `${g.title} ${c.number} - ${labelOf(c)}${g.ext}`;
      const reasonMsg = msgOf('job.reason.exists', { name });
      g.pending = g.pending.filter((p) => p !== c);
      g.jobs = [
        ...g.jobs,
        {
          title: labelOf(c),
          tags: '',
          number: c.number,
          total: 0,
          done: 0,
          state: 'skipped',
          reason: reasonMsg.text,
          reasonMsg,
          ...phaseOf(),
          volume: false,
        },
      ];
      pushLog('info', msgOf('log.exists', { name }));
      g.existing++;
    }
  }

  function beginRun(g: MockGroup, list: Chapter[]): void {
    for (const c of list) {
      g.keys.add(c.index);
    }
    g.running = true;
    g.cancelling = false;
    g.runChapters = list.length;
    g.runBundle = options.packAs === 'bundle';
    g.runVolumes = options.packAs === 'volumes' || options.packAs === 'parts';
    g.runParts = options.packAs === 'parts';
    g.summary = null;
    g.pending = [...list];
    g.existing = 0;
    g.volumeChapters = new Map();
    g.fetchLeft.clear();
    g.runRows = g.runBundle ? 1 : list.length;
    if (g.runParts) {
      // a chapter's parts make one row, as a volume's chapters do
      const groups = groupParts(list);
      g.pending = groups.filter((part) => part.chapters.length === 1).map((part) => part.chapters[0]);
      g.runRows = groups.length;
      g.jobs = groups
        .filter((part) => part.chapters.length > 1)
        .map((part) => {
          const titleMsg = msgOf('job.title.volumeParts', { volume: `Chapter ${part.number}`, count: part.chapters.length });
          const title = titleMsg.text;
          g.volumeChapters.set(title, part.chapters.length);
          return {
            title,
            titleMsg,
            tags: '',
            number: part.number,
            total: part.chapters.length * PAGES * STEPS_PER_PAGE,
            done: 0,
            state: 'downloading' as const,
            reason: '',
            ...phaseOf(msgOf('job.phase.waiting')),
            volume: true,
          };
        });
      return;
    }
    if (g.runVolumes) {
      // the backend makes a row per volume up front, and runs the chapters
      // outside them one by one
      const grouping = groupChapters(list, parseVolumes(volumesText));
      g.pending = grouping.loose;
      g.runRows = grouping.volumes.length + grouping.loose.length;
      g.jobs = grouping.volumes.map((v) => {
        const first = v.chapters[0].number;
        const last = v.chapters[v.chapters.length - 1].number;
        const vol = `Vol. ${String(v.number).padStart(2, '0')}`;
        const titleMsg =
          first === last
            ? msgOf('job.title.volumeChapter', { volume: vol, first })
            : msgOf('job.title.volume', { volume: vol, first, last });
        const title = titleMsg.text;
        g.volumeChapters.set(title, v.chapters.length);
        return {
          title,
          titleMsg,
          tags: '',
          number: first,
          total: v.chapters.length * PAGES * STEPS_PER_PAGE,
          done: 0,
          state: 'downloading' as const,
          reason: '',
          ...phaseOf(msgOf('job.phase.waiting')),
          volume: true,
        };
      });
      return;
    }
    const bundleTitle = msgOf('job.title.bundle', { range: `1-${list.length}` });
    g.jobs = g.runBundle
      ? [
          {
            title: bundleTitle.text,
            titleMsg: bundleTitle,
            tags: '',
            number: 0,
            total: list.length * PAGES * STEPS_PER_PAGE,
            done: 0,
            state: 'downloading',
            reason: '',
            ...phaseOf(msgOf('job.phase.gathering')),
            volume: false,
          },
        ]
      : [];
  }

  function tick(g: MockGroup): void {
    if (!step(g)) {
      finishRun(g);
      return;
    }
    push();
  }

  function simulateRun(list: Chapter[]): void {
    const g = newRun();
    beginRun(g, list);
    push();
    if (!freeze) {
      g.timer = setInterval(() => tick(g), TICK_MS);
    }
  }

  if (scenario === 'check') {
    url = 'https://check.example.com/manga/one-piece';
    loading = true;
    browserAuto = true;
    browserAlert = MOCK_ALERT;
    if (!freeze) {
      setTimeout(() => {
        browserAlert = undefined;
        finishLoad();
      }, 4000);
    }
  } else if (scenario === 'loading') {
    url = 'https://example.com/manga/one-piece';
    loading = true;
    if (!freeze) {
      setTimeout(finishLoad, 2500);
    }
  } else if (scenario === 'error') {
    url = 'https://unknown.example.org/manga/some-series';
    failLoad();
    log = [];
    pushLog('info', `Loading ${url}`);
    if (loadError !== undefined) pushLog('error', loadError);
  } else if (scenario !== 'empty') {
    url = 'https://example.com/manga/one-piece';
    finishLoad();
    for (const c of [...chapters.slice(0, 12), ...chapters.slice(40, 52)]) {
      if (!c.locked) {
        selected.add(c.index);
      }
    }
    selectionBus.emit(selectionOf());
  }
  if (scenario === 'stale') {
    options.language = 'es';
  }
  if (scenario === 'update') {
    // A realistic site-check failure: selector timeout including the full series URL.
    // The URL is long enough to overflow the status bar without the CSS fix.
    if (!shot) {
      pushLog(
        'warn',
        msgOf('log.siteCheck', {
          reason: 'timed out waiting for "li.list-group-item, img.PB0mN" at https://example.com/manga/the-cartographer-of-lost-harbours-collected-edition',
        }),
      );
    }
  }
  if (scenario === 'volumes' || scenario === 'volumes-running') {
    options.packAs = 'volumes';
    // three volumes and a half: the first 32 chapters (598-629) span volumes 61-64
    selected = new Set(
      chapters
        .slice(0, Math.min(32, chapters.length))
        .filter((c) => !c.locked)
        .map((c) => c.index),
    );
    selectionBus.emit(selectionOf());
  }
  if (scenario === 'parts') {
    url = 'https://example.com/manga/one-piece';
    options.packAs = 'parts';
    // every part that isn't locked: chapters 1-4, and the one open part of 19
    selected = new Set(chapters.filter((c) => !c.locked).map((c) => c.index));
    selectionBus.emit(selectionOf());
  }
  if (scenario === 'volumes-running') {
    const picked = chapters.filter((c) => selected.has(c.index));
    pushLog('info', msgOf('log.downloading', { count: picked.length, title }));
    const g = newRun();
    beginRun(g, picked);
    for (let i = 0; i < 12 && step(g); i++) {
      // advancing
    }
    if (!freeze) {
      g.timer = setInterval(() => tick(g), TICK_MS);
    }
  }
  if (scenario === 'running' || scenario === 'cancelling' || scenario === 'bundle' || scenario === 'done') {
    options.packAs = scenario === 'bundle' ? 'bundle' : 'chapters';
    const picked = chapters.filter((c) => selected.has(c.index)).slice(0, 12);
    pushLog('info', msgOf('log.downloading', { count: picked.length, title }));
    const g = newRun();
    beginRun(g, picked);
    if (exists && (scenario === 'running' || scenario === 'done')) {
      leaveOutExisting(g, picked);
    }
    // run the simulation forward to an interesting point
    const steps = scenario === 'done' ? Infinity : scenario === 'bundle' ? 8 : 14;
    for (let i = 0; i < steps && step(g); i++) {
      // advancing
    }
    if (scenario === 'cancelling') {
      g.cancelling = true;
      pushLog('info', msgOf('log.cancelling'));
    }
    if (scenario === 'done') {
      // a conversion warning on a chapter of the run (the second, 599 by default), one that is saved
      const kept = picked.filter((_, i) => !exists || !EXISTS_AT.includes(i));
      const warned = kept[1] ?? kept[0];
      if (warned !== undefined) {
        pushLog('warn', `Chapter ${warned.number}: page 7 is AVIF but could not be converted; kept as is`);
      }
      finishRun(g);
    } else if (!freeze) {
      g.timer = setInterval(() => tick(g), TICK_MS);
    }
  }
  if (scenario === 'restored') {
    // as the backend's restoreLocked: rows done or to do, nothing running
    const restore = (name: string, address: string, adult: boolean, done: number, todo: number) => {
      const g = newRun();
      Object.assign(g, { title: name, url: address, adult, restored: true, running: false });
      // rows named after the mock's list, which every load lists, so a resume finds them
      g.jobs = chapters.slice(0, done + todo).map((c, i): Job => {
        const reason = msgOf('job.reason.todo');
        return {
          title: c.title,
          tags: '',
          number: c.number,
          total: i < done ? 1 : 0,
          done: i < done ? 1 : 0,
          state: i < done ? 'done' : 'todo',
          reason: i < done ? '' : reason.text,
          reasonMsg: i < done ? undefined : reason,
          status: '',
          volume: false,
        };
      });
      g.runChapters = g.runRows = done + todo;
    };
    restore('Night Garden', 'https://example.com/manga/night-garden', true, 3, 0);
    restore('Vinland Saga', 'https://example.com/manga/vinland-saga', false, 3, 4);
  }
  if (scenario === 'two-groups') {
    // a first series' group that ended, then the loaded one's, running
    const first = newRun();
    first.title = 'Vinland Saga';
    first.url = 'https://example.com/manga/vinland-saga';
    beginRun(first, chapters.filter((c) => !c.locked).slice(0, 6));
    pushLog('info', msgOf('log.downloading', { count: 6, title: first.title }));
    while (step(first)) {
      // advancing
    }
    finishRun(first);
    const picked = chapters.filter((c) => selected.has(c.index)).slice(0, 12);
    pushLog('info', msgOf('log.downloading', { count: picked.length, title }));
    const g = newRun();
    beginRun(g, picked);
    for (let i = 0; i < 14 && step(g); i++) {
      // advancing
    }
    if (!freeze) {
      g.timer = setInterval(() => tick(g), TICK_MS);
    }
  }
  push();

  // cancels the running groups pick selects that weren't cancelled yet, logging once
  function cancelRuns(pick: (g: MockGroup) => boolean): void {
    const picked = runs.filter((g) => g.running && !g.cancelling && pick(g));
    if (picked.length === 0) {
      return;
    }
    for (const g of picked) {
      g.cancelling = true;
    }
    pushLog('info', msgOf('log.cancelling'));
    push();
  }

  // a load of next's URL: the body of the load action, shared with setOptionsAndLoad
  async function startLoad(next: string): Promise<void> {
    const trimmed = next.trim();
    if (trimmed === '') {
      throw reject('errors.enterURL');
    }
    if (loading) {
      throw reject('errors.busy');
    }
    url = trimmed;
    loadError = undefined;
    loading = true;
    browserAuto = trimmed.includes('check');
    browserAlert = browserAuto ? MOCK_ALERT : undefined;
    push();
    await new Promise((resolve) => setTimeout(resolve, browserAuto ? 3000 : 400));
    browserAlert = undefined;
    if (trimmed.includes('unknown')) {
      failLoad();
      return;
    }
    finishLoad();
  }

  const api: Api = {
    async state() {
      return snapshot();
    },
    async chapters() {
      return chapters.map((c) => ({ ...c }));
    },
    async selection() {
      return selectionOf();
    },
    async marks() {
      return marksOf();
    },
    async log() {
      return log.slice();
    },
    async clearLog() {
      log = [];
      logBus.emit(log);
    },

    async load(next) {
      await startLoad(next);
    },
    async setOptionsAndLoad(next, target) {
      if (loading) {
        throw reject('errors.busy');
      }
      Object.assign(options, next);
      await startLoad(target);
    },
    async clear() {
      if (loading) {
        throw reject('errors.busy');
      }
      url = '';
      loaded = false;
      loadError = undefined;
      identified = false;
      browserAuto = false;
      browserAlert = undefined;
      applied = { language: '', scanlator: '' };
      title = '';
      chapters = [];
      selected = new Set<number>();
      volumesText = '';
      volumesSite = '';
      volumesListed = 0;
      // the groups that ended go, the running ones stay
      runs = runs.filter((g) => g.running);
      chaptersBus.emit([]);
      selectionBus.emit([]);
      push();
    },
    async setSelected(index, on) {
      if (loading) {
        throw reject('errors.busy');
      }
      if (index < 0 || index >= chapters.length) {
        return;
      }
      if (on && !chapters[index].locked) {
        selected.add(index);
      } else if (!on) {
        selected.delete(index);
      }
      selectionBus.emit(selectionOf());
      push();
    },
    async setSelectedMany(indices, on) {
      if (loading) {
        throw reject('errors.busy');
      }
      if (indices.some((index) => index < 0 || index >= chapters.length)) {
        throw reject('errors.chapterMissing', { index: indices.find((index) => index < 0 || index >= chapters.length) as number });
      }
      for (const index of indices) {
        if (on) {
          if (!chapters[index].locked) {
            selected.add(index);
          }
        } else {
          selected.delete(index);
        }
      }
      selectionBus.emit(selectionOf());
      push();
    },
    async setAll(on) {
      if (loading) {
        throw reject('errors.busy');
      }
      selected = new Set(on ? chapters.filter((c) => !c.locked).map((c) => c.index) : []);
      selectionBus.emit(selectionOf());
      push();
    },
    async selectRange(text) {
      if (loading) {
        throw reject('errors.busy');
      }
      const matched: number[] = [];
      for (const part of text
        .split(',')
        .map((p) => p.trim())
        .filter(Boolean)) {
        const [a, b] = part.split('-').map(Number);
        if (Number.isNaN(a)) {
          throw `invalid range: ${part}`;
        }
        const hi = b ?? a;
        for (const c of chapters) {
          if (c.number >= a && c.number <= hi && !c.locked) {
            matched.push(c.index);
          }
        }
      }
      for (const i of matched) {
        selected.add(i);
      }
      selectionBus.emit(selectionOf());
      push();
      return matched.length;
    },
    async setOptions(next) {
      if (loading) {
        throw reject('errors.busy');
      }
      Object.assign(options, next);
      push();
    },
    async setVolumes(text) {
      if (loading) {
        throw reject('errors.busy');
      }
      try {
        parseVolumes(text);
      } catch (e) {
        throw (e as Error).message;
      }
      volumesText = text.trim();
      push();
    },
    async chooseFolder() {
      if (loading) {
        throw reject('errors.busy');
      }
      const picked = window.prompt('Output folder (mock dialog)', options.outputDir) ?? '';
      if (picked !== '') {
        options.outputDir = picked;
        push();
      }
      return picked;
    },
    async openFolder(id) {
      const g = runs.find((r) => r.id === id);
      pushLog('info', `would open ${g?.summary ? g.dir : options.outputDir} (mock)`);
    },
    openURL(url) {
      window.open(url, '_blank', 'noopener');
    },
    async readClipboard() {
      return 'https://example.com/manga/one-piece';
    },
    async start() {
      if (loading) {
        throw reject('errors.busy');
      }
      if (!loaded) {
        throw reject('errors.loadFirst');
      }
      if (options.language !== applied.language || options.scanlator !== applied.scanlator) {
        throw reject('errors.filtersChanged');
      }
      const picked = chapters.filter((c) => selected.has(c.index));
      if (picked.length === 0) {
        throw reject('errors.selectOne');
      }
      // one series downloads once at a time: while it does, Download adds to
      // its run, as the backend's queueLocked. The mock queues the added
      // chapters after the pending ones; only a chapters run plays them as rows.
      const running = runs.find((g) => g.running && g.url === url);
      if (running) {
        const fresh = picked.filter((c) => !running.keys.has(c.index));
        if (fresh.length === 0) {
          throw reject('errors.nothingNew');
        }
        for (const c of fresh) {
          running.keys.add(c.index);
        }
        running.pending.push(...fresh);
        running.runChapters += fresh.length;
        running.runRows += fresh.length;
        started.set(url, new Set(selected));
        pushLog('info', msgOf('log.queued', { count: fresh.length, title: running.title }));
        push();
        return;
      }
      if (
        options.packAs === 'volumes' &&
        groupChapters(picked, parseVolumes(volumesText)).volumes.length === 0 &&
        volumesListed === 0 &&
        volumesText === ''
      ) {
        throw reject('errors.noVolumes');
      }
      pushLog('info', msgOf('log.downloading', { count: picked.length, title }));
      started.set(url, new Set(selected));
      // a download takes the place of the series' group of the last session
      runs = runs.filter((g) => !(g.restored && g.url === url));
      simulateRun(picked);
    },
    async update() {
      pushLog('info', msgOf('log.downloadingUpdate', { version: UPDATE_VERSION_DEV }));
      await new Promise((resolve) => setTimeout(resolve, 1500));
      const reason = 'the mock has no installer';
      pushLog('error', msgOf('errors.updateDownload', { reason }));
      throw reject('errors.updateDownload', { reason });
    },
    async showBrowser() {
      pushLog('info', 'would bring the browser window to the front (mock)');
    },
    async dismissBrowserAlert() {
      browserAlert = undefined;
      push();
    },
    // like the backend's, never refused while a load or run is going on
    async setUILanguage(pref) {
      prefs.uiLanguage = pref;
      push();
    },
    // the backend's update check takes the system's locale; the mock makes no check
    async setSystemLocale() {},
    async setCheckUpdates(on) {
      prefs.checkUpdates = on;
      push();
    },
    async setSimpleStart(on) {
      prefs.simpleStart = on;
      push();
    },
    async setPaneSizes(side, log) {
      prefs.sideWidth = side;
      prefs.logHeight = log;
      push();
    },
    async cancel() {
      cancelRuns(() => true);
    },
    async cancelGroup(id) {
      cancelRuns((g) => g.id === id);
    },
    async history() {
      return history.map((h) => ({ ...h }));
    },
    async clearHistory() {
      history = [];
    },
    async resume(id) {
      const g = runs.find((r) => r.id === id && r.restored);
      if (!g) {
        throw reject('errors.noSuchGroup');
      }
      // the backend matches the rows' labels; the mock's list is titled
      // from One Piece, so it matches their numbers
      const todo = new Set(g.jobs.filter((j) => j.state === 'todo').map((j) => j.number));
      await startLoad(g.url);
      selected = new Set(chapters.filter((c) => !c.locked && todo.has(c.number)).map((c) => c.index));
      selectionBus.emit(selectionOf());
      push();
    },

    async downloadMissing() {
      const marks = marksOf();
      const missing = chapters.filter(
        (c) => !c.locked && (marks[c.index] === '' || marks[c.index] === 'failed' || marks[c.index] === 'cancelled'),
      );
      if (missing.length === 0) {
        throw reject('errors.nothingMissing');
      }
      selected = new Set(missing.map((c) => c.index));
      selectionBus.emit(selectionOf());
      await api.start();
    },

    async restart(id, key) {
      const g = runs.find((r) => r.id === id);
      if (!g) {
        throw reject('errors.noSuchGroup');
      }
      const numbers = new Set(key === '' ? unfinishedOf(g) : unfinishedOf(g).filter((n) => `n:${n}` === key));
      if (numbers.size === 0) {
        throw reject('errors.nothingToRestart');
      }
      if (g.running) {
        // as the backend's forget: the chapters leave the run, and their failed rows go
        g.jobs = g.jobs.filter((j) => !(j.state === 'failed' && numbers.has(j.number)));
        for (const c of chapters) {
          if (numbers.has(c.number)) {
            g.keys.delete(c.index);
          }
        }
      }
      if (g.restored || g.url !== url) {
        await startLoad(g.url);
      }
      selected = new Set(chapters.filter((c) => !c.locked && numbers.has(c.number)).map((c) => c.index));
      selectionBus.emit(selectionOf());
      await api.start();
    },

    onState: (cb) => stateBus.subscribe(cb),
    onChapters: (cb) => chaptersBus.subscribe(cb),
    onSelection: (cb) => selectionBus.subscribe(cb),
    onMarks: (cb) => marksBus.subscribe(cb),
    onLog: (cb) => logBus.subscribe(cb),
  };

  return api;
}

export { scenarioFrom };
