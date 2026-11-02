import { useEffect } from 'react';

const SITE = 'VARA Hub';

// usePageTitle sets the browser tab title for the current page, most specific
// part first: "src/main.go · demo · VARA Hub". Pass nothing for the bare name.
export function usePageTitle(...parts: (string | undefined | false)[]) {
  const title = [...parts.filter(Boolean), SITE].join(' · ');
  useEffect(() => {
    document.title = title;
  }, [title]);
}
