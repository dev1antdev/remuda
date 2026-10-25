// The View menu in the tab strip's tools slot: what the lens on screen can show or hide, in words — its own rows first
// (the chat's timeline, recap and export; the terminal's size, width, selection, text size and key bar), then the page's:
// the files rail and full screen. One labelled button where a row of icons stood.

import { Icon, Menu, type MenuItem } from "@/ui";
import { enterFull, leaveFull } from "./full";
import { actions, app } from "./store";

export default function ViewMenu({ items, shell = false }: { items: MenuItem[]; shell?: boolean }) {
  const footprint = app.use((s) => s.footprint);
  const full = app.use((s) => s.full);
  const page: MenuItem[] = [
    { label: "Files the agent touched", icon: "folder", hint: shell ? "a shell pane has no transcript" : "a rail beside the lens, edited files first", checked: footprint && !shell, disabled: shell, onSelect: actions.toggleFootprint },
    { label: "Full screen", icon: "maximize", hint: "the lens takes the whole screen; Esc brings the page back", shortcut: full ? "Esc" : undefined, checked: full, onSelect: full ? leaveFull : enterFull },
  ];
  return (
    <Menu
      label="View"
      trigger={
        <button type="button" className="btn sm view-btn" aria-label="View options">
          <Icon name="sliders" size={14} />
          <span>View</span>
          <Icon name="chevron-down" size={12} />
        </button>
      }
      items={items.length ? [...items, { sep: true }, ...page] : page}
    />
  );
}
