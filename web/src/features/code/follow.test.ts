import { describe, expect, it } from "vitest";
import type { Turn } from "@/features/chat/api";
import { newestEdit } from "./follow";

const assistant = (...parts: Turn["parts"]): Turn => ({ role: "assistant", parts });

describe("newestEdit", () => {
  it("is the last finished edit, by its tool id", () => {
    const turns = [
      assistant({ type: "tool", tool_name: "Edit", tool_id: "t1", input: { file_path: "/w/a.go" }, output: "ok" }),
      assistant({ type: "tool", tool_name: "Write", tool_id: "t2", input: { file_path: "/w/b.go" }, output: "ok" }, { type: "tool", tool_name: "Read", tool_id: "t3", input: { file_path: "/w/c.go" }, output: "…" }),
    ];
    expect(newestEdit(turns)).toEqual({ id: "t2", path: "/w/b.go" });
  });

  it("waits for the result and skips a failed edit", () => {
    expect(newestEdit([assistant({ type: "tool", tool_name: "Edit", tool_id: "t1", input: { file_path: "/w/a.go" } })])).toBeNull();
    expect(newestEdit([assistant({ type: "tool", tool_name: "Edit", tool_id: "t1", input: { file_path: "/w/a.go" }, output: "no such file", is_error: true })])).toBeNull();
    expect(newestEdit([{ role: "user", parts: [{ type: "text", text: "edit a.go" }] }])).toBeNull();
  });
});
