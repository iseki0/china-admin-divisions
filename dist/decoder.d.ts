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
export declare function decodeDivisions(stream: ReadableStream<Uint8Array>, options?: DecodeOptions): AsyncGenerator<DivisionNode[]>;
/** One fetch, no JSON.parse and no JavaScript gzip library. Publish only on success. */
export declare function loadDivisions(url: string | URL, options?: DecodeOptions & {
    fetch?: typeof globalThis.fetch;
}): Promise<DivisionNode[]>;
