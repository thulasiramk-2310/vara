import { TreeEntry } from '../api/client';

// sortEntries lists folders first, then files, each alphabetically.
export function sortEntries(entries: readonly TreeEntry[]): TreeEntry[] {
  return entries.slice().sort((a, b) => {
    const ad = a.type === 'dir';
    const bd = b.type === 'dir';
    if (ad !== bd) return ad ? -1 : 1;
    return a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: 'base' });
  });
}

// joinPath joins a folder and a name without producing a leading slash.
export const joinPath = (dir: string, name: string): string => (dir ? `${dir}/${name}` : name);
