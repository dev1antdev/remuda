import { describe, expect, it } from "vitest";
import type { Review } from "./api";
import { commentsMessage, place, verdictMessage, type DraftComment } from "./draft";

const comment: DraftComment = { id: 1, file: "internal/server/chat.go", line: 42, removed: false, code: "+\tif err != nil {", text: "handle the error\nproperly" };
const removed: DraftComment = { id: 2, file: "web/src/app.ts", line: 7, removed: true, code: "-const x = 1;", text: "why gone?" };

describe("place", () => {
  it("names the folder with its branch and HEAD when known", () => {
    expect(place("/home/me/remuda", null)).toBe("/home/me/remuda");
    expect(place("/home/me/remuda", { branch: "main", head: "1234567890ab" } as Review)).toBe("/home/me/remuda (main at 1234567)");
    expect(place("/home/me/remuda", { branch: "main" } as Review)).toBe("/home/me/remuda (main)");
  });
});

describe("commentsMessage", () => {
  it("numbers the comments with their address, the row quoted and the text indented", () => {
    const msg = commentsMessage([comment, removed], "/x (main)");
    expect(msg.startsWith("Review comments (2) on /x (main):")).toBe(true);
    expect(msg).toContain("1. internal/server/chat.go:42\n   > +\tif err != nil {\n   handle the error\n   properly");
    expect(msg).toContain("2. web/src/app.ts:7 (removed line)\n   > -const x = 1;\n   why gone?");
  });
});

describe("verdictMessage", () => {
  it("approves: the note, the comments, and that the work can move on", () => {
    const msg = verdictMessage("approved", "  nice  ", [comment], "/x");
    expect(msg.startsWith("Review of /x: approved.\n\nnice\n\nComments (1):")).toBe(true);
    expect(msg.endsWith("Any comments above are notes to take along, not blockers.")).toBe(true);
  });
  it("rejects: changes requested, and the work is not finished until approved", () => {
    const msg = verdictMessage("changes", "", [], "/x");
    expect(msg).toBe("Review of /x: changes requested.\n\nWork on what is asked above, then say when the changes are ready for another look. The work is not finished until it is approved.");
  });
});
