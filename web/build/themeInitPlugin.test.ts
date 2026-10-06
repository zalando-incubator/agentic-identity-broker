// @vitest-environment node

import { createHash } from 'node:crypto';
import { mkdtemp, readFile, realpath, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { afterEach, describe, expect, it } from 'vitest';
import { build, createServer } from 'vite';
import { themeInitPlugin } from './themeInitPlugin';

const fixtureDirs: string[] = [];
const sourceFile = new URL('../src/design-system/theme/theme-init.js', import.meta.url);

async function createFixture(): Promise<string> {
  const root = await realpath(await mkdtemp(path.join(tmpdir(), 'theme-init-plugin-')));
  fixtureDirs.push(root);
  await Promise.all([
    writeFile(path.join(root, 'index.html'), `<!doctype html>
<html lang="en">
  <head>
    <link rel="stylesheet" href="/styles.css">
    <script type="module" src="/entry.js"></script>
  </head>
  <body></body>
</html>`),
    writeFile(path.join(root, 'styles.css'), 'body { color: black; }'),
    writeFile(path.join(root, 'entry.js'), 'document.body.textContent = "ready";'),
  ]);
  return root;
}

function firstHeadScript(html: string): string {
  const head = html.match(/<head\b[^>]*>([\s\S]*?)<\/head>/i)?.[1];
  expect(head).toBeDefined();
  const firstScript = head!.match(/<script\b[^>]*>([\s\S]*?)<\/script>/i);
  expect(firstScript).not.toBeNull();
  expect(head!.slice(0, firstScript!.index)).not.toMatch(/<(?:script|style|link)\b/i);
  return firstScript![1];
}

afterEach(async () => {
  await Promise.all(fixtureDirs.splice(0).map((dir) => rm(dir, { recursive: true, force: true })));
});

describe('first-paint theme HTML', () => {
  it('puts the exact source before page scripts and styles in Vite dev HTML', async () => {
    const root = await createFixture();
    const server = await createServer({ root, configFile: false, plugins: [themeInitPlugin()], server: { middlewareMode: true }, logLevel: 'silent' });
    try {
      const html = await server.transformIndexHtml('/', await readFile(path.join(root, 'index.html'), 'utf8'));
      expect(firstHeadScript(html)).toBe(await readFile(sourceFile, 'utf8'));
    } finally {
      await server.close();
    }
  });

  it('emits a CSP hash of the exact first script in production HTML', async () => {
    const root = await createFixture();
    const outDir = path.join(root, 'dist');
    await build({ root, configFile: false, plugins: [themeInitPlugin()], logLevel: 'silent', build: { outDir } });

    const html = await readFile(path.join(outDir, 'index.html'), 'utf8');
    const injectedScript = firstHeadScript(html);
    expect(injectedScript).toBe(await readFile(sourceFile, 'utf8'));
    const hashes = JSON.parse(await readFile(path.join(outDir, 'csp-hashes.json'), 'utf8'));
    expect(hashes).toEqual({ 'script-src': `'sha256-${createHash('sha256').update(injectedScript).digest('base64')}'` });
  });
});
