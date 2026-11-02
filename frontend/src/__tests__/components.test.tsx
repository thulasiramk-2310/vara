import React from 'react';
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest';
import { render, screen, cleanup, fireEvent, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import axe from 'axe-core';
import { ErrorBoundary } from '../components/common/ErrorBoundary';
import { Loading, ErrorState, Empty } from '../components/common/States';
import { NotFoundPage } from '../pages/NotFoundPage';
import { LandingPage } from '../pages/LandingPage';
import { RepoGrid, PAGE_SIZE } from '../components/common/RepoGrid';
import { ToastProvider } from '../hooks/useToast';
import { ApiError } from '../api/client';

// The card makes network requests of its own; these tests are about the grid.
vi.mock('../components/common/RepoCard', () => ({
  RepoCard: ({ repo }: { repo: { name: string } }) => <span data-testid="card">{repo.name}</span>,
}));

const apiError = (status: number, code?: string, requestId?: string, message = 'boom'): ApiError =>
  Object.assign(new Error(message), { status, code, requestId }) as ApiError;

const inRouter = (ui: React.ReactElement, at = '/') => <MemoryRouter initialEntries={[at]}>{ui}</MemoryRouter>;

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

// jsdom cannot compute colours or page-level layout, so those two checks are left
// to the real-browser run; everything else axe knows is enforced here.
async function violations(container: HTMLElement) {
  const result = await axe.run(container, { rules: { 'color-contrast': { enabled: false }, region: { enabled: false } } });
  return result.violations.map((v) => `${v.id}: ${v.help} (${v.nodes.length})`);
}

describe('ErrorBoundary', () => {
  const Bomb: React.FC<{ boom: boolean }> = ({ boom }) => {
    if (boom) throw new Error('render failure');
    return <p>fine</p>;
  };

  it('shows a way out instead of a blank page, and recovers when the address changes', () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const { rerender } = render(
      <ErrorBoundary resetKey="/a">
        <Bomb boom />
      </ErrorBoundary>,
    );
    expect(screen.getByRole('alert').textContent).toContain('This page failed to display');
    expect(screen.getByRole('button', { name: 'Reload' })).toBeTruthy();
    rerender(
      <ErrorBoundary resetKey="/b">
        <Bomb boom={false} />
      </ErrorBoundary>,
    );
    expect(screen.getByText('fine')).toBeTruthy();
  });
});

describe('Loading', () => {
  it('announces at once but paints skeletons only after a delay', () => {
    vi.useFakeTimers();
    const { container } = render(<Loading label="Loading files" />);
    expect(screen.getByRole('status').textContent).toBe('Loading files');
    expect(container.querySelectorAll('.v-skel').length).toBe(0);
    act(() => {
      vi.advanceTimersByTime(250);
    });
    expect(container.querySelectorAll('.v-skel').length).toBeGreaterThan(0);
  });
  it('reserves the space so the page does not jump', () => {
    const { container } = render(<Loading rows={5} />);
    expect((container.firstChild as HTMLElement).style.minHeight).toBe('220px');
  });
});

