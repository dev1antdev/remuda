// A Select's options as its list draws them: gathered under their group, the groups in the order they first come. The
// keys and the pointer walk the options in this order too, or with the groups interleaved in the given list (the browser's
// voices, by name, grouped by language) a hover would focus and scroll to another option than the one under it.

export type Grouped<T> = { name: string | undefined; items: T[] };

export function groupsOf<T extends { group?: string }>(options: T[]): Grouped<T>[] {
  const groups: Grouped<T>[] = [];
  for (const option of options) {
    const group = groups.find((g) => g.name === option.group);
    if (group) group.items.push(option);
    else groups.push({ name: option.group, items: [option] });
  }
  return groups;
}

// shownOrder is every option in the order the list draws them
export const shownOrder = <T,>(groups: Grouped<T>[]): T[] => groups.flatMap((g) => g.items);
