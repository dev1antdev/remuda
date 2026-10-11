import { useEffect, useMemo, useRef, useState } from "react";
import { fileUrl, fsDiff, fsStat, fsText, type FileDiff, type FileInfo } from "./api";
import { basename, errMsg } from "./prefs";
import { Notice } from "./ui";
import Icon from "./icons";

export type FileTarget = { path: string; line?: number; prefer?: "file" | "changes" };

type Tab = "file" | "changes";

// Opens what the agent wrote: the file itself, and its uncommitted change in
// git ("what Claude changed"). Reads from the machine the pane lives on.
// Pushes a history entry so the phone's Back closes it.
export default function FileViewer({
  machineId,
  paneId,
  target,
  onClose,
  onRetarget,
}: {
  machineId: string;
  paneId: string;
  target: FileTarget;
  onClose: () => void;
  onRetarget: (t: FileTarget) => void;
}) {
  const [info, setInfo] = useState<FileInfo | null>(null);
  const [text, setText] = useState<{ text: string; truncated: boolean; max: number } | null>(null);
  const [diff, setDiff] = useState<FileDiff | null>(null);
  const [tab, setTab] = useState<Tab>(target.prefer ?? "file");
  const [error, setError] = useState<string | null>(null);
  const lineRef = useRef<HTMLDivElement>(null);
  const box = useRef<HTMLDivElement>(null);

  // a history entry so Back (Android, iOS swipe) closes the viewer; when
  // the viewer goes away some other way the entry goes with it, or the
  // next Back would do nothing
  useEffect(() => {
    history.pushState({ remuda: "file" }, "");
    const onPop = () => onClose();
    window.addEventListener("popstate", onPop);
    return () => {
      window.removeEventListener("popstate", onPop);
      if (history.state?.remuda === "file") history.back();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  // keys go to the viewer, so Escape closes it and not whatever was behind
  useEffect(() => {
    box.current?.focus();
  }, []);

  const close = () => {
    if (history.state?.remuda === "file") history.back();
    else onClose();
  };
  // keys inside the viewer reach its own handler below; when the focus
  // has wandered out of it, Escape still closes it
  const closeRef = useRef(close);
  closeRef.current = close;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || box.current?.contains(document.activeElement) || document.querySelector(".sel-pop")) return;
      e.stopPropagation();
      closeRef.current();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  useEffect(() => {
    let stop = false;
    setInfo(null);
    setText(null);
    setDiff(null);
    setError(null);
    setTab(target.prefer ?? "file"); // a new target starts on the tab it asked for
    (async () => {
      try {
        const i = await fsStat(machineId, paneId, target.path);
        if (stop) return;
        setInfo(i);
        if (i.kind === "text") {
          const [t, d] = await Promise.all([fsText(machineId, i.path), fsDiff(machineId, i.path).catch(() => null)]);
          if (stop) return;
          setText(t);
          setDiff(d);
          if (target.prefer === "changes" && !(d && d.diff)) setTab("file");
        }
      } catch (e) {
        if (!stop) setError(errMsg(e));
      }
    })();
    return () => {
      stop = true;
    };
  }, [machineId, paneId, target.path, target.prefer]);

  useEffect(() => {
    if (tab === "file" && text && target.line) {
      lineRef.current?.scrollIntoView({ block: "center" });
    }
  }, [tab, text, target.line]);

  const name = basename(info?.path ?? target.path);
  const counts = useMemo(() => countChanges(diff?.diff ?? ""), [diff]);
  const hasDiff = !!(diff && diff.diff);

  return (
    <div className="fv-scrim" onMouseDown={(e) => e.target === e.currentTarget && close()}>
      <div
        ref={box}
        className="fv"
        role="dialog"
        aria-modal="true"
        aria-label={name}
        tabIndex={-1}
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            e.stopPropagation();
            close();
          }
        }}
      >
        <header className="fv-head">
          <div className="fv-title">
            <span className="fv-name">{name}</span>
            <span className="fv-path" title={info?.path ?? target.path} dir="rtl">
              <bdi>
                {info?.path ?? target.path}
                {target.line ? `:${target.line}` : ""}
              </bdi>
            </span>
          </div>
          {info?.kind === "text" && (
            <nav className="fv-tabs" aria-label="View">
              <button type="button" className={tab === "file" ? "on" : ""} aria-pressed={tab === "file"} onClick={() => setTab("file")}>
                File
              </button>
              <button
                type="button"
                className={tab === "changes" ? "on" : ""}
                aria-pressed={tab === "changes"}
                onClick={() => setTab("changes")}
                title={diff?.untracked ? "New file, not yet in git" : diff?.repo ? "Uncommitted change against HEAD" : "Not inside a git repository"}
              >
                Changes
                {hasDiff && <span className="fv-badge">{`+${counts.add} −${counts.del}`}</span>}
              </button>
            </nav>
          )}
          <button type="button" className="icon-btn" onClick={close} title="Close (Esc)" aria-label="Close">
            <Icon name="x" size={16} />
          </button>
        </header>

        <div className="fv-body">
          {error && <Notice tone="error">{error}</Notice>}
          {!error && !info && <div className="empty">Loading…</div>}
          {info?.kind === "missing" && (
            <div className="empty">
              {info.candidates?.length ? (
                <>
                  <p>Several files match — pick one:</p>
                  {info.candidates.map((c) => (
                    <button key={c} type="button" className="btn fv-cand" onClick={() => onRetarget({ ...target, path: c })}>
                      {c}
                    </button>
                  ))}
                </>
              ) : (
                <p>File not found on this machine.</p>
              )}
            </div>
          )}
          {info?.kind === "dir" && <div className="empty">That's a folder.</div>}
          {info?.kind === "binary" && <div className="empty">Binary file · {fmtSize(info.size)}</div>}
          {info?.kind === "image" && (
            <div className="fv-media">
              <img src={fileUrl(machineId, info.path)} alt={name} />
            </div>
          )}
          {info?.kind === "media" && (
            <div className="fv-media">
              <video src={fileUrl(machineId, info.path)} controls playsInline preload="metadata" />
            </div>
          )}
          {info?.kind === "pdf" && <iframe className="fv-pdf" src={fileUrl(machineId, info.path)} title={name} />}
          {info?.kind === "text" && tab === "file" && text && <CodeView text={text.text} line={target.line} lineRef={lineRef} />}
          {info?.kind === "text" && tab === "file" && text?.truncated && (
            <div className="empty">
              … cut at {fmtSize(text.max)} of {fmtSize(info.size)}
            </div>
          )}
          {info?.kind === "text" && tab === "changes" && <DiffView diff={diff} />}
        </div>
      </div>
    </div>
  );
}

