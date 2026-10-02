import { readFile, readdir, writeFile } from 'node:fs/promises';
import { extname, join, resolve } from 'node:path';
import { brotliCompressSync, gzipSync } from 'node:zlib';
import type { Plugin } from 'vite';

const textExtensions: Record<string, true> = {
  '.css': true, '.csv': true, '.html': true, '.htm': true, '.js': true,
  '.json': true, '.map': true, '.mjs': true, '.svg': true, '.txt': true,
  '.webmanifest': true, '.xml': true,
};

export function assetCompressionPlugin(): Plugin {
  let outputDir: string;
  return {
    name: 'asset-compression',
    apply: 'build',
    configResolved(config) {
      outputDir = resolve(config.root, config.build.outDir);
    },
    async closeBundle() {
      // Unlike generateBundle, this sees both Rollup output and Vite's copied publicDir.
      for (const entry of await readdir(outputDir, { recursive: true, withFileTypes: true })) {
        if (!entry.isFile() || !textExtensions[extname(entry.name).toLowerCase()]) continue;
        const path = join(entry.parentPath, entry.name);
        const bytes = await readFile(path);
        await Promise.all([
          writeFile(`${path}.gz`, gzipSync(bytes)),
          writeFile(`${path}.br`, brotliCompressSync(bytes)),
        ]);
      }
    },
  };
}
