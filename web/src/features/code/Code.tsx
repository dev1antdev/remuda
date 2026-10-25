// The Code mode: a machine's code-server (VS Code in the browser) framed under
// remuda's own origin and token; pick the machine and a folder herdr knows.
// Once opened it stays mounted behind the other modes, so the editor never reloads.

import { useEffect, useMemo, useRef, useState } from "react";
import { app } from "@/app/store";
import { codeStatus, codeTheme, codeWorkspace, openInEditor, type CodeStatus } from "@/features/code/api";
import { useFlavor } from "@/app/theme";
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

// ALL is the picker's first row: every workspace of the machine in one editor, through its .code-workspace file
export const ALL = "*";

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
    const each = [...seen.entries()].map(([folder, name]) => ({ value: folder, label: name, hint: folder, icon: folderIcon }));
    return each.length > 1 ? [{ value: ALL, label: `All workspaces on ${machine.label}`, hint: `${each.length} folders in one editor`, icon: <Icon name="layers" size={14} /> }, ...each] : each;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [machine]);
  const roots = useMemo(() => options.filter((o) => o.value !== ALL).map((o) => o.value), [options]);

  // what is picked: a folder, or ALL — the default once a machine has more than one workspace
  const picked = machine ? folders[machine.id] || "" : "";
  const wanted = picked === ALL && roots.length <= 1 ? roots[0] || "" : picked; // ALL with one workspace is that workspace
  const folder = machine ? wanted || (roots.length > 1 ? ALL : (currentMachine === machine.id && currentFolder) || roots[0] || "") : "";
  const all = folder === ALL;
  // the workspace file: written when ALL is picked and again whenever the machine's folders change; the editor follows the file
  const [workspace, setWorkspace] = useState<{ machineId: string; roots: string; path: string } | null>(null);
  const rootsKey = roots.join("|");
  useEffect(() => {
    if (!machine || !all || !roots.length) return;
    let gone = false;
    codeWorkspace(machine.id).then(
      (w) => !gone && setWorkspace({ machineId: machine.id, roots: rootsKey, path: w.path }),
      (e) => !gone && actions.notify(errMsg(e), { error: true }),
    );
    return () => {
      gone = true;
    };
  }, [machine?.id, all, rootsKey]); // eslint-disable-line react-hooks/exhaustive-deps
  const workspacePath = workspace && machine && workspace.machineId === machine.id ? workspace.path : null;
  // a file asked for from a pane: the editor goes to its machine and folder first, then opens it
  const codeFile = app.use((s) => s.codeFile);
  useEffect(() => {
    if (!codeFile) return;
    if (codeFile.machineId !== machineId) setMachineId(codeFile.machineId);
    // ALL already holds every workspace folder: the file simply opens there; a single folder is switched to
    const keepAll = folders[codeFile.machineId] === ALL || (!folders[codeFile.machineId] && roots.length > 1 && roots.includes(codeFile.folder));
    if (codeFile.folder && !keepAll) setFolders((prev) => (prev[codeFile.machineId] === codeFile.folder ? prev : { ...prev, [codeFile.machineId]: codeFile.folder }));
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
    if (asked && (asked.machineId !== machine.id || (asked.folder && asked.folder !== folder && !(all && roots.includes(asked.folder))))) return; // the picker is on its way there
    if (asked && !status?.available) return; // the authority comes with the status; the effect runs again once it is in
    if (all && !workspacePath) return; // the workspace file is on its way
    if (asked) carried.current = asked.n;
    const place = `${machine.id}|${folder}`;
    const file = asked && !roots.includes(asked.path.replace(/\/+$/, "")) ? asked.path : null; // a folder itself is no file to open
    if (asked && file && frame && frame.place === place && ready === frame.key) {
      openLive(file, asked.line); // the editor is on screen: the file goes to it, nothing reloads
      return;
    }
    const authority = status?.authority || location.host;
    const payload = file ? `&payload=${encodeURIComponent(JSON.stringify([["openFile", `vscode-remote://${authority}${escapePath(file)}`]]))}` : "";
    const where = all ? `?workspace=${encodeURIComponent(workspacePath!)}` : folder ? `?folder=${encodeURIComponent(folder)}` : "";
    const src = `/code/${encodeURIComponent(machine.id)}/${where}${where ? payload : ""}`;
    setFrame({ key: `${place}|${asked?.n ?? 0}|${reloads}`, src, place });
  }, [machine?.id, folder, codeFile?.n, status?.available, reloads, workspacePath]); // eslint-disable-line react-hooks/exhaustive-deps

  // the editor wears the page's flavour: asked once per machine and flavour; an editor already open takes a new theme, or
  // an extension installed under it, only on a fresh load, so the frame reloads then
  const flavor = useFlavor();
  const themed = useRef<string>("");
  useEffect(() => {
    if (!machine || !status?.available) return;
    const key = `${machine.id}|${flavor}`;
    if (themed.current === key) return;
    const first = !themed.current.startsWith(`${machine.id}|`);
    themed.current = key;
    codeTheme(machine.id, flavor).then(
      (r) => (r.installed || !first) && setReloads((n) => n + 1),
      (e) => actions.notify(`The editor keeps its own theme: ${errMsg(e)}`, { error: true }),
    );
  }, [machine?.id, status?.available, flavor]); // eslint-disable-line react-hooks/exhaustive-deps

  // Follow: the pane on screen in Agents is watched at a tile's pace, and every edit its agent finishes opens here as it lands
  const [follow, setFollow] = usePref("code.follow", false);
  const watched = currentPane && machine && currentPane.machineId === machine.id && currentPane.agent ? currentPane : null;
  const following = follow && !!watched && visible && ready !== "" && ready === frame?.key;
  const tail = useChat({ machineId: watched?.machineId ?? "", paneId: watched?.paneId ?? "", sessionId: null, dir: watched?.dir ?? "", every: 3000, active: following, last: 4 });
  const latest = useMemo(() => (tail.data ? newestEdit(tail.data.turns) : null), [tail.data]);
  // armed once the first answer after Follow began is in: whatever edit it holds is not news, every later one is
  const seenEdit = useRef<{ id: string } | null>(null);
  useEffect(() => {
    if (!following) {
      seenEdit.current = null;
      return;
    }
    if (!tail.data) return;
    if (seenEdit.current === null) {
      seenEdit.current = { id: latest?.id ?? "" };
      return;
    }
    if (!latest || latest.id === seenEdit.current.id) return;
    seenEdit.current = { id: latest.id };
    openLive(latest.path);
  }, [following, tail.data, latest?.id]); // eslint-disable-line react-hooks/exhaustive-deps

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
            options={options.some((o) => o.value === folder) || !folder || folder === ALL ? options : [{ value: folder, label: basename(folder), hint: folder, icon: folderIcon }, ...options]}
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
