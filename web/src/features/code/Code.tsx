// The Code mode: a machine's code-server (VS Code in the browser) framed under
// remuda's own origin and token; pick the machine and a folder herdr knows.
// Once opened it stays mounted behind the other modes, so the editor never reloads.

import { useEffect, useMemo, useRef, useState } from "react";
import { app } from "@/app/store";
import { codeStatus, openInEditor, type CodeStatus } from "@/features/code/api";
import { newestEdit } from "@/features/code/follow";
import { useChat } from "@/features/chat/useChat";
import { actions } from "@/app/store";
import type { Machine } from "@/features/roster/api";
import { basename, errMsg } from "@/lib/text";
import { usePref } from "@/lib/prefs";
import { connected, machineOptions } from "@/features/roster/options";
import Select from "@/ui/Select";
import Icon from "@/ui/icons";
import { openUrl } from "@/lib/host";
import { Tip } from "@/ui";

// escapePath keeps a #, ? or % in a file name as part of the path when the workbench parses its URI
const escapePath = (path: string) => path.split("/").map(encodeURIComponent).join("/");

export type CurrentPane = { machineId: string; paneId: string; dir: string; agent: string } | null;

export default function Code({ machines, currentMachine, currentFolder, currentPane, visible }: {
  machines: Machine[];
  currentMachine: string | null;
  currentFolder: string | null;
  currentPane: CurrentPane; // the pane on screen in Agents: the one Follow watches
  visible: boolean;
}) {
  const live = connected(machines);
  const [machineId, setMachineId] = usePref("code.machine", "");
  const machine = live.find((m) => m.id === (machineId || currentMachine)) ?? live[0] ?? null;
  const [folders, setFolders] = usePref<Record<string, string>>("code.folders", {});
  const folderIcon = <Icon name="folder" size={14} />;

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
  // a file asked for from a pane: the editor goes to its machine and folder first, then opens it
  const codeFile = app.use((s) => s.codeFile);
  useEffect(() => {
    if (!codeFile) return;
    if (codeFile.machineId !== machineId) setMachineId(codeFile.machineId);
    if (codeFile.folder) setFolders((prev) => (prev[codeFile.machineId] === codeFile.folder ? prev : { ...prev, [codeFile.machineId]: codeFile.folder }));
  }, [codeFile]); // eslint-disable-line react-hooks/exhaustive-deps
  const [status, setStatus] = useState<CodeStatus | null>(null);
  const [checking, setChecking] = useState(false);
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
  useEffect(() => {
    if (!visible || status?.available !== false) return;
    const id = setInterval(() => probe(true), 20_000);
    return () => clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible, status?.available, machine?.id]);

  // the frame's address is settled once per machine, folder and request: a file asked for rides along as the workbench's
  // payload on that one mount — named by the authority code-server's page states ("remote" these days; the page's host
  // before, and still the fallback) — and coming back to the folder later does not open it again
  const [frame, setFrame] = useState<{ key: string; src: string; place: string } | null>(null);
  const [reloads, setReloads] = useState(0); // a reload by hand: a new frame for the same folder, when the editor lost its way
  const [ready, setReady] = useState(""); // the frame key whose editor has loaded: a file asked for then goes to it in place
  const carried = useRef(0); // the request a frame has carried
  const openLive = (path: string, line?: number) => void openInEditor(machine!.id, path, line).catch((e) => actions.notify(errMsg(e), { error: true }));
  useEffect(() => {
    if (!machine) {
      setFrame(null);
      return;
    }
    const asked = codeFile && codeFile.n !== carried.current ? codeFile : null;
    if (asked && (asked.machineId !== machine.id || (asked.folder && asked.folder !== folder))) return; // the picker is on its way there
    if (asked && !status?.available) return; // the authority comes with the status; the effect runs again once it is in
    if (asked) carried.current = asked.n;
    const place = `${machine.id}|${folder}`;
    const file = asked && asked.path.replace(/\/+$/, "") !== folder.replace(/\/+$/, "") ? asked.path : null; // the folder itself is no file to open
    if (asked && file && frame && frame.place === place && ready === frame.key) {
      openLive(file, asked.line); // the editor is on screen: the file goes to it, nothing reloads
      return;
    }
    const authority = status?.authority || location.host;
    const payload = file ? `&payload=${encodeURIComponent(JSON.stringify([["openFile", `vscode-remote://${authority}${escapePath(file)}`]]))}` : "";
    const src = folder ? `/code/${encodeURIComponent(machine.id)}/?folder=${encodeURIComponent(folder)}${payload}` : `/code/${encodeURIComponent(machine.id)}/`;
    setFrame({ key: `${place}|${asked?.n ?? 0}|${reloads}`, src, place });
  }, [machine?.id, folder, codeFile?.n, status?.available, reloads]); // eslint-disable-line react-hooks/exhaustive-deps

  // Follow: the pane on screen in Agents is watched at a tile's pace, and every edit its agent finishes opens here as it lands
  const [follow, setFollow] = usePref("code.follow", false);
  const watched = currentPane && machine && currentPane.machineId === machine.id && currentPane.agent ? currentPane : null;
  const following = follow && !!watched && visible && ready !== "" && ready === frame?.key;
  const tail = useChat({ machineId: watched?.machineId ?? "", paneId: watched?.paneId ?? "", sessionId: null, dir: watched?.dir ?? "", every: 3000, active: following, last: 4 });
  const latest = useMemo(() => (tail.data ? newestEdit(tail.data.turns) : null), [tail.data]);
  const seenEdit = useRef<string | null>(null);
  useEffect(() => {
    if (!following) {
      seenEdit.current = null;
      return;
    }
    if (!latest) return;
    if (seenEdit.current === null) {
      seenEdit.current = latest.id; // what was already there when Follow began is not news
      return;
    }
    if (latest.id === seenEdit.current) return;
    seenEdit.current = latest.id;
    openLive(latest.path);
  }, [following, latest?.id]); // eslint-disable-line react-hooks/exhaustive-deps

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
        {watched && status?.available && (
          <Tip text={follow ? "Following the agent: every file it finishes editing opens here. Click to stop." : `Follow the agent: open every file it edits in ${watched.dir.split("/").pop() || "its folder"}, as it happens`}>
            <button type="button" className={`btn sm code-follow${follow ? " on" : ""}`} aria-pressed={follow} onClick={() => setFollow(!follow)}>
              <Icon name="eye" size={14} />
              <span>Follow</span>
            </button>
          </Tip>
        )}
        <span className="code-bar-spacer" />
        {status?.available && status.version && <span className="code-version">code-server {status.version}</span>}
        {status?.available && (
          <Tip text="Reload the editor: a fresh frame for this folder, when it lost its connection or shows a stale state">
            <button type="button" className="icon-btn" aria-label="Reload the editor" onClick={() => setReloads((n) => n + 1)}>
              <Icon name="refresh-cw" size={16} />
            </button>
          </Tip>
        )}
        {status?.available && (
          <button type="button" className="icon-btn" title="Open the editor in its own tab" onClick={() => frame && openUrl(frame.src)}>
            <Icon name="external-link" size={14} />
          </button>
        )}
      </div>
      {!machine ? (
        <div className="empty">No connected machine.</div>
      ) : status === null ? (
        <div className="empty">Looking for code-server on {machine.label}…</div>
      ) : status.available ? (
        frame && <iframe key={frame.key} className="code-frame" src={frame.src} title={`VS Code on ${machine.label}`} allow="clipboard-read; clipboard-write" onLoad={() => setReady(frame.key)} />
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
