import test from 'node:test';
import assert from 'node:assert/strict';
import { JSDOM } from 'jsdom';
import { createServer } from 'vite';
import React, { act } from 'react';

const bootstrap = new JSDOM('<!doctype html><html><body></body></html>');
Object.assign(globalThis, { window: bootstrap.window, document: bootstrap.window.document });
const { createRoot } = await import('react-dom/client');
bootstrap.window.close();
const vite = await createServer({ configFile: false, root: new URL('..', import.meta.url).pathname, server: { middlewareMode: true, hmr: false, ws: false }, appType: 'custom' });
const { MatchHistory } = await vite.ssrLoadModule('/src/MatchHistory.tsx');
test.after(() => vite.close());

const match = {
  id: 'match', title: 'Friday match', hostName: 'Friends', map: 'de_mirage', createdAt: '2026-11-15T09:00:00Z',
  resultCT: 13, resultT: 11, reportedCT: 13, reportedT: 9, resultRevision: 4, canEditResult: true,
  seriesMatch: false, seriesTeam1: null, seriesTeam2: null, removedHost: false,
  participants: [{ steamId: '76561198154367261', gameName: 'Friend', side: 'ct', reportedStats: { kills: 20, deaths: 10, assists: 3 }, stats: { kills: 21, deaths: 10, assists: 3 } }],
};
const series = { ...match, seriesMatch: true, seriesTeam1: 2, seriesTeam2: 1, seriesMaps: [
  { number: 1, map: 'de_mirage', team1: 13, team2: 9 },
  { number: 2, map: 'de_nuke', team1: null, team2: null },
] };
const button = (name, scope = document) => [...scope.querySelectorAll('button')].find((node) => node.textContent === name);
async function mount(props = {}) {
  const dom = new JSDOM('<div id="root"></div>', { url: 'http://localhost:5173/play#history' });
  const { window } = dom;
  Object.assign(globalThis, { window, document: window.document, HTMLElement: window.HTMLElement, FormData: window.FormData, IS_REACT_ACT_ENVIRONMENT: true });
  const sockets = [], requests = [];
  globalThis.WebSocket = class {
    constructor(url) { this.url = new URL(url); sockets.push(this); }
    send() {}
    close() { this.onclose?.(); }
    snapshot(value) { this.onmessage({ data: JSON.stringify(value) }); }
  };
  globalThis.fetch = async (url, options) => { requests.push({ url, body: JSON.parse(options.body) }); return new Response(null, { status: 204 }); };
  const root = createRoot(document.getElementById('root'));
  await act(async () => root.render(React.createElement(MatchHistory, { token: 'test-admin', onClose() {}, ...props })));
  return {
    window, sockets, requests,
    async snapshot(matches = [match], nextCursor = '', capturedAt = '2026-11-15T10:00:00Z') {
      await act(async () => sockets.at(-1).snapshot({ capturedAt, nextCursor, matches }));
    },
    async search(value) {
      const input = document.querySelector('input[type="search"]');
      Object.getOwnPropertyDescriptor(Object.getPrototypeOf(input), 'value').set.call(input, value);
      await act(async () => input.dispatchEvent(new window.Event('input', { bubbles: true })));
      await act(async () => input.form.dispatchEvent(new window.Event('submit', { bubbles: true, cancelable: true })));
    },
    async finish() { await act(async () => root.unmount()); window.close(); },
  };
}
async function click(node) { assert.ok(node); await act(async () => node.click()); }
async function submit(ui, form) { await act(async () => form.dispatchEvent(new ui.window.Event('submit', { bubbles: true, cancelable: true }))); }

test('compact cards keep labeled score sources visible with details collapsed and player controls absent', async () => {
  const ui = await mount({ player: true });
  try {
    assert.match(document.body.textContent, /Connecting to match history/);
    await ui.snapshot([series]);
    const card = document.querySelector('.history-card');
    assert.match(card.querySelector('.history-result').textContent, /Manual result · started CT 13 – 11 started T/);
    assert.match(card.querySelector('.history-result').textContent, /Reported final map · CT 13 – 9 T/);
    assert.match(card.querySelector('.history-series-result').textContent, /Team 1 2 – 1 Team 2/);
    assert.ok([...card.querySelectorAll('details')].every((node) => !node.open));
    assert.equal(card.querySelectorAll('button').length, 0);
    await click(card.querySelector('.history-scoreboard-details summary'));
    assert.equal(card.querySelector('.history-scoreboard-details').open, true);
    assert.deepEqual([...card.querySelectorAll('tbody td')].map((node) => node.textContent), ['CT', '20 / 10 / 3', '21 / 10 / 3']);
    await click(card.querySelector('.history-series summary'));
    assert.equal(card.querySelector('.history-series').open, true);
    assert.match(card.querySelector('.history-series').textContent, /missing score does not indicate whether a map was played/);
    assert.match(card.querySelector('.history-series').textContent, /Score not recorded/);
  } finally { await ui.finish(); }
});

