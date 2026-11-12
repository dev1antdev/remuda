import { describe, expect, it } from "vitest";
import { GifWriter, lzwEncode, minCodeSize, subBlocks } from "./gif";

// lzwDecode is GIF's decoder as a browser runs it, to read back what the encoder wrote
function lzwDecode(data: Uint8Array, min: number): number[] {
  const clear = 1 << min;
  const end = clear + 1;
  let size = min + 1;
  let next = end + 1;
  let table: number[][] = [];
  const reset = () => {
    table = Array.from({ length: clear }, (_, i) => [i]);
    size = min + 1;
    next = end + 1;
  };
  reset();
  const out: number[] = [];
  let at = 0;
  const read = () => {
    let code = 0;
    for (let i = 0; i < size; i++, at++) code |= ((data[at >> 3]! >> (at & 7)) & 1) << i;
    return code;
  };
  let prev: number[] | null = null;
  while (at + size <= data.length * 8) {
    const code = read();
    if (code === clear) {
      reset();
      prev = null;
      continue;
    }
    if (code === end) return out;
    let entry: number[];
    if (code < next && table[code]) entry = table[code]!;
    else if (code === next && prev) entry = [...prev, prev[0]!];
    else throw new Error(`code ${code} past the table's ${next}`);
    out.push(...entry);
    if (prev && next < 4096) {
      table[next++] = [...prev, entry[0]!];
      if (next >= 1 << size && size < 12) size++;
    }
    prev = entry;
  }
  throw new Error("no end code");
}

// frames reads a GIF back: its screen, palette size, and every frame's place, delay and indices
function readGif(bytes: Uint8Array) {
  const text = (from: number, n: number) => String.fromCharCode(...bytes.subarray(from, from + n));
  const u16 = (at: number) => bytes[at]! | (bytes[at + 1]! << 8);
  expect(text(0, 6)).toBe("GIF89a");
  const width = u16(6);
  const height = u16(8);
  const colours = 2 << (bytes[10]! & 7);
  let at = 13 + 3 * colours;
  const frames: { x: number; y: number; width: number; height: number; delay: number; indices: number[] }[] = [];
  let delay = 0;
  let loops = false;
  while (bytes[at] !== 0x3b) {
    if (bytes[at] === 0x21) {
      const label = bytes[at + 1];
      if (label === 0xf9) delay = u16(at + 4);
      if (label === 0xff) loops = text(at + 3, 11) === "NETSCAPE2.0";
      at += 2;
      while (bytes[at] !== 0) at += bytes[at]! + 1;
      at++;
      continue;
    }
    expect(bytes[at]).toBe(0x2c);
    const frame = { x: u16(at + 1), y: u16(at + 3), width: u16(at + 5), height: u16(at + 7), delay, indices: [] as number[] };
    const min = bytes[at + 10]!;
    at += 11;
    const data: number[] = [];
    while (bytes[at] !== 0) {
      data.push(...bytes.subarray(at + 1, at + 1 + bytes[at]!));
      at += bytes[at]! + 1;
    }
    at++;
    frame.indices = lzwDecode(Uint8Array.from(data), min);
    frames.push(frame);
  }
  return { width, height, colours, frames, loops };
}

describe("lzwEncode", () => {
  it("round-trips a frame through GIF's own decoder", () => {
    const indices = Uint8Array.from({ length: 5000 }, (_, i) => (i * 7 + (i >> 5)) % 5);
    expect(lzwDecode(lzwEncode(indices, 3), 3)).toEqual([...indices]);
  });
  it("starts the table over once it is full, and carries on", () => {
    let seed = 7;
    const noise = Uint8Array.from({ length: 60000 }, () => (seed = (seed * 1103515245 + 12345) & 0x7fffffff) % 256);
    expect(lzwDecode(lzwEncode(noise, 8), 8)).toEqual([...noise]);
  });
  it("packs a long run of one colour into few codes", () => {
    const blank = new Uint8Array(100_000);
    const packed = lzwEncode(blank, 2);
    expect(packed.length).toBeLessThan(1200);
    expect(lzwDecode(packed, 2)).toHaveLength(100_000);
  });
  it("writes a clear and an end code for nothing at all", () => {
    expect(lzwDecode(lzwEncode(new Uint8Array(0), 2), 2)).toEqual([]);
  });
});

describe("minCodeSize and subBlocks", () => {
  it("asks at least two bits, and as many as the palette needs", () => {
    expect([1, 2, 4, 5, 16, 17, 256].map(minCodeSize)).toEqual([2, 2, 2, 3, 4, 5, 8]);
  });
  it("cuts the data into blocks of 255 and ends them with an empty one", () => {
    const blocks = subBlocks(new Uint8Array(600).fill(9));
    expect([blocks[0], blocks[256], blocks[512], blocks[blocks.length - 1]]).toEqual([255, 255, 90, 0]);
    expect(blocks.length).toBe(600 + 3 + 1);
  });
});

describe("GifWriter", () => {
  it("lays a looping animation out with its palette, its rectangles and their delays", () => {
    const gif = new GifWriter(4, 3, [0x303446, 0xc6d0f5, 0xa6d189]);
    gif.add({ x: 0, y: 0, width: 4, height: 3, indices: Uint8Array.of(0, 0, 0, 0, 1, 1, 1, 1, 0, 0, 0, 0), delay: 7 });
    gif.add({ x: 0, y: 1, width: 4, height: 1, indices: Uint8Array.of(2, 2, 1, 1), delay: 200 });
    const read = readGif(gif.finish());
    expect(read).toMatchObject({ width: 4, height: 3, colours: 4, loops: true });
    expect(read.frames.map((f) => [f.x, f.y, f.width, f.height, f.delay])).toEqual([
      [0, 0, 4, 3, 7],
      [0, 1, 4, 1, 200],
    ]);
    expect(read.frames[1]!.indices).toEqual([2, 2, 1, 1]);
  });
  it("refuses a palette a GIF cannot hold", () => {
    expect(() => new GifWriter(1, 1, [])).toThrow();
    expect(() => new GifWriter(1, 1, new Array(257).fill(0))).toThrow();
  });
});
