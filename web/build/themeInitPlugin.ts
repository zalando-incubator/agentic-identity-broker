import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import type { Plugin } from 'vite';

export function themeInitPlugin(): Plugin {
  const source = readFileSync(new URL('../src/design-system/theme/theme-init.js', import.meta.url), 'utf8');
  const scriptSrc = `'sha256-${createHash('sha256').update(source).digest('base64')}'`;

  return {
    name: 'theme-init',
    transformIndexHtml: {
      order: 'post',
      handler: () => [{ tag: 'script', children: source, injectTo: 'head-prepend' }],
    },
    generateBundle() {
      this.emitFile({ type: 'asset', fileName: 'csp-hashes.json', source: JSON.stringify({ 'script-src': scriptSrc }) });
    },
  };
}
