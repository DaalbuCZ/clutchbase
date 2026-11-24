import test from 'node:test';
import assert from 'node:assert/strict';
import { connectLive } from '../src/live.ts';

test('live stream authenticates in a frame, reconnects, and disables stale controls', () => {
  const original = { WebSocket: globalThis.WebSocket, window: globalThis.window, setTimeout: globalThis.setTimeout, clearTimeout: globalThis.clearTimeout };
  const sockets = [];
  const timers = new Map();
  let sequence = 0;
  class FakeSocket {
    constructor(url) { this.url = url; this.sent = []; sockets.push(this); }
    send(value) { this.sent.push(JSON.parse(value)); }
    close() { if (this.closed) return; this.closed = true; this.onclose?.(); }
  }
  globalThis.WebSocket = FakeSocket;
  globalThis.window = { location: { href: 'https://clutchbase.example/manage' } };
  globalThis.setTimeout = (callback, delay) => { const id = ++sequence; timers.set(id, { callback, delay }); return id; };
  globalThis.clearTimeout = (id) => timers.delete(id);
  let cleanup;
  try {
    const states = [];
    const online = [];
    let unauthorized = false;
    cleanup = connectLive('manage', 'private-admin-token', (value) => states.push(value), (value) => online.push(value), () => { unauthorized = true; });
    const first = sockets[0];
    assert.equal(first.url.protocol, 'wss:');
    assert.equal(first.url.searchParams.get('token'), null);
    first.onopen();
    assert.deepEqual(first.sent, [{ token: 'private-admin-token' }]);
    first.onmessage({ data: JSON.stringify({ capturedAt: '2026-10-26T00:00:00Z', hosts: [] }) });
    assert.equal(states.length, 1);
    assert.equal(online.at(-1), true);
    first.onmessage({ data: JSON.stringify({ type: 'heartbeat' }) });
    assert.equal(states.length, 1);
    const watchdog = [...timers.values()].find((timer) => timer.delay === 40000);
    watchdog.callback();
    assert.equal(online.at(-1), false);
    const retry = [...timers.values()].find((timer) => timer.delay === 500);
    assert.ok(retry);
    retry.callback();
    const second = sockets[1];
    second.onopen();
    second.onmessage({ data: JSON.stringify({ code: 401, error: 'Sign in again' }) });
    assert.equal(unauthorized, true);
    assert.equal(second.closed, true);
    second.onmessage({ data: JSON.stringify({ capturedAt: "2026-10-26T00:00:01Z", hosts: ["stale"] }) });
    assert.equal(states.length, 1, "a signed-out connection must ignore queued snapshots");
  } finally {
    cleanup?.();
    Object.assign(globalThis, original);
  }
});
