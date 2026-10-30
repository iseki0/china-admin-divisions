export interface DivisionNode {
  code: string;
  name: string;
  pinyin: string;
  deprecated: false | 'suspected' | true;
  level: number;
  parentCode: string | null;
  parentIndex: number;
}

export interface DecodeOptions {
  signal?: AbortSignal;
  batchSize?: number;
}

/** Decode browser-decompressed bytes. Batches are provisional until EOF succeeds.
 * Parents always precede children; parentIndex indexes the full record sequence.
 */
export async function* decodeDivisions(
  stream: ReadableStream<Uint8Array>,
  options: DecodeOptions = {},
): AsyncGenerator<DivisionNode[]> {
  const batchSize = options.batchSize ?? 512;
  if (!Number.isSafeInteger(batchSize) || batchSize < 1) throw new Error('Invalid batch size');
  const reader = stream.getReader();
  const decoder = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true });
  const codes: string[] = [];
  const levels: number[] = [];
  const seen = new Set<string>();
  let pending = new Uint8Array(0);
  let count: number | undefined;
  let batch: DivisionNode[] = [];
  let complete = false;
  const abort = () => { void reader.cancel(options.signal?.reason).catch(() => {}); };
  options.signal?.addEventListener('abort', abort, { once: true });
  try {
    while (true) {
      options.signal?.throwIfAborted();
      const { value, done } = await reader.read();
      options.signal?.throwIfAborted();
      if (done) break;
      let data: Uint8Array;
      if (pending.length) {
        data = new Uint8Array(pending.length + value.length);
        data.set(pending); data.set(value, pending.length);
      } else data = value;
      const view = new DataView(data.buffer, data.byteOffset, data.byteLength);
      let offset = 0;
      if (count === undefined) {
        if (data.length < 10) { pending = data.slice(); continue; }
        if (view.getUint32(0, false) !== 0x43414442 || view.getUint16(4, true) !== 1) {
          throw new Error('Unsupported division binary format');
        }
        count = view.getUint32(6, true);
        if (count === 0 || count === 0xffffffff) throw new Error('Invalid record count');
        offset = 10;
      }
      while (data.length - offset >= 20 && codes.length < count) {
        const nameLength = view.getUint16(offset + 16, true);
        const pinyinLength = view.getUint16(offset + 18, true);
        const size = 20 + nameLength + pinyinLength;
        if (data.length - offset < size) break;
        const numericCode = view.getUint32(offset, true) + view.getUint32(offset + 4, true) * 2 ** 32;
        const parentIndex = view.getUint32(offset + 8, true);
        const level = data[offset + 12];
        const state = data[offset + 13];
        const width = data[offset + 14];
        if (!Number.isSafeInteger(numericCode) || numericCode > 999999999999 ||
            width < 1 || width > 12 || String(numericCode).length > width ||
            data[offset + 15] !== 0 || state > 2 || level < 1 || level > 4 ||
            (parentIndex === 0xffffffff ? level !== 1 :
              parentIndex >= codes.length || levels[parentIndex] >= level)) {
          throw new Error(`Invalid record ${codes.length}`);
        }
        const code = String(numericCode).padStart(width, '0');
        const name = decoder.decode(data.subarray(offset + 20, offset + 20 + nameLength));
        const pinyin = decoder.decode(data.subarray(offset + 20 + nameLength, offset + size));
        if (seen.has(code) || !name.trim() || !pinyin.trim()) {
          throw new Error(`Invalid record fields: ${code}`);
        }
        const node: DivisionNode = {
          code, name, pinyin, deprecated: ([false, 'suspected', true] as const)[state],
          level, parentCode: parentIndex === 0xffffffff ? null : codes[parentIndex], parentIndex,
        };
        seen.add(code); codes.push(code); levels.push(level);
        batch.push(node); offset += size;
        if (batch.length >= batchSize) {
          yield batch; batch = [];
          // Yield to rendering even when one network chunk contains many records.
          await new Promise<void>(resolve => setTimeout(resolve, 0));
          options.signal?.throwIfAborted();
        }
      }
      pending = data.slice(offset);
      if (codes.length === count && pending.length) throw new Error('Trailing division data');
    }
    if (count === undefined || codes.length !== count || pending.length) throw new Error('Truncated division data');
    if (batch.length) yield batch;
    complete = true;
  } finally {
    options.signal?.removeEventListener('abort', abort);
    if (!complete) await reader.cancel().catch(() => {});
    reader.releaseLock();
  }
}

/** One fetch, no JSON.parse and no JavaScript gzip library. Publish only on success. */
export async function loadDivisions(
  url: string | URL,
  options: DecodeOptions & { fetch?: typeof globalThis.fetch } = {},
): Promise<DivisionNode[]> {
  options.signal?.throwIfAborted();
  const response = await (options.fetch ?? globalThis.fetch)(url, {
    signal: options.signal, cache: 'no-cache',
  });
  if (!response.ok) {
    await response.body?.cancel();
    throw new Error(`Division request failed: ${response.status}`);
  }
  if (!response.body) throw new Error('Missing response stream');
  const nodes: DivisionNode[] = [];
  for await (const batch of decodeDivisions(response.body, options)) {
    for (const node of batch) nodes.push(node);
  }
  return nodes;
}
