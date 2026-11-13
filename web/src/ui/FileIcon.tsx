// A file's or a folder's icon from Material Icon Theme, drawn as an image (an SVG as an image runs nothing), its light
// variant while the page wears a light theme.

import { useSyncExternalStore } from "react";
import { fileIconName, fileIconUrl } from "./fileIcons";

const lightNow = () => document.documentElement.dataset.theme === "light";

function watchTheme(changed: () => void) {
  const watcher = new MutationObserver(changed);
  watcher.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
  return () => watcher.disconnect();
}

export function FileIcon({ path, dir = false, open = false, size = 16, className = "" }: { path: string; dir?: boolean; open?: boolean; size?: number; className?: string }) {
  const light = useSyncExternalStore(watchTheme, lightNow, () => false);
  const icon = fileIconName(path, { dir, open, light });
  return <img className={`file-icon ${className}`.trim()} src={fileIconUrl(icon)} width={size} height={size} alt="" aria-hidden="true" draggable={false} decoding="async" data-icon={icon} />;
}
