import { MARKS } from "@/ui/marks";
import { BRAND } from "./brand";

// Logo draws the vendor's mark in the vendor's hue (a brand that signs in black or white in ink); an
// agent without a mark gets a generic one.
export default function Logo({ agent }: { agent: string }) {
  const mark = MARKS[agent];
  const color = BRAND[agent];
  if (!mark) {
    return (
      <span className="radar-logo generic" style={{ color }} title={agent}>
        ✳
      </span>
    );
  }
  return (
    <svg className="radar-logo" viewBox={mark.viewBox} style={color ? { color } : undefined} aria-label={agent}>
      {mark.paths.map((d, i) => (
        <path key={i} d={d} fill="currentColor" />
      ))}
    </svg>
  );
}
