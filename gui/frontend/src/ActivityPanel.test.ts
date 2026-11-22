import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { ActivityPanel } from './ActivityPanel';
import type { LangPref } from './i18n';
import { I18nProvider } from './i18n/react';
import type { Group, Job, State, Summary } from './types';

const summary: Summary = {
  dir: '/out',
  cancelled: false,
  saved: 3,
  volumes: 0,
  parts: false,
  bundled: false,
  bundle: '',
  skipped: 0,
  existing: 0,
  failures: [],
  error: '',
};

const job: Job = {
  title: 'Ch 1',
  tags: '',
  number: 1,
  total: 24,
  done: 6,
  state: 'downloading',
  reason: '',
  status: '',
  volume: false,
};

function render(state: Partial<State>, pref: LangPref = '') {
  return renderToStaticMarkup(
    createElement(
      I18nProvider,
      { pref, persist: async () => {}, systemLanguages: ['en-US'] },
      createElement(ActivityPanel, {
        state: state as State,
        onOpenFolder: () => undefined,
        onCancelGroup: () => undefined,
        onResume: () => undefined,
        onRestart: () => undefined,
      }),
    ),
  );
}

const run = { chapters: 1, bundle: false, volumes: false, rows: 1 };

/** A group of One Piece, running unless it has a summary */
function group(g: Partial<Group>, id = 1): Group {
  const summary = g.summary ?? null;
  return {
    id,
    title: 'One Piece',
    running: summary === null,
    cancelling: false,
    run,
    jobs: [],
    summary,
    restored: false,
    url: 'https://example.com/manga/one-piece',
    adult: false,
    ...g,
  };
}

/** The state of a window with the one group g */
function one(g: Partial<Group>): Partial<State> {
  const only = group(g);
  return { running: only.running, summary: only.summary, groups: [only] };
}

