// The machine's own ⋯ menu: rename, updates, the keep-awake switch, a wake-up for a sleeping machine with a known
// MAC address, the SSH host to copy, whether the screen-sharing mode hides it, and — for one added here — remove.

import { useState } from "react";
import { act } from "@/app/act";
import { actions } from "@/app/store";
import UpdatesDialog from "@/features/updates/Updates";
import { copyText } from "@/lib/host";
import { Icon, Menu, type MenuItem } from "@/ui";
import { setAwake, wakeMachine, type Machine } from "./api";
import { MacDialog } from "./MacDialog";
import { canWake, wokeWords } from "./wake";
import { markedMachine } from "./sharing";
import { toggleSharingMachine, useSharing } from "./useSharing";
import { t } from "@/lib/i18n";

export function MachineMenu({ machine: m, onRename, onRemove }: { machine: Machine; onRename: () => void; onRemove?: () => void }) {
  const [updates, setUpdates] = useState(false);
  const [mac, setMac] = useState(false);
  const share = useSharing();
  const name = share.machineLabel(m);
  const connected = m.state === "connected";
  const wake = () => void act(wakeMachine(m.id).then(({ sent }) => actions.notify(wokeWords(name, sent))));
  // the clipboard the way the rest of the page reaches it: the Clipboard API where the origin is secure, else the fallback
  const copyHost = (host: string) => void copyText(host).then((ok) => actions.notify(ok ? t("SSH host copied") : t("The SSH host could not be copied"), { brief: true, error: !ok }));
  const items = (): MenuItem[] => [
    { label: t("Rename…"), icon: "edit", onSelect: onRename },
    { label: t("Updates…"), icon: "refresh-cw", disabled: !connected, onSelect: () => setUpdates(true) },
    { label: t("Fleet setup…"), icon: "columns", hint: t("every machine side by side"), onSelect: () => actions.openDialog({ kind: "fleet-setup" }) },
    { label: t("Keep awake while agents work"), icon: "sun", checked: !!m.awake?.on, disabled: !connected, onSelect: () => void act(setAwake(m.id, !m.awake?.on)) },
    ...(canWake(m) ? [{ label: t("Wake up"), icon: "zap" as const, onSelect: wake }] : []),
    { label: t("MAC address…"), icon: "plug", hint: m.mac, onSelect: () => setMac(true) },
    { label: t("Leftover worktrees…"), icon: "git-branch", hint: t("worktrees no workspace has open"), disabled: !connected, onSelect: () => actions.openDialog({ kind: "leftovers", machineId: m.id }) },
    { label: t("Hide when sharing the screen"), icon: "eye-off", checked: markedMachine(share.pref, m.id), onSelect: () => toggleSharingMachine(m.id) },
    ...(m.host ? [{ label: t("Copy SSH host"), icon: "copy" as const, onSelect: () => copyHost(m.host!) }] : []),
    ...(onRemove ? [{ sep: true } as const, { label: t("Remove machine"), icon: "trash" as const, danger: true, onSelect: onRemove }] : []),
  ];
  return (
    <>
      <Menu
        label={t("{machine}: rename, wake, copy the SSH host, remove", { machine: name })}
        tip={t("Rename, wake, copy the SSH host, remove")}
        items={items}
        trigger={
          <button type="button" className="icon-btn" data-machine-menu={m.id} aria-label={t("{machine}: rename, wake, copy the SSH host, remove", { machine: name })}>
            <Icon name="more" size={14} />
          </button>
        }
      />
      {updates && <UpdatesDialog machine={m} onClose={() => setUpdates(false)} />}
      {mac && <MacDialog machine={m} onClose={() => setMac(false)} />}
    </>
  );
}
