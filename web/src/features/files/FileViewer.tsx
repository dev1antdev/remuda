import { useEffect, useMemo, useRef, useState } from "react";
import { fileUrl, fsDiff, fsStat, fsText, type FileDiff, type FileInfo } from "@/features/files/api";
import { langOf } from "@/lib/highlight";
import { basename, errMsg } from "@/lib/text";
import { Notice, Tip } from "@/ui";
import Icon from "@/ui/icons";
import { Markdown } from "@/ui/Markdown";
import { Mermaid } from "@/ui/Mermaid";
import { opensDrawn, previewOf } from "@/ui/mermaidLook";
import { FileIcon } from "@/ui/FileIcon";
import CodeLines from "./CodeLines";
import DiffView from "./DiffView";
import { changeCounts, diffLines } from "./diffLines";
import { changesOf, lineCountOf, wholeFile } from "./gutter";

import type { FileTarget } from "./target";
import { actions } from "@/app/store";
import { useRoster } from "@/features/roster/useRoster";
import { onWindows } from "@/features/roster/system";
import { t } from "@/lib/i18n";

type Tab = "file" | "drawn" | "changes";

// startTab is the view a file opens in: its changes when asked, drawn when it is Markdown or a diagram and no line was named
const startTab = (target: FileTarget): Tab => (target.prefer === "changes" ? "changes" : opensDrawn(target.path, target.line, target.prefer) ? "drawn" : "file");

