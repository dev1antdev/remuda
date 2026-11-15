import { describe, expect, it } from "vitest";

const sources = import.meta.glob("../**/*.{ts,tsx}", { query: "?raw", import: "default", eager: true }) as Record<string, string>;

// PROMISED are the calls that answer a promise in some browser: Chrome's scroll methods do since 2025, where WebKit's
// answer nothing
const PROMISED = /\.(scrollIntoView|scrollTo|scrollBy|scroll|play|requestFullscreen|then)\(/;

// expressionEffects are the effects written as one expression, whose value React takes for the cleanup
function expressionEffects(source: string): string[] {
  return source.split("\n").filter((line) => /use(Layout)?Effect\(\(\) => [^{(\s]/.test(line));
}

describe("an effect written as one expression", () => {
  it("never hands React a promise for its cleanup: React calls whatever an effect returns", () => {
    const files = Object.entries(sources).filter(([path]) => !path.endsWith(".test.ts"));
    expect(files.length).toBeGreaterThan(200);
    for (const [path, source] of files) for (const line of expressionEffects(source)) expect(line, path).not.toMatch(PROMISED);
  });
  it("is found by the check", () => {
    expect(expressionEffects('  useEffect(() => box.current?.scrollIntoView({ block: "nearest" }), [shown]);')).toHaveLength(1);
    expect('  useEffect(() => box.current?.scrollIntoView({ block: "nearest" }), [shown]);').toMatch(PROMISED);
    expect(expressionEffects("  useEffect(() => {")).toHaveLength(0);
    expect(expressionEffects("  useEffect(() => setError(\"\"), [code]);")).toHaveLength(1);
    expect("  useEffect(() => setError(\"\"), [code]);").not.toMatch(PROMISED);
  });
});
