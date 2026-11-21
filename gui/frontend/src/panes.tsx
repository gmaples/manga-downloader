// Pane sizes and the scroll wheel.
//
// The side panel's width and the log drawer's height follow a drag on a
// Splitter (or its arrow keys), within limits that keep the other panes
// usable; the size the user leaves is saved as a pref (sideWidth, logHeight).
//
// The wheel scrolls a pane wherever the pointer is over it: over a part that
// can't scroll itself (a header, a toolbar, a summary, the dock), it goes to
// the pane's first scroller that can still move that way, and a list at its
// end hands it to the scroller around it.

import { useRef } from 'react';

/** The narrowest the side panel gets */
export const SIDE_MIN = 280;
/** The room the chapter list keeps next to the side panel, splitter included */
export const CHAPTERS_MIN = 372;
/** The shortest the log drawer gets */
export const LOG_MIN = 120;
/** The room kept above the log drawer, for the source bar */
export const LOG_ROOM = 160;
/** How far an arrow key moves a splitter */
export const KEY_STEP = 16;

/** px rounded and kept within min and max; min wins when there's no room for both */
export function clampSize(px: number, min: number, max: number): number {
  return Math.round(Math.max(min, Math.min(px, max)));
}

interface SplitterProps {
  /** vertical: a bar between two columns, dragged sideways; horizontal: an edge dragged up and down */
  orientation: 'vertical' | 'horizontal';
  label: string;
  className: string;
  /** The pane's size now, in CSS pixels */
  size: () => number;
  /** The smallest and largest size the pane may take now */
  limits: () => [number, number];
  /** Each size while dragging */
  onResize: (px: number) => void;
  /** The size the user left it at: the end of a drag, or an arrow key */
  onCommit: (px: number) => void;
}

/**
 * A drag handle for a pane that sits right of it (vertical) or below it
 * (horizontal), so moving the pointer left or up makes the pane bigger.
 */
export function Splitter({ orientation, label, className, size, limits, onResize, onCommit }: SplitterProps) {
  const drag = useRef<{ from: number; start: number; last: number } | null>(null);
  const pos = (e: React.PointerEvent) => (orientation === 'vertical' ? e.clientX : e.clientY);
  const fit = (px: number) => {
    const [min, max] = limits();
    return clampSize(px, min, max);
  };

  return (
    <div
      role="separator"
      aria-orientation={orientation}
      aria-label={label}
      tabIndex={0}
      className={className}
      onPointerDown={(e) => {
        if (e.button !== 0) return;
        e.preventDefault();
        e.currentTarget.setPointerCapture?.(e.pointerId);
        const start = size();
        drag.current = { from: pos(e), start, last: start };
        document.body.classList.add(orientation === 'vertical' ? 'resizing-x' : 'resizing-y');
      }}
      onPointerMove={(e) => {
        const d = drag.current;
        if (d === null) return;
        const px = fit(d.start - (pos(e) - d.from));
        if (px !== d.last) {
          d.last = px;
          onResize(px);
        }
      }}
      onPointerUp={(e) => {
        const d = drag.current;
        if (d === null) return;
        drag.current = null;
        e.currentTarget.releasePointerCapture?.(e.pointerId);
        document.body.classList.remove('resizing-x', 'resizing-y');
        if (d.last !== d.start) onCommit(d.last);
      }}
      onPointerCancel={() => {
        drag.current = null;
        document.body.classList.remove('resizing-x', 'resizing-y');
      }}
      onKeyDown={(e) => {
        const grow = orientation === 'vertical' ? 'ArrowLeft' : 'ArrowUp';
        const shrink = orientation === 'vertical' ? 'ArrowRight' : 'ArrowDown';
        if (e.key !== grow && e.key !== shrink) return;
        e.preventDefault();
        const px = fit(size() + (e.key === grow ? KEY_STEP : -KEY_STEP));
        onResize(px);
        onCommit(px);
      }}
    />
  );
}

/** The panes the wheel is kept inside of */
const PANES = '.panel, .side, .log-drawer';
/** The scrollers a pane hands the wheel to, in order */
const SCROLLERS = '.list, .side-body, .activity.multi, .jobs, .log-lines';

function scrolls(el: Element): el is HTMLElement {
  if (!(el instanceof HTMLElement)) return false;
  const oy = getComputedStyle(el).overflowY;
  return oy === 'auto' || oy === 'scroll';
}

/** Whether el can still scroll the way dy goes */
export function canMove(el: HTMLElement, dy: number): boolean {
  return dy > 0 ? el.scrollTop + el.clientHeight < el.scrollHeight - 1 : el.scrollTop > 0;
}

/**
 * The element a wheel over from should scroll by hand, or null to leave the
 * wheel to the webview: when the scroller nearest to from can move itself, or
 * nothing in the pane can.
 */
export function wheelTarget(from: Element | null, pane: Element, dy: number): HTMLElement | null {
  let nearest = true;
  for (let el = from; el !== null; el = el.parentElement) {
    if (scrolls(el)) {
      if (canMove(el, dy)) return nearest ? null : el;
      nearest = false;
    }
    if (el === pane) break;
  }
  for (const el of pane.querySelectorAll<HTMLElement>(SCROLLERS)) {
    if (scrolls(el) && canMove(el, dy)) return el;
  }
  return null;
}

/** A wheel event's vertical distance in pixels, whatever unit it came in */
export function wheelPixels(e: Pick<WheelEvent, 'deltaY' | 'deltaMode'>, page: number): number {
  if (e.deltaMode === 1) return e.deltaY * 16;
  if (e.deltaMode === 2) return e.deltaY * page;
  return e.deltaY;
}

/** The window's wheel listener (non-passive, so it can take the wheel over) */
export function forwardWheel(e: WheelEvent): void {
  // Ctrl+wheel zooms; a sideways wheel is the webview's
  if (e.defaultPrevented || e.ctrlKey || e.deltaY === 0 || !(e.target instanceof Element)) return;
  const pane = e.target.closest(PANES);
  if (pane === null) return;
  const to = wheelTarget(e.target, pane, e.deltaY);
  if (to === null) return;
  e.preventDefault();
  to.scrollTop += wheelPixels(e, to.clientHeight);
}
