// The View menu in the tab strip's tools slot: what the lens on screen can show or hide, in words — its own rows first
// (the chat's timeline, recap and export; the terminal's size, width, selection, text size and key bar), then the page's:
// the files rail, the split, full screen and the screen-sharing mode. One labelled button where a row of icons stood.

import { Icon, Menu, type MenuItem } from "@/ui";
import { useMedia } from "@/lib/media";
import { isTouch } from "@/lib/prefs";
import { toggleSharing, useSharing } from "@/features/roster/useSharing";
import { SHARING_KEY_NAME } from "./sharingKey";
import { enterFull, leaveFull } from "./full";
import { SPLIT_QUERY, canSplit } from "./split";
import { actions, app } from "./store";
import { t } from "@/lib/i18n";

export default function ViewMenu({ items, shell = false, page = true }: { items: MenuItem[]; shell?: boolean; page?: boolean }) {
  const footprint = app.use((s) => s.footprint);
  const full = app.use((s) => s.full);
  const split = app.use((s) => s.split);
  const may = canSplit(useMedia(SPLIT_QUERY), isTouch);
  const sharing = useSharing().on;
  // a shell has its terminal alone: no transcript for the files rail, no chat to put beside it
  const agentRows: MenuItem[] = shell
    ? []
    : [
        { label: t("Files the agent touched"), icon: "folder", hint: t("a rail beside the lens, edited files first"), checked: footprint, onSelect: actions.toggleFootprint },
        { label: t("Chat beside the terminal"), icon: "columns", hint: may ? t("the chat on the left, the terminal on the right, a draggable edge between; the lens switch picks the side the tools serve") : t("needs a screen at least 1180 px wide and a mouse"), checked: split && may, disabled: !may, onSelect: actions.toggleSplit },
      ];
  const pageRows: MenuItem[] = [
    ...agentRows,
    { label: t("Full screen"), icon: "maximize", hint: t("the lens takes the whole screen; Esc brings the page back"), shortcut: full ? t("Esc") : undefined, checked: full, onSelect: full ? leaveFull : enterFull },
    { label: t("Screen-sharing mode"), icon: "eye-off", hint: t("the folders and machines you marked, or everything, read as neutral names, their panes covered"), shortcut: SHARING_KEY_NAME, checked: sharing, onSelect: toggleSharing },
  ];
  return (
    <Menu
      label={t("View")}
      trigger={
        <button type="button" className="btn sm view-btn" aria-label={t("View options")}>
          <Icon name="sliders" size={14} />
          <span>{t("View")}</span>
          <Icon name="chevron-down" size={12} />
        </button>
      }
      items={page ? (items.length ? [...items, { sep: true }, ...pageRows] : pageRows) : items}
    />
  );
}
