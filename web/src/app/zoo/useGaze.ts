// useGaze has the zoo's creatures look at the pointer: on a fine pointer, at most once a frame, each creature's pupils
// are moved towards it through --gx / --gy on its SVG (features/roster/zooLife.ts says how far); back to the middle
// when the pointer leaves the window. A touch screen or a TV has no pointer to follow.

import { useEffect, type RefObject } from "react";
import { gaze } from "@/features/roster/zooLife";

export function useGaze(root: RefObject<HTMLElement | null>) {
  useEffect(() => {
    const element = root.current;
    if (!element || !window.matchMedia?.("(pointer: fine)").matches) return;
    let frame = 0;
    let pointer: { x: number; y: number } | null = null;
    const look = () => {
      frame = 0;
      for (const svg of element.querySelectorAll<SVGSVGElement>(".creature")) {
        if (!pointer) {
          svg.style.removeProperty("--gx");
          svg.style.removeProperty("--gy");
          continue;
        }
        const box = svg.getBoundingClientRect();
        const eyes = { x: box.left + box.width / 2, y: box.top + box.height * 0.48 };
        const { x, y } = gaze(eyes, pointer);
        svg.style.setProperty("--gx", String(x));
        svg.style.setProperty("--gy", String(y));
      }
    };
    const onMove = (event: PointerEvent) => {
      pointer = { x: event.clientX, y: event.clientY };
      frame ||= requestAnimationFrame(look);
    };
    const onLeave = () => {
      pointer = null;
      frame ||= requestAnimationFrame(look);
    };
    window.addEventListener("pointermove", onMove, { passive: true });
    document.documentElement.addEventListener("pointerleave", onLeave);
    return () => {
      window.removeEventListener("pointermove", onMove);
      document.documentElement.removeEventListener("pointerleave", onLeave);
      if (frame) cancelAnimationFrame(frame);
    };
  }, [root]);
}
