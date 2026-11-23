// The fork's standalone parts of the mock (see mock.ts), moved unchanged out
// of it: none of them touches the mock's own state.

import type { Chapter, HistoryItem, Job, Summary } from './types';

export const HOUR = 3600 * 1000;
// a fixed time, so screenshots don't change from day to day
export const MOCK_NOW = Date.UTC(2026, 10, 21, 18, 30);

/** The history pop-up's series: one adult title among them, masked until clicked */
export const MOCK_HISTORY: HistoryItem[] = [
  { title: 'One Piece', url: 'https://example.com/manga/one-piece', site: 'https://example.com/', adult: false, when: MOCK_NOW, done: 24, todo: 0 },
  { title: 'Vinland Saga', url: 'https://example.com/manga/vinland-saga', site: 'https://example.com/', adult: false, when: MOCK_NOW - 3 * HOUR, done: 3, todo: 4 },
  { title: 'Night Garden', url: 'https://example.com/manga/night-garden', site: 'https://example.com/', adult: true, when: MOCK_NOW - 30 * HOUR, done: 3, todo: 0 },
];

/** One simulated download group, as the backend's: its own run, rows and outcome */
export interface MockGroup {
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

export const copySummary = (s: Summary | null): Summary | null => (s ? { ...s, failures: [...s.failures] } : null);
// the chapter numbers restart() downloads again of group g, as the
// backend's restartTargets: a restored group's to do, another's failed
// rows and, once it ended, the chapters it never started
export const unfinishedOf = (g: MockGroup): number[] => {
  if (g.restored) {
    return g.jobs.filter((j) => j.state === 'todo').map((j) => j.number);
  }
  const failed = g.runBundle ? [] : g.jobs.filter((j) => j.state === 'failed' && !j.volume).map((j) => j.number);
  return g.running ? failed : [...failed, ...g.pending.map((c) => c.number)];
};
