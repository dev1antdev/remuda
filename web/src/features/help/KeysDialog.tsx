// The keyboard sheet behind ? and the palette: every key the page answers, in groups — for a viewer, the ones it has.

import { useRoster } from "@/features/roster/useRoster";
import { isMac } from "@/lib/prefs";
import { Dialog } from "@/ui";
import { keyGroups } from "./keys";
import { t } from "@/lib/i18n";

export default function KeysDialog({ onClose }: { onClose: () => void }) {
  const { viewOnly } = useRoster();
  return (
    <Dialog title={t("Keyboard")} icon="keyboard" size="md" onClose={onClose} className="keys-dialog">
      <div className="keys-groups">
        {keyGroups(isMac, viewOnly).map((group) => (
          <section key={group.title} className="keys-group">
            <h4>{group.title}</h4>
            {group.rows.map((row) => (
              <div key={row.what} className="keys-row">
                <span className="keys-caps">
                  {row.keys.map((cap) => (
                    <kbd key={cap}>{cap}</kbd>
                  ))}
                </span>
                <span>{row.what}</span>
              </div>
            ))}
          </section>
        ))}
      </div>
    </Dialog>
  );
}