describe('ActivityPanel', () => {
  it('shows the empty state in English by default', () => {
    const html = render({ running: false, summary: null, groups: [] });
    expect(html).toContain('Nothing downloaded yet.');
  });

  it('shows the empty state in Spanish', () => {
    const html = render({ running: false, summary: null, groups: [] }, 'es');
    expect(html).toContain('Todavía no se ha descargado nada.');
    expect(html).not.toContain('Nothing downloaded');
  });

  it('labels and words the last summary in Catalan', () => {
    const html = render(one({ summary, jobs: [] }), 'ca');
    expect(html).toContain('aria-label="Última descàrrega"');
    expect(html).toContain('<h3>3 desats</h3>');
  });

  it('translates the last download\'s error in Spanish', () => {
    const html = render(
      one({
        summary: {
          ...summary,
          bundled: true,
          error: 'no chapter could be downloaded',
          errorMsg: { key: 'errors.noChapterDownloaded', text: 'no chapter could be downloaded' },
        },
        jobs: [],
      }),
      'es',
    );
    expect(html).toContain('no se pudo descargar ningún capítulo');
    expect(html).not.toContain('no chapter could be downloaded');
  });

  it('notes the chapters that were already in the folder', () => {
    const html = render(one({ summary: { ...summary, existing: 2 }, jobs: [] }));
    expect(html).toContain('2 chapters were already in the folder, so they were left out.');
    expect(html).not.toContain('were skipped');
    expect(render(one({ summary, jobs: [] }))).not.toContain('already in the folder');
  });

  it('shows a running volume row in Spanish, and counts a failed one without a row', () => {
    const volume: Job = {
      ...job,
      title: 'Vol. 01 · chapters 1–7',
      titleMsg: {
        key: 'job.title.volume',
        args: { volume: 'Vol. 01', first: '1', last: '7' },
        text: 'Vol. 01 · chapters 1–7',
      },
      state: 'downloading',
    };
    const failed: Job = {
      ...volume,
      title: 'Vol. 02 · chapters 8–9',
      titleMsg: { key: 'job.title.volume', args: { volume: 'Vol. 02', first: '8', last: '9' }, text: 'Vol. 02 · chapters 8–9' },
      state: 'failed',
      reason: 'download: x',
      reasonMsg: { key: 'job.reason.download', args: { reason: 'x' }, text: 'download: x' },
    };
    const html = render(
      one({ summary: null, jobs: [volume, failed], run: { chapters: 9, bundle: false, volumes: true, rows: 2 } }),
      'es',
    );
    expect(html).toContain('Vol. 01 · capítulos 1–7');
    expect(html).not.toContain('Vol. 02');
    expect(html).not.toContain('descarga: x');
    expect(html).toContain('1 fallido');
  });

  it('says how many duplicates the download moved to the Recycle Bin', () => {
    const html = render(one({ summary: null, jobs: [job], cleanup: { removed: 212, bytes: 1_400_000_000, failed: 1 } }));
    expect(html).toContain('Moved 212 duplicate files (1.4 GB) to the Recycle Bin');
    expect(html).toContain('1 duplicate couldn&#x27;t be moved: see the log');
    expect(html).toContain('This run · 1 downloading');
  });

  it('says live what the download is doing before any chapter starts', () => {
    const status = { key: 'activity.status.moving', args: { done: 200, total: 433 }, text: '' };
    const html = render(one({ summary: null, jobs: [], status, run: { chapters: 402, bundle: false, volumes: false, rows: 402 } }));
    expect(html).toContain('Moving duplicates to the Recycle Bin: 200 of 433');
    expect(html).toContain('402 queued');
  });

  it('translates the window words of a failure line and keeps a core reason as it is', () => {
    const failures = [
      {
        label: 'Chapter 4',
        reason: 'packing: disk full',
        reasonMsg: { key: 'job.reason.packing', args: { reason: 'disk full' }, text: 'packing: disk full' },
      },
      { label: 'Chapter 5', reason: 'page 5 returned 404' },
      {
        label: '',
        reason: 'volumes: x',
        reasonMsg: { key: 'log.volumesFailed', args: { reason: 'x' }, text: 'volumes: x' },
      },
    ];
    const html = render(one({ summary: { ...summary, failures }, jobs: [] }), 'ca');
    expect(html).toContain('<strong>Chapter 4</strong><span>empaquetat: disk full</span>');
    expect(html).toContain('<strong>Chapter 5</strong><span>page 5 returned 404</span>');
    expect(html).toContain('<span>volums: x</span>');
  });

  it('titles the running chapters and their progress in Spanish', () => {
    const html = render(
      one({ summary: null, jobs: [job], run: { chapters: 1, bundle: false, volumes: false, rows: 1 } }),
      'es',
    );
    expect(html).toContain('Esta descarga');
    expect(html).toContain('aria-label="Progreso de Ch 1"');
  });
  it('shows each group under its series, with Cancel on the running ones only', () => {
    const done = group({ title: 'Vinland Saga', summary, jobs: [{ ...job, state: 'done', done: 24 }] }, 1);
    const going = group({ title: 'One Piece', jobs: [job] }, 2);
    const html = render({ running: true, summary: null, groups: [done, going] });
    expect(html).toContain('class="activity multi"');
    expect(html.indexOf('Vinland Saga')).toBeLessThan(html.indexOf('One Piece'));
    expect(html).toContain('aria-label="Cancel One Piece"');
    expect(html).not.toContain('aria-label="Cancel Vinland Saga"');
    // the ended group has its summary, the running one its progress
    expect(html).toContain('aria-label="Last download"');
    expect(html).toContain('This run');
    // several groups each get a list of their own height
    expect(html).toContain('class="jobs" role="list" style="height:50px"');
  });

  it('says a group is cancelling, in Japanese', () => {
    const html = render(one({ jobs: [job], cancelling: true }), 'ja');
    expect(html).toContain('aria-label="One Pieceをキャンセル"');
    expect(html).toContain('aria-disabled="true"');
    expect(html).toContain('キャンセル中…');
  });

  it('lets a single group fill the panel', () => {
    const html = render(one({ jobs: [job] }));
    expect(html).toContain('class="activity"');
    expect(html).toContain('class="jobs" role="list">');
  });
});

describe('ActivityPanel restarts', () => {
  const job = (title: string, state: Job['state'], extra: Partial<Job> = {}): Job => ({
    title,
    tags: '',
    number: 1,
    total: 10,
    done: state === 'done' ? 10 : 0,
    state,
    reason: '',
    status: '',
    volume: false,
    ...extra,
  });

  it('offers the group\'s unfinished chapters from its header; a failed chapter has no row', () => {
    const html = render(
      one({
        summary,
        unfinished: 2,
        jobs: [job('Romance Dawn', 'failed', { key: 'k1', restart: true }), job('Two Years Later', 'done')],
      }),
    );
    expect(html).toContain('Restart 2 unfinished');
    expect(html).not.toContain('Romance Dawn');
    expect(html).not.toContain('job-restart');
    expect(html).toContain('1 saved · 1 failed');
  });

  it('offers nothing when nothing is unfinished', () => {
    const html = render(one({ summary, unfinished: 0, jobs: [job('Two Years Later', 'done')] }));
    expect(html).not.toContain('unfinished');
    expect(html).not.toContain('job-restart');
  });
});
