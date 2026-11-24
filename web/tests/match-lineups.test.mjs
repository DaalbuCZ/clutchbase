import test from 'node:test';
import assert from 'node:assert/strict';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { createServer } from 'vite';

const vite = await createServer({ configFile: false, root: new URL('..', import.meta.url).pathname, server: { middlewareMode: true, hmr: false, ws: false }, appType: 'custom' });
const { MatchLineups } = await vite.ssrLoadModule('/src/MatchLineups.tsx');
test.after(() => vite.close());

function render(participants) {
  return renderToStaticMarkup(React.createElement(MatchLineups, {
    participants,
    playersPerSide: 3,
    ctPlayers: participants.filter((player) => player.side === 'ct').length,
    tPlayers: participants.filter((player) => player.side === 't').length,
    renderPlayer: (player) => React.createElement('span', { key: player.gameName }, player.gameName),
  }));
}

test('side readiness follows each canonical participant snapshot', () => {
  const participants = [
    { gameName: 'One', side: 'ct', ready: true },
    { gameName: 'Two', side: 'ct', ready: false },
    { gameName: 'Three', side: 't', ready: false },
  ];
  const before = render(participants);
  assert.match(before, /Counter-Terrorists lineup[\s\S]*?1 of 2 ready/);
  assert.match(before, /aria-label="Counter-Terrorists capacity" aria-valuemin="0" aria-valuemax="3" aria-valuenow="2"/);
  assert.match(before, /Terrorists lineup[\s\S]*?0 of 1 ready/);

  const after = render(participants.map((player) => ({ ...player, ready: true })));
  assert.match(after, /Counter-Terrorists lineup[\s\S]*?2 of 2 ready/);
  assert.match(after, /Terrorists lineup[\s\S]*?1 of 1 ready/);
  assert.match(after, /aria-label="Terrorists capacity" aria-valuemin="0" aria-valuemax="3" aria-valuenow="1"/);
});

test('an empty side waits for players', () => {
  const markup = render([{ gameName: 'One', side: 'ct', ready: true }]);
  assert.match(markup, /Terrorists lineup[\s\S]*?Waiting for players/);
  assert.match(markup, /Terrorists lineup[\s\S]*?3 open slots/);
  assert.doesNotMatch(markup, /Open slot/);
});

test('an almost full side shows one remaining slot', () => {
  const markup = render([
    { gameName: 'One', side: 'ct', ready: true },
    { gameName: 'Two', side: 'ct', ready: false },
    { gameName: 'Three', side: 't', ready: false },
  ]);
  assert.match(markup, /Counter-Terrorists lineup[\s\S]*?1 open slot/);
  assert.match(markup, /Terrorists lineup[\s\S]*?2 open slots/);
});
