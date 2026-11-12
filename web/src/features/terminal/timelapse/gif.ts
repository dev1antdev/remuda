// A GIF89a encoder small enough to keep here: a global palette, the frames as rectangles of palette indices laid over
// the one before (only the rows that moved), each with its delay, looping. The LZW and the bit packing are GIF's own:
// variable-width codes from minCodeSize + 1 bits up to 12, least significant bit first, a clear code when the table is
// full. Pure: the canvas work that makes the indices lives in raster.ts.

export const MAX_CODES = 4096;

// BitWriter packs codes least significant bit first, as GIF's image data is
class BitWriter {
  private bytes: number[] = [];
  private acc = 0;
  private bits = 0;

  write(code: number, size: number) {
    this.acc |= code << this.bits;
    this.bits += size;
    while (this.bits >= 8) {
      this.bytes.push(this.acc & 0xff);
      this.acc >>>= 8;
      this.bits -= 8;
    }
  }

  finish(): Uint8Array {
    if (this.bits > 0) this.bytes.push(this.acc & 0xff);
    this.acc = 0;
    this.bits = 0;
    return Uint8Array.from(this.bytes);
  }
}

// lzwEncode is GIF's LZW over a frame's palette indices: a clear code first, the end code last
export function lzwEncode(indices: Uint8Array, minCodeSize: number): Uint8Array {
  const clear = 1 << minCodeSize;
  const end = clear + 1;
  const out = new BitWriter();
  let codeSize = minCodeSize + 1;
  let next = end + 1;
  let table = new Map<number, number>();
  out.write(clear, codeSize);
  if (indices.length === 0) {
    out.write(end, codeSize);
    return out.finish();
  }
  let prefix = indices[0]!;
  for (let i = 1; i < indices.length; i++) {
    const k = indices[i]!;
    const key = (prefix << 8) | k;
    const found = table.get(key);
    if (found !== undefined) {
      prefix = found;
      continue;
    }
    out.write(prefix, codeSize);
    if (next === MAX_CODES) {
      out.write(clear, codeSize);
      table = new Map();
      codeSize = minCodeSize + 1;
      next = end + 1;
    } else {
      // the decoder widens its codes once its table reaches the next power of two, one entry behind this one
      if (next >= 1 << codeSize) codeSize++;
      table.set(key, next++);
    }
    prefix = k;
  }
  out.write(prefix, codeSize);
  out.write(end, codeSize);
  return out.finish();
}

// minCodeSize is the bits a palette of that many colours needs, at least 2 as GIF asks
export function minCodeSize(colours: number): number {
  let bits = 2;
  while (1 << bits < colours) bits++;
  return bits;
}

export type GifFrame = { x: number; y: number; width: number; height: number; indices: Uint8Array; delay: number }; // delay in hundredths of a second

// GifWriter lays the file out: the header, the screen and its palette, the loop, every frame, the trailer
export class GifWriter {
  private parts: Uint8Array[] = [];
  private readonly codeSize: number;

  constructor(
    readonly width: number,
    readonly height: number,
    palette: number[], // 0xRRGGBB, at most 256
  ) {
    if (palette.length === 0 || palette.length > 256) throw new Error(`a GIF palette holds 1 to 256 colours, not ${palette.length}`);
    this.codeSize = minCodeSize(palette.length);
    const tableBits = Math.max(1, Math.ceil(Math.log2(palette.length)));
    const table = new Uint8Array(3 << tableBits);
    palette.forEach((rgb, i) => table.set([(rgb >> 16) & 0xff, (rgb >> 8) & 0xff, rgb & 0xff], i * 3));
    this.parts.push(ascii("GIF89a"), u16(width), u16(height), Uint8Array.of(0x80 | (7 << 4) | (tableBits - 1), 0, 0), table);
    // the NETSCAPE2.0 extension: loop for ever
    this.parts.push(Uint8Array.of(0x21, 0xff, 0x0b), ascii("NETSCAPE2.0"), Uint8Array.of(0x03, 0x01, 0, 0, 0));
  }

  add(frame: GifFrame) {
    const delay = Math.max(0, Math.min(0xffff, Math.round(frame.delay)));
    // a graphic control extension: left in place (disposal 1), so the next frame's rectangle lands over this one
    this.parts.push(Uint8Array.of(0x21, 0xf9, 0x04, 1 << 2), u16(delay), Uint8Array.of(0, 0));
    this.parts.push(Uint8Array.of(0x2c), u16(frame.x), u16(frame.y), u16(frame.width), u16(frame.height), Uint8Array.of(0));
    this.parts.push(Uint8Array.of(this.codeSize), subBlocks(lzwEncode(frame.indices, this.codeSize)));
  }

  finish(): Uint8Array {
    this.parts.push(Uint8Array.of(0x3b));
    const size = this.parts.reduce((n, part) => n + part.length, 0);
    const out = new Uint8Array(size);
    let at = 0;
    for (const part of this.parts) {
      out.set(part, at);
      at += part.length;
    }
    return out;
  }
}

// subBlocks cuts data into GIF's sub-blocks, each at most 255 bytes after its length, and ends them with an empty one
export function subBlocks(data: Uint8Array): Uint8Array {
  const out = new Uint8Array(data.length + Math.ceil(data.length / 255) + 1);
  let at = 0;
  for (let i = 0; i < data.length; i += 255) {
    const block = data.subarray(i, i + 255);
    out[at++] = block.length;
    out.set(block, at);
    at += block.length;
  }
  out[at] = 0;
  return out;
}

const u16 = (n: number) => Uint8Array.of(n & 0xff, (n >> 8) & 0xff);
const ascii = (text: string) => Uint8Array.from(text, (ch) => ch.charCodeAt(0));
