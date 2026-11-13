// Lays commits out on lanes, after git-graph's lanes.js. Commits come newest first, every child before its parents (git log
// --date-order). A lane is a column waiting for one commit and keeps its column until that commit arrives, so lines bend
// only where branches fork or merge. The fold carries its state, so the next page goes on where the last one ended,
// never laid out again from the first commit. A dashed commit (the uncommitted changes) draws its line to its parent
// dashed.
//
// Each row gets the commit's lane and its segments, y 0 the top of the row, 0.5 the dot, 1 the bottom.

export type Segment = { from: number; fromY: number; to: number; toY: number; color: number; dashed: boolean };
export type LaneRow = { lane: number; segments: Segment[] };
export type LaneState = { lanes: (string | null)[]; dashed: boolean[]; width: number }; // lanes[i]: the hash that column waits for
export type LaneCommit = { hash: string; parents: string[]; dashed?: boolean };

export const startLanes = (): LaneState => ({ lanes: [], dashed: [], width: 1 });

// foldLanes lays the commits out after the state the earlier pages left, and answers the state for the next page
export function foldLanes(commits: LaneCommit[], from: LaneState = startLanes()): { rows: LaneRow[]; state: LaneState } {
  const lanes = from.lanes.slice();
  const dashed = from.dashed.slice();
  let width = from.width;
  const free = () => {
    const i = lanes.indexOf(null);
    return i < 0 ? lanes.length : i;
  };
  const rows = commits.map((commit): LaneRow => {
    const before = lanes.slice();
    const wasDashed = dashed.slice();
    const segments: Segment[] = [];
    let lane = before.indexOf(commit.hash);
    if (lane < 0) {
      lane = free(); // a branch tip: nothing above it
      lanes[lane] = commit.hash;
      dashed[lane] = false;
    }
    before.forEach((hash, j) => {
      if (hash === null || hash === undefined) return;
      if (hash === commit.hash) {
        segments.push({ from: j, fromY: 0, to: lane, toY: 0.5, color: j, dashed: !!wasDashed[j] });
        if (j !== lane) {
          lanes[j] = null; // a branch that ends here
          dashed[j] = false;
        }
      } else segments.push({ from: j, fromY: 0, to: j, toY: 1, color: j, dashed: !!wasDashed[j] });
    });
    const [first, ...rest] = commit.parents;
    lanes[lane] = first ?? null;
    dashed[lane] = !!(first && commit.dashed);
    if (first) segments.push({ from: lane, fromY: 0.5, to: lane, toY: 1, color: lane, dashed: dashed[lane]! });
    for (const parent of rest) {
      let k = lanes.indexOf(parent);
      if (k < 0) {
        k = free();
        lanes[k] = parent;
        dashed[k] = !!commit.dashed;
      }
      segments.push({ from: lane, fromY: 0.5, to: k, toY: 1, color: k, dashed: dashed[k]! });
    }
    width = Math.max(width, before.length, lanes.length);
    while (lanes.length && lanes[lanes.length - 1] === null) {
      lanes.pop();
      dashed.length = lanes.length;
    }
    return { lane, segments };
  });
  return { rows, state: { lanes, dashed, width } };
}
