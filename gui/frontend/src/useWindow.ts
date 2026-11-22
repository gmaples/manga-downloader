import { useEffect, useRef, useState } from 'react';
import { errorText } from './format';
import type { Api } from './api';
import type { Chapter, ChapterMark, LogLine, State } from './types';

export interface WindowData {
  state: State | null;
  chapters: Chapter[];
  /** Sorted indices of the selected chapters */
  selected: number[];
  /** Each chapter's mark, by index (see ChapterMark) */
  marks: ChapterMark[];
  /** What each chapter's mark can say more, by index (see Api.markNotes) */
  markNotes: string[];
  log: LogLine[];
}

/**
 * Keeps the window's data current: it subscribes before asking for snapshots,
 * and a snapshot never overwrites a pushed value that arrived first. `initial`
 * is a State the caller already read.
 */
export function useWindow(api: Api, initial?: State): WindowData & { failure: string | null } {
  const [data, setData] = useState<WindowData>({
    state: initial ?? null,
    chapters: [],
    selected: [],
    marks: [],
    markNotes: [],
    log: [],
  });
  // the error's text; the window words it in the language shown when it renders
  const [failure, setFailure] = useState<string | null>(null);
  // which fields a push has already filled, so a late snapshot doesn't clobber them
  const pushed = useRef({ state: false, chapters: false, selected: false, marks: false, markNotes: false, log: false });

  // a newer State from the caller: it saw pushes that came before this window subscribed,
  // and stops mattering once the window's own subscription has delivered one
  useEffect(() => {
    if (initial === undefined || pushed.current.state) {
      return;
    }
    setData((d) => (d.state === initial ? d : { ...d, state: initial }));
  }, [initial]);

  useEffect(() => {
    let alive = true;
    const seen = pushed.current;

    const offs = [
      api.onState((state) => {
        seen.state = true;
        setData((d) => ({ ...d, state }));
      }),
      api.onChapters((chapters) => {
        seen.chapters = true;
        setData((d) => ({ ...d, chapters }));
      }),
      api.onSelection((selected) => {
        seen.selected = true;
        setData((d) => ({ ...d, selected }));
      }),
      api.onMarks((marks) => {
        seen.marks = true;
        setData((d) => ({ ...d, marks }));
      }),
      ...(api.onMarkNotes
        ? [
            api.onMarkNotes((markNotes) => {
              seen.markNotes = true;
              setData((d) => ({ ...d, markNotes }));
            }),
          ]
        : []),
      api.onLog((log) => {
        seen.log = true;
        setData((d) => ({ ...d, log }));
      }),
    ];

    Promise.all([
      initial ?? api.state(),
      api.chapters(),
      api.selection(),
      api.marks(),
      api.log(),
      api.markNotes ? api.markNotes() : Promise.resolve<string[]>([]),
    ])
      .then(([state, chapters, selected, marks, log, markNotes]) => {
        if (!alive) {
          return;
        }
        setData((d) => ({
          state: seen.state ? d.state : state,
          chapters: seen.chapters ? d.chapters : chapters,
          selected: seen.selected ? d.selected : selected,
          marks: seen.marks ? d.marks : marks,
          markNotes: seen.markNotes ? d.markNotes : markNotes,
          log: seen.log ? d.log : log,
        }));
      })
      .catch((e: unknown) => {
        // without a snapshot the window has nothing to show: say so instead of "Loading…"
        if (alive) {
          setFailure(errorText(e));
        }
      });

    return () => {
      alive = false;
      for (const off of offs) {
        off();
      }
    };
  }, [api]);

  return { ...data, failure };
}
