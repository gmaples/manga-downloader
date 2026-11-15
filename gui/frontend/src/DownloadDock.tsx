// The foot of the side panel, always in view: the one big action. Download
// (with the reason it can't start, if any), which starts another group while
// others run, and over it, while any group runs, their overall progress and
// Cancel for all of them.

import { useEffect, useMemo, useRef, useState } from 'react';
import type { T as Translator } from './i18n';
import { T, useI18n } from './i18n/react';
import { DownloadIcon, Spinner, XIcon } from './icons';
import { downloadBlocker, downloadLabel, overallProgress, runningProgress } from './run';
import type { Chapter, Options, PackAs, SiteBar, State } from './types';

interface Props {
  state: State;
  /** The backend's refusal of the last Download or Cancel */
  error: string;
  /** The loaded chapters and the selected indices, for the Download label's volumes */
  chapters: Chapter[];
  selected: number[];
  onStart: () => void;
  onCancel: () => void;
}

export function DownloadDock({ state, error, chapters, selected, onStart, onCancel }: Props) {
  const { t } = useI18n();
  // Cancel goes when the last group ends, so its focus moves to Download
  // rather than falling to the body
  const focused = useRef<'start' | 'cancel' | null>(null);
  const startButton = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (!state.running && focused.current === 'cancel') {
      if (startButton.current?.disabled) {
        // nothing to download: focus the chapter list instead of the body
        document.querySelector<HTMLElement>('[role="listbox"]')?.focus();
      } else {
        startButton.current?.focus();
      }
    }
  }, [state.running]);

  // the label regroups the chapters, so it's kept across the progress ticks
  const label = useMemo(
    () => downloadLabel(state, chapters, selected, t),
    [state.selectedCount, state.addTo, state.fresh, state.options.packAs, state.volumes.text, chapters, selected, t],
  );

  const blocker = downloadBlocker(state, t);
  const o = state.options;
  // the series folder when that option is on, so the folder shown is the one written to
  const dir = state.destination || o.outputDir;
  return (
    <div className={`dock${state.running ? ' running' : ''}`}>
      {state.running && <Running state={state} focused={focused} onCancel={onCancel} />}
      <p className="dock-dest" title={dir}>
        <T
          k="download.where"
          values={{ what: destination(o, t), dir: dir || '…' }}
          components={{
            dir: (children) => (
              <span className="mono">
                <bdi>{children}</bdi>
              </span>
            ),
          }}
        />
      </p>
      <button
        type="button"
        ref={startButton}
        className="btn primary block big"
        disabled={blocker !== ''}
        aria-describedby={blocker !== '' ? 'dock-blocker' : undefined}
        onFocus={() => (focused.current = 'start')}
        onBlur={() => (focused.current = null)}
        onClick={onStart}
      >
        <DownloadIcon /> {label}
      </button>
      {error !== '' ? (
        <p className="error-text small" role="alert">
          {t('download.failed', { error })}
        </p>
      ) : (
        blocker !== '' && (
          <p className="hint center" id="dock-blocker">
            {blocker}
          </p>
        )
      )}
    </div>
  );
}

