// A folder's icon on the server: set as an emoji or a picture, cleared, and a site's favicon read for the person.

import { sendJSON } from "@/lib/http";
import type { FolderIconRef } from "@/features/roster/api";

export type IconChoice = { emoji: string } | { image: string }; // image: a data URL

export const setFolderIcon = (machine: string, folder: string, choice: IconChoice): Promise<{ icon: FolderIconRef }> => sendJSON("PUT", "/api/folder-icons", { machine, folder, ...choice });

export const clearFolderIcon = (machine: string, folder: string): Promise<unknown> => sendJSON("DELETE", "/api/folder-icons", { machine, folder });

// fetchFavicon has remuda read a site's icon once, now, at the person's word; nothing is kept until they use it
export const fetchFavicon = (url: string): Promise<{ data: string; from: string }> => sendJSON("POST", "/api/folder-icons/favicon", { url });

// IconCandidate is a picture that could be the folder's icon: the project's own file, or its owner's avatar
export type IconCandidate = { data: string; from: string; kind: "file" | "github" | "gitlab" };
export type FoundIcons = { candidates: IconCandidate[]; looked: string[]; notes?: string[] };

// findIconCandidates has the folder's machine look for the pictures its project keeps, and its owner's avatar
export const findIconCandidates = (machine: string, folder: string): Promise<FoundIcons> => sendJSON("POST", "/api/folder-icons/candidates", { machine, folder });
