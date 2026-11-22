// The one module the UI talks to the backend through. createWailsApi wraps the
// bindings Wails generates into wailsjs/; createMockApi (mock.ts) stands in for
// them in a plain browser, so the UI can be worked on without the Go app.

import * as app from '../wailsjs/go/main/App';
import { BrowserOpenURL, ClipboardGetText, EventsOn } from '../wailsjs/runtime/runtime';
import type { LangPref } from './i18n';
import type { Chapter, ChapterMark, HistoryItem, LogLine, Options, State } from './types';

export type Unsubscribe = () => void;

export interface Api {
  // Snapshots, for the first render. The events below keep them current.
  state(): Promise<State>;
  chapters(): Promise<Chapter[]>;
  selection(): Promise<number[]>;
  /** The mark of each chapter of the list, by index; empty when nothing is loaded */
  marks(): Promise<ChapterMark[]>;
  /** What each chapter's mark can say more, by index: the file of a chapter in the folder, a failure's reason, else '' */
  markNotes?(): Promise<string[]>;
  log(): Promise<LogLine[]>;

  // Actions. A rejected promise carries the reason as a string (or Error).
  /** Empties the log the window shows; a running download goes on logging after it */
  clearLog(): Promise<void>;
  load(url: string): Promise<void>;
  /** Forgets the loaded series (URL, chapters, selection, load error) and the groups that ended; refused while one loads */
  clear(): Promise<void>;
  setSelected(index: number, on: boolean): Promise<void>;
  /** Selects or deselects every index at once; rejects, changing nothing, if one is out of range */
  setSelectedMany(indices: number[], on: boolean): Promise<void>;
  setAll(on: boolean): Promise<void>;
  /** Selects the chapters in the range text; resolves with how many matched */
  selectRange(text: string): Promise<number>;
  setOptions(options: Options): Promise<void>;
  /** setOptions and then load(url), in one step: the options a selector picked are never seen unapplied */
  setOptionsAndLoad(options: Options, url: string): Promise<void>;
  /** Sets the Volumes box ("1-7/8-16", "" for the site's own); rejects a value that doesn't parse */
  setVolumes(text: string): Promise<void>;
  /** Asks for the output folder with the picker titled title; "" when cancelled */
  chooseFolder(title: string): Promise<string>;
  /** Opens the folder group id saved into (the next run's, when it hasn't ended) */
  openFolder(id: number): Promise<void>;
  /** Opens the URL in the system's browser, never in the window */
  openURL(url: string): void;
  /** The clipboard's text, for the Paste button; rejects when the runtime isn't there */
  readClipboard(): Promise<string>;
  /** Starts a group with the selected chapters, beside any running */
  start(): Promise<void>;
  /** Cancels every running group */
  cancel(): Promise<void>;
  /** Cancels group id */
  cancelGroup(id: number): Promise<void>;
  /** Downloads and runs the installer, then quits; rejects with the reason (also logged) when it can't */
  update(): Promise<void>;
  /** Brings the browser window to the front again */
  showBrowser(): Promise<void>;
  /** Hides browserAlert; the window and the load waiting for it go on */
  dismissBrowserAlert(): Promise<void>;
  /** Saves the window's language; '' follows the system. Takes effect at once in the window. */
  setUILanguage(pref: LangPref): Promise<void>;
  /** Gives the backend the system's raw locale for the update check; the first report starts it */
  setSystemLocale(locale: string): Promise<void>;
  setCheckUpdates(on: boolean): Promise<void>;
  /** Saves whether the window shows the simple view while nothing is loaded; a load started keeps the full one */
  setSimpleStart(on: boolean): Promise<void>;
  /** Saves the side panel's width and the log drawer's height the user dragged them to; 0 keeps the default */
  setPaneSizes(sideWidth: number, logHeight: number): Promise<void>;
  /** The series of history.txt, newest first (the last 100) */
  history(): Promise<HistoryItem[]>;
  /** Empties the history, and with it what the next launch shows in the Activity pane */
  clearHistory(): Promise<void>;
  /** Loads the series of restored group id with its chapters to do selected; downloads nothing */
  resume(id: number): Promise<void>;
  /**
   * Downloads again the unfinished chapters of group id: the row's with its key, all with '';
   * a group of another series loads it first, then starts
   */
  restart(id: number, key: string): Promise<void>;
  /** Selects and downloads every chapter of the loaded series not saved yet, failed and cancelled included */
  downloadMissing(): Promise<void>;

