// The history drop-down of the URL box: the past series (history.txt, newest
// first) under the field. The arrow button or ↓ in the field opens it; once
// open, typing filters it by title or URL, ↑/↓ move, Enter picks and Esc
// closes. Picking fills the field and loads the series, as a paste does. An
// adult title shows masked, its URL in full; Clear empties the history.

import { useState, type KeyboardEvent } from 'react';
import { useI18n } from './i18n/react';
import { ChevronIcon } from './icons';
import { MaskedTitle } from './Masked';
import type { HistoryItem } from './types';

export const URL_HISTORY_ID = 'url-history';

/** The entries whose title or URL holds query, ignoring case; all of them for "" */
export function filterHistory(items: HistoryItem[], query: string): HistoryItem[] {
  const q = query.trim().toLowerCase();
  if (q === '') return items;
  return items.filter((i) => i.title.toLowerCase().includes(q) || i.url.toLowerCase().includes(q));
}

/** What a key in the field does to the list: open it, move the highlight, close it, pick the highlighted entry, or nothing (the field keeps the key) */
export type HistoryStep =
  | { do: 'open' }
  | { do: 'move'; active: number }
  | { do: 'close' }
  | { do: 'pick'; index: number }
  | { do: 'none' };

/** The step for key, with the list open or not, active highlighted among count entries */
export function historyStep(key: string, open: boolean, active: number, count: number): HistoryStep {
  if (key === 'ArrowDown') {
    return open ? { do: 'move', active: Math.min(active + 1, count - 1) } : { do: 'open' };
  }
  if (!open) return { do: 'none' };
  if (key === 'ArrowUp') return { do: 'move', active: Math.max(active - 1, 0) };
  if (key === 'Escape') return { do: 'close' };
  if (key === 'Enter' && active >= 0 && active < count) return { do: 'pick', index: active };
  return { do: 'none' };
}

export interface UrlHistory {
  open: boolean;
  /** The entries shown: the history filtered by what was typed since it opened */
  shown: HistoryItem[];
  /** The highlighted entry of shown, -1 for none */
  active: number;
  toggle: () => void;
  close: () => void;
  /** The field's text changed: an open list filters by it */
  typed: (value: string) => void;
  /** The field's keys; true when the list took the key */
  onKeyDown: (e: KeyboardEvent<HTMLInputElement>) => boolean;
  pick: (item: HistoryItem) => void;
  clear: () => void;
}

export function useUrlHistory(
  load: () => Promise<HistoryItem[]>,
  onClear: () => Promise<void>,
  onPick: (url: string) => void,
): UrlHistory {
  const [open, setOpen] = useState(false);
  const [items, setItems] = useState<HistoryItem[]>([]);
  const [query, setQuery] = useState('');
  const [active, setActive] = useState(-1);
  const shown = filterHistory(items, query);

  const show = () => {
    load()
      .then((list) => {
        setItems(list);
        setQuery('');
        setActive(-1);
        setOpen(true);
      })
      .catch(() => undefined);
  };
  const close = () => {
    setOpen(false);
    setActive(-1);
  };
  const pick = (item: HistoryItem) => {
    close();
    onPick(item.url);
  };

  return {
    open,
    shown,
    active,
    toggle: () => (open ? close() : show()),
    close,
    typed: (value) => {
      if (open) {
        setQuery(value);
        setActive(-1);
      }
    },
    onKeyDown: (e) => {
      const step = historyStep(e.key, open, active, shown.length);
      switch (step.do) {
        case 'open':
          show();
          return true;
        case 'move':
          setActive(step.active);
          return true;
        case 'close':
          close();
          return true;
        case 'pick':
          pick(shown[step.index]);
          return true;
        case 'none':
          return false;
      }
    },
    pick,
    clear: () => {
      onClear()
        .then(() => setItems([]))
        .catch(() => undefined);
    },
  };
}

/** The arrow button inside the URL box */
export function UrlHistoryButton({ history }: { history: UrlHistory }) {
  const { t } = useI18n();
  return (
    <button
      type="button"
      className={`url-history-btn${history.open ? ' open' : ''}`}
      aria-label={t('history.dropdown')}
      title={t('history.dropdown')}
      aria-haspopup="listbox"
      aria-expanded={history.open}
      aria-controls={history.open ? URL_HISTORY_ID : undefined}
      // the field keeps the focus, so the keys keep working
      onMouseDown={(e) => e.preventDefault()}
      onClick={history.toggle}
    >
      <ChevronIcon size={14} />
    </button>
  );
}

/** The list under the URL box, while it's open */
export function UrlHistoryList({ history }: { history: UrlHistory }) {
  const { t } = useI18n();
  if (!history.open) return null;
  return (
    // a press in the list keeps the focus in the field
    <div className="url-history" onMouseDown={(e) => e.preventDefault()}>
      {history.shown.length === 0 ? (
        <p className="url-history-empty muted small">{t('history.noMatch')}</p>
      ) : (
        <ul id={URL_HISTORY_ID} role="listbox" aria-label={t('history.title')}>
          {history.shown.map((item, i) => (
            <li
              key={item.url}
              id={`${URL_HISTORY_ID}-${i}`}
              role="option"
              aria-selected={i === history.active}
              className={`url-history-item${i === history.active ? ' active' : ''}`}
              onClick={() => history.pick(item)}
            >
              <span className="url-history-title">
                <MaskedTitle title={item.title || item.url} adult={item.adult} />
              </span>
              <span className="url-history-url muted small">{item.url}</span>
            </li>
          ))}
        </ul>
      )}
      <div className="url-history-foot">
        <button type="button" className="btn small ghost" disabled={history.shown.length === 0} onClick={history.clear}>
          {t('history.clear')}
        </button>
      </div>
    </div>
  );
}
