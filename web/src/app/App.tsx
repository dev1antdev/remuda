// The page: the roster read once, and the shell drawn from it and the app's own state. A render error anywhere
// below shows what broke and a Reload, never a blank page; a stall of the main thread is noted with where the page was.

import { useEffect } from "react";
import { RosterProvider } from "@/features/roster/useRoster";
import { ErrorBoundary, Notice, TipProvider } from "@/ui";
import { startDiagnostics } from "./diag";
import Shell from "./Shell";
import { t } from "@/lib/i18n";

export default function App() {
  useEffect(startDiagnostics, []);
  return (
    <ErrorBoundary
      fallback={(message) => (
        <div className="placeholder">
          <Notice tone="error">The page hit an error: {message}</Notice>
          <button type="button" className="btn primary" onClick={() => location.reload()}>
            {t("Reload")}
          </button>
        </div>
      )}
    >
      <TipProvider>
        <RosterProvider>
          <Shell />
        </RosterProvider>
      </TipProvider>
    </ErrorBoundary>
  );
}
