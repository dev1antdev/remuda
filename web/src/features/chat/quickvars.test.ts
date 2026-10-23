import { describe, expect, it } from "vitest";
import { expand, needs } from "./quickvars";

describe("quick prompt placeholders", () => {
  it("fills {dir}, {branch} and {file} and leaves what it cannot fill as written", () => {
    expect(expand("Review {file} on {branch} in {dir}", { dir: "/r", branch: "main", file: "/r/a.go" })).toBe("Review /r/a.go on main in /r");
    expect(expand("Commit on {branch}", { dir: "/r" })).toBe("Commit on {branch}");
    expect(expand("Nothing here {other}", { dir: "/r" })).toBe("Nothing here {other}");
    expect(expand("{dir} and {dir}", { dir: "/r" })).toBe("/r and /r");
  });

  it("says which placeholders a text carries", () => {
    expect([...needs("Look at {file}, then {dir}")]).toEqual(["file", "dir"]);
    expect(needs("plain").size).toBe(0);
    expect(needs("{file} {file}").size).toBe(1);
  });
});
