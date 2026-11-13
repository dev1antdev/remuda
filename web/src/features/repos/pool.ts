// FETCH_AT_ONCE is how many fetches Fetch all runs at a time, as Manygit does
export const FETCH_AT_ONCE = 8;

// pooled runs task over the items in their order, at most limit at a time, and ends when the last one has
export async function pooled<T>(items: T[], limit: number, task: (item: T) => Promise<void>): Promise<void> {
  let next = 0;
  const worker = async () => {
    while (next < items.length) await task(items[next++]!);
  };
  await Promise.all(Array.from({ length: Math.max(0, Math.min(limit, items.length)) }, worker));
}
