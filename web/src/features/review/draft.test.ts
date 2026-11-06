import { describe, expect, it } from "vitest";
import type { Review } from "./api";
import { COMMIT_ASK, PR_ASK, commentsMessage, draftOnRow, place, verdictMessage, type DraftComment } from "./draft";

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
    expect(msg).toBe("Review of /x: changes requested.\n\nWork on what is asked above, then say when the changes are ready for another look, ending that answer with a line reading READY FOR REVIEW. The work is not finished until it is approved.");
  });
});

describe("verdictMessage with a commit asked", () => {
  it("asks the agent to commit only on an approval that wants it", () => {
    expect(verdictMessage("approved", "", [], "/x", "commit").endsWith(COMMIT_ASK)).toBe(true);
    expect(verdictMessage("approved", "", [], "/x")).not.toContain(COMMIT_ASK);
    expect(verdictMessage("changes", "fix it", [], "/x", "commit")).not.toContain(COMMIT_ASK);
  });
});

describe("verdictMessage with a pull request asked", () => {
  it("asks for the commit, the push and the pull request, and its link, on an approval only", () => {
    const msg = verdictMessage("approved", "base is develop", [], "/x", "pr");
    expect(msg.endsWith(PR_ASK)).toBe(true);
    expect(msg).not.toContain(COMMIT_ASK);
    expect(PR_ASK).toContain("gh pr create");
    expect(PR_ASK).toContain("link");
    expect(verdictMessage("approved", "", [], "/x", "commit")).not.toContain(PR_ASK);
    expect(verdictMessage("changes", "fix it", [], "/x", "pr")).not.toContain(PR_ASK);
  });
});

describe("a comment without a row's text", () => {
  const adopted: DraftComment = { id: 3, file: "src/a.go", line: 6, removed: false, code: "", text: "[high] a race — second opinion of Codex" };
  it("is quoted by nothing in the message", () => {
    expect(commentsMessage([adopted], "/x")).toContain("1. src/a.go:6\n   [high] a race");
  });
  it("on a file as a whole is addressed by the file, never a line 0", () => {
    expect(commentsMessage([{ ...adopted, line: 0 }], "/x")).toContain("1. src/a.go (the whole file)\n   [high] a race");
  });
  it("sits under the new file's line it names; one with a row's text only under that very row", () => {
    expect(draftOnRow(adopted, { kind: "add", text: "+x", newLine: 6, oldLine: null })).toBe(true);
    expect(draftOnRow(adopted, { kind: "", text: " x", newLine: 6, oldLine: 6 })).toBe(true);
    expect(draftOnRow(adopted, { kind: "del", text: "-x", newLine: null, oldLine: 6 })).toBe(false);
    expect(draftOnRow(comment, { kind: "add", text: "+\tif err != nil {", newLine: 42, oldLine: null })).toBe(true);
    expect(draftOnRow(comment, { kind: "add", text: "+other", newLine: 42, oldLine: null })).toBe(false);
    expect(draftOnRow(removed, { kind: "del", text: "-const x = 1;", newLine: null, oldLine: 7 })).toBe(true);
  });
});
