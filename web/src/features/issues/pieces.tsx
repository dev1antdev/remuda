// The issues panel's small pieces: a state pill as GitHub draws it, a label in its own colour, a person's face (their
// initial on a tile in a steady accent), and gh's trouble said with the command to run.

import type { CSSProperties, ReactNode } from "react";
import { copyText } from "@/lib/host";
import { actions } from "@/app/store";
import { Icon, Notice, Tip } from "@/ui";
import { t } from "@/lib/i18n";
import type { Item, Label, Person, Trouble } from "./api";
import { hueOf, initials, stateLook } from "./words";

export function StatePill({ issue, filled = false, chevron = false }: { issue: Pick<Item, "state" | "reason">; filled?: boolean; chevron?: boolean }) {
  const look = stateLook(issue);
  return (
    <span className={`issue-state is-${look.tone}${filled ? " is-filled" : ""}`} title={look.title}>
      <Icon name={look.icon} size={12} />
      {look.label}
      {chevron && <Icon name="chevron-down" size={12} />}
    </span>
  );
}

// LabelChip is a label tinted by its own colour, mixed into the theme so it reads on a dark page and a light one
export function LabelChip({ label }: { label: Label }) {
  const style = label.color ? ({ "--label": `#${label.color}` } as CSSProperties) : undefined;
  return (
    <span className={`issue-label${label.color ? "" : " is-plain"}`} style={style} title={label.description || label.name}>
      {label.name}
    </span>
  );
}

export function Labels({ labels, max = 4 }: { labels: Label[]; max?: number }) {
  if (labels.length === 0) return null;
  return (
    <span className="issue-labels">
      {labels.slice(0, max).map((label) => (
        <LabelChip key={label.name} label={label} />
      ))}
      {labels.length > max && <span className="issue-more">+{labels.length - max}</span>}
    </span>
  );
}

export function Face({ person, size = 20 }: { person: Person; size?: 16 | 20 | 24 }) {
  return (
    <span className={`issue-face is-hue-${hueOf(person.login)} is-${size}`} aria-hidden="true">
      {initials(person)}
    </span>
  );
}

// Who is a person as a line: their face, their login, a bot badge
export function Who({ person, strong = true }: { person: Person; strong?: boolean }) {
  return (
    <span className="issue-who">
      <Face person={person} size={16} />
      {strong ? <b>{person.login}</b> : person.login}
      {person.bot && <span className="issue-badge">{t("bot")}</span>}
    </span>
  );
}

// TroubleNote says why gh cannot answer, with the command to run on the machine and a way to try again
export function TroubleNote({ trouble, onRetry, extra }: { trouble: Trouble; onRetry: () => void; extra?: ReactNode }) {
  const copy = (run: string) => void copyText(run).then((ok) => ok && actions.notify(t("Copied"), { brief: true }));
  return (
    <Notice tone="warn" icon="alert-triangle" className="issue-trouble">
      <span>{trouble.text}</span>
      {trouble.run && (
        <span className="issue-run">
          {t("Run there:")} <code>{trouble.run}</code>
          <Tip text={t("Copy the command")}>
            <button type="button" className="icon-btn sm" aria-label={t("Copy the command")} onClick={() => copy(trouble.run!)}>
              <Icon name="copy" size={12} />
            </button>
          </Tip>
        </span>
      )}
      {extra}
      <button type="button" className="linkish" onClick={onRetry}>
        {t("Try again")}
      </button>
    </Notice>
  );
}