/** The running groups' progress together, and Cancel for all of them */
function Running({
  state,
  focused,
  onCancel,
}: {
  state: State;
  focused: React.MutableRefObject<'start' | 'cancel' | null>;
  onCancel: () => void;
}) {
  const { t } = useI18n();
  const running = state.groups.filter((g) => g.running);
  // one group says what it's doing, as a single run always has; several, how many there are
  const one = running.length === 1 ? running[0] : null;
  const overall = one ? overallProgress(one) : runningProgress(state.groups);
  const pct = Math.round(overall.fraction * 100);
  return (
    <div className="dock-progress">
      {state.sites.length > 0 && <SiteBars sites={state.sites} />}
      <div className="dock-line">
        <span className="dock-title">
          <Spinner />
          {state.cancelling
            ? t('download.progress.finishing')
            : one === null
              ? t('download.progress.groups', { count: running.length, done: overall.finished, total: overall.total })
              : one.run.bundle
                ? t('download.progress.bundle')
                : one.run.volumes
                  ? t('download.progress.files', { done: overall.finished, total: overall.total })
                  : t('download.progress.chapters', { done: overall.finished, total: overall.total })}
        </span>
        <span className="dock-pct">{pct}%</span>
      </div>
      <div
        className="bar big"
        role="progressbar"
        aria-label={t('download.progress.label')}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={pct}
      >
        <span style={{ transform: `scaleX(${overall.fraction})` }} />
      </div>
      {state.cancelling && <p className="hint">{t('download.progress.cancelHint')}</p>}
      {/* aria-disabled, not disabled: a disabled button drops its focus, and Cancel's focus must survive to the run's end */}
      <button
        type="button"
        className="btn small danger-outline"
        aria-disabled={state.cancelling || undefined}
        onFocus={() => (focused.current = 'cancel')}
        onBlur={() => (focused.current = null)}
        onClick={() => {
          if (!state.cancelling) {
            onCancel();
          }
        }}
      >
        <XIcon size={13} />{' '}
        {state.cancelling ? t('download.cancelling') : one === null ? t('download.cancelAll') : t('common.cancel')}
      </button>
    </div>
  );
}

/** The time now, ticking every second while on, for the pauses' countdowns */
function useNow(on: boolean): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!on) {
      return;
    }
    setNow(Date.now());
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [on]);
  return now;
}

/** A bar for each site downloading: the chapter it's on, or its pause after a 429 */
function SiteBars({ sites }: { sites: SiteBar[] }) {
  const { t } = useI18n();
  const now = useNow(sites.some((s) => s.until > 0));
  return (
    <ul className="site-bars" aria-label={t('download.site.list')}>
      {sites.map((s) => {
        const waiting = s.until > now;
        const fraction = s.total > 0 ? Math.min(1, s.done / s.total) : 0;
        const pct = Math.round(fraction * 100);
        const status = waiting
          ? t('download.site.waiting', { seconds: Math.ceil((s.until - now) / 1000) })
          : s.title === ''
            ? t('download.site.next')
            : s.total === 0
              ? t('download.site.fetching')
              : `${pct}%`;
        return (
          <li key={s.site} className={`site-bar${waiting ? ' waiting' : ''}`}>
            <div className="dock-line">
              <span className="site-name" title={s.title === '' ? s.site : `${s.site} · ${s.title}${s.tags}`}>
                <strong>{s.site}</strong>
                {s.title !== '' && (
                  <span className="site-chapter">
                    {' · '}
                    {s.title}
                    {s.tags}
                  </span>
                )}
              </span>
              <span className="site-status">{status}</span>
            </div>
            <div
              className="bar"
              role="progressbar"
              aria-label={t('download.site.label', { site: s.site })}
              aria-valuemin={0}
              aria-valuemax={100}
              aria-valuenow={waiting ? undefined : pct}
            >
              <span style={{ transform: `scaleX(${waiting ? 0 : fraction})` }} />
            </div>
          </li>
        );
      })}
    </ul>
  );
}

/** What the run writes, by format and Pack as, as the dock's first words */
function destinations(t: Translator): Record<Options['format'], Record<PackAs, string>> {
  const cbz = t('download.dest.cbzFiles');
  const cbr = t('download.dest.cbrFiles');
  const epub = t('download.dest.epubFiles');
  const raw = t('download.dest.rawFiles');
  return {
    cbz: { chapters: cbz, parts: cbz, volumes: t('download.dest.cbzPerVolume'), bundle: t('download.dest.cbzBundle') },
    cbr: { chapters: cbr, parts: cbr, volumes: t('download.dest.cbrPerVolume'), bundle: t('download.dest.cbrBundle') },
    epub: {
      chapters: epub,
      parts: epub,
      volumes: t('download.dest.epubPerVolume'),
      bundle: t('download.dest.epubBundle'),
    },
    raw: {
      chapters: raw,
      parts: raw,
      volumes: t('download.dest.rawPerVolume'),
      bundle: t('download.dest.rawBundle'),
    },
  };
}

/** What the run writes, as the dock's first words */
function destination(o: Options, t: Translator): string {
  const byPack = destinations(t)[o.format];
  return byPack?.[o.packAs] ?? byPack?.chapters ?? t('download.dest.files');
}
