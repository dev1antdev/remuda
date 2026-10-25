import { describe, expect, it } from "vitest";
import { choiceFor, CUSTOM, EMPTY_DRAFT, hasModelFlag, hasOptions, lastModelKey, modelChoices, modelOf, modelPlaceholder, optionsFor } from "./startOptions";

describe("modelChoices", () => {
  it("offers the default, the known names and a typed id for claude, nothing for the others", () => {
    const claude = modelChoices("claude").map((c) => c.value);
    expect(claude).toEqual(["", "opus", "sonnet", "haiku", CUSTOM]);
    expect(modelChoices("codex")).toEqual([]);
    expect(modelChoices("pi")).toEqual([]);
  });

  it("knows which kinds take a model and shows an example id for them", () => {
    expect(hasModelFlag("claude")).toBe(true);
    expect(hasModelFlag("kimi")).toBe(true);
    expect(hasModelFlag("pi")).toBe(false);
    expect(modelPlaceholder("gemini")).toContain("gemini");
    expect(modelPlaceholder("pi")).toBe("");
  });
});

describe("optionsFor", () => {
  it("carries only what the draft says, trimmed", () => {
    expect(optionsFor("claude", EMPTY_DRAFT)).toEqual({});
    expect(optionsFor("claude", { ...EMPTY_DRAFT, choice: "opus", args: "  --permission-mode plan ", prompt: " build it\n" })).toEqual({
      model: "opus",
      args: "--permission-mode plan",
      prompt: "build it",
    });
  });

  it("takes the typed id when the choice is another model, or when the kind has no names", () => {
    expect(modelOf("claude", { ...EMPTY_DRAFT, choice: CUSTOM, custom: " claude-opus-4-1 " })).toBe("claude-opus-4-1");
    expect(modelOf("codex", { ...EMPTY_DRAFT, custom: "gpt-5-codex" })).toBe("gpt-5-codex");
    expect(modelOf("codex", { ...EMPTY_DRAFT, choice: "opus", custom: "" })).toBe("");
  });

  it("asks nothing of a shell and no model of a kind without the flag", () => {
    expect(optionsFor("shell", { ...EMPTY_DRAFT, prompt: "ls" })).toEqual({});
    expect(optionsFor("", { ...EMPTY_DRAFT, args: "-x" })).toEqual({});
    expect(optionsFor("pi", { ...EMPTY_DRAFT, custom: "m", args: "--yolo" })).toEqual({ args: "--yolo" });
    expect(hasOptions("pi", { ...EMPTY_DRAFT, custom: "m" })).toBe(false);
    expect(hasOptions("claude", { ...EMPTY_DRAFT, prompt: "go" })).toBe(true);
  });
});

describe("choiceFor", () => {
  it("turns a remembered model back into the draft's choice", () => {
    expect(choiceFor("claude", "")).toEqual({ choice: "", custom: "" });
    expect(choiceFor("claude", "sonnet")).toEqual({ choice: "sonnet", custom: "" });
    expect(choiceFor("claude", "claude-opus-4-1")).toEqual({ choice: CUSTOM, custom: "claude-opus-4-1" });
    expect(choiceFor("codex", "gpt-5-codex")).toEqual({ choice: "", custom: "gpt-5-codex" });
    expect(lastModelKey("codex")).toBe("start.model:codex");
  });
});
