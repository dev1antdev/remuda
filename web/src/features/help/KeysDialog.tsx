// The keyboard sheet behind ? and the palette: every key the page answers, in groups.

import { isMac } from "@/lib/prefs";
import { Dialog } from "@/ui";
import { keyGroups } from "./keys";

export default function KeysDialog({ onClose }: { onClose: () => void }) {
  return (
    <Dialog title="Keyboard" icon="keyboard" size="md" onClose={onClose} className="keys-dialog">
      <div className="keys-groups">
        {keyGroups(isMac).map((group) => (
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
