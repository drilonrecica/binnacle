// SPDX-License-Identifier: AGPL-3.0-only
// Writes Brotli and gzip variants next to each compressible file in the
// embedded build, so the Go server can serve them without compressing at
// request time (UI-16). A variant is kept only when it is smaller.
import { readdir, readFile, stat, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { brotliCompressSync, constants, gzipSync } from 'node:zlib';

const root = fileURLToPath(
  new URL('../../internal/webembed/dist', import.meta.url),
);
const compressible = /\.(js|css|html|svg|json|webmanifest|txt)$/;
const minimumBytes = 1024;

async function* files(dir) {
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) yield* files(path);
    else yield path;
  }
}

let written = 0;
for await (const path of files(root)) {
  if (!compressible.test(path)) continue;
  if ((await stat(path)).size < minimumBytes) continue;
  const source = await readFile(path);
  const variants = {
    br: brotliCompressSync(source, {
      params: {
        [constants.BROTLI_PARAM_QUALITY]: constants.BROTLI_MAX_QUALITY,
        [constants.BROTLI_PARAM_SIZE_HINT]: source.length,
      },
    }),
    gz: gzipSync(source, { level: 9 }),
  };
  for (const [extension, data] of Object.entries(variants)) {
    if (data.length >= source.length) continue;
    await writeFile(`${path}.${extension}`, data);
    written++;
  }
}
console.log(`precompressed ${written} asset variants`);
