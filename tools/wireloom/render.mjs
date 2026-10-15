// Renders each Markdown image link that is directly followed by its `wireloom` source block
// into the SVG file the link names, relative to the Markdown file.
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import wireloom from 'wireloom';

const files = process.argv.slice(2);
if (files.length === 0) {
  console.error('usage: node tools/wireloom/render.mjs <markdown-file>...');
  process.exit(2);
}

const imageAndSource = /!\[[^\]]*\]\(([^)\s]+\.svg)\)\n\n```wireloom\n([\s\S]*?)```/g;

for (const file of files) {
  const markdown = await readFile(file, 'utf8');
  const pairs = [...markdown.matchAll(imageAndSource)];
  if (pairs.length === 0) {
    throw new Error(`no image and wireloom source pairs in ${file}`);
  }
  for (const [, image, source] of pairs) {
    const target = join(dirname(file), image);
    const { svg } = await wireloom.render(image, source);
    await mkdir(dirname(target), { recursive: true });
    await writeFile(target, svg.endsWith('\n') ? svg : `${svg}\n`);
    console.log(`rendered ${target}`);
  }
}
