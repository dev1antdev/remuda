// The sidebar's machines in the person's order: a machine is dragged by its head and dropped before or after another,
// or stepped up and down from its menu. The order shows at once and is kept on the cockpit, so every device lists the
// machines the same way.

import { useEffect, useMemo, useState, type DragEvent } from "react";
import { act } from "@/app/act";
import { setMachineOrder, type Machine } from "./api";
import { dropMark, inOrder, lowerHalf, MACHINE_DRAG, movedBeside, sameOrder, stepped, type Drag } from "./machineOrder";

export function useMachineOrder(machines: Machine[], enabled: boolean) {
  const [drag, setDrag] = useState<Drag | null>(null);
  // the order asked for, shown until the roster lists the machines so
  const [asked, setAsked] = useState<string[] | null>(null);
  const shown = useMemo(() => inOrder(machines, asked), [machines, asked]);
  const ids = useMemo(() => shown.map((machine) => machine.id), [shown]);
  useEffect(() => {
    if (asked && sameOrder(machines.map((machine) => machine.id), ids)) setAsked(null);
  }, [machines, asked, ids]);

  const keep = (next: string[]) => {
    if (sameOrder(next, ids)) return;
    setAsked(next);
    void act(
      setMachineOrder(next).catch((error: unknown) => {
        setAsked(null);
        throw error;
      }),
    );
  };
  const sideOf = (event: DragEvent<HTMLElement>) => {
    const box = event.currentTarget.getBoundingClientRect();
    return lowerHalf(event.clientY, box.top, box.height);
  };

  return {
    shown,
    canMove: enabled && shown.length > 1,
    dragging: (id: string) => drag?.id === id,
    mark: (id: string) => dropMark(ids, drag, id),
    step: (id: string, by: -1 | 1) => keep(stepped(ids, id, by)),
    // the head a machine is dragged by
    head: (id: string) =>
      enabled && shown.length > 1
        ? {
            draggable: true,
            onDragStart: (event: DragEvent<HTMLElement>) => {
              event.dataTransfer.setData(MACHINE_DRAG, id);
              event.dataTransfer.effectAllowed = "move";
              setDrag({ id, over: null, after: false });
            },
            onDragEnd: () => setDrag(null),
          }
        : {},
    // the block a machine is dropped on: its upper half takes the machine before it, its lower half after
    block: (id: string) =>
      drag
        ? {
            onDragOver: (event: DragEvent<HTMLElement>) => {
              event.preventDefault();
              event.dataTransfer.dropEffect = "move";
              const after = sideOf(event);
              if (drag.over !== id || drag.after !== after) setDrag({ ...drag, over: id, after });
            },
            onDrop: (event: DragEvent<HTMLElement>) => {
              event.preventDefault();
              const next = movedBeside(ids, drag.id, id, sideOf(event));
              setDrag(null);
              keep(next);
            },
          }
        : {},
  };
}
