// The confirmation behind the chapter list's "Delete series": what goes (the
// folder, its file count and size), then the count moving as the files go to
// the Recycle Bin a batch at a time. It reuses the browser alert's look.

import { useEffect, useRef, useState } from 'react';
import type { Unsubscribe } from './api';
import { errorMessage } from './format';
import { useI18n } from './i18n/react';
import { AlertIcon } from './icons';
import type { DeleteInfo, DeleteProgress } from './types';

interface Props {
  load: () => Promise<DeleteInfo>;
  onDelete: (permanentOK: boolean) => Promise<void>;
  onProgress: (cb: (p: DeleteProgress) => void) => Unsubscribe;
  onClose: () => void;
}

export function DeleteSeriesDialog({ load, onDelete, onProgress, onClose }: Props) {
  const { t } = useI18n();
  const [info, setInfo] = useState<DeleteInfo | null>(null);
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState<DeleteProgress | null>(null);
  const [error, setError] = useState('');
  const cancel = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    let live = true;
    load()
      .then((i) => live && setInfo(i))
      .catch((e: unknown) => live && setError(errorMessage(e, t)));
    cancel.current?.focus();
    const stop = onProgress((p) => setProgress(p));

    return () => {
      live = false;
      stop();
    };
    // once, when the dialog opens: what goes is what's in the folder then
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const close = () => {
    if (!busy) {
      onClose();
    }
  };
  const run = () => {
    setBusy(true);
    setError('');
    onDelete(info?.permanent === true)
      .then(() => onClose())
      .catch((e: unknown) => {
        setError(errorMessage(e, t));
        setBusy(false);
      });
  };

  return (
    <div
      className="browser-alert-backdrop"
      onClick={(e) => {
        if (e.target === e.currentTarget) {
          close();
        }
      }}
    >
      <section
        className="browser-alert delete-series"
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="delete-series-title"
        aria-describedby="delete-series-text"
        onKeyDown={(e) => {
          if (e.key === 'Escape') {
            close();
          }
        }}
      >
        <h2 id="delete-series-title">
          <AlertIcon size={18} /> {t('chapters.delete.title')}
        </h2>
        {info && (
          <p id="delete-series-text">
            {info.whole
              ? t('chapters.delete.bodyWhole', { title: info.title, folder: info.dir, count: info.files, size: info.size })
              : t('chapters.delete.bodyFiles', { title: info.title, folder: info.dir, count: info.files, size: info.size })}
          </p>
        )}
        {info?.permanent && <p className="error-text">{t('chapters.delete.permanentLine')}</p>}
        {busy && progress && (
          <p className="delete-progress" role="status">
            {t('chapters.delete.progress', { done: progress.done, total: progress.total })}
          </p>
        )}
        {error !== '' && (
          <p className="update-error" role="alert">
            {error}
          </p>
        )}
        <div className="browser-alert-actions">
          <button type="button" className="btn ghost" ref={cancel} onClick={close} disabled={busy}>
            {t('common.cancel')}
          </button>
          <button type="button" className="btn danger-outline" onClick={run} disabled={busy || info === null}>
            {busy
              ? t('chapters.delete.deleting')
              : info?.permanent
                ? t('chapters.delete.confirmPermanent')
                : t('chapters.delete.confirm')}
          </button>
        </div>
      </section>
    </div>
  );
}
