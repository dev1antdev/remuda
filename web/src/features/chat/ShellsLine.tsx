// The shells the agent started in the background that still run, in a line over the composer, as its terminal's
// footer counts them: each by what it is for and how long it has run, a click opening the file its output goes to.

import { useNow } from "@/lib/clock";
import { took } from "@/lib/time";
import { Icon } from "@/ui";
import { runningShells, runningWords, shellLabel, shellSeconds, type Shell } from "./shells";
import { t } from "@/lib/i18n";

export function ShellsLine({ shells, onOpen }: { shells: Shell[]; onOpen?: (shell: Shell) => void }) {
  const running = runningShells(shells);
  if (running.length === 0) return null;
  return (
    <div className="chat-shells" role="status">
      <Icon name="square-terminal" size={14} />
      <span className="chat-shells-count">{runningWords(running.length)}</span>
      {running.map((shell) => (
        <RunningShell key={shell.toolId} shell={shell} onOpen={onOpen} />
      ))}
    </div>
  );
}

function RunningShell({ shell, onOpen }: { shell: Shell; onOpen?: (shell: Shell) => void }) {
  const now = useNow(1000);
  const seconds = shellSeconds(shell, now);
  const words = (
    <>
      <span className="chat-shell-name">{shellLabel(shell)}</span>
      {seconds !== null && <span className="chat-shell-took">{took(seconds)}</span>}
    </>
  );
  if (!onOpen || !shell.output) {
    return (
      <span className="chat-shell" title={shell.command}>
        {words}
      </span>
    );
  }
  return (
    <button type="button" className="linkish chat-shell" title={`${shell.command}\n${t("Open the file its output goes to")}`} onClick={() => onOpen(shell)}>
      {words}
    </button>
  );
}
