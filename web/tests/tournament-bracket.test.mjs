import test from 'node:test';
import assert from 'node:assert/strict';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { createServer } from 'vite';

const vite = await createServer({ configFile: false, root: new URL('..', import.meta.url).pathname, server: { middlewareMode: true, hmr: false, ws: false }, appType: 'custom' });
const { TournamentBracket } = await vite.ssrLoadModule('/src/TournamentBracket.tsx');
test.after(() => vite.close());

test('bracket groups and orders rounds, surfaces current progress, and offers round jumps for large events', () => {
  const matches = [
    { id: 'm3', round: 3, number: 1, status: 'pending', roomStatus: '', vetoStep: 0 },
    { id: 'm4', round: 4, number: 1, status: 'pending', roomStatus: '', vetoStep: 0 },
    { id: 'm2b', round: 2, number: 2, status: 'pending', roomStatus: 'open', vetoStep: 0 },
    { id: 'm1', round: 1, number: 1, status: 'complete', roomStatus: '', vetoStep: 7 },
    { id: 'm2a', round: 2, number: 1, status: 'complete', roomStatus: '', vetoStep: 7 },
  ];
  const html = renderToStaticMarkup(React.createElement(TournamentBracket, {
    matches, label: 'Summer bracket', format: 'single_elimination', renderMatch: (match) => React.createElement('article', { key: match.id, 'data-match': match.id }, match.id),
  }));
  assert.match(html, /aria-label="Summer bracket"/);
  assert.match(html, /aria-label="Summer bracket rounds"/);
  assert.match(html, /aria-current="location"/);
  assert.ok(html.indexOf('data-match="m1"') < html.indexOf('data-match="m2a"'));
  assert.ok(html.indexOf('data-match="m2a"') < html.indexOf('data-match="m2b"'));
  assert.match(html, /1\/2 complete/);
  assert.match(html, /class="bracket-round is-current-round" id="summer-bracket-round-2"/);
});

test('small fixtures keep caller rendered actions in order without bracket navigation', () => {
  const html = renderToStaticMarkup(React.createElement(TournamentBracket, {
    matches: [{ id: 'f1', round: 1, number: 1, action: 'Confirm winner' }], label: 'League fixtures', format: 'round_robin',
    renderMatch: (match) => React.createElement('article', { key: match.id }, React.createElement('button', null, match.action)),
  }));
  assert.match(html, /round-robin-fixtures/);
  assert.match(html, /Confirm winner/);
  assert.doesNotMatch(html, /Jump to round/);
});

test('double elimination separates winners, losers, grand final, and reset lanes', () => {
  const matches = [
    { id: 'w1', bracketStage: 'winners', round: 1, number: 1, status: 'complete' },
    { id: 'w2', bracketStage: 'winners', round: 2, number: 1, status: 'pending', canReportWinner: true },
    { id: 'l1', bracketStage: 'losers', round: 3, number: 1, status: 'pending' },
    { id: 'l2', bracketStage: 'losers', round: 4, number: 1, status: 'pending' },
    { id: 'gf', bracketStage: 'grand_final', round: 5, number: 1, status: 'pending' },
    { id: 'reset', bracketStage: 'reset_final', round: 6, number: 1, status: 'pending' },
  ];
  const html = renderToStaticMarkup(React.createElement(TournamentBracket, {
    matches, label: 'Double cup bracket', format: 'double_elimination',
    renderMatch: (match) => React.createElement('article', { key: match.id, 'data-match': match.id }, match.id),
  }));
  assert.match(html, /Winners · Round 1/);
  assert.match(html, /Losers · Round 1/);
  assert.match(html, /Grand final/);
  assert.match(html, /Grand final reset/);
  assert.match(html, />W1</);
  assert.match(html, />L1</);
  assert.ok(html.indexOf('data-match="gf"') < html.indexOf('data-match="reset"'));
});
