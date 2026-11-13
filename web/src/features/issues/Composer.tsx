// The comment box under an issue's timeline, as GitHub's: Write | Preview, then Close with comment ▾ (as completed or
// not planned) or Reopen, and Comment. A close or a reopen takes what is in the box along. The draft stays on this
// device until it is sent.

import { useState } from "react";
import type { OpenFile } from "@/features/files/target";
import { isMac, usePref } from "@/lib/prefs";
import { Icon, Menu } from "@/ui";
import { t } from "@/lib/i18n";
import type { Detail } from "./api";
import { Body } from "./Timeline";

export default function Composer({ issue, repo, draftKey, busy, onComment, onClose, onReopen, onOpenFile }: {
  issue: Detail;
  repo: string;
  draftKey: string;
  busy: string | null; // what is being done, while a write is under way
  onComment: (body: string) => Promise<boolean>;
  onClose: (reason: "completed" | "not_planned", body: string) => Promise<boolean>;
  onReopen: (body: string) => Promise<boolean>;
  onOpenFile: OpenFile;
}) {
  const [text, setText] = usePref(draftKey, "");
  const [tab, setTab] = useState<"write" | "preview">("write");
  const has = text.trim() !== "";
  const done = (ok: boolean) => {
    if (!ok) return;
    setText("");
    setTab("write");
  };
  const comment = () => has && !busy && void onComment(text).then(done);
  const close = (reason: "completed" | "not_planned") => !busy && void onClose(reason, text).then(done);
  const reopen = () => !busy && void onReopen(text).then(done);
  const open = issue.state === "open";
  return (
    <section className="issue-composer" aria-label={t("Add a comment")}>
      <h3 className="issue-composer-title">{t("Add a comment")}</h3>
      <div className="issue-composer-box">
        <nav className="fv-tabs issue-composer-tabs" aria-label={t("Comment")}>
          <button type="button" className={tab === "write" ? "on" : ""} aria-pressed={tab === "write"} onClick={() => setTab("write")}>
            {t("Write")}
          </button>
          <button type="button" className={tab === "preview" ? "on" : ""} aria-pressed={tab === "preview"} onClick={() => setTab("preview")}>
            {t("Preview")}
          </button>
        </nav>
        {tab === "write" ? (
          <textarea
            className="issue-composer-text"
            value={text}
            rows={5}
            placeholder={t("Use Markdown to format your comment")}
            aria-label={t("Comment")}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
                e.preventDefault();
                comment();
              }
            }}
          />
        ) : (
          <div className="issue-composer-preview">{has ? <Body text={text} repo={repo} onOpenFile={onOpenFile} /> : <p className="issue-nothing">{t("Nothing to preview")}</p>}</div>
        )}
        <div className="issue-composer-foot">
          <span className="issue-composer-hint" aria-live="polite">
            {busy ?? `${t("Markdown is supported")} · ${isMac ? "⌘↵" : "Ctrl+↵"} ${t("comments")}`}
          </span>
          {open ? (
            <span className="issue-split">
              <button type="button" className="btn sm" disabled={!!busy} onClick={() => close("completed")}>
                <Icon name="circle-check" size={14} className="is-done" /> {has ? t("Close with comment") : t("Close issue")}
              </button>
              <Menu
                label={t("Close as")}
                tip={t("Close as completed or not planned")}
                items={[
                  { label: t("Close as completed"), icon: "circle-check", hint: t("Done, fixed, resolved"), onSelect: () => close("completed") },
                  { label: t("Close as not planned"), icon: "circle-slash", hint: t("Won't fix, can't repro, stale"), onSelect: () => close("not_planned") },
                ]}
                trigger={
                  <button type="button" className="btn sm issue-split-more" disabled={!!busy} aria-label={t("Close as")}>
                    <Icon name="chevron-down" size={12} />
                  </button>
                }
              />
            </span>
          ) : (
            <button type="button" className="btn sm" disabled={!!busy} onClick={reopen}>
              <Icon name="circle-dot" size={14} className="is-open" /> {has ? t("Reopen with comment") : t("Reopen issue")}
            </button>
          )}
          <button type="button" className="btn sm primary issue-comment-btn" disabled={!has || !!busy} onClick={comment}>
            {t("Comment")}
          </button>
        </div>
      </div>
    </section>
  );
}
