import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { CATALOG, createT } from './i18n';
import { I18nProvider } from './i18n/react';
import {
  ChapterBox,
  ChapterMarkIcon,
  ChapterPanel,
  ChapterTitle,
  RangeForm,
  canApplyRange,
  partsCount,
  unfinishedOf,
} from './ChapterPanel';
import { createMockApi } from './mock';

const LOCKED_HINT = createT('en')('chapters.locked');

function title(locked: boolean): string {
  return renderToStaticMarkup(createElement(ChapterTitle, { title: 'Chapter 186', locked }));
}

describe('ChapterTitle', () => {
  it('marks a locked chapter with a lock that names why, in the title', () => {
    const html = title(true);
    expect(html).toContain(`class="lock" role="img" title="${LOCKED_HINT}" aria-label="${LOCKED_HINT}"`);
    expect(html).toContain('class="title"');
    expect(html.indexOf('class="lock"')).toBeLessThan(html.indexOf('Chapter 186'));
  });

  it('shows a free chapter with no lock at all', () => {
    const html = title(false);
    expect(html).not.toContain('lock');
    expect(html).toBe('<span class="title">Chapter 186</span>');
  });
});

describe('ChapterBox', () => {
  const box = (on: boolean, locked: boolean) => renderToStaticMarkup(createElement(ChapterBox, { on, locked }));

  it('greys out the box of a locked chapter', () => {
    expect(box(false, true)).toBe('<span class="box disabled" aria-hidden="true"></span>');
  });

  it('leaves a free chapter\'s box as it was', () => {
    expect(box(false, false)).toBe('<span class="box" aria-hidden="true"></span>');
    expect(box(true, false)).toBe('<span class="box on" aria-hidden="true"></span>');
  });
});


describe('RangeForm', () => {
  const form = (text: string) =>
    renderToStaticMarkup(createElement(RangeForm, { text, bad: false, onText: () => {}, onApply: () => {} }));
  const plus = '<div class="input-action"><button type="submit" class="btn ghost small icon-only" aria-label="Add range to selection" title="Add range to selection"';

  it('shows the + inside the field only once there is text', () => {
    expect(form('')).not.toContain('input-action');
    expect(form('   ')).not.toContain('input-action');
    expect(form('1-10')).toContain(plus);
    expect(form('1-10')).toMatch(/<div class="input-wrap">.*<\/button><\/div><\/div>/);
  });

  it('applies Enter only when the box has a range in it', () => {
    expect(canApplyRange('   ')).toBe(false);
    expect(canApplyRange('')).toBe(false);
    expect(canApplyRange('1-10')).toBe(true);
  });
});

describe('partsCount', () => {
  it('says whether a chapter goes whole, in part, or not at all', () => {
    const t = createT('en');
    expect(partsCount(0, 2, t)).toBe('none selected');
    expect(partsCount(2, 2, t)).toBe('whole chapter');
    expect(partsCount(1, 2, t)).toBe('1 of 2 parts');
  });

  it('translates the header of a chapter with its parts picked', () => {
    expect(partsCount(1, 2, createT('es'))).toBe('1 de 2 partes');
    expect(partsCount(2, 2, createT('ca'))).toBe('capítol sencer');
  });
});

describe('ChapterTitle in another language', () => {
  it('names the lock in the language of the app', () => {
    const html = renderToStaticMarkup(
      createElement(
        I18nProvider,
        { pref: 'es', persist: async () => {}, catalog: CATALOG, systemLanguages: [] },
        createElement(ChapterTitle, { title: 'Capítulo 186', locked: true }),
      ),
    );
    expect(html).toContain('title="Bloqueado, así que no se puede seleccionar"');
  });
});

describe('ChapterPanel of a parts run', () => {
  it('heads a merged chapter with its title and count, and shows each part by its part alone', async () => {
    const api = createMockApi({ scenario: 'parts', uiLanguage: 'ca' });
    const state = await api.state();
    const chapters = await api.chapters();
    const noop = async () => {};
    const markup = renderToStaticMarkup(
      createElement(ChapterPanel, {
        state,
        chapters,
        selected: [],
        marks: [],
        disabled: false,
        onToggle: noop,
        onToggleMany: noop,
        onSetAll: noop,
        onRange: async () => 0,
        onVolumes: noop,
        onReload: () => {},
        onError: () => {},
      }),
    );
    const first = chapters.find((c) => c.number === 2 && c.part === 'Part 1');
    expect(first).toBeDefined();
    const title = first?.wholeTitle ?? '';
    expect(title).not.toBe('');
    expect(markup).toContain(`Chapter 2 · ${escapeHtml(title)} · 2 parts`);
    expect(markup).toMatch(/class="[^"]*\bpart-row\b/);
    expect(markup).toMatch(/>Part 1</);
    expect(markup).toMatch(/>Part 2</);
    // a part row never repeats the chapter's title
    expect(markup).not.toContain(escapeHtml(first?.title ?? ''));
  });
});

