// A folder on a machine, chosen instead of typed: the path with its
// crumbs, the subfolders (repositories marked), the workspaces already
// known there as quick picks, hidden folders on request.

import { useEffect, useState } from "react";
import { listDirs, type Machine } from "./api";
import Icon from "./icons";
import { folderOptions } from "./options";
import { basename } from "./prefs";
import { Dialog, Notice, useAsync } from "./ui";

export default function FolderPicker({ machine, machines, start, onPick, onClose }: {
  machine: Machine;
  machines: Machine[];
  start: string; // where to begin ("" is home)
  onPick: (path: string) => void;
  onClose: () => void;
}) {
  const [path, setPath] = useState(start);
  const [typed, setTyped] = useState(start);
  const [hidden, setHidden] = useState(false);
  const { data, error, loading } = useAsync(() => listDirs(machine.id, path, hidden), [machine.id, path, hidden]);
  useEffect(() => {
    if (data?.path) setTyped(data.path);
  }, [data?.path]);
  const go = (p: string) => setPath(p);
  const crumbs = data?.path ? data.path.split("/").filter(Boolean) : [];
  const known = folderOptions(machines, machine.id);
  const here = data?.path ?? path;
  return (
    <Dialog title={`Folder on ${machine.label.split(" · ")[0]}`} icon="folder" size="md" onClose={onClose} className="picker">
      <form
        className="picker-path"
        onSubmit={(e) => {
          e.preventDefault();
          go(typed.trim());
        }}
      >
        <button type="button" className="icon-btn" title="Up one folder" disabled={!data?.parent} onClick={() => data?.parent && go(data.parent)}>
          <Icon name="arrow-left" size={14} />
        </button>
        <button type="button" className="icon-btn" title="Home" onClick={() => go("")}>
          <Icon name="user" size={14} />
        </button>
        <input className="sm" value={typed} spellCheck={false} onChange={(e) => setTyped(e.target.value)} placeholder="/path/to/folder" aria-label="Path" />
        <button type="submit" className="btn sm" title="Go to the typed path">
          Go
        </button>
      </form>
      {crumbs.length > 0 && (
        <div className="picker-crumbs" aria-label="Path">
          <button type="button" className="linkish" onClick={() => go("/")}>
            /
          </button>
          {crumbs.map((c, i) => (
            <button key={i} type="button" className="linkish" onClick={() => go("/" + crumbs.slice(0, i + 1).join("/"))}>
              {c}
            </button>
          ))}
        </div>
      )}
      {error && <Notice tone="error">{error}</Notice>}
      <div className="picker-list" aria-busy={loading}>
        {data && data.dirs.length === 0 && !loading && <div className="notice hint">No subfolders here.</div>}
        {data?.dirs.map((d) => {
          const full = (data.path === "/" ? "" : data.path) + "/" + d.name;
          return (
            <div key={d.name} className="picker-row">
              <button type="button" className="picker-open" onClick={() => go(full)} title={full}>
                <Icon name={d.git ? "git-branch" : "folder"} size={14} className={d.git ? "picker-git" : undefined} />
                <span className="picker-name">{d.name}</span>
                {d.git && <span className="picker-tag">repo</span>}
              </button>
              <button type="button" className="icon-btn sm" title={`Choose ${d.name}`} aria-label={`Choose ${d.name}`} onClick={() => onPick(full)}>
                <Icon name="check" size={12} />
              </button>
            </div>
          );
        })}
      </div>
      {known.length > 0 && (
        <div className="picker-known">
          <span className="dim">Workspaces on {machine.label}</span>
          {known.map((o) => (
            <button key={o.value} type="button" className="linkish" title={o.value} onClick={() => onPick(o.value)}>
              <Icon name="folder" size={12} /> {o.label}
            </button>
          ))}
        </div>
      )}
      <div className="modal-actions picker-foot">
        <label className="row">
          <input type="checkbox" checked={hidden} onChange={(e) => setHidden(e.target.checked)} /> Hidden folders
        </label>
        <span className="spacer" />
        <button type="button" className="btn" onClick={onClose}>
          Cancel
        </button>
        <button type="button" className="btn primary" disabled={!here} onClick={() => onPick(here)}>
          Choose {here ? basename(here) || "/" : "…"}
        </button>
      </div>
    </Dialog>
  );
}
