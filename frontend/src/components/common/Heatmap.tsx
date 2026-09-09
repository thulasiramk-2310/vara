import React from 'react';
import { WeekContribution } from '../../types';

interface HeatmapProps {
  weeks: WeekContribution[];
  showLegend?: boolean;
}

export const Heatmap: React.FC<HeatmapProps> = ({ weeks, showLegend = true }) => {
  const months = ['Aug', 'Sep', 'Oct', 'Nov', 'Dec', 'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul'];

  // Contribution shades using adaptive theme variables
  const getShade = (color: string, count: number) => {
    if (count === 0 || color === 'var(--heat0)') return 'var(--heat0)';
    if (count <= 2 || color === 'var(--heat1)') return 'var(--heat1)';
    if (count <= 5 || color === 'var(--heat2)') return 'var(--heat2)';
    if (count <= 9 || color === 'var(--heat3)') return 'var(--heat3)';
    return 'var(--heat4)';
  };

  return (
    <div style={{ display: 'grid', gap: '8px', minWidth: 'max-content' }}>
      {/* Month Header Strip */}
      <div style={{ display: 'flex', paddingLeft: '32px', fontSize: '11.5px', color: 'var(--dim)', justifyContent: 'space-between', paddingRight: '12px', fontWeight: 550 }}>
        {months.map((m, idx) => (
          <span key={idx}>{m}</span>
        ))}
      </div>

      {/* Grid with Day Labels */}
      <div style={{ display: 'flex', gap: '8px' }}>
        {/* Day labels column */}
        <div style={{ display: 'grid', gridTemplateRows: 'repeat(7, 12px)', gap: '3px', fontSize: '11px', color: 'var(--dim)', paddingRight: '4px', lineHeight: '12px', fontWeight: 550 }}>
          <span></span>
          <span>Mon</span>
          <span></span>
          <span>Wed</span>
          <span></span>
          <span>Fri</span>
          <span></span>
        </div>

        {/* 52 Weeks Grid */}
        <div style={{ display: 'flex', gap: '3px', overflow: 'hidden' }}>
          {weeks.map((week, wIndex) => (
            <div key={wIndex} style={{ display: 'grid', gridTemplateRows: 'repeat(7, 12px)', gap: '3px' }}>
              {week.days.map((day, dIndex) => {
                const bg = getShade(day.color, day.count);
                return (
                  <span
                    key={dIndex}
                    title={`${day.count} contributions on ${['Sun','Mon','Tue','Wed','Thu','Fri','Sat'][dIndex]}`}
                    style={{
                      width: '12px',
                      height: '12px',
                      borderRadius: '2px',
                      background: bg,
                      transition: 'transform .1s ease, outline .1s ease',
                      cursor: 'pointer'
                    }}
                    onMouseEnter={(e) => { e.currentTarget.style.transform = 'scale(1.3)'; e.currentTarget.style.outline = '1px solid #fff'; e.currentTarget.style.zIndex = '5'; }}
                    onMouseLeave={(e) => { e.currentTarget.style.transform = 'none'; e.currentTarget.style.outline = 'none'; e.currentTarget.style.zIndex = 'auto'; }}
                  />
                );
              })}
            </div>
          ))}
        </div>
      </div>

      {/* Legend Footer */}
      {showLegend && (
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            marginTop: '12px',
            paddingTop: '12px',
            borderTop: '1px solid var(--border)',
            fontSize: '12.5px',
            color: 'var(--dim)',
          }}
        >
          <a href="#how" onClick={e => e.preventDefault()} style={{ color: 'var(--dim)', textDecoration: 'none' }} onMouseEnter={e => e.currentTarget.style.color = 'var(--accent)'} onMouseLeave={e => e.currentTarget.style.color = 'var(--dim)'}>
            Learn how we count contributions
          </a>
          
          <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
            <span>Less</span>
            <span style={{ width: '12px', height: '12px', borderRadius: '2px', background: 'var(--heat0)', display: 'inline-block' }} />
            <span style={{ width: '12px', height: '12px', borderRadius: '2px', background: 'var(--heat1)', display: 'inline-block' }} />
            <span style={{ width: '12px', height: '12px', borderRadius: '2px', background: 'var(--heat2)', display: 'inline-block' }} />
            <span style={{ width: '12px', height: '12px', borderRadius: '2px', background: 'var(--heat3)', display: 'inline-block' }} />
            <span style={{ width: '12px', height: '12px', borderRadius: '2px', background: 'var(--heat4)', display: 'inline-block' }} />
            <span>More</span>
          </div>
        </div>
      )}
    </div>
  );
};
