// CollisionNotice is the Changes lens's word on the collision radar: atop its files, a notice per other checkout of the
// repository changing the same files — which files and where, and which of the two should land first. Dismiss sets
// them aside until they say something else.

import { useState } from "react";
import { collisionWords } from "@/features/roster/collide";
import { usePeerName } from "@/features/roster/CollisionMark";
import { useRoster } from "@/features/roster/useRoster";
import { Notice } from "@/ui";

export function CollisionNotice({ machineId, dir }: { machineId: string; dir: string }) {
  const { machines } = useRoster();
  const peerName = usePeerName();
  const [dismissed, setDismissed] = useState("");
  const session = machines.find((machine) => machine.id === machineId)?.sessions.find((candidate) => candidate.dir === dir && candidate.collisions?.length);
  const collisions = session?.collisions ?? [];
  const notes = collisions.map((collision) => ({ collision, lines: collisionWords(collision, peerName(collision, machineId).full) }));
  const shown = notes.map((note) => note.lines.join("\n")).join("\n\n");
  if (notes.length === 0 || dismissed === shown) return null;
  return (
    <div className="review-collisions">
      {notes.map(({ collision, lines }) => (
        <Notice key={`${collision.with.machine}|${collision.with.workspace}`} tone={collision.lines ? "warn" : "info"} icon="zap" className="review-collision" onDismiss={() => setDismissed(shown)}>
          <span className="review-collision-head">{lines[0]}</span>
          <ul className="review-collision-files">
            {lines.slice(1, -1).map((line) => (
              <li key={line}>{line.replace(/^· /, "")}</li>
            ))}
          </ul>
          <span className="review-collision-order">{lines[lines.length - 1]}</span>
        </Notice>
      ))}
    </div>
  );
}
