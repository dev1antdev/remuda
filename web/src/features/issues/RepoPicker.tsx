// The repository picker: a button naming the repository on screen, opening Follow the pane, the recent ones and the
// panel's machine's checkouts, with a filter over them all.

import { useState } from "react";
import { Icon, Popover } from "@/ui";
import { t } from "@/lib/i18n";
import type { Repo } from "./api";
import { pickerRows, type Picked, type Recent } from "./repos";

export default function RepoPicker({ repo, picked, machine, following, recent, repos, machineLabel, onPick }: {
  repo: string; // the repository on screen, "" when none
  picked: Picked;
  machine: string;
  following: string; // the pane's repository, "" when its folder is none
  recent: Recent[];
  repos: Repo[];
  machineLabel: (id: string) => string;
  onPick: (picked: Picked) => void;
}) {
  const [open, setOpen] = useState(false);
  const [filter, setFilter] = useState("");
  const rows = pickerRows({ picked, machine, following, recent, repos, filter, machineLabel });
  const choose = (next: Picked) => {
    setOpen(false);
    setFilter("");
    onPick(next);
  };
  const first = rows.find((r) => r.kind === "repo");
  return (
    <Popover
      open={open}
      onOpenChange={setOpen}
      label={t("Repository")}
      className="issue-repos"
      trigger={
        <button type="button" className="btn sm issue-repo-btn">
          <Icon name="git-branch" size={14} />
          <span className="issue-repo-name">{repo ? (repo.split("/")[1] ?? repo) : t("Choose repository")}</span>
          <Icon name="chevron-down" size={12} />
        </button>
      }
    >
      <label className="issue-repos-filter">
        <Icon name="search" size={14} />
        <input
          autoFocus
          className="bare"
          value={filter}
          placeholder={t("Search repositories")}
          aria-label={t("Search repositories")}
          onChange={(e) => setFilter(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && first?.kind === "repo") choose({ machine: first.machine, repo: first.repo });
          }}
        />
      </label>
      <div className="issue-repos-list" role="listbox" aria-label={t("Repository")}>
        {rows.map((row, i) =>
          row.kind === "heading" ? (
            <div key={i} className="issue-repos-heading">
              {row.text}
            </div>
          ) : row.kind === "follow" ? (
            <button key="follow" type="button" role="option" aria-selected={row.checked} className="btn ghost issue-repos-row" onClick={() => choose(null)}>
              <span className="issue-repos-check">{row.checked && <Icon name="check" size={12} />}</span>
              <span className="issue-repos-text">
                {t("Follow the pane")}
                <small>{row.hint}</small>
              </span>
            </button>
          ) : (
            <button key={`${row.machine}|${row.repo}`} type="button" role="option" aria-selected={row.checked} className="btn ghost issue-repos-row" onClick={() => choose({ machine: row.machine, repo: row.repo })}>
              <span className="issue-repos-check">{row.checked && <Icon name="check" size={12} />}</span>
              <span className="issue-repos-text">{row.repo}</span>
              {row.hint && <small className="issue-repos-hint">{row.hint}</small>}
            </button>
          ),
        )}
        {rows.every((r) => r.kind !== "repo") && filter && <p className="issue-repos-none">{t("No repositories match")}</p>}
      </div>
    </Popover>
  );
}
