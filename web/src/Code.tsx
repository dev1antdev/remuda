// The Code mode: a machine's code-server (VS Code in the browser) framed
// under herdr-ui's own origin and token. Pick the machine and the folder —
// the workspaces herdr knows are offered — and the editor fills the screen.
// Once opened it stays mounted behind the other modes, so switching back
// does not reload the editor.

import { useEffect, useMemo, useState } from "react";
import { codeStatus, type CodeStatus, type Machine } from "./api";
import { errMsg, usePref } from "./prefs";
import Select from "./Select";
import Icon from "./icons";
import { openUrl } from "./host";

export default function Code({ machines, currentMachine, currentFolder, visible }: {
  machines: Machine[];
  currentMachine: string | null; // the pane in view, as a default
  currentFolder: string | null;
  visible: boolean;
}) {
  const connected = machines.filter((m) => m.state === "connected");
  const [machineId, setMachineId] = usePref("code.machine", "");
  const machine = connected.find((m) => m.id === (machineId || currentMachine)) ?? connected[0] ?? null;
  const [folders, setFolders] = usePref<Record<string, string>>("code.folders", {}); // last folder per machine

  // the folders on offer: every workspace's checkout (or its first pane's cwd)
  const options = useMemo(() => {
    if (!machine) return [];
    const seen = new Map<string, string>();
    for (const s of machine.sessions) {
      const folder = s.worktree?.checkout_path || s.tabs.flatMap((t) => t.panes).find((p) => p.cwd)?.cwd || "";
      if (folder && !seen.has(folder)) seen.set(folder, s.name);
    }
    return [...seen.entries()].map(([folder, name]) => ({ value: folder, label: name, hint: folder, icon: <Icon name="folder" size={14} /> }));
  }, [machine]);

  const folder = machine ? (folders[machine.id] || (currentMachine === machine.id && currentFolder) || options[0]?.value || "") : "";
  const [status, setStatus] = useState<CodeStatus | null>(null);
  const [checking, setChecking] = useState(false);
  const probe = (force = false) => {
    if (!machine) return;
    setChecking(true);
    codeStatus(machine.id, force).then(setStatus, (e) => setStatus({ available: false, port: 0, error: errMsg(e) })).finally(() => setChecking(false));
  };
  useEffect(() => {
    setStatus(null);
    probe();
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
        {connected.length > 1 && (
          <Select
            value={machine?.id ?? ""}
            onChange={setMachineId}
            options={connected.map((m) => ({ value: m.id, label: m.label, hint: m.host, icon: <span className={`machine-state is-${m.state}`} /> }))}
          />
        )}
        {machine && (
          <Select
            className="code-folder"
            value={folder}
            placeholder="Pick a folder"
            onChange={(f) => setFolders({ ...folders, [machine.id]: f })}
            options={options.some((o) => o.value === folder) || !folder ? options : [{ value: folder, label: folder.split("/").pop() || folder, hint: folder, icon: <Icon name="folder" size={14} /> }, ...options]}
          />
        )}
        <span className="code-bar-spacer" />
        {status?.available && (
          <button className="icon-btn" title="Open the editor in its own tab" onClick={() => openUrl(src)}>
            <Icon name="external-link" size={15} />
          </button>
        )}
      </div>
      {!machine ? (
        <div className="placeholder">No connected machine.</div>
      ) : status === null ? (
        <div className="placeholder">Looking for code-server on {machine.label}…</div>
      ) : status.available ? (
        <iframe key={frameKey} className="code-frame" src={src} title={`VS Code on ${machine.label}`} allow="clipboard-read; clipboard-write" />
      ) : (
        <div className="code-install">
          <h2>
            <Icon name="terminal" size={18} /> code-server is not running on {machine.label}
          </h2>
          <p className="modal-hint">{status.error}</p>
          <p className="modal-hint">
            Install it there as a background service (VS Code in the browser, bound to 127.0.0.1:{status.port || "7320"} with no auth of its own — herdr-ui's
            token is the door):
          </p>
          <pre className="code-cmd">{machine.id === "local" ? "make code" : `make code HOST=${machine.host ?? machine.label}`}</pre>
          <p className="modal-hint">
            …from a checkout of herdr-ui, or <code>sh deploy/install-code.sh</code> on the machine itself. It downloads a pinned release of code-server
            and verifies its checksum.
          </p>
          <div className="modal-actions">
            <button type="button" onClick={() => probe(true)} disabled={checking}>
              {checking ? "…" : "Check again"}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
