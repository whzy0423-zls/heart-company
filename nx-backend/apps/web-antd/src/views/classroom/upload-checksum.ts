export async function crc64File(
  file: File,
  options: {
    onProgress?: (progress: number) => void;
    signal?: AbortSignal;
  } = {},
) {
  const chunkSize = 256 * 1024;
  let state: [number, number] = [0, 0];
  for (let offset = 0; offset < file.size; offset += chunkSize) {
    if (options.signal?.aborted)
      throw new DOMException('Upload cancelled', 'AbortError');
    const bytes = new Uint8Array(
      await file.slice(offset, offset + chunkSize).arrayBuffer(),
    );
    state = crc64Chunk(bytes, state);
    options.onProgress?.(
      Math.min(100, Math.round(((offset + bytes.length) / file.size) * 100)),
    );
    await new Promise<void>((resolve) => setTimeout(resolve, 0));
  }
  return `crc64:${crc64Decimal(state)}`;
}

function crc64Chunk(
  bytes: Uint8Array,
  seed: [number, number] = [0, 0],
): [number, number] {
  let [high, low] = seed;
  high = ~high >>> 0;
  low = ~low >>> 0;
  for (const byte of bytes) {
    low ^= byte;
    for (let bit = 0; bit < 8; bit++) {
      const lsb = low & 1;
      low = ((low >>> 1) | (high << 31)) >>> 0;
      high >>>= 1;
      if (lsb) {
        high = (high ^ 0xc96c5795) >>> 0;
        low = (low ^ 0xd7870f42) >>> 0;
      }
    }
  }
  return [~high >>> 0, ~low >>> 0];
}

function crc64Decimal([high, low]: [number, number]) {
  if (high === 0 && low === 0) return '0';
  const digits: number[] = [];
  while (high || low) {
    const q = Math.floor(high / 10);
    const r = high - q * 10;
    const combined = r * 0x100000000 + low;
    high = q;
    low = Math.floor(combined / 10);
    digits.push(combined % 10);
  }
  return digits.reverse().join('');
}