test('disconnect retains expanded details and drafts, labels the snapshot and disables every write and paging action', async () => {
  const ui = await mount();
  try {
    await ui.snapshot([series], 'older');
    await click(document.querySelector('.history-scoreboard-details summary'));
    await click(document.querySelector('.history-series summary'));
    await click(button('Edit stats'));
    await click(button('Edit result'));
    await click(button('Record map score'));
    const input = document.querySelector('.history-result input');
    input.value = '16';
    await act(async () => ui.sockets[0].onclose());
    assert.match(document.querySelector('.history-snapshot').textContent, /Last received snapshot.*Page 1.*Editing and paging unavailable/);
    assert.equal(document.querySelectorAll('.history-card').length, 1);
    assert.equal(input.value, '16');
    assert.ok([...document.querySelectorAll('details')].every((node) => node.open));
    assert.ok([...document.querySelectorAll('.history-card button')].filter((node) => node.textContent !== 'Cancel').every((node) => node.disabled));
    assert.equal(button('Next').disabled, true);
    assert.equal(button('Previous').disabled, true);
    await submit(ui, input.form);
    assert.equal(ui.requests.length, 0);
    await ui.snapshot([series], 'older', '2026-11-15T10:00:01Z');
    assert.equal(button('Save', input.form).disabled, false);
    assert.equal(input.value, '16');
    await ui.snapshot([{ ...series, resultRevision: 5 }], 'older', '2026-11-15T10:00:02Z');
    assert.ok([...document.querySelectorAll('button')].filter((node) => node.textContent === 'Save').every((node) => node.disabled));
    assert.match(document.body.textContent, /The result changed/);
    assert.match(document.body.textContent, /The series changed/);
    await submit(ui, input.form);
    assert.equal(ui.requests.length, 0);
  } finally { await ui.finish(); }
});

test('search and pagination retain accurately labeled previous results until the next canonical page arrives', async () => {
  const ui = await mount({ player: true });
  try {
    await ui.snapshot([match], 'older');
    await ui.search('nuke');
    assert.equal(ui.sockets.at(-1).url.searchParams.get('search'), 'nuke');
    assert.match(document.body.textContent, /Friday match/);
    assert.doesNotMatch(document.querySelector('.history-snapshot').textContent, /Search: nuke/);
    assert.equal(button('Next').disabled, true);
    await ui.snapshot([{ ...match, title: 'Nuke match' }], 'older', '2026-11-15T10:00:01Z');
    assert.match(document.querySelector('.history-snapshot').textContent, /Page 1 · Search: nuke/);
    await click(button('Next'));
    assert.equal(ui.sockets.at(-1).url.searchParams.get('search'), 'nuke');
    assert.equal(ui.sockets.at(-1).url.searchParams.get('before'), 'older');
    assert.match(document.querySelector('.history-snapshot').textContent, /Last received snapshot.*Page 1 · Search: nuke/);
    assert.match(document.body.textContent, /Nuke match/);
    await ui.snapshot([], '', '2026-11-15T10:00:02Z');
    assert.match(document.querySelector('.history-snapshot').textContent, /Page 2 · Search: nuke/);
    assert.match(document.body.textContent, /No matches found/);
    await ui.search('mirage');
    assert.equal(ui.sockets.at(-1).url.searchParams.has('before'), false);
    assert.equal(ui.sockets.at(-1).url.searchParams.get('search'), 'mirage');
  } finally { await ui.finish(); }
});

test('manual result save and clear use revisions and wait for snapshots; removed servers stay read-only', async () => {
  const ui = await mount();
  try {
    await ui.snapshot();
    await click(button('Edit result'));
    const form = document.querySelector('.history-result form');
    form.elements.ct.value = '16';
    form.elements.t.value = '12';
    await submit(ui, form);
    assert.deepEqual(ui.requests[0], { url: '/api/matches/match/result', body: { revision: 4, ct: 16, t: 12 } });
    assert.match(document.querySelector('.history-result').textContent, /started CT 13 – 11 started T/);
    await ui.snapshot([{ ...match, resultCT: 16, resultT: 12, resultRevision: 5 }], '', '2026-11-15T10:00:01Z');
    await click(button('Edit result'));
    await click(button('Clear', document.querySelector('.history-result form')));
    assert.deepEqual(ui.requests[1].body, { revision: 5, clear: true });
    assert.match(document.querySelector('.history-result').textContent, /started CT 16 – 12 started T/);
    await ui.snapshot([{ ...match, resultCT: null, resultT: null, reportedCT: null, reportedT: null, removedHost: true, canEditResult: false }], '', '2026-11-15T10:00:02Z');
    assert.match(document.body.textContent, /removed server/);
    assert.match(document.querySelector('.history-result').textContent, /No score recorded/);
    assert.equal(document.querySelectorAll('.history-card button').length, 0);
  } finally { await ui.finish(); }
});

test('older snapshots cannot replace canonical results and session expiry leaves a disabled labeled snapshot', async () => {
  const ui = await mount();
  try {
    await ui.snapshot([match], 'older', '2026-11-15T10:00:02Z');
    await ui.snapshot([{ ...match, title: 'Outdated match', resultCT: 0 }], '', '2026-11-15T10:00:01Z');
    assert.match(document.querySelector('.history-card').textContent, /Friday match/);
    assert.doesNotMatch(document.querySelector('.history-card').textContent, /Outdated match/);
    await act(async () => ui.sockets[0].snapshot({ code: 401 }));
    assert.match(document.body.textContent, /Session expired. Sign in again/);
    assert.match(document.querySelector('.history-snapshot').textContent, /Last received snapshot/);
    assert.equal(document.querySelectorAll('.history-card').length, 1);
    assert.equal(button('Edit result').disabled, true);
    assert.equal(button('Next').disabled, true);
  } finally { await ui.finish(); }
});
