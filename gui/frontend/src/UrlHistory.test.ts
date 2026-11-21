import { describe, expect, it } from 'vitest';
import { filterHistory, historyStep } from './UrlHistory';
import type { HistoryItem } from './types';

const items: HistoryItem[] = [
  { title: 'One Piece', url: 'https://example.com/manga/one-piece', site: '', adult: false, when: 2, done: 1, todo: 0 },
  { title: 'Vinland Saga', url: 'https://other.org/vinland', site: '', adult: false, when: 1, done: 0, todo: 3 },
];

describe('filterHistory', () => {
  it('matches the title or the URL, ignoring case, and keeps everything for nothing typed', () => {
    expect(filterHistory(items, '')).toEqual(items);
    expect(filterHistory(items, 'vinLAND').map((i) => i.title)).toEqual(['Vinland Saga']);
    expect(filterHistory(items, 'example.com').map((i) => i.title)).toEqual(['One Piece']);
    expect(filterHistory(items, 'zzz')).toEqual([]);
  });
});

describe('historyStep', () => {
  it('opens on ↓ and leaves every other key to the field while closed', () => {
    expect(historyStep('ArrowDown', false, -1, 0)).toEqual({ do: 'open' });
    for (const key of ['ArrowUp', 'Enter', 'Escape', 'a']) {
      expect(historyStep(key, false, -1, 2)).toEqual({ do: 'none' });
    }
  });

  it('moves the highlight within the list, picks it on Enter and closes on Esc', () => {
    expect(historyStep('ArrowDown', true, -1, 2)).toEqual({ do: 'move', active: 0 });
    expect(historyStep('ArrowDown', true, 1, 2)).toEqual({ do: 'move', active: 1 });
    expect(historyStep('ArrowUp', true, 1, 2)).toEqual({ do: 'move', active: 0 });
    expect(historyStep('ArrowUp', true, 0, 2)).toEqual({ do: 'move', active: 0 });
    expect(historyStep('Enter', true, 1, 2)).toEqual({ do: 'pick', index: 1 });
    expect(historyStep('Escape', true, 1, 2)).toEqual({ do: 'close' });
  });

  it('lets Enter load the typed link when nothing is highlighted', () => {
    expect(historyStep('Enter', true, -1, 2)).toEqual({ do: 'none' });
  });
});