describe('ChapterMarkIcon', () => {
  it('shows nothing but its place for a chapter with no file and no download', () => {
    const html = renderToStaticMarkup(createElement(ChapterMarkIcon, { mark: '' }));
    expect(html).toBe('<span class="mark" aria-hidden="true"></span>');
  });

  it('names each mark, so it can be read and hovered', () => {
    const names = {
      done: 'Downloaded: its file is in the folder',
      downloading: 'Downloading',
      queued: 'Waiting to download',
      failed: 'Failed: pick it to download it again',
      cancelled: 'Cancelled before it was saved: pick it to download it again',
    } as const;
    for (const [mark, name] of Object.entries(names)) {
      const html = renderToStaticMarkup(createElement(ChapterMarkIcon, { mark: mark as keyof typeof names }));
      expect(html).toContain(`class="mark mark-${mark}"`);
      expect(html).toContain(`title="${name}"`);
      expect(html).toContain(`aria-label="${name}"`);
    }
  });

  it('draws failed and cancelled with different glyphs', () => {
    const failed = renderToStaticMarkup(createElement(ChapterMarkIcon, { mark: 'failed' }));
    const cancelled = renderToStaticMarkup(createElement(ChapterMarkIcon, { mark: 'cancelled' }));
    expect(failed.replace(/^<span[^>]*>/, '')).not.toBe(cancelled.replace(/^<span[^>]*>/, ''));
  });
});

describe('unfinishedOf', () => {
  const ch = (index: number, locked = false) => ({ index, number: index + 1, title: '', language: '', group: '', volume: 0, locked });

  it('picks the failed and cancelled chapters, never a locked one', () => {
    const chapters = [ch(0), ch(1), ch(2), ch(3, true), ch(4), ch(5)];
    expect(unfinishedOf(chapters, ['done', 'failed', 'cancelled', 'failed', 'downloading', ''])).toEqual([1, 2]);
  });

  it('picks nothing before the marks arrive', () => {
    expect(unfinishedOf([ch(0), ch(1)], [])).toEqual([]);
  });
});

describe('ChapterPanel marks', () => {
  it('shows each row\'s mark beside its box, and offers to pick the unfinished ones', async () => {
    const api = createMockApi({ scenario: 'loaded' });
    const state = await api.state();
    const chapters = await api.chapters();
    const noop = async () => {};
    const marks = chapters.map((_, i) => (i === 0 ? 'done' : i === 1 ? 'failed' : i === 2 ? 'cancelled' : '')) as ('' | 'done' | 'failed' | 'cancelled')[];
    const markup = renderToStaticMarkup(
      createElement(ChapterPanel, {
        state,
        chapters,
        selected: [],
        marks,
        disabled: false,
        onToggle: noop,
        onToggleMany: noop,
        onSetAll: noop,
        onRange: async () => 0,
        onVolumes: noop,
        onReload: () => {},
        onError: () => {},
      }),
    );
    expect(markup).toMatch(/class="box"[^>]*><\/span><span class="mark mark-done"/);
    expect(markup).toContain('mark mark-failed');
    expect(markup).toContain('mark mark-cancelled');
    expect(markup).toContain('Pick 2 unfinished');
  });

  it('offers nothing to pick while the series downloads, as its run never takes a chapter twice', async () => {
    const api = createMockApi({ scenario: 'loaded' });
    const state = { ...(await api.state()), addTo: 1 };
    const chapters = await api.chapters();
    const noop = async () => {};
    const markup = renderToStaticMarkup(
      createElement(ChapterPanel, {
        state,
        chapters,
        selected: [],
        marks: chapters.map(() => 'failed' as const),
        disabled: false,
        onToggle: noop,
        onToggleMany: noop,
        onSetAll: noop,
        onRange: async () => 0,
        onVolumes: noop,
        onReload: () => {},
        onError: () => {},
      }),
    );
    expect(markup).toContain('mark mark-failed');
    expect(markup).not.toContain('unfinished');
  });

  it('offers nothing to pick while no chapter is unfinished', async () => {
    const api = createMockApi({ scenario: 'loaded' });
    const state = await api.state();
    const chapters = await api.chapters();
    const noop = async () => {};
    const markup = renderToStaticMarkup(
      createElement(ChapterPanel, {
        state,
        chapters,
        selected: [],
        marks: chapters.map(() => 'done' as const),
        disabled: false,
        onToggle: noop,
        onToggleMany: noop,
        onSetAll: noop,
        onRange: async () => 0,
        onVolumes: noop,
        onReload: () => {},
        onError: () => {},
      }),
    );
    expect(markup).not.toContain('unfinished');
  });
});

function escapeHtml(text: string): string {
  return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#x27;');
}