// CodeView splits once per text, not per render: a 512KB file is 10k rows.
function CodeView({ text, line, lineRef }: { text: string; line?: number; lineRef: React.RefObject<HTMLDivElement> }) {
  const lines = useMemo(() => text.split("\n"), [text]);
  return (
    <div className="fv-code">
      {lines.map((l, i) => {
        const n = i + 1;
        const hit = n === line;
        return (
          <div key={n} className={hit ? "fv-line hit" : "fv-line"} ref={hit ? lineRef : undefined}>
            <span className="fv-ln">{n}</span>
            <span className="fv-src">{l}</span>
          </div>
        );
      })}
    </div>
  );
}

type DiffKind = "add" | "del" | "hunk" | "meta" | "";

function diffLineKind(l: string): DiffKind {
  if (l.startsWith("+++") || l.startsWith("---") || l.startsWith("diff ") || l.startsWith("index ")) return "meta";
  if (l.startsWith("+")) return "add";
  if (l.startsWith("-")) return "del";
  if (l.startsWith("@@")) return "hunk";
  return "";
}

// the lines added and removed, for the tab's badge
function countChanges(diff: string): { add: number; del: number } {
  let add = 0;
  let del = 0;
  if (diff) {
    for (const l of diff.split("\n")) {
      const k = diffLineKind(l);
      if (k === "add") add++;
      else if (k === "del") del++;
    }
  }
  return { add, del };
}

function DiffView({ diff }: { diff: FileDiff | null }) {
  const lines = useMemo(() => (diff?.diff ?? "").split("\n"), [diff]);
  if (!diff) return <div className="empty">Could not run git here.</div>;
  if (!diff.repo) return <div className="empty">Not inside a git repository.</div>;
  if (diff.untracked) return <div className="empty">New file — not yet added to git, so the whole file is the change.</div>;
  if (!diff.diff) return <div className="empty">No uncommitted changes — the file matches HEAD.</div>;
  return (
    <div className="fv-code fv-diff">
      {lines.map((l, i) => (
        <div key={i} className={`fv-line ${diffLineKind(l)}`}>
          <span className="fv-src">{l}</span>
        </div>
      ))}
    </div>
  );
}

function fmtSize(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}
