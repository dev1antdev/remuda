import Icon from "./icons";
import { MARKS } from "./marks";
import { BRAND } from "./brand";

// Logo draws the vendor's mark in the vendor's hue (a brand that signs in black or white in ink); an
// agent without a mark gets the generic one, drawn from the UI's own icons so it never depends on a font.
export default function Logo({ agent }: { agent: string }) {
  const mark = MARKS[agent];
  const color = BRAND[agent];
  if (!mark) {
    return <Icon name="asterisk" size={14} className="radar-logo generic" style={{ color }} role="img" aria-label={agent} aria-hidden={undefined} />;
  }
  return (
    <svg className="radar-logo" viewBox={mark.viewBox} style={color ? { color } : undefined} aria-label={agent}>
      {mark.paths.map((d, i) => (
        <path key={i} d={d} fill="currentColor" />
      ))}
    </svg>
  );
}
