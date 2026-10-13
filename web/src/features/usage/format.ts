// The usage numbers as words: dollars, tokens, how full a context window is.

export const usd = (n: number) => (n >= 100 ? `$${Math.round(n)}` : n >= 10 ? `$${n.toFixed(1)}` : `$${n.toFixed(2)}`);
export const tok = (n: number) => (n >= 1e9 ? `${(n / 1e9).toFixed(1)}B` : n >= 1e6 ? `${(n / 1e6).toFixed(1)}M` : n >= 1e3 ? `${Math.round(n / 1e3)}k` : String(n));

// how full a context window is: nothing, amber past 70%, red past 90%
export const fillClass = (f: number) => (f >= 0.9 ? " is-red" : f >= 0.7 ? " is-amber" : "");
