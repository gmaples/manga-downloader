// Pure helpers about the run as a whole: whether Download can start, how far
// along it is, and how it ended. format.ts has the per-job and summary wording.

import { jobFraction } from './format';
import type { T } from './i18n';
import type { Chapter, Group, Job, JobState, State, Summary } from './types';
import { groupChapters, groupParts, isSiteVolumes, parseVolumes, selectedIn, type Grouping } from './volumes';

/** The states a job ends in */
export function isFinished(j: Job): boolean {
  return j.state === 'done' || j.state === 'failed' || j.state === 'skipped';
}

export type Tally = Record<JobState, number>;

export function tally(jobs: Job[]): Tally {
  const t: Tally = { fetching: 0, downloading: 0, archiving: 0, done: 0, failed: 0, skipped: 0, todo: 0 };
  for (const j of jobs) {
    t[j.state]++;
  }

  return t;
}

/** A job that is happening now: its page list or pages being fetched, or its file being written */
export function isActive(j: Job): boolean {
  return j.state === 'fetching' || j.state === 'downloading' || j.state === 'archiving';
}

/** What a group's header counts: its chapters (or volumes) by what became of them */
export interface GroupCounts {
  active: number;
  queued: number;
  saved: number;
  /** Left out because the folder already holds the chapter */
  had: number;
  /** Left out because the skip-pages option excluded every page */
  skipped: number;
  failed: number;
  /** A restored group's chapters still to do */
  todo: number;
}

export function groupCounts(group: Pick<Group, 'jobs' | 'run' | 'running'>): GroupCounts {
  const c: GroupCounts = { active: 0, queued: 0, saved: 0, had: 0, skipped: 0, failed: 0, todo: 0 };
  for (const j of group.jobs) {
    if (isActive(j)) {
      c.active++;
    } else if (j.state === 'done') {
      c.saved++;
    } else if (j.state === 'failed') {
      c.failed++;
    } else if (j.state === 'todo') {
      c.todo++;
    } else if (j.reasonMsg?.key === 'job.reason.exists') {
      c.had++;
    } else {
      c.skipped++;
    }
  }
  if (group.running) {
    c.queued = overallProgress(group).queued;
  }
  return c;
}

/** A size in bytes the way people read it: 912 kB, 1.4 GB */
export function sizeText(n: number): string {
  if (n < 1000) {
    return `${n} B`;
  }
  const units = 'kMGTPE';
  let v = n / 1000;
  let i = 0;
  while (v >= 1000 && i < units.length - 1) {
    v /= 1000;
    i++;
  }
  return `${v.toFixed(1)} ${units[i]}B`;
}

export interface Overall {
  /** Rows (chapters, volumes or the one bundle) that reached an end state */
  finished: number;
  /** What the run covers: its chapters, its volumes plus the chapters outside them, or 1 for a bundle */
  total: number;
  /** Chapters the backend hasn't started yet; they have no job until they do */
  queued: number;
  /** 0 to 1, page-weighted inside each job */
  fraction: number;
}

/**
 * A group's overall progress. Jobs are only created as chapters start, so the
 * run's chapter count is the denominator, not the job count.
 */
export function overallProgress(group: Pick<Group, 'jobs' | 'run'>): Overall {
  const { jobs, run } = group;
  if (run.bundle) {
    const job = jobs[0];
    const finished = job !== undefined && isFinished(job) ? 1 : 0;
    return { finished, total: 1, queued: 0, fraction: job ? jobFraction(job) : 0 };
  }

  const total = Math.max(run.rows > 0 ? run.rows : run.chapters, jobs.length);
  if (total === 0) {
    return { finished: 0, total: 0, queued: 0, fraction: 0 };
  }
  let sum = 0;
  let finished = 0;
  for (const j of jobs) {
    if (isFinished(j)) {
      finished++;
      sum += 1;
    } else {
      sum += jobFraction(j);
    }
  }

  return { finished, total, queued: total - jobs.length, fraction: Math.min(1, sum / total) };
}

/**
 * The progress of the running groups together: their rows add up, and the
 * fraction weighs each group by its rows
 */
export function runningProgress(groups: Group[]): Overall {
  const out: Overall = { finished: 0, total: 0, queued: 0, fraction: 0 };
  let sum = 0;
  for (const g of groups) {
    if (!g.running) {
      continue;
    }
    const o = overallProgress(g);
    out.finished += o.finished;
    out.total += o.total;
    out.queued += o.queued;
    sum += o.fraction * o.total;
  }
  out.fraction = out.total > 0 ? Math.min(1, sum / out.total) : 0;

  return out;
}

