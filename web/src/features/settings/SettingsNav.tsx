// The list of Settings' sections: a column beside the section at a desk, a row of chips over it on a phone. The
// section on screen takes the focus as the dialog opens, and the arrows, Home and End walk the list.

import { useEffect, useRef, type KeyboardEvent } from "react";
import { t } from "@/lib/i18n";
import { Icon } from "@/ui";
import { listedIds, stepSection, type SectionGroup, type SectionId } from "./sections";

// tabId names a section's tab, for the pane it labels
export const tabId = (id: SectionId) => `settings-tab-${id}`;

export default function SettingsNav({ groups, shown, onPick }: { groups: SectionGroup[]; shown: SectionId; onPick: (id: SectionId) => void }) {
  const list = useRef<HTMLDivElement>(null);
  const tabOf = (id: SectionId) => list.current?.querySelector<HTMLElement>(`#${tabId(id)}`);
  const focusTab = (id: SectionId) => tabOf(id)?.focus();
  useEffect(() => {
    focusTab(shown);
  }, []); // eslint-disable-line react-hooks/exhaustive-deps
  // a chip half off the row comes whole into view once it is the one on screen
  useEffect(() => {
    tabOf(shown)?.scrollIntoView({ block: "nearest", inline: "nearest" });
  }, [shown]); // eslint-disable-line react-hooks/exhaustive-deps
  const onKey = (e: KeyboardEvent) => {
    const next = stepSection(listedIds(groups), shown, e.key);
    if (!next) return;
    e.preventDefault();
    onPick(next);
    focusTab(next);
  };
  return (
    <div ref={list} className="settings-nav" role="tablist" aria-label={t("Settings")} aria-orientation="vertical" onKeyDown={onKey}>
      {groups.map((group) => (
        <div key={group.id} className="settings-nav-group" role="presentation">
          <span className="settings-nav-head" aria-hidden="true">
            {group.label}
          </span>
          {group.sections.map((section) => (
            <button
              key={section.id}
              id={tabId(section.id)}
              type="button"
              role="tab"
              aria-selected={section.id === shown}
              aria-controls="settings-pane"
              tabIndex={section.id === shown ? 0 : -1}
              data-section={section.id}
              className={`btn ghost settings-nav-item${section.id === shown ? " on" : ""}`}
              onClick={() => onPick(section.id)}
            >
              <Icon name={section.icon} size={16} />
              <span className="settings-nav-label">{section.label}</span>
            </button>
          ))}
        </div>
      ))}
    </div>
  );
}
