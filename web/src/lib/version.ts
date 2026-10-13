// newer: b is a newer x.y.z than a, whatever follows the numbers (a build past a tag, "v0.4.0-24-g5dac31e", is 0.4.0)
export function newer(a: string, b: string): boolean {
  const pa = a.match(/\d+/g)?.slice(0, 3).map(Number) ?? [];
  const pb = b.match(/\d+/g)?.slice(0, 3).map(Number) ?? [];
  if (pa.length < 3 || pb.length < 3) return false;
  for (let i = 0; i < 3; i++) if (pa[i] !== pb[i]) return pb[i]! > pa[i]!;
  return false;
}
