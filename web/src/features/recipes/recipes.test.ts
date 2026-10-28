import { describe, expect, it } from "vitest";
import { CUSTOM, EMPTY_DRAFT } from "@/features/roster/startOptions";
import type { Recipe } from "./api";
import { draftOf, firstWords, folderFor, lastRecipeKey, machineFor, pickFirst, recipeDirKey, recipeOf, recipeWords, startRequest } from "./recipes";

const agents = [
  { id: "claude", label: "Claude Code" },
  { id: "codex", label: "Codex" },
];

describe("a recipe and the Options fold", () => {
  it("becomes a draft: a known model is a choice, another one a typed id", () => {
    expect(draftOf({ name: "p", agent: "claude", model: "opus", args: "--permission-mode plan", prompt: "Review." })).toEqual({ choice: "opus", custom: "", args: "--permission-mode plan", prompt: "Review." });
    expect(draftOf({ name: "p", agent: "claude", model: "claude-opus-4-1" })).toMatchObject({ choice: CUSTOM, custom: "claude-opus-4-1", args: "", prompt: "" });
    expect(draftOf({ name: "p", agent: "codex", model: "gpt-5" })).toMatchObject({ choice: "", custom: "gpt-5" });
  });

  it("is made from a draft with only what says something, nothing but the agent for a shell", () => {
    expect(recipeOf("Planner", "claude", { choice: "opus", custom: "", args: " --permission-mode plan ", prompt: " Review the diff. " })).toEqual({ name: "Planner", agent: "claude", model: "opus", args: "--permission-mode plan", prompt: "Review the diff." });
    expect(recipeOf("Shell", "shell", { ...EMPTY_DRAFT, prompt: "ignored" })).toEqual({ name: "Shell", agent: "shell" });
    expect(recipeOf("Here", "codex", EMPTY_DRAFT, { dir: " /x ", machine: "gog" })).toEqual({ name: "Here", agent: "codex", dir: "/x", machine: "gog" });
    expect(recipeOf("Any", "codex", EMPTY_DRAFT, { dir: "", machine: "gog" })).toEqual({ name: "Any", agent: "codex" });
  });
});

describe("starting a recipe", () => {
  const scout: Recipe = { id: "s", name: "Scout", agent: "codex", model: "gpt-5", args: "-a", prompt: "Look around.", dir: "/home/me/app", machine: "gog" };

  it("asks the create route for the fields with something in them", () => {
    expect(startRequest(scout, "gog", "/home/me/app")).toEqual({ machine: "gog", dir: "/home/me/app", name: "", agent: "codex", model: "gpt-5", args: "-a", prompt: "Look around." });
    expect(startRequest({ name: "Shell", agent: "shell", prompt: "x" }, "local", "/x")).toEqual({ machine: "local", dir: "/x", name: "", agent: "shell" });
    expect(startRequest({ name: "Bare", agent: "claude" }, "local", "/x")).toEqual({ machine: "local", dir: "/x", name: "", agent: "claude" });
  });

  it("starts in its own folder on its own machine, else where it was started last, else where the person is", () => {
    expect(folderFor(scout, "/else", "/here")).toBe("/home/me/app");
    expect(machineFor(scout, "local")).toBe("gog");
    const any: Recipe = { id: "a", name: "Any", agent: "claude" };
    expect(folderFor(any, "/else", "/here")).toBe("/else");
    expect(folderFor(any, "", "/here")).toBe("/here");
    expect(machineFor(any, "local")).toBe("local");
    expect(machineFor({ ...any, machine: "gog" }, "local")).toBe("local");
  });

  it("remembers by folder and by recipe, and starts on the remembered one while it exists", () => {
    expect(lastRecipeKey("gog", "/home/me/app")).toBe("recipe.last:gog|/home/me/app");
    expect(recipeDirKey("s")).toBe("recipe.dir:s");
    const list = [scout, { id: "a", name: "Any", agent: "claude" }];
    expect(pickFirst(list, "a")?.name).toBe("Any");
    expect(pickFirst(list, "gone")?.name).toBe("Scout");
    expect(pickFirst([], "a")).toBeNull();
  });
});

describe("the words", () => {
  it("say the agent, the model by name, the arguments, the prompt's start and the folder", () => {
    expect(recipeWords({ name: "p", agent: "claude", model: "opus", args: "--permission-mode plan", prompt: "Review the diff and say what is wrong with it, in one line per finding.", dir: "/home/me/app" }, agents)).toBe(
      "Claude Code · Opus · --permission-mode plan · Review the diff and say what is wrong with it,… · only in app",
    );
    expect(recipeWords({ name: "p", agent: "codex", model: "gpt-5" }, agents)).toBe("Codex · gpt-5");
    expect(recipeWords({ name: "p", agent: "shell" }, agents)).toBe("Plain shell");
    expect(recipeWords({ name: "p", agent: "kimi" }, agents)).toBe("kimi");
  });

  it("cut a prompt at a word", () => {
    expect(firstWords("short", 10)).toBe("short");
    expect(firstWords("one two three four", 9)).toBe("one two…");
    expect(firstWords("first line\nsecond", 40)).toBe("first line");
    expect(firstWords("averyveryverylongwordwithoutaspace", 10)).toBe("averyveryv…");
  });
});
