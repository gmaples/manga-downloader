import { createElement, type ReactElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ActivityPanel } from './ActivityPanel';
import { HistoryRow } from './HistoryDialog';
import { I18nProvider } from './i18n/react';
import { MaskedTitle, resetReveals, reveal } from './Masked';
import { createMockApi } from './mock';
import type { Group, State } from './types';

function render(element: ReactElement): string {
  return renderToStaticMarkup(
    createElement(I18nProvider, { pref: 'en', persist: async () => {}, systemLanguages: ['en-US'] }, element),
  );
}

afterEach(() => resetReveals());

describe('adult titles', () => {
  it('show masked until revealed, and a title without a rating shows as it is', () => {
    expect(render(createElement(MaskedTitle, { title: 'Night Garden', adult: true }))).toContain('[Adult Title]');
    expect(render(createElement(MaskedTitle, { title: 'Night Garden', adult: true }))).not.toContain('Night Garden');
    expect(render(createElement(MaskedTitle, { title: 'One Piece', adult: false }))).toBe('One Piece');
    reveal('Night Garden');
    expect(render(createElement(MaskedTitle, { title: 'Night Garden', adult: true }))).toBe('Night Garden');
  });

  it('are masked in a group header and in the folder its summary names', () => {
    const group: Group = {
      id: 1,
      title: 'Night Garden',
      running: false,
      cancelling: false,
      run: { chapters: 1, bundle: false, volumes: false, rows: 1 },
      jobs: [],
      summary: {
        dir: '/out/Night Garden',
        cancelled: false,
        saved: 1,
        volumes: 0,
        parts: false,
        bundled: false,
        bundle: '',
        skipped: 0,
        existing: 0,
        failures: [],
        error: '',
      },
      restored: false,
      url: 'https://example.com/n',
      adult: true,
    };
    const html = render(
      createElement(ActivityPanel, {
        state: { groups: [group], running: false, summary: group.summary, loading: false } as unknown as State,
        onOpenFolder: () => undefined,
        onCancelGroup: () => undefined,
        onResume: () => undefined,
      }),
    );
    expect(html).toContain('[Adult Title]');
    expect(html).not.toContain('Night Garden');
  });
});

describe('the history pop-up', () => {
  it('masks an adult series until clicked, and links a plain one to its page', () => {
    const item = { title: 'Night Garden', url: 'https://example.com/n', site: 'https://example.com/', when: 0, done: 3, todo: 1 };
    const masked = render(createElement(HistoryRow, { item: { ...item, adult: true }, onOpenURL: () => undefined }));
    expect(masked).toContain('[Adult Title]');
    expect(masked).not.toContain('Night Garden');
    expect(masked).toContain('example.com');
    expect(masked).toContain('3 saved, 1 to do');
    const plain = render(createElement(HistoryRow, { item: { ...item, adult: false }, onOpenURL: () => undefined }));
    expect(plain).toContain('href="https://example.com/n"');
    expect(plain).toContain('Night Garden');
  });
});

describe('the last session', () => {
  it('shows its groups with the chapters done and to do, and a button to resume', () => {
    const api = createMockApi('restored');
    return api.state().then((state) => {
      const vinland = state.groups.find((g) => g.title === 'Vinland Saga');
      expect(vinland?.restored).toBe(true);
      const html = render(
        createElement(ActivityPanel, {
          state,
          onOpenFolder: () => undefined,
          onCancelGroup: () => undefined,
          onResume: () => undefined,
        }),
      );
      expect(html).toContain('From last session');
      expect(html).toContain('Load to resume');
      expect(html).toContain('[Adult Title]');
    });
  });

  it('resumes by loading the series with its chapters to do selected, and downloads nothing', async () => {
    vi.useFakeTimers();
    try {
      const api = createMockApi('restored');
      const vinland = (await api.state()).groups.find((g) => g.title === 'Vinland Saga');
      if (!vinland) throw new Error('no restored group');
      const done = api.resume(vinland.id);
      await vi.runAllTimersAsync();
      await done;
      const state = await api.state();
      const chapters = await api.chapters();
      const picked = (await api.selection()).map((i) => chapters[i].number).sort((a, b) => a - b);
      const todo = vinland.jobs.filter((j) => j.state === 'todo').map((j) => j.number);
      expect(todo.length).toBe(4);
      expect(picked).toEqual([...todo].sort((a, b) => a - b));
      expect(state.running).toBe(false);
      expect(state.url).toBe(vinland.url);
    } finally {
      vi.useRealTimers();
    }
  });

  it('keeps the history until Clear empties it', async () => {
    const api = createMockApi('empty');
    expect((await api.history()).length).toBe(3);
    await api.clearHistory();
    expect(await api.history()).toEqual([]);
  });
});
