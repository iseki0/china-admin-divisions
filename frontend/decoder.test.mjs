import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { gunzipSync } from 'node:zlib';
import { decodeDivisions, loadDivisions } from './decoder.ts';

const wire = gunzipSync(readFileSync(new URL('../data.bin.gz', import.meta.url)));
const source = JSON.parse(readFileSync(new URL('../data.json', import.meta.url), 'utf8'));
function stream(bytes, chunk = 65536) {
  let offset = 0;
  return new ReadableStream({ pull(controller) {
    if (offset === bytes.length) { controller.close(); return; }
    controller.enqueue(bytes.subarray(offset, offset + chunk)); offset = Math.min(offset + chunk, bytes.length);
  }});
}
async function decode(bytes, chunk, options) {
  const nodes = [];
  for await (const batch of decodeDivisions(stream(bytes, chunk), options)) nodes.push(...batch);
  return nodes;
}
test('full data preserves every source field and direct parent indexes', async () => {
  const nodes = await decode(wire, 4093, { batchSize: 1024 });
  const expected = new Map(source.map(n => [n.code, n]));
  assert.equal(nodes.length, source.length);
  for (const { parentIndex, ...node } of nodes) {
    assert.deepEqual(node, expected.get(node.code));
    if (parentIndex !== 0xffffffff) assert.equal(nodes[parentIndex].code, node.parentCode);
  }
});
test('header, UTF-8 and record fields may cross every byte boundary', async () => {
  const firstLength = 20 + wire.readUInt16LE(26) + wire.readUInt16LE(28);
  const single = Buffer.from(wire.subarray(0, 10 + firstLength)); single.writeUInt32LE(1, 6);
  const nodes = await decode(single, 1);
  assert.equal(nodes.length, 1); assert.equal(nodes[0].code, source[0].code);
});
test('rejects truncation, unsupported format, trailing data, invalid UTF-8 and parents', async () => {
  for (const bytes of [wire.subarray(0, 9), wire.subarray(0, -1), Buffer.concat([wire, Buffer.of(0)])]) {
    await assert.rejects(decode(bytes, 65536, { batchSize: 100000 }));
  }
  for (const [offset, value] of [[0, 0], [4, 2], [25, 1], [30, 255]]) {
    const bad = Buffer.from(wire); bad[offset] = value;
    await assert.rejects(decode(bad, 65536, { batchSize: 100000 }));
  }
  const bad = Buffer.from(wire); bad.writeUInt32LE(0, 18);
  await assert.rejects(decode(bad));
});
test('one fetch returns complete data; cancellation and HTTP failures propagate', async () => {
  let requests = 0;
  const nodes = await loadDivisions('https://example.test/data.bin', {
    batchSize: 100000,
    fetch: async () => { requests++; return new Response(stream(wire)); },
  });
  assert.equal(requests, 1); assert.equal(nodes.length, source.length);
  await assert.rejects(loadDivisions('x', { fetch: async () => new Response(null, { status: 500 }) }));
  const controller = new AbortController(); controller.abort();
  await assert.rejects(loadDivisions('x', { signal: controller.signal, fetch: async () => { throw Error('unexpected fetch'); } }), { name: 'AbortError' });
});
test('abort pending read and release upstream on early iterator exit', async () => {
  let cancelled = false;
  const controller = new AbortController();
  const pending = new ReadableStream({ cancel() { cancelled = true; } });
  const iterator = decodeDivisions(pending, { signal: controller.signal });
  const next = iterator.next(); controller.abort();
  await assert.rejects(next, { name: 'AbortError' }); assert.equal(cancelled, true);
  cancelled = false;
  const full = new ReadableStream({start(c) { c.enqueue(wire); }, cancel() { cancelled = true; }});
  for await (const _ of decodeDivisions(full, { batchSize: 1 })) break;
  assert.equal(cancelled, true);
});
