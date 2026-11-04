// FolderIcon is a folder's face before its name: the emoji or picture the person gave it, else its initial on a tile.

import type { CSSProperties } from "react";
import { iconURL, initialOf, tileColour } from "./folderFace";

export function FolderIcon({ icon, name, size = 16 }: { icon?: { emoji?: string; image?: string }; name: string; size?: number }) {
  const box = { "--folder-icon": `${size}px` } as CSSProperties;
  if (icon?.image) return <img className="folder-icon" src={iconURL(icon.image)} alt="" width={size} height={size} draggable={false} style={box} />;
  if (icon?.emoji) {
    return (
      <span className="folder-icon emoji" aria-hidden="true" style={box}>
        {icon.emoji}
      </span>
    );
  }
  return (
    <span className="folder-icon letter" aria-hidden="true" style={{ ...box, background: `var(--ctp-${tileColour(name)})` }}>
      {initialOf(name)}
    </span>
  );
}
