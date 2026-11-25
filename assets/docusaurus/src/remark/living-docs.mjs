// Remark plugin for living-documentation pages (docs/specification/).
//
// It turns `example:<id>` inline markers into badges that link to the bound
// acceptance scenario, and fenced http blocks with example="<id>" into
// HttpExchange components. The checker in tools/livingdoc reads the same
// Markdown; this plugin only changes how the site presents it.

import fs from 'node:fs';
import path from 'node:path';

const MARKER = /^example:([A-Za-z0-9._-]+)$/;
const BINDING = /(?:\bexample\(|livingdoc\.Example\(|evidence\.ForTest\(\s*\w+\s*,\s*)"([^"]+)"\)/g;
const EXAMPLE_ATTR = /\bexample="([^"]+)"/;
const STATUS_LINE = /^HTTP\/\d(?:\.\d)? (\d{3})\s*(.*)$/;
const REQUEST_LINE = /^([A-Z]+) (\S+)(?: HTTP\/\d(?:\.\d)?)?$/;

function goTestFiles(dir) {
  if (!fs.existsSync(dir)) return [];
  return fs.readdirSync(dir, {withFileTypes: true}).flatMap((entry) => {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) return goTestFiles(full);
    return entry.name.endsWith('_test.go') ? [full] : [];
  });
}

export function findBindings(repoRoot, testDirs) {
  const bindings = {};
  for (const file of [testDirs].flat().flatMap((dir) => goTestFiles(path.join(repoRoot, dir)))) {
    fs.readFileSync(file, 'utf8').split('\n').forEach((line, index) => {
      for (const match of line.matchAll(BINDING)) {
        bindings[match[1]] ??= {file: path.relative(repoRoot, file).split(path.sep).join('/'), line: index + 1};
      }
    });
  }
  return bindings;
}

function parseMessage(lines) {
  const head = [];
  let body = '';
  for (let i = 0; i < lines.length; i++) {
    if (lines[i].trim() === '') {
      body = lines.slice(i + 1).join('\n').trim();
      break;
    }
    if (head.length > 0 && /^\s/.test(lines[i])) head[head.length - 1] += lines[i].trim();
    else head.push(lines[i].trim());
  }
  const headers = head.slice(1).map((line) => {
    const colon = line.indexOf(':');
    return {name: line.slice(0, colon).trim(), value: line.slice(colon + 1).trim()};
  });
  return {start: head[0] ?? '', headers, body};
}

export function parseExchange(text) {
  const lines = text.split('\n');
  const split = lines.findIndex((line) => STATUS_LINE.test(line));
  if (split < 0) return null;
  const request = parseMessage(lines.slice(0, split));
  const response = parseMessage(lines.slice(split));
  const requestLine = REQUEST_LINE.exec(request.start);
  const statusLine = STATUS_LINE.exec(response.start);
  if (!requestLine) return null;
  return {
    request: {method: requestLine[1], target: requestLine[2], headers: request.headers, body: request.body},
    response: {status: Number(statusLine[1]), reason: statusLine[2], headers: response.headers, body: response.body},
  };
}

const attribute = (name, value) => ({type: 'mdxJsxAttribute', name, value});

export default function livingDocs({repoRoot, testDirs = ['tests', 'internal', 'cmd'], statusFile, sourceUrl}) {
  return (tree) => {
    const bindings = findBindings(repoRoot, testDirs);
    const status = statusFile && fs.existsSync(statusFile) ? JSON.parse(fs.readFileSync(statusFile, 'utf8')) : {};
    const walk = (node) => {
      (node.children ?? []).forEach((child, index) => {
        const marker = child.type === 'inlineCode' && MARKER.exec(child.value);
        if (marker) {
          const binding = bindings[marker[1]];
          const attributes = [attribute('example', marker[1])];
          if (status[marker[1]]) attributes.push(attribute('verdict', status[marker[1]]));
          if (binding) attributes.push(attribute('href', sourceUrl(binding.file, binding.line)));
          node.children[index] = {type: 'mdxJsxTextElement', name: 'ExampleBadge', attributes, children: []};
          return;
        }
        if (child.type === 'code' && child.lang === 'http' && EXAMPLE_ATTR.test(child.meta ?? '')) {
          const exchange = parseExchange(child.value);
          if (exchange) {
            node.children[index] = {
              type: 'mdxJsxFlowElement',
              name: 'HttpExchange',
              attributes: [attribute('exchange', JSON.stringify(exchange))],
              children: [],
            };
          }
          return;
        }
        walk(child);
      });
    };
    walk(tree);
  };
}
