// @vitest-environment node
import { mkdtemp, rm } from 'node:fs/promises';
import {
  createServer as createHTTPServer,
  request,
  type RequestOptions,
  type Server,
} from 'node:http';
import type { AddressInfo } from 'node:net';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createServer, type ViteDevServer } from 'vite';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';

describe('development proxy', () => {
  let backend: Server;
  let frontend: ViteDevServer;
  let origin: string;
  let cacheDir: string;
  const previousTarget = process.env.VITE_API_URL;

  beforeAll(async () => {
    backend = createHTTPServer((req, res) => {
      res.setHeader('Content-Type', 'application/json');
      res.end(
        JSON.stringify({
          path: req.url,
          method: req.method,
          authorization: req.headers.authorization,
          principal: req.headers['x-remote-user'],
        }),
      );
    });
    await new Promise<void>((resolve) =>
      backend.listen(0, '127.0.0.1', resolve),
    );
    const address = backend.address() as AddressInfo;
    process.env.VITE_API_URL = `http://127.0.0.1:${address.port}`;
    cacheDir = await mkdtemp(join(tmpdir(), 'aib-vite-proxy-'));
    frontend = await createServer({
      cacheDir,
      configFile: './vite.config.ts',
      optimizeDeps: { noDiscovery: true, include: [] },
      server: { host: '127.0.0.1', port: 0, open: false, hmr: false },
    });
    await frontend.listen();
    origin = `http://127.0.0.1:${(frontend.httpServer!.address() as AddressInfo).port}`;
  });

  afterAll(async () => {
    await frontend?.close();
    if (cacheDir) await rm(cacheDir, { recursive: true, force: true });
    if (backend)
      await new Promise<void>((resolve) => backend.close(() => resolve()));
    if (previousTarget === undefined) delete process.env.VITE_API_URL;
    else process.env.VITE_API_URL = previousTarget;
  });

  it.each([
    ['GET', '/.well-known/authzen-configuration'],
    ['POST', '/access/v1/evaluation'],
    ['GET', '/api/me'],
  ])('proxies %s %s on the shared Compose origin', async (method, path) => {
    const response = await send(`${origin}${path}`, {
      method,
      headers: {
        Host: 'aib.localhost:3000',
        Authorization: 'Bearer synthetic-test-assertion',
      },
    });
    expect(response.status).toBe(200);
    expect(JSON.parse(response.body)).toEqual({
      path,
      method,
      authorization: 'Bearer synthetic-test-assertion',
      principal: 'dev@example.com',
    });
  });

  it('continues to reject unrelated hostnames', async () => {
    const response = await send(`${origin}/api/me`, {
      headers: { Host: 'untrusted.example' },
    });
    expect(response.status).toBe(403);
  });
});

function send(
  url: string,
  options: RequestOptions,
): Promise<{ status: number | undefined; body: string }> {
  return new Promise((resolve, reject) => {
    const req = request(url, options, (res) => {
      let body = '';
      res.setEncoding('utf8');
      res.on('data', (chunk) => {
        body += chunk;
      });
      res.on('end', () => resolve({ status: res.statusCode, body }));
    });
    req.on('error', reject);
    req.end();
  });
}
