import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { LogDrawer } from './LogPanel';
import { canMove, clampSize, Splitter, wheelPixels, wheelTarget } from './panes';

// A stand-in for the DOM the wheel walks: parents, classes, overflow and scroll sizes
class El {
  parentElement: El | null = null;
  kids: El[] = [];
  scrollTop = 0;
  constructor(
    public cls: string,
    public overflowY = 'visible',
    public clientHeight = 100,
    public scrollHeight = 100,
  ) {}
  add(...kids: El[]): this {
    for (const k of kids) {
      k.parentElement = this;
      this.kids.push(k);
    }
    return this;
  }
  // the selectors panes.tsx asks for are lists of classes, with .activity.multi the one compound
  querySelectorAll(sel: string): El[] {
    const wants = sel.split(',').map((s) => s.trim().split('.').filter(Boolean));
    const out: El[] = [];
    const walk = (e: El) => {
      for (const k of e.kids) {
        const has = k.cls.split(' ');
        if (wants.some((w) => w.every((c) => has.includes(c)))) out.push(k);
        walk(k);
      }
    };
    walk(this);
    return out;
  }
}

beforeEach(() => {
  vi.stubGlobal('HTMLElement', El);
  vi.stubGlobal('getComputedStyle', (e: El) => ({ overflowY: e.overflowY }));
});
afterEach(() => {
  vi.unstubAllGlobals();
});

const target = (from: El, pane: El, dy: number) =>
  wheelTarget(from as unknown as Element, pane as unknown as Element, dy) as unknown as El | null;

describe('clampSize', () => {
  it('keeps a size within its limits, rounded', () => {
    expect(clampSize(400.4, 280, 600)).toBe(400);
    expect(clampSize(100, 280, 600)).toBe(280);
    expect(clampSize(900, 280, 600)).toBe(600);
  });

  it('gives the minimum when there is no room for it', () => {
    expect(clampSize(400, 280, 200)).toBe(280);
  });
});

describe('canMove', () => {
  it('tells whether a scroller has room either way', () => {
    const el = new El('list', 'auto', 100, 300) as unknown as HTMLElement;
    expect(canMove(el, 10)).toBe(true);
    expect(canMove(el, -10)).toBe(false);
    el.scrollTop = 200;
    expect(canMove(el, 10)).toBe(false);
    expect(canMove(el, -10)).toBe(true);
  });
});

describe('wheelTarget', () => {
  it('leaves a scroller under the pointer that can move to the webview', () => {
    const list = new El('list', 'auto', 100, 1000);
    const row = new El('row');
    const pane = new El('panel').add(new El('series-head'), list.add(row));
    expect(target(row, pane, 50)).toBeNull();
  });

  it("sends the wheel over a header or toolbar to the pane's list", () => {
    const head = new El('series-head');
    const list = new El('list', 'auto', 100, 1000);
    const pane = new El('panel').add(head, list);
    expect(target(head, pane, 50)).toBe(list);
    // the list at the top has nowhere to go up
    expect(target(head, pane, -50)).toBeNull();
  });

  it("sends the wheel over a run's summary to its job list, and past a hidden tab", () => {
    const hidden = new El('side-body', 'auto', 0, 0);
    const summary = new El('summary');
    const jobs = new El('jobs', 'auto', 80, 600);
    const body = new El('side-body', 'auto', 300, 300).add(new El('activity').add(summary, jobs));
    const dock = new El('dock');
    const pane = new El('side').add(hidden, body, dock);
    expect(target(summary, pane, 50)).toBe(jobs);
    expect(target(dock, pane, 50)).toBe(jobs);
  });

  it('hands the wheel from a list at its end to the scroller around it', () => {
    const jobs = new El('jobs', 'auto', 100, 300);
    jobs.scrollTop = 200;
    const row = new El('job');
    const multi = new El('activity multi', 'auto', 300, 900).add(jobs.add(row));
    const pane = new El('side').add(new El('side-body', 'auto', 300, 300).add(multi));
    expect(target(row, pane, 50)).toBe(multi);
    // up, the list itself still has room
    expect(target(row, pane, -50)).toBeNull();
  });

  it('does nothing when nothing in the pane can move', () => {
    const head = new El('log-head');
    const pane = new El('log-drawer').add(head, new El('log-lines', 'auto', 200, 150));
    expect(target(head, pane, 50)).toBeNull();
  });
});

describe('wheelPixels', () => {
  it('turns lines and pages into pixels', () => {
    expect(wheelPixels({ deltaY: 120, deltaMode: 0 }, 400)).toBe(120);
    expect(wheelPixels({ deltaY: 3, deltaMode: 1 }, 400)).toBe(48);
    expect(wheelPixels({ deltaY: -1, deltaMode: 2 }, 400)).toBe(-400);
  });
});

describe('Splitter', () => {
  it('is a focusable separator named for what it resizes', () => {
    const html = renderToStaticMarkup(
      createElement(Splitter, {
        orientation: 'vertical',
        label: 'Resize the side panel',
        className: 'splitter',
        size: () => 340,
        limits: (): [number, number] => [280, 600],
        onResize: () => undefined,
        onCommit: () => undefined,
      }),
    );
    expect(html).toContain('role="separator"');
    expect(html).toContain('aria-orientation="vertical"');
    expect(html).toContain('aria-label="Resize the side panel"');
    expect(html).toContain('tabindex="0"');
  });

  it('puts an edge on the log drawer that sets its height', () => {
    const html = renderToStaticMarkup(
      createElement(LogDrawer, {
        log: [],
        onClose: () => undefined,
        onClear: () => undefined,
        height: 300,
        limits: (): [number, number] => [120, 600],
        onResize: () => undefined,
        onCommit: () => undefined,
      }),
    );
    expect(html).toContain('class="log-resize"');
    expect(html).toContain('aria-label="Resize the log"');
    expect(html).toContain('--log-height:300px');
  });
});
