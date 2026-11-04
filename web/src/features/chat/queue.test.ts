import { describe, expect, it } from "vitest";
import { MAX_NOTES, addNote, noteLine, numbered, queueKey, removeNote, sentAway, type QueuedNote } from "./queue";

const note = (id: string, text: string): QueuedNote => ({ id, text, at: 0 });

describe("the queue", () => {
  it("keeps a pane's notes under its machine, folder and pane", () => {
    expect(queueKey("gog", "/p/api", "w1:p2")).toBe("queue:gog|/p/api|w1:p2");
  });

  it("adds a note at the end, trimmed, and nothing for blanks", () => {
    const one = addNote([], "  check the tests  ", "a", 5);
    expect(one).toEqual([{ id: "a", text: "check the tests", at: 5 }]);
    expect(addNote(one, " \n ", "b", 6)).toBe(one);
    expect(addNote(one, "then the docs", "b", 6).map((n) => n.text)).toEqual(["check the tests", "then the docs"]);
  });

  it("keeps the newest MAX_NOTES", () => {
    let notes: QueuedNote[] = [];
    for (let i = 0; i < MAX_NOTES + 3; i++) notes = addNote(notes, `n${i}`, `${i}`, i);
    expect(notes.length).toBe(MAX_NOTES);
    expect(notes[0]!.text).toBe("n3");
  });

  it("removes one note, and after a send only the ones sent", () => {
    const notes = [note("a", "one"), note("b", "two"), note("c", "three")];
    expect(removeNote(notes, "b").map((n) => n.id)).toEqual(["a", "c"]);
    expect(sentAway([...notes, note("d", "added meanwhile")], [notes[0]!, notes[1]!]).map((n) => n.id)).toEqual(["c", "d"]);
  });

  it("sends one note as it is and several numbered, later lines indented", () => {
    expect(numbered([note("a", "only this")])).toBe("only this");
    expect(numbered([note("a", "first"), note("b", "second\nwith a detail\n\nand more")])).toBe("1. first\n2. second\n   with a detail\n\n   and more");
  });

  it("names a note by its first line", () => {
    expect(noteLine(note("a", "the title\nthe rest"))).toBe("the title");
  });
});
