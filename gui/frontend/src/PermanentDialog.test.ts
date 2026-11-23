import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { I18nProvider } from './i18n/react';
import { PermanentDialog } from './PermanentDialog';

const ask = { id: 1, group: 2, dir: 'J:\\Manga\\Gintama', files: 3, bytes: 6_400_000 };
const render = (pref: 'en' | 'uk') =>
  renderToStaticMarkup(
    createElement(
      I18nProvider,
      { pref, persist: async () => undefined, systemLanguages: ['en'] },
      createElement(PermanentDialog, { ask, onAnswer: () => undefined }),
    ),
  );

describe('PermanentDialog', () => {
  it('says the drive has no Recycle Bin, what would go, and that it cannot be restored', () => {
    const html = render('en');
    expect(html).toContain('No Recycle Bin on this drive');
    expect(html).toContain('J:\\Manga\\Gintama has no Recycle Bin. Delete 3 duplicate files (6.4 MB) for good? They can&#x27;t be restored.');
  });

  it('puts Keep them first, as the safe button, and Delete for good second', () => {
    const html = render('en');
    const keep = html.indexOf('Keep them');
    const del = html.indexOf('Delete for good');
    expect(keep).toBeGreaterThan(0);
    expect(del).toBeGreaterThan(keep);
    expect(html).toMatch(/class="btn primary"[^>]*>Keep them/);
    expect(html).toMatch(/class="btn danger-outline"[^>]*>Delete for good/);
  });

  it('is translated, with the language’s plural', () => {
    const html = render('uk');
    expect(html).toContain('Остаточно видалити 3 файли-дублікати');
    expect(html).toContain('Залишити');
  });
});
