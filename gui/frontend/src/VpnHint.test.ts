import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { ChapterPanel } from './ChapterPanel';
import { I18nProvider } from './i18n/react';
import { createMockApi } from './mock';
import type { State } from './types';
import { VpnHint } from './VpnHint';

function hint(reason: string, lang: 'en' | 'es' | 'ca' | 'ja' = 'en'): string {
  return renderToStaticMarkup(
    createElement(I18nProvider, { pref: lang, persist: async () => {} }, createElement(VpnHint, { reason })),
  );
}

async function panel(over: Partial<State>): Promise<string> {
  const api = createMockApi('loaded');
  const state = { ...(await api.state()), ...over };
  const chapters = await api.chapters();
  const noop = async () => {};
  return renderToStaticMarkup(
    createElement(
      I18nProvider,
      { pref: 'en', persist: async () => {} },
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
        onDownloadMissing: noop,
        onReload: () => {},
        onError: () => {},
      }),
    ),
  );
}

describe('VpnHint', () => {
  it('shows nothing when no answer of the site said so', () => {
    expect(hint('')).toBe('');
    expect(hint('something else')).toBe('');
  });

  it('only ever says a VPN might help, and why', () => {
    const region = hint('region');
    expect(region).toContain('class="vpn-hint"');
    expect(region).toContain('>VPN?</span>');
    expect(region).toContain('might need a VPN');
    expect(region).toContain('country or region');
    const dns = hint('dns');
    expect(dns).toContain('might need a VPN');
    expect(dns).toContain("address didn&#x27;t resolve");
  });

  it('speaks the language of the app', () => {
    expect(hint('region', 'es')).toContain('¿VPN?');
    expect(hint('region', 'es')).toContain('país o región');
    expect(hint('dns', 'ca')).toContain('necessita una VPN');
    expect(hint('region', 'ja')).toContain('VPNが必要かもしれません');
  });
});

describe('ChapterPanel', () => {
  it('tags the site of the loaded series, and nothing without a hint', async () => {
    expect(await panel({ vpn: '' })).not.toContain('vpn-hint');
    const html = await panel({ vpn: 'region' });
    // right after the site's name
    expect(html).toMatch(/class="host">[^<]+<\/span><span class="vpn-hint"/);
  });

  it('tags the site of a load that failed', async () => {
    const failed = { loaded: false, loading: false, loadError: 'site not recognised', url: 'https://desu.me/manga/x' };
    expect(await panel({ ...failed, vpn: '' })).not.toContain('vpn-hint');
    const html = await panel({ ...failed, vpn: 'dns' });
    expect(html).toContain('https://desu.me/manga/x <span class="vpn-hint"');
  });
});
