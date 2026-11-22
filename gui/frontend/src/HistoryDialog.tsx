// The history pop-up: the series loaded or downloaded, newest first (the last
// 100, as history.txt keeps them). A series opens its page in the system's
// browser, its site the site's home page, and its Load button puts it in the
// URL box and loads it, closing the pop-up. An adult title shows masked until
// clicked. Clear empties the history.

import { useEffect, useRef, useState, type MouseEvent } from 'react';
import { errorMessage } from './format';
import { useI18n } from './i18n/react';
import { XIcon } from './icons';
import { linkClick } from './links';
import { MaskedTitle, useShown } from './Masked';
import { hostOf } from './run';
import type { HistoryItem } from './types';

interface Props {
  /** Reads the history; called when the dialog opens */
  load: () => Promise<HistoryItem[]>;
  onClear: () => Promise<void>;
  onClose: () => void;
  onOpenURL: (url: string) => void;
  /** Loads a series into the URL box and the chapter list */
  onLoadURL?: (url: string) => void;
}

export function HistoryDialog({ load, onClear, onClose, onOpenURL, onLoadURL }: Props) {
  const { t } = useI18n();
  const dialog = useRef<HTMLDialogElement>(null);
  const [items, setItems] = useState<HistoryItem[] | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    const el = dialog.current;
    if (el && !el.open) el.showModal();
    // closing on unmount hands focus back to the element that opened the dialog
    return () => {
      if (el?.open) el.close();
    };
  }, []);

  useEffect(() => {
    let live = true;
    load()
      .then((list) => live && setItems(list))
      .catch((e: unknown) => live && setError(errorMessage(e, t)));
    return () => {
      live = false;
    };
    // read once, when the dialog opens
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // A click on the dialog itself is a backdrop click, but only when the press started there too.
  const pressedBackdrop = useRef(false);
  const onBackdropDown = (e: MouseEvent<HTMLDialogElement>) => {
    pressedBackdrop.current = e.target === e.currentTarget;
  };
  const onBackdrop = (e: MouseEvent<HTMLDialogElement>) => {
    if (pressedBackdrop.current && e.target === e.currentTarget) onClose();
  };

  const clear = () => {
    onClear()
      .then(() => setItems([]))
      .catch((e: unknown) => setError(errorMessage(e, t)));
  };

  return (
    <dialog
      ref={dialog}
      className="settings-dialog history-dialog"
      aria-labelledby="history-title"
      onCancel={(e) => {
        e.preventDefault();
        onClose();
      }}
      onMouseDown={onBackdropDown}
      onClick={onBackdrop}
    >
      <div className="settings-content">
        <header className="settings-head">
          <h2 id="history-title">{t('history.title')}</h2>
          <button type="button" className="btn ghost icon-only" aria-label={t('history.close')} onClick={onClose}>
            <XIcon size={16} />
          </button>
        </header>

        <div className="settings-body">
          {error !== '' && <p className="error-text small">{error}</p>}
          {items !== null && items.length === 0 && <p className="muted">{t('history.empty')}</p>}
          {items !== null && items.length > 0 && (
            <>
              <p className="muted small">{t('history.hint')}</p>
              <ul className="history-list">
                {items.map((item) => (
                  <HistoryRow key={item.url} item={item} onOpenURL={onOpenURL} onLoadURL={onLoadURL} />
                ))}
              </ul>
            </>
          )}
        </div>

        <footer className="history-foot">
          <button type="button" className="btn small" disabled={!items || items.length === 0} onClick={clear}>
            {t('history.clear')}
          </button>
        </footer>
      </div>
    </dialog>
  );
}

/** One series of the list; exported for the rendered-strings test */
export function HistoryRow({
  item,
  onOpenURL,
  onLoadURL,
}: {
  item: HistoryItem;
  onOpenURL: (url: string) => void;
  onLoadURL?: (url: string) => void;
}) {
  const { t, lang } = useI18n();
  const shown = useShown(item.title, item.adult);
  const when = item.when > 0 ? new Date(item.when).toLocaleString(lang, { dateStyle: 'medium', timeStyle: 'short' }) : '';
  return (
    <li className="history-item">
      {/* while masked, the title is a button that reveals it; once shown, a link to the series */}
      {shown ? (
        <a className="history-title" href={item.url} title={item.url} onClick={linkClick(item.url, onOpenURL)}>
          {item.title || item.url}
        </a>
      ) : (
        <span className="history-title">
          <MaskedTitle title={item.title} adult={item.adult} />
        </span>
      )}
      <span className="history-meta muted small">
        {item.site !== '' && (
          <a href={item.site} onClick={linkClick(item.site, onOpenURL)}>
            {hostOf(item.site)}
          </a>
        )}
        {when !== '' && <span>{when}</span>}
        {item.done + item.todo > 0 && <span>{t('history.counts', { done: item.done, todo: item.todo })}</span>}
      </span>
      {onLoadURL && (
        <button
          type="button"
          className="btn small history-load"
          title={t('history.loadLabel')}
          onClick={() => onLoadURL(item.url)}
        >
          {t('history.load')}
        </button>
      )}
    </li>
  );
}