describe('ErrorState', () => {
  it('offers sign-in for 401 and keeps the page the visitor wanted', () => {
    render(inRouter(<ErrorState error={apiError(401)} />, '/r/demo?ref=x'));
    const link = screen.getByRole('link', { name: 'Sign in' });
    expect(link.getAttribute('href')).toBe('/login?next=%2Fr%2Fdemo%3Fref%3Dx');
  });
  it('offers retry for server failures and shows the request id', async () => {
    const retry = vi.fn();
    render(inRouter(<ErrorState error={apiError(503, 'UNAVAILABLE_UPSTREAM', 'abc123def456')} onRetry={retry} />));
    expect(screen.getByRole('alert').textContent).toContain('request abc123def456');
    await userEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(retry).toHaveBeenCalledOnce();
  });
  it('says plainly when the network is down', () => {
    render(inRouter(<ErrorState error={apiError(0, 'NETWORK')} />));
    expect(screen.getByText('Cannot reach the server')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Try again' })).toBeTruthy();
  });
  it('does not offer retry when retrying cannot help', () => {
    for (const status of [403, 404]) {
      cleanup();
      render(inRouter(<ErrorState error={apiError(status)} />));
      expect(screen.queryByRole('button')).toBeNull();
    }
  });
  it('explains a malformed server response', () => {
    render(inRouter(<ErrorState error={apiError(0, 'BAD_RESPONSE')} />));
    expect(screen.getByText('Unexpected response')).toBeTruthy();
  });
});

describe('RepoGrid', () => {
  const repos = Array.from({ length: 60 }, (_, i) => ({ name: `proj-${String(i + 1).padStart(2, '0')}`, visibility: 'public', state: 'active' }));

  it('draws one page, then more on request', async () => {
    render(inRouter(<RepoGrid repos={repos} />));
    expect(screen.getAllByTestId('card')).toHaveLength(PAGE_SIZE);
    expect(screen.getByText(/Showing 24 of 60/)).toBeTruthy();
    await userEvent.click(screen.getByRole('button', { name: 'Show more' }));
    expect(screen.getAllByTestId('card')).toHaveLength(48);
  });
  it('filters, says when nothing matches, and clears', async () => {
    render(inRouter(<RepoGrid repos={repos} />));
    const box = screen.getByRole('searchbox', { name: 'Filter repositories' });
    await userEvent.type(box, 'proj-5');
    expect(screen.getAllByTestId('card').map((c) => c.textContent)).toEqual(['proj-50', 'proj-51', 'proj-52', 'proj-53', 'proj-54', 'proj-55', 'proj-56', 'proj-57', 'proj-58', 'proj-59']);
    await userEvent.clear(box);
    await userEvent.type(box, 'zzz');
    expect(screen.getByText(/Nothing matches/)).toBeTruthy();
    await userEvent.click(screen.getByRole('button', { name: 'Clear the filter' }));
    expect(screen.getAllByTestId('card')).toHaveLength(PAGE_SIZE);
  });
  it('sorts Z to A', async () => {
    render(inRouter(<RepoGrid repos={repos} />));
    await userEvent.selectOptions(screen.getByRole('combobox', { name: 'Sort repositories' }), 'name-desc');
    expect(screen.getAllByTestId('card')[0].textContent).toBe('proj-60');
  });
  it('explains an empty server', () => {
    render(inRouter(<RepoGrid repos={[]} />));
    expect(screen.getByText(/No repositories yet/)).toBeTruthy();
  });
  it('survives 5,000 repositories without drawing them all', () => {
    const many = Array.from({ length: 5000 }, (_, i) => ({ name: `r${i}`, visibility: 'public', state: 'active' }));
    render(inRouter(<RepoGrid repos={many} />));
    expect(screen.getAllByTestId('card')).toHaveLength(PAGE_SIZE);
  });
});

describe('accessibility (axe)', () => {
  it('404 page', async () => {
    const { container } = render(inRouter(<NotFoundPage />, '/nowhere'));
    expect(await violations(container)).toEqual([]);
  });
  it('landing page', async () => {
    const { container } = render(
      inRouter(
        <ToastProvider>
          <LandingPage />
        </ToastProvider>,
      ),
    );
    expect(await violations(container)).toEqual([]);
  });
  it('error and empty states', async () => {
    const { container } = render(
      inRouter(
        <>
          <ErrorState error={apiError(401)} />
          <ErrorState error={apiError(503)} />
          <Empty>Nothing here.</Empty>
        </>,
      ),
    );
    expect(await violations(container)).toEqual([]);
  });
  it('repository grid controls', async () => {
    const { container } = render(inRouter(<RepoGrid repos={Array.from({ length: 5 }, (_, i) => ({ name: `r${i}`, visibility: 'public', state: 'active' }))} />));
    expect(await violations(container)).toEqual([]);
  });
});

describe('page title', () => {
  beforeEach(() => {
    document.title = '';
  });
  it('is set by the 404 page', () => {
    render(inRouter(<NotFoundPage />));
    expect(document.title).toBe('Page not found · VARA Hub');
    fireEvent.focus(document.body);
  });
});
