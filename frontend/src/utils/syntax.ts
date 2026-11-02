// Syntax highlighting for the file viewer.
//
// The highlighter (lowlight) returns a tree of plain nodes, never an HTML string.
// This file flattens that tree into lines of {text, class} tokens that React draws
// as ordinary text nodes and <span>s, so repository content can never inject
// markup. The library and its grammars are loaded on demand, only when someone
// opens a file, and stay out of the main bundle.

export interface Token {
  text: string;
  cls?: string; // an hljs-* class naming the kind of token, if any
}
export type HighlightedLines = Token[][];

// Highlighting a very large file would freeze the page, so past these limits the
// file is shown as plain text.
export const MAX_HIGHLIGHT_CHARS = 300_000;
export const MAX_HIGHLIGHT_LINES = 8_000;

const BY_EXTENSION: Record<string, string> = {
  go: 'go', js: 'javascript', mjs: 'javascript', cjs: 'javascript', jsx: 'javascript',
  ts: 'typescript', tsx: 'typescript', py: 'python', rs: 'rust', c: 'c', h: 'c',
  cc: 'cpp', cpp: 'cpp', cxx: 'cpp', hpp: 'cpp', java: 'java', kt: 'kotlin', swift: 'swift',
  cs: 'csharp', rb: 'ruby', php: 'php', lua: 'lua', pl: 'perl', r: 'r', sql: 'sql',
  sh: 'bash', bash: 'bash', zsh: 'bash', json: 'json', yml: 'yaml', yaml: 'yaml',
  toml: 'ini', ini: 'ini', css: 'css', scss: 'scss', less: 'less',
  html: 'xml', htm: 'xml', xml: 'xml', svg: 'xml', md: 'markdown', markdown: 'markdown',
  diff: 'diff', patch: 'diff', graphql: 'graphql', wasm: 'wasm',
};

const BY_NAME: Record<string, string> = { makefile: 'makefile', gnumakefile: 'makefile' };

// languageFor picks a grammar from the file name, or returns undefined when the
// file is not one we know (it is then shown as plain text).
export function languageFor(path: string): string | undefined {
  const name = (path.split('/').pop() || '').toLowerCase();
  if (BY_NAME[name]) return BY_NAME[name];
  const dot = name.lastIndexOf('.');
  if (dot <= 0) return undefined;
  return BY_EXTENSION[name.slice(dot + 1)];
}

interface HNode {
  type: string;
  value?: string;
  properties?: { className?: string[] };
  children?: HNode[];
}

// flatten walks the highlighter's tree and cuts it into lines. A token that spans
// a line break (a block comment, a template string) is split so each line keeps
// its own colour.
export function flatten(root: HNode): HighlightedLines {
  const lines: HighlightedLines = [[]];
  const walk = (node: HNode, cls: string | undefined) => {
    if (node.type === 'text') {
      const parts = String(node.value ?? '').split('\n');
      parts.forEach((part, i) => {
        if (i > 0) lines.push([]);
        if (part) lines[lines.length - 1].push({ text: part, cls });
      });
      return;
    }
    const own = node.properties?.className?.find((c) => c.startsWith('hljs-'));
    (node.children ?? []).forEach((child) => walk(child, own ?? cls));
  };
  walk(root, undefined);
  return lines;
}

// highlightLines returns the file as coloured tokens per line, or null when it
// should be shown plain (unknown language, too large, or the grammar failed).
export async function highlightLines(code: string, language: string | undefined): Promise<HighlightedLines | null> {
  if (!language || code.length > MAX_HIGHLIGHT_CHARS) return null;
  try {
    const { lowlight: low } = await import('./languages');
    if (!low.registered(language)) return null;
    const lines = flatten(low.highlight(language, code) as unknown as HNode);
    return lines.length > MAX_HIGHLIGHT_LINES ? null : lines;
  } catch (err) {
    // Highlighting is decoration. If it fails the file is still readable.
    console.error('syntax highlighting failed:', err);
    return null;
  }
}
