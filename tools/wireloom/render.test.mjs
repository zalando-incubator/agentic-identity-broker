import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { promisify } from 'node:util';

const exec = promisify(execFile);
const renderer = new URL('./render.mjs', import.meta.url).pathname;

async function withSpec(markdown, check) {
  const dir = await mkdtemp(join(tmpdir(), 'wireloom-render-'));
  try {
    const spec = join(dir, 'spec.md');
    await writeFile(spec, markdown);
    await check(spec, dir);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
}

test('renders a reference-style image despite CRLF spacing; ignores examples inside fences', async () => {
  const markdown = [
    '~~~md',
    '![fake](fake.svg)',
    '',
    '```wireloom',
    'window "Fake":',
    '  text "Ignored"',
    '```',
    '~~~',
    '',
    '![Consent][screen]',
    '',
    '```wireloom title=consent',
    'window "Consent":',
    '  text "Visible"',
    '```',
    '',
    '[screen]: wireframes/consent.svg',
  ].join('\r\n');
  await withSpec(markdown, async (spec, dir) => {
    await exec(process.execPath, [renderer, spec]);
    const svg = await readFile(join(dir, 'wireframes/consent.svg'), 'utf8');
    assert.match(svg, /Visible/);
    await assert.rejects(readFile(join(dir, 'fake.svg'), 'utf8'), { code: 'ENOENT' });
  });
});

test('rejects unpaired Wireloom blocks instead of silently skipping them', async () => {
  await withSpec('```wireloom\nwindow:\n  text "No image"\n```\n', async spec => {
    await assert.rejects(exec(process.execPath, [renderer, spec]), /needs a preceding SVG image link/);
  });
});

test('rejects an SVG path that escapes the spec directory', async () => {
  await withSpec('![Outside](../outside.svg)\n\n```wireloom\nwindow:\n  text "No"\n```\n', async spec => {
    await assert.rejects(exec(process.execPath, [renderer, spec]), /must stay within/);
  });
});
