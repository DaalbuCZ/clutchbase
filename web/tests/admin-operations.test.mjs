import test from 'node:test';
import assert from 'node:assert/strict';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { createServer } from 'vite';

const vite = await createServer({ configFile: false, root: new URL('..', import.meta.url).pathname, server: { middlewareMode: true, hmr: false, ws: false }, appType: 'custom' });
const { AdminServerFleet } = await vite.ssrLoadModule('/src/AdminServerFleet.tsx');
test.after(() => vite.close());

test('fleet attention and resources follow the latest snapshot', () => {
  const host = { id: 'one', name: 'Friends', machineId: 'm', address: 'localhost', port: 27015, map: 'de_nuke', status: 'stopped', desiredState: 'stopped', observedState: 'stopped', online: true, preset: null, whitelist: null, gameBuildStatus: 'outdated', accessStatus: 'blocked' };
  const machine = { id: 'm', name: 'Host', online: true, resources: { cpuPercent: 25, memoryTotalBytes: 8 * 1024 ** 3, memoryAvailableBytes: 3 * 1024 ** 3 }, storage: { availableBytes: 20 * 1024 ** 3 } };
  const render = (server, node, live) => renderToStaticMarkup(React.createElement(AdminServerFleet, { hosts: [server], machines: [node], live, onSelect() {} }));
  const initial = render(host, machine, true);
  assert.match(initial, /CS2 update available · Access verification blocked/);
  assert.match(initial, /Machine CPU 25.0% · Memory 5.0 \/ 8.0 GiB · 20.0 GiB free/);
  assert.match(render(host, machine, false), /Last received · Machine CPU/);
  const offline = render({ ...host, online: false, status: 'offline', gameBuildStatus: 'current', accessStatus: 'verified' }, { ...machine, online: false }, true);
  assert.match(offline, /Agent disconnected/);
  assert.match(offline, /Machine resources unavailable/);
  assert.doesNotMatch(offline, /25.0%|CS2 update available/);
});
