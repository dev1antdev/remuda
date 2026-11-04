import { describe, expect, it } from "vitest";
import { fromWords, lookedWords, noteWords } from "./autoicon";

describe("fromWords", () => {
  it("names a file by its path and an avatar by its owner", () => {
    expect(fromWords({ data: "", from: "public/favicon.svg", kind: "file" })).toBe("public/favicon.svg");
    expect(fromWords({ data: "", from: "github.com/acme", kind: "github" })).toBe("GitHub avatar · acme");
    expect(fromWords({ data: "", from: "gitlab.gog.com/gog-backend/games-db", kind: "gitlab" })).toBe("GitLab avatar · gog-backend/games-db");
  });
});

describe("lookedWords", () => {
  it("lists the folders searched and why no avatar came", () => {
    expect(lookedWords({ candidates: [], looked: ["./", "public/"], notes: ["no origin remote, so no owner's avatar"] })).toBe("Looked in ./, public/. No origin remote, so no owner's avatar.");
    expect(lookedWords({ candidates: [], looked: ["./"] })).toBe("Looked in ./.");
    expect(noteWords(["no glab on GOG to read the GitLab avatar"])).toBe("No glab on GOG to read the GitLab avatar.");
    expect(noteWords()).toBe("");
  });
});
