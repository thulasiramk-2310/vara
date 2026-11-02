import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App';
import { ErrorBoundary } from './components/common/ErrorBoundary';
import './styles/index.css';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    {/* Outside the router, so it also catches a crash in the router itself. */}
    <ErrorBoundary>
      <App />
    </ErrorBoundary>
  </React.StrictMode>
);
