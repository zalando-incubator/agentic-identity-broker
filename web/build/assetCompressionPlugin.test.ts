// @vitest-environment node
import { mkdtemp, mkdir, readFile, readdir, realpath, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { brotliDecompressSync, gunzipSync } from 'node:zlib';
import { build } from 'vite';
import { expect, it } from 'vitest';
import { assetCompressionPlugin } from './assetCompressionPlugin';

async function outputFiles(directory: string, prefix = ''): Promise<string[]> {
  const files: string[] = [];
  for (const entry of await readdir(join(directory, prefix), { withFileTypes: true })) {
    const name = join(prefix, entry.name);
    if (entry.isDirectory()) files.push(...await outputFiles(directory, name));
    else if (entry.isFile()) files.push(name);
  }
  return files.sort();
}

it('adds negotiable gzip and Brotli companions to generated and copied text without changing originals or compressing binaries', async () => {
  const root = await realpath(await mkdtemp(join(tmpdir(), 'asset-compression-')));
  try {
    const publicDir = join(root, 'public');
    await mkdir(join(publicDir, 'brand'), { recursive: true });
    await mkdir(join(publicDir, 'fonts'), { recursive: true });
    await mkdir(join(publicDir, 'data'), { recursive: true });

    const brand = '<svg xmlns="http://www.w3.org/2000/svg"><title>Brand</title></svg>\n';
    const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScL/nwAAAABJRU5ErkJggg==', 'base64');
    const font = Buffer.from('wOF2\0\0\0\0\xff\x00\x01\x02', 'binary');
    await writeFile(join(root, 'index.html'), '<!doctype html><html><head><meta charset="utf-8"></head><body><div id="app"></div><script type="module" src="/main.js"></script></body></html>');
    await writeFile(join(root, 'main.js'), "import './main.css'; import artwork from './artwork.svg?url'; import image from './pixel.png?url'; document.querySelector('#app').textContent = artwork + image; import('./lazy.js');");
    await writeFile(join(root, 'lazy.js'), "export const message = 'lazy chunk';");
    await writeFile(join(root, 'main.css'), 'body { color: rebeccapurple; }');
    await writeFile(join(root, 'artwork.svg'), '<svg xmlns="http://www.w3.org/2000/svg"><circle r="4"/></svg>');
    await writeFile(join(root, 'pixel.png'), png);
    await writeFile(join(publicDir, 'brand', 'AIB_Wordmark_Black.svg'), brand);
    await writeFile(join(publicDir, 'data', 'settings.json'), '{"consent":true}\n');
    await writeFile(join(publicDir, 'robots.txt'), 'User-agent: *\nDisallow: /api/\n');
    await writeFile(join(publicDir, 'site.webmanifest'), '{"name":"Consent"}\n');
    await writeFile(join(publicDir, 'sitemap.xml'), '<?xml version="1.0"?><urlset/>\n');
    await writeFile(join(publicDir, 'fonts', 'LICENSE.txt'), 'Self-hosted font license\n');
    await writeFile(join(publicDir, 'fonts', 'font.woff2'), font);
    await writeFile(join(publicDir, 'photo.png'), png);
    await writeFile(join(publicDir, 'orphan.gz'), Buffer.from('already gzip'));
    await writeFile(join(publicDir, 'orphan.br'), Buffer.from('already brotli'));

    const baseline = join(root, 'baseline');
    const compressed = join(root, 'compressed');
    const options = { configFile: false as const, root, publicDir, logLevel: 'silent' as const,
      build: { manifest: true, assetsInlineLimit: 0, minify: false as const, emptyOutDir: true } };
    await build({ ...options, build: { ...options.build, outDir: baseline } });
    await build({ ...options, plugins: [assetCompressionPlugin()], build: { ...options.build, outDir: compressed } });

    const originals = await outputFiles(baseline);
    const result = await outputFiles(compressed);
    expect(originals).toContain('index.html');
    expect(originals).toContain(join('.vite', 'manifest.json'));
    expect(originals).toContain(join('brand', 'AIB_Wordmark_Black.svg'));
    expect(originals).toContain(join('data', 'settings.json'));
    expect(originals).toContain(join('fonts', 'LICENSE.txt'));
    expect(originals).toContain('site.webmanifest');
    expect(originals).toContain('sitemap.xml');
    expect(originals.some((name) => /^assets\/.*\.js$/.test(name))).toBe(true);
    expect(originals.some((name) => /^assets\/.*\.css$/.test(name))).toBe(true);
    expect(originals.some((name) => /^assets\/.*\.svg$/.test(name))).toBe(true);
    expect(originals.some((name) => /^assets\/.*\.png$/.test(name))).toBe(true);
    expect(await readFile(join(compressed, 'brand', 'AIB_Wordmark_Black.svg'), 'utf8')).toBe(brand);
    expect(await readFile(join(compressed, 'fonts', 'font.woff2'))).toEqual(font);

    const text = originals.filter((name) => /\.(?:html|js|css|json|svg|txt|webmanifest|xml)$/.test(name));
    expect(result).toEqual([...originals, ...text.flatMap((name) => [`${name}.gz`, `${name}.br`])].sort());
    for (const name of originals) {
      const bytes = await readFile(join(baseline, name));
      expect(await readFile(join(compressed, name)), `Original changed: ${name}`).toEqual(bytes);
      if (text.includes(name)) {
        expect(gunzipSync(await readFile(join(compressed, `${name}.gz`))), `Bad gzip: ${name}`).toEqual(bytes);
        expect(brotliDecompressSync(await readFile(join(compressed, `${name}.br`))), `Bad Brotli: ${name}`).toEqual(bytes);
      }
    }
  } finally {
    await rm(root, { recursive: true, force: true });
  }
}, 30_000);
