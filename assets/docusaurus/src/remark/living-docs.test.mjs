import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';

import livingDocs, {findBindings, parseExchange} from './living-docs.mjs';

test('parseExchange splits request and response and joins wrapped targets', () => {
  const exchange = parseExchange(
    'PUT /api/agents?external_id=x\n    &dry=1 HTTP/1.1\nContent-Type: application/json\n\n{"a": 1}\n\n' +
      'HTTP/1.1 201 Created\nLocation: /api/agents/1\n\n{"id": 1}',
  );
  assert.deepEqual(exchange.request, {
    method: 'PUT',
    target: '/api/agents?external_id=x&dry=1',
    headers: [{name: 'Content-Type', value: 'application/json'}],
    body: '{"a": 1}',
  });
  assert.deepEqual(exchange.response, {
    status: 201,
    reason: 'Created',
    headers: [{name: 'Location', value: '/api/agents/1'}],
    body: '{"id": 1}',
  });
});

test('plugin replaces markers and example blocks, and leaves other code alone', () => {
  const repoRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'living-docs-'));
  fs.mkdirSync(path.join(repoRoot, 'tests/e2e'), {recursive: true});
  fs.writeFileSync(
    path.join(repoRoot, 'tests/e2e/agents_test.go'),
    'package e2e_test\n\nIt("creates", example("agents.create"), func() {})\n',
  );
  fs.mkdirSync(path.join(repoRoot, 'tests/integration'), {recursive: true});
  fs.writeFileSync(
    path.join(repoRoot, 'tests/integration/config_test.go'),
    'package integration\n\nfunc TestX(t *testing.T) {\n\tevidence.ForTest(t, "config.precedence")\n}\n',
  );
  const statusFile = path.join(repoRoot, 'status.json');
  fs.writeFileSync(statusFile, JSON.stringify({'agents.create': 'passed'}));
  const tree = {
    type: 'root',
    children: [
      {
        type: 'paragraph',
        children: [
          {type: 'inlineCode', value: 'example:agents.create'},
          {type: 'inlineCode', value: 'example:agents.unbound'},
          {type: 'inlineCode', value: 'github-read'},
        ],
      },
      {type: 'code', lang: 'http', meta: 'example="agents.create"', value: 'GET /a HTTP/1.1\n\nHTTP/1.1 200 OK\n\n[]'},
      {type: 'code', lang: 'http', meta: null, value: 'GET /b HTTP/1.1'},
    ],
  };
  livingDocs({repoRoot, statusFile, sourceUrl: (file, line) => `https://example.test/${file}#L${line}`})(tree);

  const [bound, unbound, plain] = tree.children[0].children;
  const attributes = (node) => Object.fromEntries(node.attributes.map((a) => [a.name, a.value]));
  assert.deepEqual(attributes(bound), {
    example: 'agents.create',
    verdict: 'passed',
    href: 'https://example.test/tests/e2e/agents_test.go#L3',
  });
  assert.deepEqual(attributes(unbound), {example: 'agents.unbound'});
  assert.deepEqual(findBindings(repoRoot, 'tests')['config.precedence'], {file: 'tests/integration/config_test.go', line: 4});
  assert.equal(plain.type, 'inlineCode');
  assert.equal(tree.children[1].name, 'HttpExchange');
  assert.equal(JSON.parse(attributes(tree.children[1]).exchange).response.status, 200);
  assert.equal(tree.children[2].type, 'code');
});
