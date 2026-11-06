// A picture from the person's computer made into an icon: an SVG small enough is kept as it is, anything else is drawn
// into a square of ICON_SIZE pixels, the whole picture inside it. The pure part; IconDialog draws.

export const ICON_SIZE = 128;
export const MAX_SVG = 64 * 1024;

// pictureKind says how a file becomes an icon: kept as an SVG, drawn as a raster, or refused with the reason
export function pictureKind(type: string, size: number): "svg" | "raster" | { refused: string } {
  if (type === "image/svg+xml") return size <= MAX_SVG ? "svg" : { refused: "An SVG icon can be at most 64 KB" };
  if (/^image\/(png|jpeg|gif|webp|bmp|x-icon|vnd\.microsoft\.icon)$/.test(type)) return "raster";
  return { refused: "Pick a PNG, JPEG, GIF, WebP, ICO or SVG picture" };
}

// fitInside is where a picture of w × h goes in a square of side, centred, its proportions kept
export function fitInside(w: number, h: number, side: number): { x: number; y: number; w: number; h: number } {
  if (w <= 0 || h <= 0) return { x: 0, y: 0, w: side, h: side };
  const scale = Math.min(side / w, side / h);
  const dw = Math.round(w * scale);
  const dh = Math.round(h * scale);
  return { x: Math.round((side - dw) / 2), y: Math.round((side - dh) / 2), w: dw, h: dh };
}

// siteOf turns what was typed into the address a favicon is read from: a scheme added when none was typed
export function siteOf(typed: string): string {
  const text = typed.trim();
  if (!text) return "";
  return /^[a-z][a-z0-9+.-]*:\/\//i.test(text) ? text : `https://${text}`;
}