// FileViewer opens what the agent wrote: the file itself and its uncommitted change in git, read from the machine the pane lives on
export default function FileViewer({
  machineId,
  paneId,
  dir = "",
  target,
  onClose,
  onRetarget,
}: {
  machineId: string;
  paneId: string;
  dir?: string; // stands in for the pane's folder when there is no pane
  target: FileTarget;
  onClose: () => void;
  onRetarget: (t: FileTarget) => void;
}) {
  const [info, setInfo] = useState<FileInfo | null>(null);
  const [text, setText] = useState<{ text: string; truncated: boolean; max: number } | null>(null);
  const [diff, setDiff] = useState<FileDiff | null>(null);
  const [tab, setTab] = useState<Tab>(() => startTab(target));
  const [error, setError] = useState<string | null>(null);
  const { viewOnly, machines } = useRoster(); // a viewer link's screen reads the file only: no diff, no editor
  const editor = !viewOnly && !onWindows(machines.find((m) => m.id === machineId)); // a Windows PC has no code-server
  const lineRef = useRef<HTMLDivElement>(null);
  const box = useRef<HTMLDivElement>(null);

  // a history entry so Back (Android, an iOS swipe) closes the viewer; gone another way, the entry goes too, or the next Back would do nothing
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
  useEffect(() => {
    box.current?.focus();
  }, []);

  const close = () => {
    if (history.state?.remuda === "file") history.back();
    else onClose();
  };
  // when the focus has wandered out of the viewer, Escape still closes it
  const closeRef = useRef(close);
  closeRef.current = close;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || box.current?.contains(document.activeElement) || document.querySelector(".sel-pop, .modal-scrim")) return;
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
    setTab(startTab(target));
    (async () => {
      try {
        const i = await fsStat(machineId, paneId, target.path, dir);
        if (stop) return;
        setInfo(i);
        if (i.kind === "text") {
          const [t, d] = await Promise.all([fsText(machineId, i.path), viewOnly ? null : fsDiff(machineId, i.path).catch(() => null)]);
          if (stop) return;
          setText(t);
          setDiff(d);
          if (target.prefer === "changes" && !(d && d.diff)) setTab("file");
        } else if (i.kind === "missing" && !i.candidates?.length && !viewOnly) {
          const d = await fsDiff(machineId, i.path).catch(() => null); // a file deleted since HEAD: its removal is its change
          if (stop) return;
          if (d?.diff) {
            setDiff(d);
            setTab("changes");
          }
        }
      } catch (e) {
        if (!stop) setError(errMsg(e));
      }
    })();
    return () => {
      stop = true;
    };
  }, [machineId, paneId, dir, target.path, target.prefer, viewOnly]);

  useEffect(() => {
    if (tab === "file" && text && target.line) {
      lineRef.current?.scrollIntoView({ block: "center" });
    }
  }, [tab, text, target.line]);

  const name = basename(info?.path ?? target.path);
  const lang = langOf(info?.path ?? target.path);
  const changes = useMemo(() => diffLines(diff?.diff ?? "", lang), [diff, lang]);
  const counts = useMemo(() => changeCounts(changes), [changes]);
  const hasDiff = !!(diff && diff.diff);
  const gone = info?.kind === "missing" && hasDiff;
  const drawnAs = info?.kind === "text" ? previewOf(info.path) : null;
  const shown: Tab = gone ? "changes" : tab === "drawn" && !drawnAs ? "file" : tab;
  const lineChanges = useMemo(() => (!text || !diff ? [] : diff.untracked ? wholeFile(lineCountOf(text.text)) : changesOf(diff.diff)), [text, diff]);

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
            <FileIcon path={info?.path ?? target.path} className="fv-icon" />
            <span className="fv-name">{name}</span>
            <span className="fv-path" title={info?.path ?? target.path} dir="rtl">
              <bdi>
                {info?.path ?? target.path}
                {target.line ? `:${target.line}` : ""}
              </bdi>
            </span>
          </div>
          {(info?.kind === "text" || gone) && (!viewOnly || drawnAs) && (
            <nav className="fv-tabs" aria-label={t("View")}>
              {!gone && (
                <button type="button" className={shown === "file" ? "on" : ""} aria-pressed={shown === "file"} onClick={() => setTab("file")}>
                  {t("File")}
                </button>
              )}
              {drawnAs && (
                <button type="button" className={shown === "drawn" ? "on" : ""} aria-pressed={shown === "drawn"} onClick={() => setTab("drawn")} title={drawnAs === "mermaid" ? t("The diagram, drawn") : t("The Markdown, drawn")}>
                  {t("Preview")}
                </button>
              )}
              {!viewOnly && (
                <button
                  type="button"
                  className={shown === "changes" ? "on" : ""}
                  aria-pressed={shown === "changes"}
                  onClick={() => setTab("changes")}
                  title={gone ? t("Deleted since HEAD") : diff?.untracked ? t("New file, not yet in git") : diff?.repo ? t("Uncommitted change against HEAD") : t("Not inside a git repository")}
                >
                  {t("Changes")}
                  {hasDiff && <span className="fv-badge">{`+${counts.add} −${counts.del}`}</span>}
                </button>
              )}
            </nav>
          )}
          {editor && (
            <Tip text={t("Open in Code (VS Code on this machine), at this line")}>
              <button
                type="button"
                className="icon-btn"
                aria-label={t("Open in Code")}
                onClick={() => {
                  close();
                  actions.openInCode(machineId, dir, info?.path ?? target.path, target.line);
                }}
              >
                <Icon name="code" size={16} />
              </button>
            </Tip>
          )}
          <Tip text={t("Close (Esc)")}>
            <button type="button" className="icon-btn" onClick={close} aria-label={t("Close")}>
              <Icon name="x" size={16} />
            </button>
          </Tip>
        </header>

        <div className="fv-body">
          {error && <Notice tone="error">{error}</Notice>}
          {!error && !info && <div className="empty">{t("Loading…")}</div>}
          {info?.kind === "missing" && !gone && (
            <div className="empty">
              {info.candidates?.length ? (
                <>
                  <p>{t("Several files match — pick one:")}</p>
                  {info.candidates.map((c) => (
                    <button key={c} type="button" className="btn fv-cand" onClick={() => onRetarget({ ...target, path: c })}>
                      {c}
                    </button>
                  ))}
                </>
              ) : (
                <p>{t("File not found on this machine.")}</p>
              )}
            </div>
          )}
          {info?.kind === "dir" && <div className="empty">{t("That's a folder.")}</div>}
          {info?.kind === "binary" && <div className="empty">{t("Binary file · {size}", { size: fmtSize(info.size) })}</div>}
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
          {info?.kind === "text" && shown === "file" && text && <CodeLines text={text.text} lang={lang} line={target.line} lineRef={lineRef} changes={lineChanges} />}
          {info?.kind === "text" && shown === "drawn" && text && (
            <div className="fv-drawn">
              {drawnAs === "mermaid" ? <FileDiagram source={text.text} /> : <Markdown text={text.text} onOpenFile={onRetarget} diagrams />}
            </div>
          )}
          {info?.kind === "text" && shown !== "changes" && text?.truncated && (
            <div className="empty">
              {t("… cut at {cut} of {size}", { cut: fmtSize(text.max), size: fmtSize(info.size) })}
            </div>
          )}
          {(info?.kind === "text" || gone) && shown === "changes" && <DiffView diff={diff} lines={changes} />}
        </div>
      </div>
    </div>
  );
}

// FileDiagram draws a .mmd file; one mermaid cannot read says why, the File tab keeping its text
function FileDiagram({ source }: { source: string }) {
  const [error, setError] = useState("");
  useEffect(() => setError(""), [source]);
  return error ? <Notice tone="error">{t("Mermaid could not read it: {error}", { error })}</Notice> : <Mermaid source={source} onError={setError} />;
}

function fmtSize(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return t("{n} KB", { n: (n / 1024).toFixed(1) });
  return t("{n} MB", { n: (n / 1024 / 1024).toFixed(1) });
}
