// Asked while a download's dedupe waits: the folder's drive has no Recycle
// Bin (a memory card), so its duplicate files would be deleted for good.
// Keep them is the default; the download goes on either way. It reuses the
// browser alert's look.

import { useEffect, useRef } from 'react';
import { useI18n } from './i18n/react';
import { AlertIcon } from './icons';
import { sizeText } from './run';
import type { PermanentAsk } from './types';

interface Props {
  ask: PermanentAsk;
  onAnswer: (yes: boolean) => void;
}

export function PermanentDialog({ ask, onAnswer }: Props) {
  const { t } = useI18n();
  const keep = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    keep.current?.focus();
  }, [ask.id]);

  return (
    <div className="browser-alert-backdrop">
      <section
        className="browser-alert"
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="permanent-title"
        aria-describedby="permanent-text"
        onKeyDown={(e) => {
          if (e.key === 'Escape') {
            onAnswer(false);
          }
        }}
      >
        <h2 id="permanent-title">
          <AlertIcon size={18} /> {t('chapters.permanent.title')}
        </h2>
        <p id="permanent-text">
          {t('chapters.permanent.body', { dir: ask.dir, count: ask.files, size: sizeText(ask.bytes) })}
        </p>
        <div className="browser-alert-actions">
          <button type="button" className="btn primary" ref={keep} onClick={() => onAnswer(false)}>
            {t('chapters.permanent.keep')}
          </button>
          <button type="button" className="btn danger-outline" onClick={() => onAnswer(true)}>
            {t('chapters.permanent.confirm')}
          </button>
        </div>
      </section>
    </div>
  );
}
