import React from 'react';

// A deliberately small, SAFE Markdown renderer for README previews. It NEVER uses
// dangerouslySetInnerHTML — every piece of repository content is emitted as a React
// text node (auto-escaped) or a known element with sanitized attributes. It covers
// headings, paragraphs, lists, blockquotes, tables, code, bold, links and images;
// anything it doesn't recognize renders as plain text, which is the safe failure.

// safeHref only permits http(s), root-relative, and anchor links. Anything with a
// scheme like javascript:/data:/vbscript: is dropped (rendered as plain text).
function safeHref(url: string): string | null {
  const u = url.trim();
  if (/^(https?:\/\/|\/|#)/i.test(u)) return u;
  if (/^[a-z][a-z0-9+.-]*:/i.test(u)) return null;
  return u; // relative path
}

let keySeq = 0;
const k = () => `md${keySeq++}`;

function renderInline(text: string): React.ReactNode[] {
  const out: React.ReactNode[] = [];
  const parts = text.split(/(`[^`]+`)/g);
  for (const part of parts) {
    if (!part) continue;
    if (part.startsWith('`') && part.endsWith('`')) {
      out.push(
        <code key={k()} style={{ fontFamily: 'var(--font-mono)', fontSize: '0.88em', background: 'var(--inset)', border: '1px solid var(--border)', borderRadius: '5px', padding: '1px 5px' }}>
          {part.slice(1, -1)}
        </code>,
      );
      continue;
    }
    renderRich(part, out);
  }
  return out;
}

// Handles ![img](url), [link](url) and **bold** in a run of text.
function renderRich(text: string, out: React.ReactNode[]): void {
  const re = /(!?)\[([^\]]*)\]\(([^)]+)\)/g;
  let last = 0;
  let m: RegExpExecArray | null;
  while ((m = re.exec(text)) !== null) {
    if (m.index > last) renderBold(text.slice(last, m.index), out);
    const href = safeHref(m[3]);
    if (m[1] === '!') {
      // Image — strict CSP blocks external image loads, so show the alt caption
      // rather than a broken image icon (keeps it safe and clean).
      out.push(<em key={k()} style={{ color: 'var(--dim)' }}>{m[2] || 'image'}</em>);
    } else if (href) {
      out.push(
        <a key={k()} href={href} target="_blank" rel="noopener noreferrer nofollow" style={{ color: 'var(--accent)', textDecoration: 'none' }}>
          {m[2]}
        </a>,
      );
    } else {
      out.push(m[2]);
    }
    last = m.index + m[0].length;
  }
  if (last < text.length) renderBold(text.slice(last), out);
}

function renderBold(text: string, out: React.ReactNode[]): void {
  const parts = text.split(/(\*\*[^*]+\*\*)/g);
  for (const p of parts) {
    if (!p) continue;
    if (p.startsWith('**') && p.endsWith('**')) out.push(<strong key={k()}>{p.slice(2, -2)}</strong>);
    else out.push(p);
  }
}

const cells = (line: string): string[] =>
  line.replace(/^\s*\|/, '').replace(/\|\s*$/, '').split('|').map((c) => c.trim());

const isTableSep = (line: string): boolean => /^\s*\|?[\s:|-]*-[\s:|-]*\|?\s*$/.test(line) && line.includes('-');

export const Markdown: React.FC<{ source: string }> = ({ source }) => {
  const lines = source.replace(/\r\n/g, '\n').split('\n');
  const blocks: React.ReactNode[] = [];
  let i = 0;

  while (i < lines.length) {
    const line = lines[i];

    // Fenced code block
    if (line.trimStart().startsWith('```')) {
      const body: string[] = [];
      i++;
      while (i < lines.length && !lines[i].trimStart().startsWith('```')) body.push(lines[i++]);
      i++;
      blocks.push(
        <pre key={k()} style={{ background: 'var(--inset)', border: '1px solid var(--border)', borderRadius: '10px', padding: '14px 16px', overflowX: 'auto', margin: '0 0 14px' }}>
          <code style={{ fontFamily: 'var(--font-mono)', fontSize: '12.5px', lineHeight: 1.6 }}>{body.join('\n')}</code>
        </pre>,
      );
      continue;
    }

    // Table (header row + separator row)
    if (line.includes('|') && i + 1 < lines.length && isTableSep(lines[i + 1])) {
      const header = cells(line);
      i += 2;
      const rows: string[][] = [];
      while (i < lines.length && lines[i].includes('|') && lines[i].trim() !== '') rows.push(cells(lines[i++]));
      blocks.push(
        <div key={k()} style={{ overflowX: 'auto', margin: '0 0 14px' }}>
          <table style={{ borderCollapse: 'collapse', width: '100%', fontSize: '13px' }}>
            <thead>
              <tr>
                {header.map((h) => (
                  <th key={k()} style={{ textAlign: 'left', padding: '7px 12px', borderBottom: '2px solid var(--border)', fontWeight: 640, color: 'var(--text)' }}>{renderInline(h)}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={k()}>
                  {r.map((c) => (
                    <td key={k()} style={{ padding: '7px 12px', borderBottom: '1px solid var(--border)', color: 'var(--dim)' }}>{renderInline(c)}</td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>,
      );
      continue;
    }

    // Blockquote
    if (/^\s*>\s?/.test(line)) {
      const quote: string[] = [];
      while (i < lines.length && /^\s*>\s?/.test(lines[i])) quote.push(lines[i++].replace(/^\s*>\s?/, ''));
      blocks.push(
        <blockquote key={k()} style={{ margin: '0 0 14px', paddingLeft: '14px', borderLeft: '3px solid var(--border-strong)', color: 'var(--dim)', fontSize: '14px', lineHeight: 1.7 }}>
          {renderInline(quote.join(' '))}
        </blockquote>,
      );
      continue;
    }

    // Headings
    const h = /^(#{1,6})\s+(.*)$/.exec(line);
    if (h) {
      const depth = h[1].length;
      const size = [26, 21, 17, 15, 14, 13][depth - 1];
      blocks.push(
        <div key={k()} style={{ fontSize: `${size}px`, fontWeight: depth <= 2 ? 700 : 640, letterSpacing: depth <= 2 ? '-0.3px' : 0, margin: blocks.length ? '22px 0 10px' : '0 0 10px', paddingBottom: depth === 1 ? '8px' : 0, borderBottom: depth === 1 ? '1px solid var(--border)' : 'none' }}>
          {renderInline(h[2])}
        </div>,
      );
      i++;
      continue;
    }

    // Unordered list
    if (/^\s*[-*]\s+/.test(line)) {
      const items: string[] = [];
      while (i < lines.length && /^\s*[-*]\s+/.test(lines[i])) items.push(lines[i++].replace(/^\s*[-*]\s+/, ''));
      blocks.push(
        <ul key={k()} style={{ margin: '0 0 14px', paddingLeft: '22px', display: 'grid', gap: '4px' }}>
          {items.map((it) => (
            <li key={k()} style={{ fontSize: '14px', lineHeight: 1.6 }}>{renderInline(it)}</li>
          ))}
        </ul>,
      );
      continue;
    }

    // Blank line
    if (line.trim() === '') {
      i++;
      continue;
    }

    // Paragraph
    const para: string[] = [];
    while (
      i < lines.length &&
      lines[i].trim() !== '' &&
      !/^(#{1,6})\s+/.test(lines[i]) &&
      !/^\s*[-*]\s+/.test(lines[i]) &&
      !/^\s*>\s?/.test(lines[i]) &&
      !lines[i].trimStart().startsWith('```') &&
      !(lines[i].includes('|') && i + 1 < lines.length && isTableSep(lines[i + 1]))
    ) {
      para.push(lines[i++]);
    }
    blocks.push(
      <p key={k()} style={{ margin: '0 0 14px', fontSize: '14px', lineHeight: 1.7, color: 'var(--text)' }}>
        {renderInline(para.join(' '))}
      </p>,
    );
  }

  return <div style={{ color: 'var(--text)' }}>{blocks}</div>;
};
