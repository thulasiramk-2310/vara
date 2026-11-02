import React from 'react';

// ErrorBoundary catches a crash while rendering a page, so one broken view shows
// a message and a way out instead of a blank screen. It does not catch errors in
// event handlers or in async code; those are handled where they happen.
interface Props {
  children: React.ReactNode;
  // Changing this value (the current URL) clears a caught error, so moving to a
  // different page recovers without a reload.
  resetKey?: string;
}

interface State {
  failed: boolean;
}

export class ErrorBoundary extends React.Component<Props, State> {
  state: State = { failed: false };

  static getDerivedStateFromError(): State {
    return { failed: true };
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    // The console is the only sink: the Hub sends nothing to any third party.
    console.error('Hub page crashed:', error, info.componentStack);
  }

  componentDidUpdate(prev: Props) {
    if (this.state.failed && prev.resetKey !== this.props.resetKey) {
      this.setState({ failed: false });
    }
  }

  render() {
    if (!this.state.failed) return this.props.children;
    return (
      <div role="alert" style={{ maxWidth: '560px', margin: 'var(--s-8) auto', padding: '0 var(--s-4)' }}>
        <h1 style={{ fontSize: '22px', fontWeight: 650, margin: '0 0 var(--s-2)' }}>This page failed to display</h1>
        <p style={{ color: 'var(--dim)', margin: '0 0 var(--s-5)' }}>
          Something in the page broke while drawing it. Your data is not affected. Reload to try again, or go back to your
          repositories.
        </p>
        <div style={{ display: 'flex', gap: 'var(--s-3)', flexWrap: 'wrap' }}>
          <button type="button" className="v-btn v-btn-primary" onClick={() => window.location.reload()}>
            Reload
          </button>
          <a href="/dashboard" className="v-btn">
            Repositories
          </a>
        </div>
      </div>
    );
  }
}
