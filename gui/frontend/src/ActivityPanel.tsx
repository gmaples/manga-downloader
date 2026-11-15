// What the downloads are doing and how they ended: one section per group (each
// press of Download), in the order they started, under a header with the
// series and its own Cancel. Each has its summary once it ended, then one row
// per chapter (or the single bundle job). Rows are windowed like the chapter
// list, since "select all" on a long series makes thousands of them.

import { say } from './i18n';
import { useI18n } from './i18n/react';
import { jobFraction, jobShortStatus, jobStatus, summaryHeadline, summarySentence } from './format';
import { AlertIcon, CheckIcon, ErrorIcon, FolderIcon, PackageIcon, SkipIcon, Spinner, XIcon } from './icons';
import { overallProgress, summaryTone } from './run';
import type { Group, Job, State, Summary } from './types';
import { useVirtualList } from './virtual';

const JOB_HEIGHT = 50;
/** With several groups, the rows each one shows before its list scrolls */
const GROUP_ROWS = 6;

interface Props {
  state: State;
  /** Opens the folder group id saved into */
  onOpenFolder: (id: number) => void;
  onCancelGroup: (id: number) => void;
}

export function ActivityPanel({ state, onOpenFolder, onCancelGroup }: Props) {
  const { t } = useI18n();

  if (state.groups.length === 0) {
    return (
      <div className="activity empty-activity">
        <p className="muted">{t('activity.emptyTitle')}</p>
        <p className="muted small">{t('activity.emptyHint')}</p>
      </div>
    );
  }

  // one group fills the panel as a single run always has; several share it,
  // each with a list of its own height, and the panel scrolls
  const multi = state.groups.length > 1;
  return (
    <div className={`activity${multi ? ' multi' : ''}`}>
      {state.groups.map((g) => (
        <GroupSection
          key={g.id}
          group={g}
          multi={multi}
          onOpenFolder={() => onOpenFolder(g.id)}
          onCancel={() => onCancelGroup(g.id)}
        />
      ))}
    </div>
  );
}

function GroupSection({
  group,
  multi,
  onOpenFolder,
  onCancel,
}: {
  group: Group;
  multi: boolean;
  onOpenFolder: () => void;
  onCancel: () => void;
}) {
  const { t } = useI18n();
  return (
    <section className="run-group" aria-label={group.title}>
      <div className="run-group-head">
        <h3 className="run-group-title" title={group.title}>
          {group.title}
        </h3>
        {group.running && (
          // aria-disabled, not disabled: a disabled button drops its focus
          <button
            type="button"
            className="btn small danger-outline"
            aria-label={t('activity.cancelGroup', { title: group.title })}
            aria-disabled={group.cancelling || undefined}
            onClick={() => {
              if (!group.cancelling) {
                onCancel();
              }
            }}
          >
            <XIcon size={13} /> {group.cancelling ? t('download.cancelling') : t('common.cancel')}
          </button>
        )}
      </div>
      {group.summary && <SummaryCard summary={group.summary} onOpenFolder={onOpenFolder} />}
      {group.jobs.length > 0 &&
        (group.run.bundle ? (
          <BundleCard job={group.jobs[0]} chapters={group.run.chapters} />
        ) : (
          <JobList group={group} multi={multi} />
        ))}
    </section>
  );
}

function SummaryCard({ summary, onOpenFolder }: { summary: Summary; onOpenFolder: () => void }) {
  const { t } = useI18n();
  const tone = summaryTone(summary);
  const ToneIcon =
    tone === 'success' ? CheckIcon : tone === 'neutral' ? XIcon : tone === 'warning' ? AlertIcon : ErrorIcon;

  return (
    <section className={`summary tone-${tone}`} aria-label={t('activity.lastDownload')}>
      <div className="summary-head">
        <span className="summary-icon">
          <ToneIcon size={16} />
        </span>
        <h3>{summaryHeadline(summary, t)}</h3>
      </div>
      <p className="summary-text">{summarySentence(summary, t)}</p>
      {summary.error !== '' && <p className="error-text small">{say(t, summary.errorMsg, summary.error)}</p>}
      {summary.skipped > 0 && (
        <p className="muted small">
          {t('activity.skippedNote', { count: summary.skipped })}
        </p>
      )}
      {summary.existing > 0 && (
        <p className="muted small">
          {t('activity.existingNote', { count: summary.existing })}
        </p>
      )}
      {summary.failures.length > 0 && (
        <ul className="failures" aria-label={t('activity.failedChapters')}>
          {summary.failures.map((f, i) => (
            <li key={i}>
              {f.label !== '' && <strong>{f.label}</strong>}
              <span>{say(t, f.reasonMsg, f.reason)}</span>
            </li>
          ))}
        </ul>
      )}
      <button type="button" className="btn small" onClick={onOpenFolder}>
        <FolderIcon size={14} /> {t('activity.openFolder')}
      </button>
    </section>
  );
}

