import React, { useState } from 'react';
import { Link } from 'react-router-dom';
import { ContribDay, ContribWeek, ActivityEntry, level } from '../../hooks/useContributions';
import { routes } from '../../routes';
import { short, when } from '../../api/client';
import { Identicon } from './Identicon';

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
const DAY_LABELS = ['', 'Mon', '', 'Wed', '', 'Fri', '']; // rows Sun..Sat
const CELL = 13;
const GAP = 3;
const DAYW = 26; // width of the left day-label column

interface YearGrid {
  weeks: ContribWeek[];
  total: number;
  monthLabels: string[]; // per-week: month name at the week that holds the 1st, else ''
}

const WEEKS = 53; // always a full GitHub-style grid: 53 columns × 7 = 371 cells

// buildYear turns the raw per-day counts into a fixed 53-week grid (Sunday-aligned
// to the week containing Jan 1). EVERY cell renders at level 0..4 — missing/future
// dates are visible level-0 cells, never blanks — so the grid is always full width.
function buildYear(counts: Record<string, number>, year: number): YearGrid {
  const start = new Date(Date.UTC(year, 0, 1));
  start.setUTCDate(start.getUTCDate() - start.getUTCDay()); // Sunday on/before Jan 1

  const weeks: ContribWeek[] = [];
  let total = 0;
  const cur = new Date(start);
  for (let w = 0; w < WEEKS; w++) {
    const days: ContribDay[] = [];
    for (let i = 0; i < 7; i++) {
      const key = cur.toISOString().slice(0, 10);
      const count = counts[key] || 0;
      if (cur.getUTCFullYear() === year) total += count; // "this year" counts in-year only
      days.push({ date: key, count, level: level(count) });
      cur.setUTCDate(cur.getUTCDate() + 1);
    }
    weeks.push({ days });
  }

  const monthLabels = weeks.map((w) => {
    for (const d of w.days) {
      const dt = new Date(d.date + 'T00:00:00Z');
      if (dt.getUTCDate() === 1 && dt.getUTCFullYear() === year) return MONTHS[dt.getUTCMonth()];
    }
    return '';
  });

  return { weeks, total, monthLabels };
}

function fmtDate(iso: string): string {
  const d = new Date(iso + 'T00:00:00Z');
  return d.toLocaleDateString(undefined, { month: 'long', day: 'numeric', year: 'numeric', timeZone: 'UTC' });
}

interface Tip {
  count: number;
  date: string;
  x: number;
  y: number;
}

