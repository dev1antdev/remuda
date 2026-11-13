// The Files tab: what the pull request changes as a tree, folders first and each closable, every file with its lines
// added and removed; a click shows that file's patch, GitHub's, drawn by the file viewer's diff rows. Read once per head
// commit.

import { useState } from "react";
import type { PRAt } from "@/app/store";
import { openUrl } from "@/lib/host";
import { useQuery } from "@/lib/query";
import { t, tn } from "@/lib/i18n";
import { Icon, Notice } from "@/ui";
import { fetchFiles, type Detail, type PRFile } from "./api";
import { filesURL } from "./links";
import { fileTotals, fileTree } from "./tree";
import Patch from "./Patch";

const indent = (depth: number) => ({ paddingLeft: `${8 + depth * 14}px` });

export default function Files({ at, pr }: { at: PRAt; pr: Detail }) {
  const files = useQuery(() => fetchFiles(at.machineId, pr.repository, pr.number), [at.machineId, pr.repository, pr.number, pr.head_oid]);
  const [closed, setClosed] = useState<Set<string>>(new Set());
  const [picked, setPicked] = useState("");
  if (files.loading) return <p className="pr-empty">{t("Reading the files…")}</p>;
  if (files.error) return <Notice tone="error">{files.error}</Notice>;
  const list = files.data?.files ?? [];
  if (list.length === 0) return <p className="pr-empty">{t("No file changed.")}</p>;
  const totals = fileTotals(list);
  const toggle = (path: string) => {
    const next = new Set(closed);
    if (next.has(path)) next.delete(path);
    else next.add(path);
    setClosed(next);
  };
  return (
    <div className="pr-files">
      <div className="pr-files-head">
        <span>{tn(list.length, "{n} file", "{n} files")}</span>
        <span className="pr-add">+{totals.additions}</span>
        <span className="pr-del">−{totals.deletions}</span>
      </div>
      {files.data?.cut && <Notice tone="hint">{t("The list runs past what was read; GitHub has the rest.")}</Notice>}
      {fileTree(list, closed).map((row) =>
        row.kind === "folder" ? (
          <button key={`d:${row.path}`} type="button" className="pr-file-row" style={indent(row.depth)} aria-expanded={!closed.has(row.path)} onClick={() => toggle(row.path)}>
            <Icon name={closed.has(row.path) ? "chevron-right" : "chevron-down"} size={12} />
            <Icon name="folder" size={14} />
            <span className="pr-file-name">{row.name}</span>
          </button>
        ) : (
          <FileRow key={row.file.path} file={row.file} name={row.name} depth={row.depth} picked={picked === row.file.path} onPick={() => setPicked(picked === row.file.path ? "" : row.file.path)} prURL={pr.url} />
        ),
      )}
    </div>
  );
}

function FileRow({ file, name, depth, picked, onPick, prURL }: { file: PRFile; name: string; depth: number; picked: boolean; onPick: () => void; prURL: string }) {
  return (
    <>
      <button type="button" className={`pr-file-row${picked ? " on" : ""}`} style={indent(depth)} title={file.previous ? `${file.previous} → ${file.path}` : file.path} aria-expanded={picked} onClick={onPick}>
        <span className="pr-file-gap" />
        <Icon name="file" size={14} />
        <span className="pr-file-name">{name}</span>
        <span className="pr-add">+{file.additions}</span>
        <span className="pr-del">−{file.deletions}</span>
      </button>
      {picked &&
        (file.patch ? (
          <Patch patch={file.patch} path={file.path} />
        ) : (
          <p className="pr-empty">
            {t("GitHub sends no patch for this file (a binary or a very large one).")}{" "}
            <button type="button" className="linkish" onClick={() => openUrl(filesURL(prURL))}>
              {t("Its changes on GitHub")}
            </button>
          </p>
        ))}
    </>
  );
}
