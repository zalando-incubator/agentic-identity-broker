import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { dirname, isAbsolute, relative, resolve, sep } from 'node:path';
import MarkdownIt from 'markdown-it';
import wireloom from 'wireloom';

const files = process.argv.slice(2);
if (files.length === 0) {
  console.error('usage: node tools/wireloom/render.mjs <markdown-file>...');
  process.exit(2);
}

const markdownParser = new MarkdownIt();

for (const file of files) {
  const tokens = markdownParser.parse(await readFile(file, 'utf8'), {});
  const pairs = [];
  const pairedFences = new Set();

  for (let i = 0; i < tokens.length - 3; i++) {
    if (tokens[i].type !== 'paragraph_open' || tokens[i + 1].type !== 'inline' ||
        tokens[i + 2].type !== 'paragraph_close' || tokens[i + 3].type !== 'fence') continue;
    const children = tokens[i + 1].children.filter(
      child => child.type !== 'text' || child.content.trim() !== '',
    );
    if (children.length !== 1 || children[0].type !== 'image' ||
        tokens[i + 3].info.trim().split(/\s+/)[0] !== 'wireloom') continue;

    const image = children[0].attrGet('src');
    if (!image?.endsWith('.svg') || isAbsolute(image) || /^[a-z][a-z\d+.-]*:/i.test(image)) {
      throw new Error(`Wireloom image must be a relative SVG path in ${file}: ${image}`);
    }
    const directory = resolve(dirname(file));
    const target = resolve(directory, image);
    const relativeTarget = relative(directory, target);
    if (relativeTarget === '..' || relativeTarget.startsWith(`..${sep}`)) {
      throw new Error(`Wireloom image must stay within ${directory}: ${image}`);
    }
    pairs.push({ image, source: tokens[i + 3].content, target });
    pairedFences.add(i + 3);
  }

  for (const [i, token] of tokens.entries()) {
    if (token.type === 'fence' && token.info.trim().split(/\s+/)[0] === 'wireloom' &&
        !pairedFences.has(i)) {
      throw new Error(`Wireloom block in ${file} needs a preceding SVG image link`);
    }
  }
  if (pairs.length === 0) throw new Error(`no image and wireloom source pairs in ${file}`);

  for (const { image, source, target } of pairs) {
    const { svg } = await wireloom.render(image, source);
    await mkdir(dirname(target), { recursive: true });
    await writeFile(target, svg.endsWith('\n') ? svg : `${svg}\n`);
    console.log(`rendered ${target}`);
  }
}
