import React from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { useTheme } from './hooks/useTheme';
import { ToastProvider } from './hooks/useToast';
import { AuthProvider } from './hooks/useAuth';
import { Toast } from './components/common/Toast';
import { StubPage } from './components/common/StubPage';
import { AppLayout } from './layouts/AppLayout';
import { LandingPage } from './pages/LandingPage';
import { AuthPage } from './pages/AuthPage';
import { DashboardPage } from './pages/DashboardPage';
import { NewRepoPage } from './pages/NewRepoPage';
import { CodePage } from './pages/CodePage';
import { BlobPage } from './pages/BlobPage';
import { CommitsPage } from './pages/CommitsPage';
import { IssuesPage } from './pages/IssuesPage';
import { PullsPage } from './pages/PullsPage';
import { DiffPage } from './pages/DiffPage';
import { SearchPage } from './pages/SearchPage';
import { LogoutPage } from './pages/LogoutPage';
import { PRDetailPage } from './pages/PRDetailPage';
import { ProfilePage } from './pages/ProfilePage';
import { SettingsPage } from './pages/SettingsPage';
import './styles/index.css';

// Routing note: this is a BrowserRouter (real URL paths). Deep links and refreshes
// work because `vara serve --hub` serves index.html for any unknown, non-API path
// (internal/server/static.go), and that fallback is registered on the least
// specific "GET /" so it never shadows the /_vara/... API. Repositories are keyed
// by name only — owner namespaces are a v0.5 concern (RFC-0019 §14).
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
          <Routes>
            <Route path="/login" element={<AuthPage initialMode="login" />} />
            <Route path="/signup" element={<AuthPage initialMode="signup" />} />
            <Route path="/logout" element={<LogoutPage />} />

            <Route element={<AppLayout theme={theme} toggleTheme={toggleTheme} />}>
              <Route path="/" element={<LandingPage />} />
              <Route path="/dashboard" element={<DashboardPage />} />
              <Route path="/new" element={<NewRepoPage />} />
              <Route path="/profile" element={<ProfilePage />} />

              {/* Repository views wrapped directly in AppLayout */}
              <Route path="/r/:repo">
                <Route index element={<CodePage />} />
                <Route path="tree/*" element={<CodePage />} />
                <Route path="blob/*" element={<BlobPage />} />
                <Route path="commits" element={<CommitsPage />} />
                <Route path="commit/:id" element={<DiffPage />} />
                <Route path="search" element={<SearchPage />} />

                {/* v0.5 un-implemented stubs */}
                <Route path="pulls" element={<PullsPage />} />
                <Route path="pulls/:num" element={<PRDetailPage />} />
                <Route path="issues" element={<IssuesPage />} />
              </Route>

              {/* v0.5 collaboration stubs — designed, not yet backed */}
              <Route
                path="/explore"
                element={<StubPage title="Explore" icon="projects" blurb="Discover public repositories across the hub. Discovery lands once organizations and public visibility ship in v0.5." />}
              />
              <Route
                path="/notifications"
                element={<StubPage title="Notifications" icon="status-smile" blurb="Activity on repositories you follow. Notifications are part of the v0.5 collaboration layer." />}
              />
              {/* /profile is served by the real ProfilePage above (whoami + real
                  repos/contributions); no stub duplicate. */}
              <Route path="/settings" element={<SettingsPage />} />
            </Route>

            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
          <Toast />
        </BrowserRouter>
        </AuthProvider>
      </ToastProvider>
    </div>
  );
};
export default App;