// ContributionGraph renders a GitHub-style heatmap from real per-day commit counts:
// month labels across the top, weekday labels down the side, a year selector on the
// right, styled hover tooltips, and an integrated legend. All data is real.
export const ContributionGraph: React.FC<{ counts: Record<string, number>; years: number[] }> = ({ counts, years }) => {
  const yrs = years.length ? years : [new Date().getUTCFullYear()];
  const [year, setYear] = useState(yrs[0]);
  const active = yrs.includes(year) ? year : yrs[0];
  const { weeks, total, monthLabels } = buildYear(counts, active);
  const [tip, setTip] = useState<Tip | null>(null);

  const onEnter = (e: React.MouseEvent, d: ContribDay) => {
    if (d.level < 0) return;
    const cell = e.currentTarget as HTMLElement;
    const host = cell.closest('[data-cg]') as HTMLElement;
    const r = cell.getBoundingClientRect();
    const h = host.getBoundingClientRect();
    setTip({ count: d.count, date: d.date, x: r.left - h.left + CELL / 2, y: r.top - h.top });
  };

  return (
    <div style={{ display: 'flex', gap: '16px', alignItems: 'flex-start', minWidth: 0, maxWidth: '100%' }}>
      {/* Graph card */}
      <div data-cg style={{ position: 'relative', flex: 1, minWidth: 0, background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '16px', padding: '20px 22px', boxShadow: 'var(--shadow)' }}>
        {/* Contribution count as the header (GitHub-style) */}
        <div style={{ marginBottom: '16px', fontSize: '15px', fontWeight: 600 }}>
          {total.toLocaleString()} contribution{total === 1 ? '' : 's'} in {active}
        </div>

        {/* Graph (scrolls internally only if the card is very narrow) */}
        <div style={{ maxWidth: '100%', overflowX: 'auto', paddingBottom: '2px' }}>
          {/* Month labels (clipped so the last label can't overflow into a scrollbar) */}
          <div style={{ display: 'flex', gap: `${GAP}px`, paddingLeft: `${DAYW + GAP}px`, marginBottom: '6px', height: '13px', overflow: 'hidden' }}>
            {monthLabels.map((m, i) => (
              <div key={i} style={{ width: `${CELL}px`, position: 'relative' }}>
                {m ? <span style={{ position: 'absolute', left: 0, top: 0, fontSize: '10px', color: 'var(--dim)', whiteSpace: 'nowrap' }}>{m}</span> : null}
              </div>
            ))}
          </div>
          {/* Weekday labels + cells */}
          <div style={{ display: 'flex', gap: `${GAP}px` }}>
            <div style={{ display: 'grid', gridTemplateRows: `repeat(7, ${CELL}px)`, gap: `${GAP}px`, width: `${DAYW}px` }}>
              {DAY_LABELS.map((d, i) => (
                <span key={i} style={{ fontSize: '9px', color: 'var(--faint)', lineHeight: `${CELL}px`, textAlign: 'right' }}>{d}</span>
              ))}
            </div>
            <div style={{ display: 'flex', gap: `${GAP}px` }}>
              {weeks.map((w, i) => (
                <div key={i} style={{ display: 'grid', gridTemplateRows: `repeat(7, ${CELL}px)`, gap: `${GAP}px` }}>
                  {w.days.map((d, j) => (
                    <span
                      key={j}
                      onMouseEnter={(e) => onEnter(e, d)}
                      onMouseLeave={() => setTip(null)}
                      style={{
                        width: `${CELL}px`,
                        height: `${CELL}px`,
                        borderRadius: '3px',
                        background: d.level < 0 ? 'transparent' : `var(--heat${d.level})`,
                        boxShadow: d.level < 0 ? 'none' : 'inset 0 0 0 1px rgba(128,128,128,0.10)',
                        cursor: d.level < 0 ? 'default' : 'pointer',
                        transition: 'background .3s ease, transform .1s ease',
                      }}
                    />
                  ))}
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* Footer: note + legend */}
        <div style={{ borderTop: '1px solid var(--border)', marginTop: '16px', paddingTop: '12px', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '10px', flexWrap: 'wrap' }}>
          <span style={{ fontSize: '12px', color: 'var(--faint)' }}>Measured from commits across your repositories.</span>
          <div style={{ display: 'flex', alignItems: 'center', gap: '5px', fontSize: '11px', color: 'var(--faint)' }}>
            Less
            {[0, 1, 2, 3, 4].map((l) => (
              <span key={l} style={{ width: `${CELL}px`, height: `${CELL}px`, borderRadius: '3px', background: `var(--heat${l})`, boxShadow: 'inset 0 0 0 1px rgba(128,128,128,0.10)' }} />
            ))}
            More
          </div>
        </div>

        {/* Hover tooltip (positioned within the card) */}
        {tip ? (
          <div
            style={{
              position: 'absolute',
              left: `${tip.x}px`,
              top: `${tip.y}px`,
              transform: 'translate(-50%, calc(-100% - 8px))',
              background: '#1b1712',
              color: '#f6f1ea',
              borderRadius: '8px',
              padding: '7px 11px',
              fontSize: '11.5px',
              lineHeight: 1.4,
              whiteSpace: 'nowrap',
              pointerEvents: 'none',
              zIndex: 20,
              boxShadow: '0 8px 22px rgba(0,0,0,0.30)',
            }}
          >
            <div style={{ fontWeight: 640 }}>{tip.count === 0 ? 'No commits' : `${tip.count} commit${tip.count === 1 ? '' : 's'}`}</div>
            <div style={{ opacity: 0.72 }}>{fmtDate(tip.date)}</div>
          </div>
        ) : null}
      </div>

      {/* Year selector — a column outside the card (GitHub-style) */}
      <div style={{ display: 'grid', gap: '2px', paddingTop: '2px' }}>
        {yrs.map((y) => (
          <button
            key={y}
            onClick={() => setYear(y)}
            style={{
              background: y === active ? 'var(--accent)' : 'transparent',
              color: y === active ? 'var(--accent-fg)' : 'var(--dim)',
              border: 'none',
              borderRadius: '8px',
              padding: '6px 18px',
              fontSize: '13px',
              fontWeight: y === active ? 600 : 500,
              cursor: 'pointer',
              textAlign: 'left',
              minWidth: '72px',
            }}
          >
            {y}
          </button>
        ))}
      </div>
    </div>
  );
};

// ActivityFeed renders recent commits across repositories as an activity stream —
// real data from the read API, each row linking to the commit's diff.
export const ActivityFeed: React.FC<{ items: ActivityEntry[] }> = ({ items }) => {
  if (!items.length) {
    return (
      <div style={{ color: 'var(--dim)', fontSize: '13px', background: 'var(--panel)', border: '1px dashed var(--border)', borderRadius: '12px', padding: '20px' }}>
        No recent activity yet. Commits you push to repositories on this server will appear here.
      </div>
    );
  }
  return (
    <div style={{ background: 'var(--panel)', border: '1px solid var(--border)', borderRadius: '12px', overflow: 'hidden', boxShadow: 'var(--shadow)' }}>
      {items.map((a, i) => (
        <Link
          key={`${a.repo}-${a.commit.id}`}
          to={routes.commit(a.repo, a.commit.id)}
          style={{ display: 'flex', alignItems: 'center', gap: '12px', padding: '12px 15px', borderTop: i === 0 ? 'none' : '1px solid var(--border)', textDecoration: 'none', color: 'var(--text)' }}
        >
          <Identicon seed={a.commit.author} size={26} />
          <span style={{ fontSize: '13px', color: 'var(--dim)', flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            committed <span style={{ color: 'var(--accent)', fontFamily: 'var(--font-mono)' }}>{short(a.commit.id)}</span> to{' '}
            <span style={{ color: 'var(--text)', fontWeight: 560, fontFamily: 'var(--font-mono)' }}>{a.repo}</span>
            {' — '}
            <span style={{ color: 'var(--text)' }}>{a.commit.message.split('\n')[0]}</span>
          </span>
          <span style={{ color: 'var(--faint)', fontSize: '12px', whiteSpace: 'nowrap' }}>{when(a.commit.timestamp)}</span>
        </Link>
      ))}
    </div>
  );
};
