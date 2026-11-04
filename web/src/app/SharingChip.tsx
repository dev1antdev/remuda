// SharingChip is the header's mark while the screen-sharing mode is on, and its small menu: hide everything or only the
// folders and machines marked in their ⋯ menus, or turn the mode off. Alt+H turns it on and off (useSharingKey).

import { useEffect } from "react";
import { setSharingAll, toggleSharing, useSharing } from "@/features/roster/useSharing";
import { isMac } from "@/lib/prefs";
import { Icon, Menu } from "@/ui";
import { SHARING_KEY_NAME, sharingKeyAsked } from "./sharingKey";
import { app } from "./store";
import { t, tn } from "@/lib/i18n";

export function SharingChip() {
  const share = useSharing();
  if (!share.on) return null;
  const marks = share.pref.folders.length + share.pref.machines.length;
  return (
    <Menu
      label={t("Screen-sharing mode")}
      align="start"
      tip={share.pref.all ? t("Screen-sharing mode: every folder and machine reads as a neutral name") : t("Screen-sharing mode: the folders and machines you marked read as neutral names")}
      trigger={
        <button type="button" className="btn sm sharing-chip" aria-label={t("Screen-sharing mode is on")}>
          <Icon name="eye-off" size={12} />
          <span>{t("Sharing")}</span>
        </button>
      }
      items={[
        { label: t("Hide everything"), icon: "eye-off", hint: t("every folder and machine reads as a neutral name"), checked: share.pref.all, onSelect: () => setSharingAll(true) },
        { label: t("Hide only what I marked"), icon: "filter", hint: marks ? tn(marks, "{n} marked in a ⋯ menu", "{n} marked in the ⋯ menus") : t("nothing marked yet: “Hide when sharing the screen” in a workspace's or a machine's ⋯ menu"), checked: !share.pref.all, onSelect: () => setSharingAll(false) },
        { sep: true },
        { label: t("Turn screen-sharing off"), icon: "eye", shortcut: SHARING_KEY_NAME, onSelect: toggleSharing },
      ]}
    />
  );
}

// useSharingKey turns the mode on and off on Alt+H by key code (sharingKey.ts)
export function useSharingKey(): void {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const press = { code: e.code, altKey: e.altKey, metaKey: e.metaKey, ctrlKey: e.ctrlKey, shiftKey: e.shiftKey, target: e.target as Element | null };
      if (!sharingKeyAsked(press, !!app.get().dialog, isMac)) return;
      e.preventDefault();
      toggleSharing();
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, []);
}
