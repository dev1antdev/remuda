// What a path link opens: a file on the pane's machine, at a line, the file itself or its changes first.
export type FileTarget = { path: string; line?: number; prefer?: "file" | "changes" };

export type OpenFile = (t: FileTarget) => void;
