// The usage numbers as words: dollars, tokens, how full a context window is.

export const usd = (n: number) => (n >= 100 ? `$${Math.round(n)}` : n >= 10 ? `$${n.toFixed(1)}` : `$${n.toFixed(2)}`);
export const tok = (n: number) => (n >= 1e9 ? `${(n / 1e9).toFixed(1)}B` : n >= 1e6 ? `${(n / 1e6).toFixed(1)}M` : n >= 1e3 ? `${Math.round(n / 1e3)}k` : String(n));

// tokFine is a count as the CLI words it: a tenth under ten thousand ("9.1k"), whole thousands to a million ("418k"),
// then millions with a tenth when there is one ("1M", "1.2M")
export function tokFine(n: number): string {
  const tenth = (x: number) => x.toFixed(1).replace(/\.0$/, "");
  if (n < 1e3) return String(Math.round(n));
  if (n < 1e4) return `${tenth(n / 1e3)}k`;
  if (n < 999_500) return `${Math.round(n / 1e3)}k`;
  if (n < 1e9) return `${tenth(n / 1e6)}M`;
  return `${tenth(n / 1e9)}B`;
}

// how full a context window is: nothing, amber past 70%, red past 90%
export const fillClass = (f: number) => (f >= 0.9 ? " is-red" : f >= 0.7 ? " is-amber" : "");