/** A bundle is one job; its phase text is the story, so it gets room to show it */
function BundleCard({ job, chapters }: { job: Job; chapters: number }) {
  const { t, lang } = useI18n();
  const fraction = jobFraction(job);
  const moving = job.state === 'downloading' || job.state === 'archiving';
  return (
    <section className={`bundle-card job-${job.state}`} aria-label={t('activity.bundle')}>
      <div className="job-line">
        <span className="job-title">{say(t, job.titleMsg, job.title)}</span>
        <span className="job-status">{t('count.chapter', { count: chapters })}</span>
      </div>
      <p className="bundle-phase">
        <JobIcon state={job.state} />
        <span>{moving ? jobShortStatus(job, t, lang) : jobStatus(job, t)}</span>
      </p>
      {moving && (
        <div
          className="bar"
          role="progressbar"
          aria-label={t('activity.progressBundle')}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={Math.round(fraction * 100)}
        >
          <span style={{ transform: `scaleX(${fraction})` }} />
        </div>
      )}
    </section>
  );
}

function JobList({ group, multi }: { group: Group; multi: boolean }) {
  const { t } = useI18n();
  const overall = overallProgress(group);
  const queued = group.running ? overall.queued : 0;
  const rows = group.jobs.length + (queued > 0 ? 1 : 0);
  const list = useVirtualList(rows, JOB_HEIGHT, 4);
  const { first, last } = list.span;

  return (
    <section className="jobs-section" aria-label={t('activity.chaptersInRun')}>
      <h3 className="jobs-title">
        {group.running ? t('activity.thisRun') : t('activity.lastRun')}
        <span className="muted">
          {' '}
          {t('activity.progress', { finished: overall.finished, total: overall.total })}
        </span>
      </h3>
      <div
        className="jobs"
        ref={list.ref}
        onScroll={list.onScroll}
        role="list"
        style={multi ? { height: Math.min(rows, GROUP_ROWS) * JOB_HEIGHT } : undefined}
      >
        <div className="list-inner" style={{ height: rows * JOB_HEIGHT }}>
          {Array.from({ length: last - first }, (_, i) => {
            const pos = first + i;
            const style = { transform: `translateY(${pos * JOB_HEIGHT}px)`, height: JOB_HEIGHT };
            const job = group.jobs[pos];
            if (job === undefined) {
              return (
                <div key="queued" role="listitem" className="job job-queued" style={style}>
                  <span className="job-icon" aria-hidden="true" />
                  <span className="job-title muted">{t('activity.queued', { count: queued })}</span>
                </div>
              );
            }
            return <JobRow key={pos} job={job} style={style} />;
          })}
        </div>
      </div>
    </section>
  );
}

function JobRow({ job, style }: { job: Job; style: React.CSSProperties }) {
  const { t, lang } = useI18n();
  const fraction = jobFraction(job);
  const status = jobStatus(job, t);
  const title = say(t, job.titleMsg, job.title);

  return (
    <div role="listitem" className={`job job-${job.state}`} style={style} title={status}>
      <span className="job-icon" aria-hidden="true">
        <JobIcon state={job.state} />
      </span>
      <div className="job-body">
        <div className="job-line">
          <span className="job-title">{title}</span>
          {job.tags && <span className="job-tags">{job.tags}</span>}
          <span className="job-status">{jobShortStatus(job, t, lang)}</span>
        </div>
        {job.state === 'failed' || job.state === 'skipped' ? (
          <span className="job-reason">{say(t, job.reasonMsg, job.reason)}</span>
        ) : (
          <div
            className="bar"
            role="progressbar"
            aria-label={t('activity.progressOf', { title, tags: job.tags })}
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={Math.round(fraction * 100)}
          >
            <span style={{ transform: `scaleX(${fraction})` }} />
          </div>
        )}
      </div>
    </div>
  );
}

function JobIcon({ state }: { state: Job['state'] }) {
  switch (state) {
    case 'fetching':
    case 'downloading':
      return <Spinner size={14} />;
    case 'archiving':
      return <PackageIcon size={15} />;
    case 'done':
      return <CheckIcon size={15} />;
    case 'failed':
      return <ErrorIcon size={15} />;
    case 'skipped':
      return <SkipIcon size={15} />;
  }
}
