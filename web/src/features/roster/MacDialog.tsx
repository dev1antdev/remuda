// The machine's MAC address, the one a wake-up packet names: set from the machine's ⋯ menu, kept by the server.

import { useState } from "react";
import { act } from "@/app/act";
import { FormDialog, useSubmit } from "@/ui";
import { t } from "@/lib/i18n";
import { setMachineMAC, type Machine } from "./api";
import { isMAC } from "./wake";

export function MacDialog({ machine, onClose }: { machine: Machine; onClose: () => void }) {
  const [mac, setMac] = useState(machine.mac ?? "");
  const submit = useSubmit(async () => {
    const value = mac.trim();
    if (value && !isMAC(value)) throw new Error(t("An address looks like aa:bb:cc:dd:ee:ff"));
    await act(setMachineMAC(machine.id, value));
    onClose();
  });
  return (
    <FormDialog
      title={t("MAC address of {machine}", { machine: machine.label })}
      icon="monitor"
      hint={t("Wake up sends the magic packet to this address on every network this remuda is on — the machine must be on the same one, with Wake-on-LAN allowed in its firmware. Leave it empty to forget the address.")}
      onClose={onClose}
      submit={submit}
      submitLabel={t("Keep")}
      size="sm"
    >
      <label>
        {t("MAC address")}
        <input autoFocus value={mac} maxLength={23} placeholder="aa:bb:cc:dd:ee:ff" spellCheck={false} onChange={(e) => setMac(e.target.value)} />
      </label>
    </FormDialog>
  );
}
