import React, { Suspense, lazy, useEffect } from 'react';
import { BrowserRouter, Routes, Route, Navigate, useLocation } from 'react-router-dom';
import { useTheme } from './hooks/useTheme';
import { ToastProvider } from './hooks/useToast';
import { AuthProvider } from './hooks/useAuth';
import { Toast } from './components/common/Toast';
import { StubPage } from './components/common/StubPage';
import { ErrorBoundary } from './components/common/ErrorBoundary';
import { RequireAuth } from './components/common/RequireAuth';
import { PageSkeleton } from './components/common/Skeleton';
import { AppLayout } from './layouts/AppLayout';
import { LandingPage } from './pages/LandingPage';
import './styles/index.css';

// Every page except the landing page and the layout is loaded on demand, so a
// visitor downloads only the code for the page they open. `page` adapts the
// named exports the pages already have to what React.lazy expects.
const page = <K extends string>(load: () => Promise<Record<K, React.ComponentType<any>>>, name: K) =>
  lazy(() => load().then((m) => ({ default: m[name] })));

const AuthPage = page(() => import('./pages/AuthPage'), 'AuthPage');
const LogoutPage = page(() => import('./pages/LogoutPage'), 'LogoutPage');
const DashboardPage = page(() => import('./pages/DashboardPage'), 'DashboardPage');
const NewRepoPage = page(() => import('./pages/NewRepoPage'), 'NewRepoPage');
const ProfilePage = page(() => import('./pages/ProfilePage'), 'ProfilePage');
const SettingsPage = page(() => import('./pages/SettingsPage'), 'SettingsPage');
const CodePage = page(() => import('./pages/CodePage'), 'CodePage');
const BlobPage = page(() => import('./pages/BlobPage'), 'BlobPage');
const CommitsPage = page(() => import('./pages/CommitsPage'), 'CommitsPage');
const DiffPage = page(() => import('./pages/DiffPage'), 'DiffPage');
const SearchPage = page(() => import('./pages/SearchPage'), 'SearchPage');
const PullsPage = page(() => import('./pages/PullsPage'), 'PullsPage');
const PRDetailPage = page(() => import('./pages/PRDetailPage'), 'PRDetailPage');
const IssuesPage = page(() => import('./pages/IssuesPage'), 'IssuesPage');
const NotFoundPage = page(() => import('./pages/NotFoundPage'), 'NotFoundPage');

// A new page starts at the top. A link to a place inside a page (#L12) is left
// to the browser, which scrolls to it.
const ScrollToTop: React.FC = () => {
  const { pathname, hash } = useLocation();
  useEffect(() => {
    if (!hash) window.scrollTo(0, 0);
  }, [pathname, hash]);
  return null;
};

// Routes holds the route table inside the router so the error boundary can reset
// itself when the address changes.
const AppRoutes: React.FC<{ theme: string; toggleTheme: () => void }> = ({ theme, toggleTheme }) => {
  const { pathname } = useLocation();
  return (
    <ErrorBoundary resetKey={pathname}>
      <Suspense fallback={<PageSkeleton />}>
        <Routes>
          <Route path="/login" element={<AuthPage />} />
          <Route path="/signup" element={<Navigate to="/login" replace />} />
          <Route path="/logout" element={<LogoutPage />} />

          <Route element={<AppLayout theme={theme} toggleTheme={toggleTheme} />}>
            <Route path="/" element={<LandingPage />} />
            <Route path="/dashboard" element={<DashboardPage />} />
            <Route path="/new" element={<RequireAuth><NewRepoPage /></RequireAuth>} />
            <Route path="/profile" element={<ProfilePage />} />
            <Route path="/settings" element={<RequireAuth><SettingsPage /></RequireAuth>} />

            {/* Repositories are addressed by name only; owner namespaces are v0.5. */}
            <Route path="/r/:repo">
              <Route index element={<CodePage />} />
              <Route path="tree/*" element={<CodePage />} />
              <Route path="blob/*" element={<BlobPage />} />
              <Route path="commits" element={<CommitsPage />} />
              <Route path="commit/:id" element={<DiffPage />} />
              <Route path="search" element={<SearchPage />} />

              {/* v0.5: designed, not yet backed by the server */}
              <Route path="pulls" element={<PullsPage />} />
              <Route path="pulls/:num" element={<PRDetailPage />} />
              <Route path="issues" element={<IssuesPage />} />
            </Route>

            <Route
              path="/explore"
              element={<StubPage title="Explore" icon="projects" blurb="Discover public repositories across the hub. Discovery lands once organizations and public visibility ship in v0.5." />}
            />
            <Route
              path="/notifications"
              element={<StubPage title="Notifications" icon="status-smile" blurb="Activity on repositories you follow. Notifications are part of the v0.5 collaboration layer." />}
            />

            {/* Anything else is a real "not found", not a silent redirect home. */}
            <Route path="*" element={<NotFoundPage />} />
          </Route>
        </Routes>
      </Suspense>
    </ErrorBoundary>
  );
};

// Routing note: this is a BrowserRouter (real URL paths). Deep links and
// refreshes work because `vara serve --hub` serves index.html for any unknown
// path outside /assets/ (internal/server/static.go), registered on the least
// specific "GET /" so it never shadows the /_vara/... API.
export const App: React.FC = () => {
  const { theme, toggleTheme } = useTheme();

  return (
    <div
      className="app"
      data-theme={theme}
      style={{
        minHeight: '100vh',
        background: 'var(--bg)',
        color: 'var(--text)',
        fontFamily: 'var(--font-sans)',
        fontSize: '14px',
        lineHeight: 1.5,
        WebkitFontSmoothing: 'antialiased',
        MozOsxFontSmoothing: 'grayscale',
      }}
    >
      <ToastProvider>
        <AuthProvider>
          <BrowserRouter>
            <ScrollToTop />
            <AppRoutes theme={theme} toggleTheme={toggleTheme} />
            <Toast />
          </BrowserRouter>
        </AuthProvider>
      </ToastProvider>
    </div>
  );
};
export default App;
