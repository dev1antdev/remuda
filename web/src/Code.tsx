// The Code mode: a machine's code-server (VS Code in the browser) framed
// under remuda's own origin and token. Pick the machine and the folder —
// the workspaces herdr knows are offered — and the editor fills the screen.
// Once opened it stays mounted behind the other modes, so switching back
// does not reload the editor.

import { useEffect, useMemo, useRef, useState } from "react";
import { codeStatus, type CodeStatus, type Machine } from "./api";
import { basename, errMsg, usePref } from "./prefs";
import { connected, machineOptions } from "./options";
import Select from "./Select";
import Icon from "./icons";
import { openUrl } from "./host";

export default function Code({ machines, currentMachine, currentFolder, visible }: {
  machines: Machine[];
  currentMachine: string | null; // the pane in view, as a default
  currentFolder: string | null;
  visible: boolean;
}) {
  const live = connected(machines);
  const [machineId, setMachineId] = usePref("code.machine", "");
  const machine = live.find((m) => m.id === (machineId || currentMachine)) ?? live[0] ?? null;
  const [folders, setFolders] = usePref<Record<string, string>>("code.folders", {}); // last folder per machine
  const folderIcon = <Icon name="folder" size={14} />;

  // the folders on offer: every workspace's checkout (or its first pane's cwd)
  const options = useMemo(() => {
    if (!machine) return [];
    const seen = new Map<string, string>();
    for (const s of machine.sessions) {
      const folder = s.worktree?.checkout_path || s.tabs.flatMap((t) => t.panes).find((p) => p.cwd)?.cwd || "";
      if (folder && !seen.has(folder)) seen.set(folder, s.name);
    }
    return [...seen.entries()].map(([folder, name]) => ({ value: folder, label: name, hint: folder, icon: folderIcon }));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [machine]);

  const folder = machine ? (folders[machine.id] || (currentMachine === machine.id && currentFolder) || options[0]?.value || "") : "";
  const [status, setStatus] = useState<CodeStatus | null>(null);
  const [checking, setChecking] = useState(false);
  // a probe answers for the machine it was sent to: an answer that comes
  // back after the pick moved on (or the mode unmounted) is dropped
  const probeSeq = useRef(0);
  const probe = (force = false) => {
    if (!machine) return;
    const n = ++probeSeq.current;
    const mine = () => n === probeSeq.current;
    setChecking(true);
    codeStatus(machine.id, force)
      .then(
        (s) => mine() && setStatus(s),
        (e) => mine() && setStatus({ available: false, port: 0, error: errMsg(e) }),
      )
      .finally(() => mine() && setChecking(false));
  };
  useEffect(() => {
    setStatus(null);
    probe();
    return () => {
      probeSeq.current++;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [machine?.id]);
  // while it is down, look again now and then (an install in progress)
  useEffect(() => {
    if (!visible || status?.available !== false) return;
    const id = setInterval(() => probe(true), 20_000);
    return () => clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible, status?.available, machine?.id]);

  const src = machine && folder ? `/code/${encodeURIComponent(machine.id)}/?folder=${encodeURIComponent(folder)}` : machine ? `/code/${encodeURIComponent(machine.id)}/` : "";
  // the frame is keyed by machine + folder: a new pick is a fresh editor
  const frameKey = `${machine?.id ?? ""}|${folder}`;

  return (
    <div className="code-mode">
      <div className="code-bar">
        {live.length > 1 && (
          <Select value={machine?.id ?? ""} onChange={setMachineId} options={machineOptions(machines).map((o) => ({ ...o, hint: machines.find((m) => m.id === o.value)?.host }))} />
        )}
        {machine && (
          <Select
            className="code-folder"
            value={folder}
            placeholder="Pick a folder"
            onChange={(f) => setFolders({ ...folders, [machine.id]: f })}
            options={options.some((o) => o.value === folder) || !folder ? options : [{ value: folder, label: basename(folder), hint: folder, icon: folderIcon }, ...options]}
          />
        )}
        <span className="code-bar-spacer" />
        {status?.available && (
          <button type="button" className="icon-btn" title="Open the editor in its own tab" onClick={() => openUrl(src)}>
            <Icon name="external-link" size={14} />
          </button>
        )}
      </div>
      {!machine ? (
        <div className="empty">No connected machine.</div>
      ) : status === null ? (
        <div className="empty">Looking for code-server on {machine.label}…</div>
      ) : status.available ? (
        <iframe key={frameKey} className="code-frame" src={src} title={`VS Code on ${machine.label}`} allow="clipboard-read; clipboard-write" />
      ) : (
        <div className="code-install">
          <h2>
            <Icon name="terminal" size={18} /> code-server is not running on {machine.label}
          </h2>
          <p className="modal-hint">{status.error}</p>
          <p className="modal-hint">
            Install it there as a background service (VS Code in the browser, bound to 127.0.0.1:{status.port || "7320"} with no auth of its own — remuda's
            token is the door):
          </p>
          <pre className="code-cmd">{machine.id === "local" ? "remuda code" : `remuda code -host ${machine.host ?? machine.label}`}</pre>
          <p className="modal-hint">
            …in a terminal on the machine that serves this page (<code>~/.local/bin/remuda</code> when it is not on your PATH). It downloads a pinned
            release of code-server and verifies its checksum.
          </p>
          <div className="modal-actions">
            <button type="button" className="btn" onClick={() => probe(true)} disabled={checking}>
              {checking ? "…" : "Check again"}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
