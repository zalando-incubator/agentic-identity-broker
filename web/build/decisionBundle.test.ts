import { readFileSync } from 'node:fs';
import path from 'node:path';
import { gzipSync } from 'node:zlib';
import { describe, expect, it } from 'vitest';

interface ManifestChunk {
  file: string;
  imports?: string[];
  dynamicImports?: string[];
  css?: string[];
}

const dist = path.resolve(import.meta.dirname, '../dist');
const manifest: Record<string, ManifestChunk> = JSON.parse(readFileSync(path.join(dist, '.vite/manifest.json'), 'utf8'));
const modules: Record<string, string[]> = JSON.parse(readFileSync(path.join(dist, '.vite/decision-modules.json'), 'utf8'));

function decisionAssets(route: string, includeDeferred: boolean): Set<string> {
  const visited = new Set<string>();
  const assets = new Set<string>();
  function visit(key: string) {
    if (visited.has(key)) return;
    visited.add(key);
    const chunk = manifest[key];
    expect(chunk, `Missing manifest entry: ${key}`).toBeDefined();
    assets.add(chunk.file);
    for (const css of chunk.css ?? []) assets.add(css);
    for (const dependency of chunk.imports ?? []) visit(dependency);
    if (includeDeferred && key !== 'index.html') {
      for (const dependency of chunk.dynamicImports ?? []) visit(dependency);
    }
  }
  visit('index.html');
  visit(route);
  return assets;
}

describe('initial decision bundles', () => {
  it.each(['AgentDecisionPage', 'ApprovalPage'])('%s stays below 190 kB gzip without console-only modules', (page) => {
    const route = `src/pages/${page}.tsx`;
    const assets = decisionAssets(route, false);
    const includedModules = new Set<string>();
    let compressedBytes = 0;
    for (const asset of assets) {
      compressedBytes += gzipSync(readFileSync(path.join(dist, asset))).byteLength;
      if (!asset.endsWith('.js')) continue;
      expect(modules[asset], `Missing module inventory: ${asset}`).toBeDefined();
      for (const module of modules[asset]) {
        includedModules.add(module);
        expect(module).not.toMatch(/(?:@tanstack\/react-table|\/cmdk\/|data-display\/Table\/|advanced\/Command\/|(?:^|\/)(?:motion|framer-motion|motion-dom|motion-utils)\/)/);
      }
    }
    expect(includedModules.has(`src/pages/${page}.tsx`), 'The route must be present in its module inventory').toBe(true);
    console.info(`${page}: ${compressedBytes} bytes gzip`);
    expect(compressedBytes).toBeLessThan(190_000);
    for (const asset of decisionAssets(route, true)) {
      if (!asset.endsWith('.js')) continue;
      expect(modules[asset], `Missing module inventory: ${asset}`).toBeDefined();
      for (const module of modules[asset]) {
        expect(module).not.toMatch(/(?:@tanstack\/react-table|\/cmdk\/|data-display\/Table\/|advanced\/Command\/|(?:^|\/)(?:motion|framer-motion|motion-dom|motion-utils)\/)/);
      }
    }
  });
});