  // Pushes. Each returns a function that stops the callback.
  onState(cb: (s: State) => void): Unsubscribe;
  onChapters(cb: (c: Chapter[]) => void): Unsubscribe;
  onSelection(cb: (indices: number[]) => void): Unsubscribe;
  onMarks(cb: (marks: ChapterMark[]) => void): Unsubscribe;
  onMarkNotes?(cb: (notes: string[]) => void): Unsubscribe;
  onLog(cb: (lines: LogLine[]) => void): Unsubscribe;
}

/** Events are registered once each; callbacks are fanned out from a set */
function fanOut<T>(event: string): { subscribe(cb: (v: T) => void): Unsubscribe } {
  const callbacks = new Set<(v: T) => void>();
  let registered = false;

  return {
    subscribe(cb) {
      callbacks.add(cb);
      if (!registered) {
        registered = true;
        EventsOn(event, (v: T) => {
          for (const fn of callbacks) {
            fn(v);
          }
        });
      }

      return () => {
        callbacks.delete(cb);
      };
    },
  };
}

export function createWailsApi(): Api {
  const state = fanOut<State>('state');
  const chapters = fanOut<Chapter[]>('chapters');
  const selection = fanOut<number[]>('selection');
  const marks = fanOut<ChapterMark[]>('marks');
  const markNotes = fanOut<string[]>('markNotes');
  const log = fanOut<LogLine[]>('log');

  return {
    // The bindings' types are structurally the same as ours; the cast only
    // narrows the string-typed enums the Go side fills in.
    state: () => app.State() as unknown as Promise<State>,
    chapters: () => app.Chapters() as unknown as Promise<Chapter[]>,
    selection: () => app.Selection(),
    marks: () => app.Marks() as Promise<ChapterMark[]>,
    markNotes: () => app.MarkNotes(),
    log: () => app.Log() as unknown as Promise<LogLine[]>,

    clearLog: () => app.ClearLog(),
    load: (url) => app.Load(url),
    clear: () => app.Clear(),
    setSelected: (index, on) => app.SetSelected(index, on),
    setSelectedMany: (indices, on) => app.SetSelectedMany(indices, on),
    setAll: (on) => app.SetAll(on),
    selectRange: (text) => app.SelectRange(text),
    setOptions: (options) => app.SetOptions(options as unknown as Parameters<typeof app.SetOptions>[0]),
    setOptionsAndLoad: (options, url) =>
      app.SetOptionsAndLoad(options as unknown as Parameters<typeof app.SetOptionsAndLoad>[0], url),
    setVolumes: (text) => app.SetVolumes(text),
    chooseFolder: (title) => app.ChooseFolder(title),
    openFolder: (id) => app.OpenFolder(id),
    openURL: (url) => BrowserOpenURL(url),
    // async, so a missing runtime rejects rather than throwing into the click
    readClipboard: async () => ClipboardGetText(),
    start: () => app.Start(),
    cancel: () => app.Cancel(),
    cancelGroup: (id) => app.CancelGroup(id),
    update: () => app.Update(),
    showBrowser: () => app.ShowBrowser(),
    dismissBrowserAlert: () => app.DismissBrowserAlert(),
    setUILanguage: (pref) => app.SetUILanguage(pref),
    setSystemLocale: (locale) => app.SetSystemLocale(locale),
    setCheckUpdates: (on) => app.SetCheckUpdates(on),
    setSimpleStart: (on) => app.SetSimpleStart(on),
    setPaneSizes: (side, log) => app.SetPaneSizes(side, log),
    history: () => app.History(),
    clearHistory: () => app.ClearHistory(),
    resume: (id) => app.Resume(id),
    restart: (id, key) => app.Restart(id, key),
    downloadMissing: () => app.DownloadMissing(),

    onState: (cb) => state.subscribe(cb),
    onChapters: (cb) => chapters.subscribe(cb),
    onSelection: (cb) => selection.subscribe(cb),
    onMarks: (cb) => marks.subscribe(cb),
    onMarkNotes: (cb) => markNotes.subscribe(cb),
    onLog: (cb) => log.subscribe(cb),
  };
}
