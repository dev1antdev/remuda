// A machine's folder to scan for repositories, and how deep: Manygit's scan setting, kept by the server per machine.

import { useState } from "react";
import { FormDialog, Select, useSubmit } from "@/ui";
import { t, tn } from "@/lib/i18n";
import { setScan, type Scan } from "./api";

const DEPTHS = [1, 2, 3, 4, 5];

export function ScanDialog({ machineId, machineName, scan, onClose, onSaved }: { machineId: string; machineName: string; scan?: Scan; onClose: () => void; onSaved: () => void }) {
  const [folder, setFolder] = useState(scan?.folder ?? "~/");
  const [depth, setDepth] = useState(scan?.depth ?? 3);
  const submit = useSubmit(async () => {
    await setScan(machineId, folder.trim(), depth);
    onSaved();
    onClose();
  });
  return (
    <FormDialog
      title={t("Scan a folder on {machine}", { machine: machineName })}
      icon="folder"
      hint={t("The repositories in this folder join the list beside your workspaces' own. Hidden folders and node_modules, vendor, build, dist and target are skipped; at most 500 are listed. A home folder is too big to scan: pick the one that holds your repositories.")}
      onClose={onClose}
      submit={submit}
      submitLabel={t("Scan")}
      size="sm"
    >
      <label>
        {t("Folder")}
        <input autoFocus value={folder} placeholder="~/code" spellCheck={false} onChange={(e) => setFolder(e.target.value)} />
      </label>
      <label>
        {t("Depth")}
        <Select value={depth} onChange={setDepth} options={DEPTHS.map((n) => ({ value: n, label: tn(n, "{n} folder down", "{n} folders down") }))} />
      </label>
    </FormDialog>
  );
}
