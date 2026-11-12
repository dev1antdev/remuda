// CollisionMark is the quiet mark a workspace wears while another checkout of its repository changes the same files:
// a bolt and the other's name, amber when the lines themselves touch, the whole story in its tooltip. The sidebar
// card, the wall tile and the review queue wear it; usePeerName names the other the way the screen-sharing mode allows.

import { useRoster } from "./useRoster";
import { useSharing } from "./useSharing";
import { folderOf } from "./workspaces";
import { collisionWords, gravest } from "./collide";
import type { Collision, Session } from "./api";
import { Icon } from "@/ui";
import { t, tn } from "@/lib/i18n";

// Peer is the workspace a collision is with, by name, and by name and machine when it stands on another one
export type Peer = { name: string; full: string };

// usePeerName names the workspace a collision is with, the neutral names while the screen-sharing mode hides it
export function usePeerName(): (collision: Collision, fromMachine: string) => Peer {
  const { machines } = useRoster();
  const share = useSharing();
  return (collision, fromMachine) => {
    const machine = machines.find((candidate) => candidate.id === collision.with.machine);
    const session = machine?.sessions.find((candidate) => candidate.id === collision.with.workspace);
    const hidden = machine && session ? share.sessionHidden(machine.id, session) : false;
    const name = hidden && machine && session ? share.folderName(machine.id, folderOf(session), session.name) : collision.with.name;
    const full = machine && machine.id !== fromMachine ? t("{name} on {machine}", { name, machine: share.machineLabel(machine) }) : name;
    return { name, full };
  };
}

// collisionTip is every collision of a workspace in words, the gravest first
export function collisionTip(collisions: Collision[], fromMachine: string, peerName: ReturnType<typeof usePeerName>): string {
  const collision = gravest(collisions);
  if (!collision) return "";
  const more = collisions.length - 1;
  return [...collisionWords(collision, peerName(collision, fromMachine).full), ...(more > 0 ? [tn(more, "And {n} more workspace of the repository.", "And {n} more workspaces of the repository.")] : [])].join("\n");
}

// CollisionMark is the bolt and the other's name; compact, the bolt alone (the sidebar card's first line, beside its badges)
export function CollisionMark({ machineId, session, className = "", compact = false }: { machineId: string; session: Session; className?: string; compact?: boolean }) {
  const peerName = usePeerName();
  const collision = gravest(session.collisions);
  if (!collision) return null;
  const words = collisionTip(session.collisions ?? [], machineId, peerName);
  return (
    <span className={`collide-mark${collision.lines ? " is-lines" : ""}${className ? ` ${className}` : ""}`} title={words} aria-label={words}>
      <Icon name="zap" size={12} />
      {!compact && <span className="collide-name">{peerName(collision, machineId).name}</span>}
    </span>
  );
}
