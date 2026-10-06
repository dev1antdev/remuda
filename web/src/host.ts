// Links and the clipboard, the way a browser does them.

// copyText puts text on the clipboard: the Clipboard API, else the old
// execCommand path (http origins, older WebViews).
export async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand("copy");
    ta.remove();
    return ok;
  }
}

// openUrl opens a link in a new tab.
export function openUrl(url: string): void {
  window.open(url, "_blank", "noopener,noreferrer");
}
