// Links, the clipboard and a file to keep, the way a browser does them.

// copyText puts text on the clipboard: the Clipboard API, else execCommand (http origins, older WebViews).
export async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.readOnly = true; // no keyboard on a phone
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    ta.style.fontSize = "16px"; // iOS zooms into a smaller field
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

// saveText hands the browser a text file to keep, named; the download goes through a link the page never shows.
export function saveText(name: string, text: string, type = "text/plain"): void {
  const url = URL.createObjectURL(new Blob([text], { type: `${type};charset=utf-8` }));
  const link = document.createElement("a");
  link.href = url;
  link.download = name;
  document.body.appendChild(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 10_000);
}
