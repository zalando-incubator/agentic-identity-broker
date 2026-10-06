import { readFileSync, readdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const directory = join(dirname(fileURLToPath(import.meta.url)), '../../../public/brand');
const assets = readdirSync(directory).filter((name) => name.endsWith('.svg'));

function readSvg(name: string) {
  const source = readFileSync(join(directory, name), 'utf8');
  const document = new DOMParser().parseFromString(source, 'image/svg+xml');
  return { source, document };
}

function pathGeometry(name: string) {
  const { document } = readSvg(name);
  return [...document.querySelectorAll('path')].map((path) => ({
    d: path.getAttribute('d'),
    transform: path.getAttribute('transform'),
  }));
}

describe('self-contained brand artwork', () => {
  it.each(assets)('%s renders without installed fonts or remote resources', (name) => {
    const { source, document } = readSvg(name);
    expect(document.querySelector('parsererror')).toBeNull();
    expect(document.documentElement.localName).toBe('svg');
    expect(document.documentElement.getAttribute('viewBox')).toMatch(/^\d+(?:\.\d+)? \d+(?:\.\d+)? \d+(?:\.\d+)? \d+(?:\.\d+)?$/);
    const withoutNamespace = source.replace(/xmlns="http:\/\/www\.w3\.org\/2000\/svg"/g, '');
    expect(withoutNamespace).not.toMatch(/<text\b|@import|url\s*\(|https?:\/\//i);
    expect(document.querySelector('image, foreignObject, script, use')).toBeNull();
    expect(pathGeometry(name).every(({ d }) => d && /[MLCQ]/i.test(d))).toBe(true);
  });

  it('keeps black and white wordmarks geometrically identical', () => {
    const black = readSvg('AIB_Wordmark_Black.svg').document.documentElement;
    const white = readSvg('AIB_Wordmark_White.svg').document.documentElement;
    expect(black.getAttribute('viewBox')).toBe('47.4 15 406.7 38.5');
    expect(white.getAttribute('viewBox')).toBe(black.getAttribute('viewBox'));
    expect(pathGeometry('AIB_Wordmark_White.svg')).toEqual(pathGeometry('AIB_Wordmark_Black.svg'));
    expect(black.querySelector('text')).toBeNull();
    expect(white.querySelector('text')).toBeNull();
  });
  it('lets the compact mark inherit its surface foreground without a background tile', () => {
    const { document } = readSvg('aib-mark.svg');
    expect(document.documentElement.getAttribute('viewBox')).toBe('10 17.9 46.2 27.7');
    expect(document.documentElement.getAttribute('fill')).toBe('currentColor');
    expect(document.querySelector('rect')).toBeNull();
  });
});