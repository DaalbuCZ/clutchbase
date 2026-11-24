import test from 'node:test';
import assert from 'node:assert/strict';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { createServer } from 'vite';
const vite = await createServer({ configFile: false, root: new URL('..', import.meta.url).pathname, server: { middlewareMode: true, hmr: false, ws: false }, appType: 'custom' });
const { TournamentDispute } = await vite.ssrLoadModule('/src/TournamentDispute.tsx');
test.after(() => vite.close());
test('dispute controls follow canonical permissions and review state', () => {
  const render = props => renderToStaticMarkup(React.createElement(TournamentDispute, props));
  assert.equal(render({ connected: true }), '');
  assert.match(render({ connected: true, canDispute: true }), /Submit dispute/);
  const open = { reason: 'Wrong score', revision: 0 };
  assert.match(render({ connected: true, admin: true, dispute: open }), /Resolve dispute/);
  assert.doesNotMatch(render({ connected: true, dispute: open }), /Resolve dispute/);
  const resolved = render({ connected: true, admin: true, dispute: { ...open, resolution: 'Confirmed' } });
  assert.match(resolved, /Resolved: Confirmed/);
  assert.doesNotMatch(resolved, /Resolve dispute/);
});
