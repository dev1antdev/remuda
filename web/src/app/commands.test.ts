import { describe, expect, it } from "vitest";
import source from "./commands.tsx?raw";

// hookValues are the names useCommands binds before its useCallback: every value it reads from a hook
function hookValues(body: string): string[] {
  const names: string[] = [];
  for (const m of body.matchAll(/const (\w+) = /g)) names.push(m[1]!);
  for (const m of body.matchAll(/const \[([^\]]+)\] = /g)) names.push(...m[1]!.split(",").map((s) => s.trim()).filter(Boolean));
  for (const m of body.matchAll(/const \{ ([^}]+) \} = /g)) names.push(...m[1]!.split(",").map((s) => s.trim()));
  return names;
}

describe("the palette's commands", () => {
  it("are rebuilt whenever a value they read changes: every hook value is in the callback's deps", () => {
    const start = source.indexOf("function useCommands");
    const callback = source.indexOf("return useCallback(", start);
    const values = hookValues(source.slice(start, callback));
    const deps = source.slice(callback).match(/\n {4}\[([^\]]+)\],\n {2}\);/)?.[1]?.split(",").map((s) => s.trim()) ?? [];
    expect(values.length).toBeGreaterThan(20);
    expect(values.filter((name) => !deps.includes(name))).toEqual([]);
  });
});
