// A series title the site rates 18+ shows as "[Adult Title]" until clicked.
// A click reveals that title for the rest of the session, everywhere it shows
// (the chapter list's header, the Activity pane, the history pop-up).

import { useSyncExternalStore } from 'react';
import { useI18n } from './i18n/react';

const revealed = new Set<string>();
const listeners = new Set<() => void>();
let version = 0;

/** Shows title everywhere from now on */
export function reveal(title: string): void {
  revealed.add(title);
  version++;
  for (const fn of listeners) fn();
}

/** Forgets every reveal; for tests */
export function resetReveals(): void {
  revealed.clear();
  version++;
  for (const fn of listeners) fn();
}

function subscribe(fn: () => void): () => void {
  listeners.add(fn);
  return () => {
    listeners.delete(fn);
  };
}

/** Whether title can show: it isn't adult, or it was revealed */
export function useShown(title: string, adult: boolean): boolean {
  useSyncExternalStore(subscribe, () => version, () => version);
  return !adult || revealed.has(title);
}

/** text with title replaced by the mask while it's hidden (file and folder names carry it too) */
export function useMask(title: string, adult: boolean): (text: string) => string {
  const { t } = useI18n();
  const shown = useShown(title, adult);
  return (text) => (shown || title === '' ? text : text.split(title).join(t('common.adultTitle')));
}

/** The title, or a button that reveals it while it's masked */
export function MaskedTitle({ title, adult }: { title: string; adult: boolean }) {
  const { t } = useI18n();
  if (useShown(title, adult)) {
    return <>{title}</>;
  }
  return (
    <button
      type="button"
      className="adult-mask"
      title={t('common.adultReveal')}
      aria-label={t('common.adultReveal')}
      onClick={(e) => {
        // a reveal is all the click does, never the row's own action
        e.stopPropagation();
        e.preventDefault();
        reveal(title);
      }}
    >
      {t('common.adultTitle')}
    </button>
  );
}
