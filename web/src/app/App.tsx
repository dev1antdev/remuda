// The page: the roster read once, and the shell drawn from it and the app's own state. A render error anywhere
// below shows what broke and a Reload, never a blank page.

import { RosterProvider } from "@/features/roster/useRoster";
import { ErrorBoundary, Notice } from "@/ui";
import Shell from "./Shell";

export default function App() {
  return (
    <ErrorBoundary
      fallback={(message) => (
        <div className="placeholder">
          <Notice tone="error">The page hit an error: {message}</Notice>
          <button type="button" className="btn primary" onClick={() => location.reload()}>
            Reload
          </button>
        </div>
      )}
    >
      <RosterProvider>
        <Shell />
      </RosterProvider>
    </ErrorBoundary>
  );
}