/**
 * Why Download can't start right now, or "" when it can. These are the
 * refusals the backend would answer with, shown before the click instead of
 * after it. The backend stays the authority: its own refusal still shows.
 */
export function downloadBlocker(state: State, t: T): string {
  if (state.loading) {
    return t('download.blocker.loading');
  }
  if (!state.loaded) {
    return t('download.blocker.notLoaded');
  }
  if (state.stale) {
    return t('download.blocker.stale');
  }
  if (state.selectedCount === 0) {
    return t('download.blocker.noChapters');
  }
  // a running series takes only the chapters it doesn't have yet
  if (state.addTo !== 0 && state.fresh === 0) {
    return t('download.blocker.allQueued');
  }
  if (state.unchanged) {
    return t('download.blocker.unchanged');
  }
  if (state.options.packAs === 'volumes' && isSiteVolumes(state.volumes.text) && state.volumes.listed === 0) {
    return t('download.blocker.noVolumes', { example: '1-7/8-16' });
  }

  return '';
}

/** The Download button's label; chapters and selected are the loaded list and its selected indices */
export function downloadLabel(
  state: Pick<State, 'selectedCount' | 'options' | 'volumes' | 'addTo' | 'fresh'>,
  chapters: Chapter[],
  selected: number[],
  t: T,
): string {
  const n = state.selectedCount;
  if (n === 0) {
    return t('download.label.download');
  }
  if (state.addTo !== 0 && state.fresh > 0) {
    return t('download.label.add', { chapters: t('count.chapter', { count: state.fresh }) });
  }
  if (state.options.packAs === 'bundle' && n > 1) {
    return t('download.label.bundle', { chapters: t('count.chapter', { count: n }) });
  }
  if (state.options.packAs === 'volumes') {
    const label = volumesLabel(state.volumes.text, chapters, selected, t);
    if (label !== '') {
      return label;
    }
  }
  if (state.options.packAs === 'parts') {
    return partsLabel(chapters, selected, t);
  }

  return t('download.label.chapters', { chapters: t('count.chapter', { count: n }) });
}

/**
 * The label of a volumes run: the volumes with a selected chapter, and the
 * selected chapters outside them. "" when the Volumes box doesn't parse or no
 * volume has a selected chapter, so the caller falls back to the chapter count.
 */
function volumesLabel(text: string, chapters: Chapter[], selected: number[], t: T): string {
  let grouping: Grouping;
  try {
    grouping = groupChapters(chapters, parseVolumes(text));
  } catch {
    return '';
  }
  const chosen = new Set(selected);
  const volumes = grouping.volumes.map((v) => selectedIn(v.chapters, chosen)).filter((k) => k > 0);
  if (volumes.length === 0) {
    return '';
  }
  const inVolumes = volumes.reduce((sum, k) => sum + k, 0);
  const loose = selectedIn(grouping.loose, chosen);
  const named = t('count.volume', { count: volumes.length });
  if (loose > 0) {
    return t('download.label.volumesAndChapters', { volumes: named, chapters: t('count.chapter', { count: loose }) });
  }

  return t('download.label.volumes', { volumes: named, chapters: t('count.chapter', { count: inVolumes }) });
}

/**
 * The label of a parts run: one file per chapter, so it counts the chapters
 * the selected parts make, and says so only when some of them merge.
 */
function partsLabel(chapters: Chapter[], selected: number[], t: T): string {
  const chosen = new Set(selected);
  let files = 0;
  let merged = false;
  for (const g of groupParts(chapters)) {
    const k = selectedIn(g.chapters, chosen);
    files += k > 0 ? 1 : 0;
    merged ||= k > 1;
  }
  const noun = t('count.chapter', { count: files });

  return merged
    ? t('download.label.partsMerged', { chapters: noun })
    : t('download.label.chapters', { chapters: noun });
}

export type Tone = 'success' | 'warning' | 'danger' | 'neutral';

/** The colour a finished run is shown in */
export function summaryTone(s: Summary): Tone {
  if (s.error !== '') {
    return 'danger';
  }
  if (s.cancelled) {
    return 'neutral';
  }
  if (s.failures.length > 0) {
    return s.saved > 0 || s.volumes > 0 || (s.bundled && s.bundle !== '') ? 'warning' : 'danger';
  }
  if (s.saved === 0 && s.volumes === 0 && !(s.bundled && s.bundle !== '')) {
    return 'neutral';
  }

  return 'success';
}

/** The host of a URL without "www.", or "" when it doesn't parse */
export function hostOf(url: string): string {
  try {
    return new URL(url.trim()).hostname.replace(/^www\./, '');
  } catch {
    return '';
  }
}
