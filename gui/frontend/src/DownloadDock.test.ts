import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { I18nProvider } from './i18n/react';
import { DownloadDock } from './DownloadDock';
import { createMockApi } from './mock';
import type { Chapter, Group, State } from './types';

/** A running group of chapters chapters, none started */
const group = (id: number, chapters: number): Group => ({
  id,
  title: `Series ${id}`,
  running: true,
  cancelling: false,
  run: { chapters, bundle: false, volumes: false, rows: 0 },
  jobs: [],
  summary: null,
});

async function render(lang: 'en' | 'es' | 'ca' | 'ja', over: Partial<State> = {}, error = ''): Promise<string> {
  const state = { ...(await createMockApi('loaded').state()), ...over };
  const chapters: Chapter[] = [];
  return renderToStaticMarkup(
    createElement(
      I18nProvider,
      { pref: lang, persist: async () => {} },
      createElement(DownloadDock, { state, error, chapters, selected: [], onStart: () => {}, onCancel: () => {} }),
    ),
  );
}

/** The Download button's opening tag */
const startButton = (html: string): string => /<button[^>]*class="btn primary block big"[^>]*>/.exec(html)?.[0] ?? '';

describe('DownloadDock', () => {
  it('shows the idle dock in the chosen language', async () => {
    const es = await render('es');
    expect(es).toContain('Descargar ');
    expect(es).toContain('Archivos CBZ en');
    const ca = await render('ca');
    expect(ca).toContain('Descarrega ');
    expect(ca).toContain('Fitxers CBZ a');
  });

  it('shows the series folder a run would save into, else the picked folder', async () => {
    const base = await createMockApi('loaded').state();
    const picked = await render('en', { destination: '' });
    expect(picked).toContain(base.options.outputDir);
    const series = await render('en', { destination: '/home/reader/Downloads/One Piece [en]' });
    expect(series).toContain('One Piece [en]');
  });

  it('translates the running progress and the cancelling state', async () => {
    const one = group(1, 4);
    const es = await render('es', { running: true, cancelling: true, groups: [{ ...one, cancelling: true }] });
    expect(es).toContain('Terminando los capítulos en curso');
    expect(es).toContain('Cancelando…');
    const ca = await render('ca', { running: true, groups: [one] });
    expect(ca).toContain('0 de 4 capítols acabats');
    expect(ca).toContain('Cancel·la');
  });

  it('adds to the running series, and greys out what it has already', async () => {
    const one = group(1, 4);
    const adding = await render('en', { running: true, groups: [one], addTo: 1, fresh: 3 });
    expect(adding).toContain('Add 3 chapters to the queue');
    expect(startButton(adding)).not.toBe('');
    expect(startButton(adding)).not.toContain('disabled');
    const queued = await render('en', { running: true, groups: [one], addTo: 1, fresh: 0 });
    expect(startButton(queued)).toContain('disabled=""');
    expect(queued).toContain("already in this series&#x27; download");
    const started = await render('en', { unchanged: true });
    expect(startButton(started)).toContain('disabled=""');
    expect(started).toContain('This selection was already started');
  });

  it('keeps Download under the progress while groups run', async () => {
    const en = await render('en', { running: true, groups: [group(1, 4)] });
    expect(en).toContain('0 of 4 chapters done');
    expect(en).toMatch(/<button[^>]*class="btn primary block big"[^>]*>.*Download/);
    expect(en.indexOf('role="progressbar"')).toBeLessThan(en.indexOf('btn primary block big'));
  });

  it('adds up several running groups and cancels them all', async () => {
    const en = await render('en', { running: true, groups: [group(1, 4), group(2, 6), { ...group(3, 9), running: false }] });
    expect(en).toContain('2 downloads · 0 of 10 done');
    expect(en).toContain('Cancel all');
    const ja = await render('ja', { running: true, groups: [group(1, 4), group(2, 6)] });
    expect(ja).toContain('すべてキャンセル');
  });

  it('translates the refused download', async () => {
    expect(await render('es', {}, 'disco lleno')).toContain('No se puede descargar: disco lleno');
  });
});
