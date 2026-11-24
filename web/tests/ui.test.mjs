import test from 'node:test';
import assert from 'node:assert/strict';
import { JSDOM } from 'jsdom';
import { createServer } from 'vite';
import React, { act } from 'react';
// Initialize DOM feature detection before React DOM; each page still gets an isolated DOM.
const bootstrapDOM = new JSDOM('<!doctype html><html><body></body></html>');
Object.assign(globalThis, { window: bootstrapDOM.window, document: bootstrapDOM.window.document });
const { createRoot } = await import('react-dom/client');
bootstrapDOM.window.close();

const vite = await createServer({ configFile: false, root: new URL('..', import.meta.url).pathname, server: { middlewareMode: true, hmr: false, ws: false }, appType: 'custom' });
const { App } = await vite.ssrLoadModule('/src/App.tsx');
const { matchRoomStatus } = await vite.ssrLoadModule('/src/api.ts');
test.after(() => vite.close());

async function page(path, fetcher) {
  const dom = new JSDOM('<div id="root"></div>', { url: `http://localhost:5173${path}` });
  const { window } = dom;
  Object.assign(globalThis, { window, document: window.document, HTMLElement: window.HTMLElement, FormData: window.FormData, localStorage: window.localStorage, sessionStorage: window.sessionStorage, IS_REACT_ACT_ENVIRONMENT: true });
  window.HTMLDialogElement.prototype.showModal = function () { this.open = true; };
  window.HTMLDialogElement.prototype.close = function () { this.open = false; };
  const sockets = [];
  globalThis.WebSocket = class {
    constructor(url) { this.url = url; sockets.push(this); }
    send() {}
    close() { this.onclose?.(); }
    open() { this.onopen(); }
    snapshot(value) { this.onmessage({ data: JSON.stringify(value) }); }
  };
  globalThis.fetch = fetcher;
  const root = createRoot(document.getElementById('root'));
  return { window, sockets, root, async render() { await act(async () => root.render(React.createElement(App))); }, async finish() { await act(async () => root.unmount()); dom.window.close(); } };
}
function button(name) {
  const found = [...document.querySelectorAll('button')].find((item) => (item.getAttribute('aria-label') || item.textContent).trim().includes(name));
  assert.ok(found, `button not found: ${name}`);
  return found;
}
const response = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });

test('room status labels follow setup observations', () => {
  const room = { status: 'open', loadStatus: '', setupUnloaded: false, endedAt: null };
  assert.equal(matchRoomStatus(room).label, 'Open room');
  assert.equal(matchRoomStatus({ ...room, loadStatus: 'pending' }).label, 'Loading setup');
  assert.equal(matchRoomStatus({ ...room, loadStatus: 'failed' }).label, 'Load failed');
  assert.equal(matchRoomStatus({ ...room, loadStatus: 'complete' }).label, 'Setup loaded');
  assert.equal(matchRoomStatus({ ...room, loadStatus: 'complete', setupUnloaded: true }).label, 'Setup ended');
  assert.equal(matchRoomStatus({ ...room, status: 'closed', loadStatus: 'complete' }).label, 'Closed');
});

test('bracket round jump keeps the tournament route', async () => {
  const { TournamentBracket } = await vite.ssrLoadModule('/src/TournamentBracket.tsx');
  const ui = await page('/play#tournaments?tournament=cup', async () => { throw new Error('No request expected'); });
  try {
    const matches = [1, 2, 3, 4].map((round) => ({ id: `m${round}`, round, number: 1, status: 'pending' }));
    await act(async () => ui.root.render(React.createElement(TournamentBracket, { matches, label: 'Cup bracket', renderMatch: (match) => React.createElement('article', { key: match.id }, match.id) })));
    let scrolled = false;
    document.getElementById('cup-bracket-round-3').scrollIntoView = () => { scrolled = true; };
    await act(async () => button('R3').click());
    assert.equal(scrolled, true);
    assert.equal(ui.window.location.hash, '#tournaments?tournament=cup');
  } finally { await ui.finish(); }
});

test('management opens on the control room and follows live snapshot links', async () => {
  const ui = await page('/manage', async () => response({}));
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token');
    await ui.render();
    const socket = ui.sockets[0];
    await act(async () => { socket.open(); socket.snapshot({
      capturedAt: '2026-11-14T10:00:00Z',
      hosts: [{ id: 'server', name: 'Friends', status: 'running', online: true, machineId: 'machine', address: '127.0.0.1', port: 27015, map: 'de_mirage', actions: { openMatch: false } }],
      machines: [{ id: 'machine', name: 'Byttorrent', online: true, lastSeen: '2026-11-14T10:00:00Z' }],
      matches: [{ id: 'match', title: 'Friday final', hostName: 'Friends', map: 'de_mirage', status: 'open', loadStatus: 'complete', endedAt: null, players: 10, playersPerSide: 5, ctPlayers: 5, tPlayers: 5, ready: 10, canLoad: false, participants: [] }],
      players: [], maps: [], tournaments: [], teams: [],
    }); });
    assert.equal(button('Overview').getAttribute('aria-current'), 'page');
    assert.match(document.body.textContent, /Live matches/);
    assert.match(document.body.textContent, /Friday final/);
    await act(async () => document.querySelector('.overview-match').click());
    assert.equal(button('Matches').getAttribute('aria-current'), 'page');
    assert.equal(ui.window.location.hash, '#matches?room=match');
    assert.equal(document.activeElement?.id, 'match-room-match');
    assert.match(document.body.textContent, /Friday final/);
    await act(async () => button('Overview').click());
    assert.equal(button('Overview').getAttribute('aria-current'), 'page');
    await act(async () => socket.snapshot({
      capturedAt: '2026-11-14T10:00:01Z', hosts: [], machines: [], players: [], maps: [], tournaments: [], teams: [],
      matches: [{ id: 'match', title: 'Friday final', hostName: 'Friends', map: 'de_mirage', status: 'open', loadStatus: 'complete', setupUnloaded: true, players: 10, playersPerSide: 5, ctPlayers: 5, tPlayers: 5, ready: 10, participants: [] }],
    }));
    assert.match(button('Live matches').textContent, /Live matches0/);
    assert.match(document.querySelector('.overview-match').textContent, /Setup ended/);
    assert.equal(document.querySelector('.overview-match-state').classList.contains('is-live'), false);
    await act(async () => document.querySelector('.overview-match').click());
    assert.match(document.body.textContent, /No match setup is currently loaded/);
    assert.doesNotMatch(document.body.textContent, /MatchZy confirmed this setup was loaded/);
  } finally { await ui.finish(); }
});

test('public room URL reveals a closed room and handles an unavailable room', async () => {
  const ui = await page('/play#matches?room=closed-room', async (url) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' });
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    await ui.render();
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot({
      capturedAt: '2026-11-14T10:00:00Z', servers: [], tournaments: [],
      matches: [{ id: 'closed-room', title: 'Completed cup match', hostName: 'Friends', map: 'de_mirage', status: 'closed', players: 2, playersPerSide: 1, ctPlayers: 1, tPlayers: 1, ready: 2, participants: [], endedAt: '2026-11-14T09:00:00Z', resultCT: null, resultT: null, reportedCT: 13, reportedT: 8, seriesMatch: true, actions: {} }],
    }); });
    assert.equal(button('Matches').getAttribute('aria-current'), 'page');
    assert.equal(document.activeElement?.id, 'match-room-closed-room');
    assert.match(document.body.textContent, /Reported final map · CT 13 – 8 T/);
    await act(async () => { ui.window.location.hash = '#matches?room=missing-room'; ui.window.dispatchEvent(new ui.window.HashChangeEvent('hashchange')); });
    assert.match(document.body.textContent, /not available to your account/);
    assert.ok(button('View match history'));
  } finally { await ui.finish(); }
});

test('public and management rooms share a public link with a manual-copy fallback', async () => {
  const descriptor = Object.getOwnPropertyDescriptor(globalThis.navigator, 'clipboard');
  const copied = [];
  Object.defineProperty(globalThis.navigator, 'clipboard', { configurable: true, value: { writeText: async (text) => { copied.push(text); } } });
  const room = { id: 'room-with-id', hostId: 'server', title: 'Friday', hostName: 'Friends', map: 'de_mirage', status: 'open', players: 0, playersPerSide: 1, ctPlayers: 0, tPlayers: 0, ready: 0, participants: [], joined: false, actions: {}, resultCT: null, resultT: null, reportedCT: null, reportedT: null };
  try {
    const publicUI = await page('/play#matches', async (url) => url === '/api/player/session' ? response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' }) : response({}));
    try {
      await publicUI.render();
      await act(async () => { publicUI.sockets[0].open(); publicUI.sockets[0].snapshot({ capturedAt: '2026-11-14T10:00:00Z', servers: [], matches: [room], tournaments: [] }); });
      await act(async () => button('Share Friday').click());
      assert.deepEqual(copied, ['http://localhost:5173/play#matches?room=room-with-id']);
      assert.match(document.body.textContent, /Friends must have access/);
    } finally { await publicUI.finish(); }
    Object.defineProperty(globalThis.navigator, 'clipboard', { configurable: true, value: { writeText: async () => { throw new Error('denied'); } } });
    const adminUI = await page('/manage#matches', async () => response({}));
    try {
      sessionStorage.setItem('clutchbase-token', 'test-admin-token'); await adminUI.render();
      await act(async () => { adminUI.sockets[0].open(); adminUI.sockets[0].snapshot({ capturedAt: '2026-11-14T10:00:00Z', hosts: [], machines: [], players: [], matches: [room], tournaments: [], teams: [], maps: [] }); });
      await act(async () => button('Share Friday').click());
      const input = document.querySelector('.share-room input');
      assert.equal(input.value, 'http://localhost:5173/play#matches?room=room-with-id');
      assert.equal(input.readOnly, true);
      assert.match(document.body.textContent, /Copy the match link below/);
    } finally { await adminUI.finish(); }
  } finally {
    if (descriptor) Object.defineProperty(globalThis.navigator, 'clipboard', descriptor);
    else delete globalThis.navigator.clipboard;
  }
});

test('public server shows the canonical join blocker', async () => {
  const ui = await page('/play', async (url) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' });
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    await ui.render();
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot({ capturedAt: '2026-11-14T10:00:00Z', matches: [], tournaments: [], servers: [{ id: 'server', name: 'Friends', address: '127.0.0.1', port: 27015, map: 'de_mirage', status: 'running', players: 0, maxPlayers: 10, canJoin: false, joinBlocker: 'SteamID access verification is pending.', preset: null, activePreset: '', matchPhase: 'idle', simulated: false }] }); });
    assert.match(document.body.textContent, /SteamID access verification is pending/);
    assert.equal(button('Copy join command').disabled, true);
  } finally { await ui.finish(); }
});

test('public server filters follow the current snapshot without losing the search', async () => {
  const ui = await page('/play', async (url) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' });
    throw new Error(`Unexpected fetch ${url}`);
  });
  const server = (id, name, map, canJoin) => ({ id, name, map, canJoin, address: '127.0.0.1', port: 27015, status: 'running', players: 0, maxPlayers: 10 });
  const state = { capturedAt: '2026-11-14T10:00:00Z', matches: [], tournaments: [], servers: [server('one', 'Friday', 'de_nuke', false), server('two', 'Practice', 'de_mirage', true)] };
  try {
    await ui.render();
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot(state); });
    const search = document.querySelector('input[type="search"]');
    await act(async () => { Object.getOwnPropertyDescriptor(ui.window.HTMLInputElement.prototype, 'value').set.call(search, 'nuke'); search.dispatchEvent(new ui.window.Event('input', { bubbles: true })); });
    assert.match(document.body.textContent, /Friday/);
    assert.doesNotMatch(document.body.textContent, /Practice/);
    const filter = document.querySelector('select');
    await act(async () => { filter.value = 'joinable'; filter.dispatchEvent(new ui.window.Event('change', { bubbles: true })); });
    assert.match(document.body.textContent, /No servers match these filters/);
    await act(async () => ui.sockets[0].snapshot({ ...state, servers: [server('one', 'Friday', 'de_nuke', true), state.servers[1]] }));
    assert.equal(search.value, 'nuke');
    assert.match(document.body.textContent, /Friday/);
    assert.equal(button('Copy join command').disabled, false);
    await act(async () => ui.sockets[0].onclose());
    assert.equal(button('Copy join command').disabled, true);
  } finally { await ui.finish(); }
});

test('a public server links to its open room without overriding room filters on later snapshots', async () => {
  const ui = await page('/play', async (url) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' });
    throw new Error(`Unexpected fetch ${url}`);
  });
  const room = { id: 'room', hostId: 'server', title: 'Friday match', hostName: 'Friends', map: 'de_mirage', status: 'open', players: 0, playersPerSide: 1, ctPlayers: 0, tPlayers: 0, ready: 0, participants: [], joined: false, actions: {}, resultCT: null, resultT: null, reportedCT: null, reportedT: null };
  const state = { capturedAt: '2026-11-14T10:00:00Z', servers: [{ id: 'server', name: 'Friends', address: '127.0.0.1', port: 27015, map: 'de_mirage', status: 'running', players: 0, maxPlayers: 10, canJoin: true }], tournaments: [], matches: [room] };
  try {
    await ui.render();
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot(state); });
    await act(async () => button('Friday match').click());
    assert.equal(ui.window.location.hash, '#matches?room=room');
    assert.equal(document.activeElement?.id, 'match-room-room');
    await act(async () => button('Closed').click());
    assert.equal(ui.window.location.hash, '#matches');
    await act(async () => ui.sockets[0].snapshot({ ...state, capturedAt: '2026-11-14T10:00:01Z', matches: [{ ...room }] }));
    assert.equal(button('Closed').getAttribute('aria-pressed'), 'true');
    assert.match(document.body.textContent, /No closed match rooms/);
  } finally { await ui.finish(); }
});

test('machine token replacement requires confirmation and displays the one-time credential', async () => {
  let rotations = 0;
  const ui = await page('/manage#servers', async (url, options) => {
    if (url === '/api/machines/machine/rotate-token') { rotations++; assert.equal(options.method, 'POST'); return response({ agentToken: 'new-agent-secret' }); }
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token');
    await ui.render();
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot({ capturedAt: '2026-11-14T10:00:00Z', hosts: [], machines: [{ id: 'machine', name: 'Byttorrent', address: '10.0.0.2', online: true, driver: 'managed' }], matches: [], players: [], maps: [], tournaments: [], teams: [] }); });
    await act(async () => button('Replace agent token').click());
    assert.equal(rotations, 0);
    assert.match(document.body.textContent, /current token stops working immediately/);
    await act(async () => button('Replace token').click());
    assert.equal(rotations, 1);
    assert.match(document.body.textContent, /Restart agent on Byttorrent/);
    assert.match(document.querySelector('dialog pre').textContent, /new-agent-secret/);
    await act(async () => document.querySelector('dialog button[aria-label="Close dialog"]').click());
    assert.doesNotMatch(document.body.textContent, /new-agent-secret/);
  } finally { await ui.finish(); }
});

test('public views use live snapshots and disable actions on disconnect', async () => {
  const ui = await page('/play', async (url) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' });
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    await ui.render();
    const socket = ui.sockets[0];
    await act(async () => { socket.open(); socket.snapshot({ capturedAt: '2026-11-12T10:00:00Z', servers: [{ id: 's', name: 'Friends', address: '127.0.0.1', port: 27015, map: 'de_mirage', status: 'running', players: 1, maxPlayers: 10, canJoin: true, preset: null, activePreset: '', matchPhase: 'idle', simulated: false }], matches: [] }); });
    assert.match(document.body.textContent, /Friends/);
    assert.equal(button('Copy join command').disabled, false);
    await act(async () => button('Matches').click());
    assert.match(document.body.textContent, /No matches available/);
    assert.equal(button('Matches').getAttribute('aria-current'), 'page');
    await act(async () => button('Profile').click());
    assert.equal(document.querySelector('input').value, 'Friend');
    await act(async () => button('Servers').click());
    await act(async () => socket.close());
    assert.equal(button('Copy join command').disabled, true);
    assert.match(document.body.textContent, /Reconnecting/);
    assert.match(document.body.textContent, /Last reported: Ready to join/);
  } finally { await ui.finish(); }
});

test('team check-in shows the server stage and waits for the next snapshot', async () => {
  let checkinRequested = false;
  const ui = await page('/play', async (url, options) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' });
    if (url === '/api/player/tournaments/cup/checkin') { checkinRequested = true; assert.equal(options.method, 'POST'); return new Response(null, { status: 204 }); }
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    await ui.render();
    const socket = ui.sockets[0];
    await act(async () => { socket.open(); socket.snapshot({
      capturedAt: '2026-11-14T10:00:00Z', servers: [], matches: [],
      team: { id: 'alpha', name: 'Alpha', captainSteamId: '76561198154367261', members: [{ steamId: '76561198154367261', gameName: 'Friend' }] },
      tournaments: [{ id: 'cup', name: 'Saturday cup', playersPerSide: 1, bestOf: 1, mapPool: [], status: 'checkin', winnerTeamId: '', entries: [{ teamId: 'alpha', teamName: 'Alpha', status: 'registered', members: 1 }], matches: [], myEntryStatus: 'registered', canRegisterTeam: false, canCheckInTeam: true }],
    }); });
    await act(async () => button('Tournaments').click());
    await act(async () => button('View Saturday cup').click());
    assert.match(document.body.textContent, /0 of 1 teams checked in/);
    await act(async () => button('Check in Alpha').click());
    assert.equal(checkinRequested, true);
    assert.match(document.body.textContent, /Your team: registered/);
    await act(async () => socket.snapshot({
      capturedAt: '2026-11-14T10:00:01Z', servers: [], matches: [],
      team: { id: 'alpha', name: 'Alpha', captainSteamId: '76561198154367261', members: [{ steamId: '76561198154367261', gameName: 'Friend' }] },
      tournaments: [{ id: 'cup', name: 'Saturday cup', playersPerSide: 1, bestOf: 1, mapPool: [], status: 'checkin', winnerTeamId: '', entries: [{ teamId: 'alpha', teamName: 'Alpha', status: 'checked_in', members: 1 }], matches: [], myEntryStatus: 'checked_in', canRegisterTeam: false, canCheckInTeam: false }],
    }));
    assert.match(document.body.textContent, /1 teams checked in/);
    assert.match(document.body.textContent, /Your team: checked_in/);
  } finally { await ui.finish(); }
});

test('public BO3 veto presents a map pick and sends it to the server', async () => {
  let selection;
  const ui = await page('/play', async (url, options) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' });
    if (url === '/api/player/tournaments/cup/matches/fixture/veto') {
      assert.equal(options.method, 'POST');
      selection = JSON.parse(options.body);
      return new Response(null, { status: 204 });
    }
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    await ui.render();
    const socket = ui.sockets[0];
    await act(async () => { socket.open(); socket.snapshot({
      capturedAt: '2026-11-14T10:00:00Z', servers: [], matches: [],
      team: { id: 'alpha', name: 'Alpha', captainSteamId: '76561198154367261', members: [{ steamId: '76561198154367261', gameName: 'Friend' }] },
      tournaments: [{ id: 'cup', name: 'Saturday cup', playersPerSide: 5, bestOf: 3, mapPool: ['de_mirage','de_inferno','de_dust2','de_nuke','de_ancient','de_anubis','de_train'], status: 'active', winnerTeamId: '', entries: [], matches: [{ id: 'fixture', round: 1, number: 1, status: 'pending', teamA: { teamId: 'alpha', teamName: 'Alpha' }, teamB: { teamId: 'bravo', teamName: 'Bravo' }, winnerTeamId: '', vetoBans: ['de_dust2','de_train'], vetoMaps: [], mapSequence: [], vetoStep: 2, vetoTurnTeam: 'Alpha', canVeto: true, canReportWinner: false, canOpenRoom: false }], canRegisterTeam: false, canCheckInTeam: false }],
    }); });
    await act(async () => button('Tournaments').click());
    await act(async () => button('View Saturday cup').click());
    assert.ok(button('Pick Inferno'));
    await act(async () => button('Pick Inferno').click());
    assert.deepEqual(selection, { map: 'de_inferno' });
  } finally { await ui.finish(); }
});

test('fixture participants can submit a dispute from the public tournament bracket', async () => {
  let submitted;
  const ui = await page('/play', async (url, options = {}) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' });
    if (url === '/api/player/tournaments/cup/matches/fixture/dispute') { submitted = JSON.parse(options.body); return new Response(null, { status: 204 }); }
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    await ui.render();
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot({
      capturedAt: '2026-11-14T10:00:00Z', servers: [], matches: [],
      team: { id: 'alpha', name: 'Alpha', captainSteamId: '76561198154367261', members: [{ steamId: '76561198154367261', gameName: 'Friend' }] },
      tournaments: [{ id: 'cup', name: 'Saturday cup', playersPerSide: 1, bestOf: 1, mapPool: [], status: 'active', winnerTeamId: '', entries: [], matches: [{ id: 'fixture', round: 1, number: 1, status: 'complete', teamA: { teamId: 'alpha', teamName: 'Alpha' }, teamB: { teamId: 'bravo', teamName: 'Bravo' }, winnerTeamId: 'alpha', reportedCT: 13, reportedT: 9, dispute: undefined, canDispute: true, vetoBans: [], vetoMaps: [], mapSequence: [], vetoStep: 7, canVeto: false, canReportWinner: false, canOpenRoom: false }], canRegisterTeam: false, canCheckInTeam: false }],
    }); });
    await act(async () => button('Tournaments').click());
    await act(async () => button('View Saturday cup').click());
    const textarea = document.querySelector('[aria-label="Fixture dispute"] textarea');
    Object.getOwnPropertyDescriptor(ui.window.HTMLTextAreaElement.prototype, 'value').set.call(textarea, 'The reported score is incorrect.');
    await act(async () => textarea.dispatchEvent(new ui.window.Event('input', { bubbles: true })));
    await act(async () => button('Submit dispute').click());
    assert.deepEqual(submitted, { reason: 'The reported score is incorrect.' });
  } finally { await ui.finish(); }
});

test('management retains usable controls after an action error', async () => {
  const ui = await page('/manage', async (url) => {
    if (url === '/api/machines') return response({ error: 'Machine name already exists' }, 409);
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token');
    await ui.render();
    const socket = ui.sockets[0];
    await act(async () => { socket.open(); socket.snapshot({ capturedAt: '2026-11-12T10:00:00Z', hosts: [], machines: [], matches: [], players: [], maps: ['de_mirage'] }); });
    await act(async () => button('Connect machine').click());
    const dialog = document.querySelector('dialog');
    dialog.querySelector('[name=name]').value = 'Example';
    dialog.querySelector('[name=address]').value = 'example.test';
    await act(async () => dialog.querySelector('form').dispatchEvent(new ui.window.Event('submit', { bubbles: true, cancelable: true })));
    assert.match(dialog.textContent, /Machine name already exists/);
    assert.equal(dialog.querySelector('button[type=submit]').disabled, false);
    await act(async () => dialog.querySelector('[aria-label="Close dialog"]').click());
    assert.equal(button('Connect machine').disabled, false);
  } finally { await ui.finish(); }
});

test('joining a match waits for the server snapshot', async () => {
  let requests = 0;
  const ui = await page('/play', async (url, options) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' });
    if (url === '/api/player/matches/room/join') {
      requests++;
      assert.equal(JSON.parse(options.body).side, 'ct');
      return new Response(null, { status: 204 });
    }
    throw new Error(`Unexpected fetch ${url}`);
  });
  const match = { id: 'room', title: 'Friday', hostName: 'Friends', map: 'de_mirage', status: 'open', players: 0, playersPerSide: 5, ctPlayers: 0, tPlayers: 0, ready: 0, participants: [], joined: false, myReady: false, mySide: '', canJoin: true, actions: { joinCT: true, joinT: true, switchSide: false, ready: false, unready: false, leave: false }, resultCT: null, resultT: null, reportedCT: null, reportedT: null };
  try {
    await ui.render();
    const socket = ui.sockets[0];
    await act(async () => { socket.open(); socket.snapshot({ capturedAt: '2026-11-12T10:00:00Z', servers: [], matches: [match] }); });
    await act(async () => button('Matches').click());
    await act(async () => button('Join CT').click());
    assert.equal(requests, 1);
    assert.ok(button('Join CT'), 'successful request must not optimistically join');
    await act(async () => socket.snapshot({ capturedAt: '2026-11-12T10:00:01Z', servers: [], matches: [{ ...match, joined: true, mySide: 'ct', players: 1, ctPlayers: 1, actions: { ...match.actions, ready: true, leave: true } }] }));
    assert.ok(button('Ready up'));
    assert.match(document.body.textContent, /Your side: CT/);
    await act(async () => socket.snapshot({ capturedAt: '2026-11-12T10:00:02Z', servers: [], matches: [{ ...match, status: 'closed', joined: true, mySide: 'ct', players: 1, ctPlayers: 1, actions: {} }] }));
    await act(async () => button('Closed').click());
    assert.match(document.body.textContent, /Your side: CT/);
    assert.doesNotMatch(document.body.textContent, /Leave room/);
  } finally { await ui.finish(); }
});

test('loaded rooms explain balanced auto-ready and uneven manual-ready behavior', async () => {
  const match = { id: 'room', title: 'Friday', hostName: 'Friends', map: 'de_mirage', status: 'open', players: 4, playersPerSide: 5, ctPlayers: 2, tPlayers: 2, ready: 4, participants: [], joined: false, myReady: false, mySide: '', canJoin: true, actions: { joinCT: true, joinT: true, switchSide: false, ready: false, unready: false, leave: false }, loadStatus: 'complete', setupUnloaded: false };
  const ui = await page('/play', async (url) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' });
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    await ui.render();
    const socket = ui.sockets[0];
    await act(async () => { socket.open(); socket.snapshot({ capturedAt: '2026-11-12T10:00:00Z', servers: [], matches: [match] }); });
    await act(async () => button('Matches').click());
    assert.match(document.body.textContent, /Balanced rosters auto-ready in MatchZy/);
    await act(async () => socket.snapshot({ capturedAt: '2026-11-12T10:00:01Z', servers: [], matches: [{ ...match, ctPlayers: 2, tPlayers: 1 }] }));
    assert.match(document.body.textContent, /Uneven rosters use MatchZy’s in-game \.ready command/);
    await act(async () => socket.snapshot({ capturedAt: '2026-11-12T10:00:02Z', servers: [], matches: [{ ...match, manualReady: true }] }));
    assert.match(document.body.textContent, /Players use MatchZy’s in-game \.ready command before starting/);
    assert.doesNotMatch(document.body.textContent, /Balanced rosters auto-ready/);
    const manual = { ...match, hostId: 'server', manualReady: true, loadStatus: '' };
    const playerServer = { id: 'server', address: '127.0.0.1', port: 27015, supportsManualReady: false };
    await act(async () => socket.snapshot({ capturedAt: '2026-11-12T10:00:03Z', servers: [playerServer], matches: [manual] }));
    assert.match(document.body.textContent, /host must update the agent and reporter before loading this match/);
    await act(async () => socket.snapshot({ capturedAt: '2026-11-12T10:00:04Z', servers: [{ ...playerServer, supportsManualReady: true }], matches: [manual] }));
    assert.doesNotMatch(document.body.textContent, /host must update the agent and reporter before loading this match/);
    const readyServer = { ...playerServer, canJoin: true };
    await act(async () => socket.snapshot({ capturedAt: '2026-11-12T10:00:05Z', servers: [readyServer], matches: [{ ...match, hostId: 'server' }] }));
    assert.match(document.body.textContent, /Match loaded. Its roster is locked/);
    assert.doesNotMatch(document.body.textContent, /Copy connect command|your side is locked/);
    await act(async () => socket.snapshot({ capturedAt: '2026-11-12T10:00:06Z', servers: [readyServer], matches: [{ ...match, joined: true, mySide: 'ct', hostId: 'server' }] }));
    assert.ok(button('Copy connect command'));
    assert.match(document.body.textContent, /Use the join password shared by the host/);
    await act(async () => socket.onclose());
    assert.equal(button('Copy connect command').disabled, true);
    await act(async () => socket.snapshot({ capturedAt: '2026-11-12T10:00:07Z', servers: [readyServer], matches: [{ ...match, hostId: 'server', setupUnloaded: true }] }));
    assert.match(document.body.textContent, /No match setup is currently loaded/);
    assert.doesNotMatch(document.body.textContent, /Copy connect command|confirmed the setup was loaded|Waiting for the server to be ready/);
  } finally { await ui.finish(); }
});

test('ready participants see who must load the match', async () => {
  const ui = await page('/play#matches', async (url) => url === '/api/player/session' ? response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' }) : response({}));
  const match = { id: 'room', hostId: 'server', title: 'Friday', hostName: 'Friends', map: 'de_mirage', status: 'open', loadStatus: '', players: 2, playersPerSide: 1, ctPlayers: 1, tPlayers: 1, ready: 1, participants: [], joined: true, myReady: true, mySide: 'ct', actions: { unready: true } };
  try {
    await ui.render();
    const socket = ui.sockets[0];
    await act(async () => { socket.open(); socket.snapshot({ capturedAt: '2026-11-12T10:00:00Z', servers: [], matches: [match] }); });
    assert.match(document.body.textContent, /Ready. Waiting for the others/);
    await act(async () => socket.snapshot({ capturedAt: '2026-11-12T10:00:01Z', servers: [], matches: [{ ...match, ready: 2 }] }));
    assert.match(document.body.textContent, /Everyone is ready. Waiting for the host to load/);
    await act(async () => socket.snapshot({ capturedAt: '2026-11-12T10:00:02Z', servers: [], matches: [{ ...match, ready: 2, actions: { unready: true, load: true } }] }));
    assert.match(document.body.textContent, /Everyone is ready. You can load the match/);
    await act(async () => socket.onclose());
    assert.match(document.body.textContent, /Reconnecting. Actions will return/);
    assert.equal(button('Load match').disabled, true);
  } finally { await ui.finish(); }
});

test('management start waits for an authoritative server update', async () => {
  const host = { id: 'server', name: 'Friends', address: '127.0.0.1', port: 27015, map: 'de_mirage', activeMap: '', desiredState: 'stopped', observedState: 'stopped', status: 'stopped', online: true, lastSeen: '2026-11-12T10:00:00Z', driver: 'simulated', machineId: '', revision: 4, whitelist: null, actions: { start: true, stop: false, changeMap: true, openMatch: false } };
  const state = { capturedAt: '2026-11-12T10:00:00Z', hosts: [host], machines: [], matches: [], players: [], maps: ['de_mirage'], presets: [] };
  let requested = false;
  const ui = await page('/manage', async (url, options) => {
    if (url === '/api/hosts/server') { requested = true; assert.deepEqual(JSON.parse(options.body), { desiredState: 'running', revision: 4 }); return response({}); }
    if (url === '/api/state') return response(state);
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token');
    await ui.render();
    const socket = ui.sockets[0];
    await act(async () => { socket.open(); socket.snapshot(state); });
    await act(async () => button('Servers').click());
    await act(async () => button('Manage Friends').click());
    await act(async () => button('Start server').click());
    assert.equal(requested, true);
    assert.ok(button('Start server'), 'request alone must not change lifecycle state');
    await act(async () => socket.snapshot({ ...state, capturedAt: '2026-11-12T10:00:01Z', hosts: [{ ...host, desiredState: 'running', status: 'starting', actions: { ...host.actions, start: false, stop: true, changeMap: false } }] }));
    assert.ok(button('Stop server'));
    assert.match(document.body.textContent, /Starting/);
    await act(async () => socket.close());
    assert.equal(button('Stop server').disabled, true);
  } finally { await ui.finish(); }
});

test('management navigation shows the selected view, including empty views', async () => {
  const ui = await page('/manage', async () => { throw new Error('Unexpected fetch'); });
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token');
    await ui.render();
    const socket = ui.sockets[0];
    await act(async () => { socket.open(); socket.snapshot({ capturedAt: '2026-11-12T10:00:00Z', hosts: [], machines: [], matches: [], players: [], maps: [] }); });
    await act(async () => button('Matches').click());
    assert.equal(button('Matches').getAttribute('aria-current'), 'page');
    assert.match(document.body.textContent, /No matches available/);
    assert.equal(button('Open match').disabled, true);
    await act(async () => button('Players').click());
    assert.match(document.body.textContent, /Players appear here after signing in/);
    assert.equal(button('Players').getAttribute('aria-current'), 'page');
    await act(async () => button('Servers').click());
    assert.ok(button('Connect machine'));
  } finally { await ui.finish(); }
});

test('management copies SteamID and handles clipboard failure', async () => {
  const descriptor = Object.getOwnPropertyDescriptor(globalThis.navigator, 'clipboard');
  const copied = [];
  Object.defineProperty(globalThis.navigator, 'clipboard', { configurable: true, value: { writeText: async (value) => copied.push(value) } });
  const ui = await page('/manage#players', async () => { throw new Error('Directory uses snapshots'); });
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token'); await ui.render();
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot({ capturedAt: '2026-11-15T10:00:00Z', hosts: [], machines: [], matches: [], maps: [], players: [{ steamId: '76561198154367261', gameName: 'Alice' }] }); });
    await act(async () => button('Copy SteamID for Alice').click());
    assert.deepEqual(copied, ['76561198154367261']);
    assert.match(document.body.textContent, /Copied SteamID for Alice/);
    Object.defineProperty(globalThis.navigator, 'clipboard', { configurable: true, value: undefined });
    await act(async () => button('Copy SteamID for Alice').click());
    assert.match(document.body.textContent, /Copy failed. Select the displayed SteamID 76561198154367261/);
  } finally {
    await ui.finish();
    if (descriptor) Object.defineProperty(globalThis.navigator, 'clipboard', descriptor); else delete globalThis.navigator.clipboard;
  }
});

test('management team search follows updated rosters', async () => {
  const ui = await page('/manage#teams', async () => { throw new Error('Directory uses snapshots'); });
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token'); await ui.render();
    const team = { id: 'team', name: 'Friends', captainSteamId: '1', members: [{ steamId: '1', gameName: 'Alice' }, { steamId: '2', gameName: 'Bob' }] };
    const state = { capturedAt: '2026-11-15T10:00:00Z', hosts: [], machines: [], matches: [], players: [], maps: [], teams: [team] };
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot(state); });
    const input = document.querySelector('.team-directory input');
    Object.getOwnPropertyDescriptor(Object.getPrototypeOf(input), 'value').set.call(input, 'bob');
    await act(async () => input.dispatchEvent(new ui.window.Event('input', { bubbles: true })));
    assert.match(document.querySelector('.team-directory').textContent, /1 of 1 teams/);
    await act(async () => ui.sockets[0].snapshot({ ...state, capturedAt: '2026-11-15T10:00:01Z', teams: [{ ...team, members: [team.members[0]] }] }));
    assert.equal(input.value, 'bob');
    assert.match(document.querySelector('.team-directory').textContent, /No teams or members match/);
  } finally { await ui.finish(); }
});

test('management retrieves join passwords without host shell commands', async () => {
  const host = { id: 'server', name: 'Friends', address: '100.89.5.109', port: 27245, map: 'de_mirage', activeMap: '', desiredState: 'stopped', observedState: 'stopped', status: 'stopped', online: true, driver: 'managed', machineId: 'machine', revision: 2, whitelist: null, actions: { start: false, stop: false, changeMap: false, openMatch: false } };
  const state = { capturedAt: '2026-11-12T10:00:00Z', hosts: [host], machines: [], matches: [], players: [], maps: [], presets: [] };
  let resolvePassword;
  let requests = 0;
  const ui = await page('/manage', async (url) => {
    if (url === '/api/hosts/server/join-password') {
      requests++;
      if (requests === 2) return new Promise((resolve) => { resolvePassword = resolve; });
      return response({ password: 'friend-password' });
    }
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token');
    await ui.render();
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot(state); });
    await act(async () => button('Servers').click());
    await act(async () => button('Manage Friends').click());
    await act(async () => button('Player access').click());
    await act(async () => button('Show join password').click());
    assert.match(document.body.textContent, /friend-password/);
    assert.doesNotMatch(document.body.textContent, /clutchbase-host credentials/);
    await act(async () => button('Hide password').click());
    assert.doesNotMatch(document.body.textContent, /friend-password/);
    await act(async () => button('Show join password').click());
    await act(async () => button('Sign out').click());
    const token = document.querySelector('input[name="token"]');
    token.value = 'test-admin-token';
    await act(async () => token.form.dispatchEvent(new ui.window.Event('submit', { bubbles: true, cancelable: true })));
    await act(async () => { ui.sockets[1].open(); ui.sockets[1].snapshot({ ...state, capturedAt: '2026-11-12T10:00:01Z' }); });
    await act(async () => { resolvePassword(response({ password: 'stale-password' })); });
    await act(async () => button('Servers').click());
    await act(async () => button('Manage Friends').click());
    await act(async () => button('Player access').click());
    assert.doesNotMatch(document.body.textContent, /stale-password|friend-password/);
    assert.ok(button('Show join password'));
    await act(async () => button('Show join password').click());
    assert.match(document.body.textContent, /friend-password/);
    await act(async () => button('Sign out').click());
    const nextToken = document.querySelector('input[name="token"]');
    nextToken.value = 'test-admin-token';
    await act(async () => nextToken.form.dispatchEvent(new ui.window.Event('submit', { bubbles: true, cancelable: true })));
    await act(async () => { ui.sockets[2].open(); ui.sockets[2].snapshot({ ...state, capturedAt: '2026-11-12T10:00:02Z' }); });
    await act(async () => button('Servers').click());
    await act(async () => button('Manage Friends').click());
    await act(async () => button('Player access').click());
    assert.doesNotMatch(document.body.textContent, /friend-password/);
  } finally { await ui.finish(); }
});

test('management updates CS2 only when the stopped server action is offered', async () => {
  const host = { id: 'server', name: 'Friends', address: '100.89.5.109', port: 27245, map: 'de_mirage', activeMap: '', desiredState: 'stopped', observedState: 'stopped', status: 'stopped', online: true, driver: 'managed', machineId: 'machine', revision: 2, whitelist: null, gameUpdateStatus: '', gameUpdateError: '', reporterInventory: { status: 'different', installedHash: 'a'.repeat(64), stagedHash: 'b'.repeat(64), checkedAt: '2026-11-12T10:00:00Z' }, actions: { start: true, stop: false, changeMap: true, openMatch: false, updateGame: true } };
  const state = { capturedAt: '2026-11-12T10:00:00Z', hosts: [host], machines: [], matches: [], players: [], maps: [], presets: [] };
  let requested = false;
  const ui = await page('/manage', async (url, options = {}) => {
    if (url === '/api/hosts/server/game-update') { requested = true; assert.deepEqual(JSON.parse(options.body), { revision: 2 }); return response({ status: 'pending' }, 202); }
    if (url === '/api/state') return response(state);
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token');
    await ui.render();
    const socket = ui.sockets[0];
    await act(async () => { socket.open(); socket.snapshot(state); });
    await act(async () => button('Servers').click());
    await act(async () => button('Manage Friends').click());
    await act(async () => button('Update CS2').click());
    assert.equal(requested, true);
    await act(async () => socket.snapshot({ ...state, capturedAt: '2026-11-12T10:00:01Z', hosts: [{ ...host, gameUpdateStatus: 'pending', actions: { ...host.actions, updateGame: false, start: false } }] }));
    assert.match(document.body.textContent, /CS2 update requested/);
    assert.match(document.body.textContent, /Server changes are unavailable until the update finishes/);
    assert.equal([...document.querySelectorAll('button')].some(x => x.textContent.trim() === 'Update CS2'), false);
    await act(async () => socket.snapshot({ ...state, capturedAt: '2026-11-12T10:00:02Z', hosts: [{ ...host, reporterUpdateStatus: 'pending', actions: { ...host.actions, updateGame: false, start: false } }] }));
    await act(async () => button('Updates').click());
    assert.match(document.body.textContent, /Wait for the plugin update to finish before updating CS2/);
    assert.match(document.body.textContent, /Differs from staged bundle/);
    assert.ok(document.body.textContent.includes('Installed SHA-256: ' + 'a'.repeat(12) + '…'));
  } finally { await ui.finish(); }
});

test('page selection follows URL fragments for direct links and history navigation', async () => {
  const ui = await page('/play#profile', async () => response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' }));
  try {
    await ui.render();
    assert.equal(document.querySelector('input').value, 'Friend');
    assert.equal(button('Profile').getAttribute('aria-current'), 'page');
    await act(async () => button('Matches').click());
    assert.equal(ui.window.location.hash, '#matches');
    await act(async () => {
      ui.window.location.hash = '#servers';
      ui.window.dispatchEvent(new ui.window.HashChangeEvent('hashchange'));
    });
    assert.equal(button('Servers').getAttribute('aria-current'), 'page');
    assert.match(document.body.textContent, /Connecting to your servers/);
  } finally { await ui.finish(); }
});

test('signed-out deep links preserve their destination through sign in', async () => {
  const ui = await page('/play#matches?room=friday', async () => response({ steamId: null, steamEnabled: true, gameName: '' }));
  try {
    await ui.render();
    assert.match(document.body.textContent, /Sign in to see the servers/);
    assert.equal(new URL(document.querySelector('.steam-sign-in').href).searchParams.get('returnTo'), '#matches?room=friday');
    await act(async () => { ui.window.location.hash = '#tournaments?tournament=cup'; ui.window.dispatchEvent(new ui.window.HashChangeEvent('hashchange')); });
    assert.equal(new URL(document.querySelector('.steam-sign-in').href).searchParams.get('returnTo'), '#tournaments?tournament=cup');
  } finally { await ui.finish(); }
});

test('server access links preserve the selected server through Steam sign in', async () => {
  const ui = await page('/play#access?server=opaque-server', async (url) => {
    if (url === '/api/player/session') return response({ steamId: null, steamEnabled: true, gameName: '' });
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    await ui.render();
    const link = document.querySelector('.steam-sign-in');
    assert.ok(link);
    assert.match(link.getAttribute('href'), /returnTo=%23access%3Fserver%3Dopaque-server/);
    assert.match(document.body.textContent, /Sign in with Steam to request access/);
  } finally { await ui.finish(); }
});

test('player submits an explicit access request and follows its canonical pending state', async () => {
  let submitted = false;
  const pending = { id: 'request', hostId: 'opaque-server', hostName: 'Friday friends', status: 'pending', revision: 1 };
  const ui = await page('/play#access?server=opaque-server', async (url, options) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' });
    if (url === '/api/player/access-requests' && options?.method === 'POST') {
      submitted = true; assert.deepEqual(JSON.parse(options.body), { hostId: 'opaque-server' }); return response(pending);
    }
    if (url === '/api/player/access-requests') return response(submitted ? [pending] : []);
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    await ui.render();
    await act(async () => button('Submit access request').click());
    assert.equal(submitted, true);
    assert.match(document.body.textContent, /Access request submitted/);
    assert.match(document.body.textContent, /Friday friends/);
    assert.match(document.body.textContent, /Pending/);
  } finally { await ui.finish(); }
});

test('management approves an access request only after live state and confirmation', async () => {
  let approved = false;
  const item = { id: 'request', hostId: 'server', hostName: 'Friday friends', steamId: '76561198154367261', gameName: 'Friend', status: 'pending', revision: 4, canApprove: true, requiresWhitelistEnable: false, blocker: '' };
  const ui = await page('/manage#access-requests', async (url, options) => {
    if (url === '/api/access-requests/request' && options?.method === 'PATCH') {
      assert.deepEqual(JSON.parse(options.body), { revision: 4, action: 'approve' }); approved = true; return new Response(null, { status: 204 });
    }
    if (url === '/api/access-requests') return response([approved ? { ...item, status: 'approved', revision: 5, canApprove: false } : item]);
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token');
    await ui.render();
    const accessSocket = ui.sockets.find((socket) => new URL(socket.url).searchParams.get('view') === 'access-requests');
    assert.ok(accessSocket);
    await act(async () => { accessSocket.open(); accessSocket.snapshot([item]); });
    await act(async () => button('Approve').click());
    assert.match(document.body.textContent, /Confirm approve/);
    await act(async () => button('Confirm approve').click());
    assert.equal(approved, true);
    assert.match(document.body.textContent, /Approved/);
  } finally { await ui.finish(); }
});

test('management requires explicit confirmation before enabling an empty whitelist', async () => {
  let approved = false;
  const item = { id: 'request', hostId: 'server', hostName: 'Friday friends', steamId: '76561198154367261', gameName: 'Friend', status: 'pending', revision: 4, canApprove: true, requiresWhitelistEnable: true, blocker: '' };
  const ui = await page('/manage#access-requests', async (url, options) => {
    if (url === '/api/access-requests/request' && options?.method === 'PATCH') {
      assert.deepEqual(JSON.parse(options.body), { revision: 4, action: 'approve', enableWhitelist: true }); approved = true; return new Response(null, { status: 204 });
    }
    if (url === '/api/access-requests') return response([approved ? { ...item, status: 'approved', revision: 5, canApprove: false, requiresWhitelistEnable: false } : item]);
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token');
    await ui.render();
    const accessSocket = ui.sockets.find((socket) => new URL(socket.url).searchParams.get('view') === 'access-requests');
    await act(async () => { accessSocket.open(); accessSocket.snapshot([item]); });
    await act(async () => button('Approve').click());
    assert.match(document.body.textContent, /Approval will enable whitelist enforcement/);
    assert.match(document.body.textContent, /password.*not on the roster will lose access/);
    const acknowledgement = document.querySelector('input[type="checkbox"]');
    assert.ok(acknowledgement);
    assert.equal(acknowledgement.checked, false);
    assert.equal(button('Confirm approve').disabled, true);
    await act(async () => { acknowledgement.click(); });
    assert.equal(button('Confirm approve').disabled, false);
    await act(async () => button('Confirm approve').click());
    assert.equal(approved, true);
    assert.match(document.body.textContent, /Approved/);
  } finally { await ui.finish(); }
});

test('management resets whitelist confirmation after cancel and rejects stale access approvals', async () => {
  const item = { id: 'request', hostId: 'server', hostName: 'Friday friends', steamId: '76561198154367261', gameName: 'Friend', status: 'pending', revision: 4, canApprove: true, requiresWhitelistEnable: true, blocker: '' };
  const ui = await page('/manage#access-requests', async (url) => {
    if (url === '/api/access-requests') return response([item]);
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token');
    await ui.render();
    const accessSocket = ui.sockets.find((socket) => new URL(socket.url).searchParams.get('view') === 'access-requests');
    await act(async () => { accessSocket.open(); accessSocket.snapshot([item]); });
    await act(async () => button('Approve').click());
    await act(async () => document.querySelector('input[type="checkbox"]').click());
    await act(async () => button('Cancel').click());
    await act(async () => button('Approve').click());
    assert.equal(document.querySelector('input[type="checkbox"]').checked, false);
    await act(async () => accessSocket.snapshot([{ ...item, revision: 5 }]));
    assert.match(document.body.textContent, /Request changed/);
    assert.equal(button('Confirm approve').disabled, true);
  } finally { await ui.finish(); }
});

test('public session check can be retried after an API failure', async () => {
  let attempts = 0;
  const ui = await page('/play', async () => {
    attempts++;
    if (attempts === 1) throw new Error('API unavailable');
    return response({ steamId: null, steamEnabled: true, gameName: '' });
  });
  try {
    await ui.render();
    assert.match(document.body.textContent, /Unable to load your session/);
    await act(async () => button('Try again').click());
    assert.equal(attempts, 2);
    assert.match(document.body.textContent, /Sign in to see the servers/);
    assert.doesNotMatch(document.body.textContent, /API is unavailable/);
  } finally { await ui.finish(); }
});

test('public round-robin standings update only from the canonical snapshot and show shared first place', async () => {
  const ui = await page('/play#tournaments?tournament=league', async (url) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' });
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    await ui.render();
    const tournament = { id: 'league', name: 'Friends league', format: 'round_robin', bestOf: 1, playersPerSide: 1, mapPool: [], status: 'active', winnerTeamId: '', entries: [], matches: [], standings: [{ teamId: 'a', teamName: 'Alpha', rank: 1, played: 1, wins: 1, losses: 0 }, { teamId: 'b', teamName: 'Bravo', rank: 2, played: 1, wins: 0, losses: 1 }] };
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot({ capturedAt: '2026-11-15T10:00:00Z', servers: [], matches: [], tournaments: [tournament] }); });
    const table = document.querySelector('.tournament-standings table');
    assert.ok(table);
    assert.equal(table.querySelectorAll('tbody tr').length, 2);
    assert.match(table.textContent, /Alpha/);
    assert.equal(document.querySelectorAll('.tournament-standings button').length, 0);
    await act(async () => ui.sockets[0].snapshot({ capturedAt: '2026-11-15T10:01:00Z', servers: [], matches: [], tournaments: [{ ...tournament, status: 'complete', standings: tournament.standings.map((s) => ({ ...s, rank: 1, played: 2, wins: 1, losses: 1 })) }] }));
    assert.match(document.body.textContent, /Shared first place/);
    assert.equal(table.querySelectorAll('tbody th').length, 2);
    assert.ok([...table.querySelectorAll('tbody th')].every((row) => row.textContent.includes('First place')));
  } finally { await ui.finish(); }
});

test('history presents reported and manual stats separately and keeps player view read-only', async () => {
  const { MatchHistory } = await vite.ssrLoadModule('/src/MatchHistory.tsx');
  const ui = await page('/play#history', async () => { throw new Error('History rendering must use its live snapshot'); });
  try {
    await act(async () => ui.root.render(React.createElement(MatchHistory, { player: true, onClose() {} })));
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot({ capturedAt: '2026-11-15T10:00:00Z', nextCursor: '', matches: [{ id: 'match', title: 'Friday match', hostName: 'Friends', map: 'de_mirage', createdAt: '2026-11-15T09:00:00Z', resultCT: 13, resultT: 11, reportedCT: 13, reportedT: 9, resultRevision: 1, canEditResult: false, seriesMatch: false, participants: [{ steamId: '76561198154367261', gameName: 'Friend', side: 'ct', reportedStats: { kills: 20, deaths: 10, assists: 3 }, stats: { kills: 21, deaths: 10, assists: 3 } }] }] }); });
    assert.equal(document.querySelectorAll('.history-card').length, 1);
    const row = document.querySelector('.history-scoreboard tbody tr');
    assert.deepEqual([...row.querySelectorAll('td')].map((cell) => cell.textContent), ['CT', '20 / 10 / 3', '21 / 10 / 3']);
    assert.match(document.body.textContent, /Reported final score/);
    assert.match(document.body.textContent, /Manual result/);
    assert.equal(document.querySelectorAll('.history-card button').length, 0);
  } finally { await ui.finish(); }
});

test('history search and pagination use the server query', async () => {
  const { MatchHistory } = await vite.ssrLoadModule('/src/MatchHistory.tsx');
  const ui = await page('/play#history', async () => { throw new Error('History uses live snapshots'); });
  try {
    await act(async () => ui.root.render(React.createElement(MatchHistory, { player: true, onClose() {} })));
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot({ capturedAt: '2026-11-15T10:00:00Z', nextCursor: '', matches: [] }); });
    const input = document.querySelector('input[type="search"]');
    Object.getOwnPropertyDescriptor(Object.getPrototypeOf(input), 'value').set.call(input, 'nuke');
    await act(async () => input.dispatchEvent(new ui.window.Event('input', { bubbles: true })));
    await act(async () => input.form.dispatchEvent(new ui.window.Event('submit', { bubbles: true, cancelable: true })));
    assert.equal(new URL(ui.sockets[1].url).searchParams.get('search'), 'nuke');
    await act(async () => { ui.sockets[1].open(); ui.sockets[1].snapshot({ capturedAt: '2026-11-15T10:00:01Z', nextCursor: 'cursor-1', matches: [] }); });
    await act(async () => button('Next').click());
    assert.equal(new URL(ui.sockets[2].url).searchParams.get('before'), 'cursor-1');
    assert.equal(new URL(ui.sockets[2].url).searchParams.get('search'), 'nuke');
    await act(async () => { ui.sockets[2].open(); ui.sockets[2].snapshot({ capturedAt: '2026-11-15T10:00:02Z', nextCursor: '', matches: [] }); });
    Object.getOwnPropertyDescriptor(Object.getPrototypeOf(input), 'value').set.call(input, 'mirage');
    await act(async () => input.dispatchEvent(new ui.window.Event('input', { bubbles: true })));
    await act(async () => input.form.dispatchEvent(new ui.window.Event('submit', { bubbles: true, cancelable: true })));
    assert.equal(new URL(ui.sockets[3].url).searchParams.get('search'), 'mirage');
    assert.equal(new URL(ui.sockets[3].url).searchParams.has('before'), false);
  } finally { await ui.finish(); }
});

test('series history orders configured maps, separates reported totals and edits map scores by revision', async () => {
  const { MatchHistory } = await vite.ssrLoadModule('/src/MatchHistory.tsx');
  let requestBody;
  const ui = await page('/play#history', async (url, options) => {
    if (url === '/api/matches/match/series-map-result') {
      requestBody = JSON.parse(options.body);
      return new Response(null, { status: 204 });
    }
    throw new Error(`Unexpected fetch ${url}`);
  });
  const match = {
    id: 'match', title: 'Friday series', hostName: 'Friends', map: 'de_mirage', createdAt: '2026-11-15T09:00:00Z',
    resultCT: null, resultT: null, reportedCT: 11, reportedT: 8, seriesMatch: true, seriesTeam1: 2, seriesTeam2: 1,
    seriesMaps: [
      { number: 1, map: 'de_mirage', team1: 13, team2: 9 },
      { number: 2, map: 'de_nuke', team1: null, team2: null },
      { number: 3, map: 'de_inferno', team1: null, team2: null },
    ],
    resultRevision: 4, canEditResult: true, removedHost: false, participants: [],
  };
  try {
    await act(async () => ui.root.render(React.createElement(MatchHistory, { token: 'admin-token', onClose() {} })));
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot({ capturedAt: '2026-11-15T10:00:00Z', nextCursor: '', matches: [match] }); });
    const maps = [...document.querySelectorAll('.history-series-map-heading strong')].map((node) => node.textContent);
    assert.deepEqual(maps, ['Map 1 · mirage', 'Map 2 · nuke', 'Map 3 · inferno']);
    assert.match(document.body.textContent, /Score not recorded/);
    assert.match(document.body.textContent, /Reported series result/);
    assert.match(document.body.textContent, /Team 1 2 – 1 Team 2/);
    assert.match(document.body.textContent, /Reported final map · CT 11 – 8 T/);
    assert.ok(button('Record map score'));
    await act(async () => button('Record map score').click());
    const form = document.querySelector('.history-series-map-form');
    form.querySelector('[name="team1"]').value = '16';
    form.querySelector('[name="team2"]').value = '12';
    await act(async () => form.dispatchEvent(new ui.window.Event('submit', { bubbles: true, cancelable: true })));
    assert.deepEqual(requestBody, { mapNumber: 2, revision: 4, team1: 16, team2: 12 });
    assert.match(document.body.textContent, /Score not recorded/);
    await act(async () => ui.sockets[0].snapshot({ capturedAt: '2026-11-15T10:00:01Z', nextCursor: '', matches: [{ ...match, resultRevision: 5, seriesMaps: match.seriesMaps.map((seriesMap) => seriesMap.number === 2 ? { ...seriesMap, team1: 16, team2: 12 } : seriesMap) }] }));
    assert.match(document.body.textContent, /Team 1: 16/);
    assert.match(document.body.textContent, /Team 2: 12/);
  } finally { await ui.finish(); }
});

test('player pause requests wait for canonical state and disable controls on disconnect', async () => {
  let requested;
  const ui = await page('/play#matches', async (url, options) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' });
    if (url === '/api/player/matches/room/control') { requested = JSON.parse(options.body); return response({ status: 'pending' }, 202); }
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    await ui.render();
    const room = { id: 'room', hostId: 'host', title: 'Friday match', hostName: 'Friends', map: 'de_mirage', status: 'open', loadStatus: 'complete', players: 2, ready: 2, playersPerSide: 1, ctPlayers: 1, tPlayers: 1, participants: [], joined: true, mySide: 'ct', myReady: true, controlRevision: 7, pauseVoteThreshold: 2, pauseVotes: 0, myPauseVote: false, matchPhase: 'live', controlStatus: '', controlAction: '', actions: { pause: true, resume: false } };
    const snapshot = (match, capturedAt) => ({ capturedAt, servers: [], matches: [match], tournaments: [] });
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot(snapshot(room, '2026-11-15T10:00:00Z')); });
    assert.match(document.body.textContent, /Pause votes: 0 \/ 2/);
    await act(async () => button('Vote to pause').click());
    assert.deepEqual(requested, { action: 'pause', revision: 7 });
    assert.match(document.body.textContent, /Match live/);
    assert.doesNotMatch(document.body.textContent, /Match paused or pause requested/);
    await act(async () => ui.sockets[0].snapshot(snapshot({ ...room, controlRevision: 8, controlStatus: 'pending', controlAction: 'pause', actions: {} }, '2026-11-15T10:00:01Z')));
    assert.match(document.body.textContent, /Pause requested. Waiting for MatchZy/);
    assert.ok(![...document.querySelectorAll('button')].some((b) => b.textContent === 'Vote to pause'));
    await act(async () => ui.sockets[0].snapshot(snapshot({ ...room, controlRevision: 8, controlStatus: 'complete', matchPhase: 'paused', actions: { resume: true } }, '2026-11-15T10:00:02Z')));
    assert.match(document.body.textContent, /Match paused or pause requested/);
    assert.equal(button('Request resume').disabled, false);
    await act(async () => ui.sockets[0].onclose());
    assert.equal(button('Request resume').disabled, true);
  } finally { await ui.finish(); }
});

test('management configures pause voting with the room control revision', async () => {
  let submitted;
  const match = { id: 'room', hostId: 'host', title: 'Friday match', hostName: 'Friends', map: 'de_mirage', status: 'open', players: 2, ready: 2, playersPerSide: 1, ctPlayers: 1, tPlayers: 1, participants: [], pauseVoteThreshold: 1, pauseVotes: 0, controlRevision: 9, canConfigurePauseVoting: true, canLoad: false, loadStatus: '', endedAt: null, setupUnloaded: false, resultCT: null, resultT: null, reportedCT: null, reportedT: null, actions: {} };
  const state = { capturedAt: '2026-11-15T10:00:00Z', hosts: [], machines: [], players: [], matches: [match], tournaments: [], teams: [], maps: [] };
  const ui = await page('/manage#matches', async (url, options = {}) => {
    if (url === '/api/matches/room/pause-voting') { submitted = JSON.parse(options.body); return response({ status: 'updated' }); }
    if (url === '/api/state') return response(state);
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token'); await ui.render();
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot(state); });
    await act(async () => button('Matches').click());
    const input = document.querySelector('[aria-label="Pause voting"] input');
    Object.getOwnPropertyDescriptor(ui.window.HTMLInputElement.prototype, 'value').set.call(input, '2');
    await act(async () => input.dispatchEvent(new ui.window.Event('input', { bubbles: true })));
    await act(async () => button('Save threshold').click());
    assert.deepEqual(submitted, { threshold: 2, revision: 9 });
  } finally { await ui.finish(); }
});

test('statistics navigation renders null stats as unavailable and handles canonical updates and expiry', async () => {
  const ui = await page('/manage', async () => response({}));
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token');
    await ui.render();
    const main = ui.sockets[0];
    await act(async () => { main.open(); main.snapshot({ capturedAt: '2026-11-15T10:00:00Z', hosts: [], machines: [], matches: [], players: [], maps: [], tournaments: [], teams: [] }); });
    await act(async () => button('Statistics').click());
    const stats = ui.sockets.at(-1);
    assert.equal(stats.url.searchParams.get("view"), "stats");
    await act(async () => { stats.open(); stats.snapshot({ capturedAt: '2026-11-15T10:01:00Z', truncated: false, players: [{ steamId: '765', gameName: 'Ace', rooms: 1, scoredRooms: 1, manualScores: 0, wins: 1, losses: 0, draws: 0, statsRooms: 1, manualStats: 0, kills: null, deaths: null, assists: null }] }); });
    assert.match(document.body.textContent, /Unavailable/);
    await act(async () => stats.snapshot({ capturedAt: '2026-11-15T10:01:01Z', truncated: false, players: [{ steamId: '765', gameName: 'Ace', rooms: 3, scoredRooms: 1, wins: 1, losses: 0, draws: 0, statsRooms: 0, kills: null, deaths: null, assists: null, mapExcludedRooms: 1, maps: [{ map: 'de_nuke', rooms: 2, scoredRooms: 1, wins: 1, losses: 0, draws: 0 }] }] }));
    assert.match(document.querySelector('.stats-maps').textContent, /nuke/);
    assert.match(document.querySelector('.stats-maps').textContent, /1 room has no map appearance here/);
    assert.match(document.querySelector('.stats-maps').textContent, /Unrecorded series maps are not counted/);
    await act(async () => stats.snapshot({ capturedAt: '2026-11-15T10:02:00Z', truncated: false, players: [] }));
    assert.match(document.body.textContent, /No statistics are available yet/);
    await act(async () => stats.snapshot({ code: 401, error: 'Sign in again' }));
    assert.match(document.body.textContent, /Session expired/);
  } finally { await ui.finish(); }
});

test('public statistics stays in the sidebar and preserves a labeled snapshot while reconnecting', async () => {
  const ui = await page('/play#stats', async (url) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' });
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    await ui.render();
    const stats = ui.sockets.find((socket) => socket.url.searchParams.get('view') === 'player-stats');
    assert.ok(stats);
    assert.ok(document.querySelector('aside').contains(button('My statistics')));
    assert.equal(button('My statistics').getAttribute('aria-current'), 'page');
    await act(async () => { stats.open(); stats.snapshot({ capturedAt: '2026-11-15T10:00:00Z', truncated: false, players: [{ steamId: '76561198154367261', gameName: 'Friend', rooms: 2, scoredRooms: 1, manualScores: 0, wins: 1, losses: 0, draws: 0, statsRooms: 0, manualStats: 0, kills: null, deaths: null, assists: null }] }); });
    assert.match(document.querySelector('.statistics').textContent, /Rooms played2/);
    assert.match(document.querySelector('.statistics').textContent, /Unavailable/);
    await act(async () => stats.onclose());
    assert.match(document.querySelector('.statistics').textContent, /last received snapshot/);
    assert.match(document.querySelector('.statistics').textContent, /Rooms played2/);
    await act(async () => stats.snapshot({ capturedAt: '2026-11-15T09:59:00Z', truncated: false, players: [] }));
    assert.match(document.querySelector('.statistics').textContent, /Rooms played2/);
  } finally { await ui.finish(); }
});

test('team removal and disband require confirmation and wait for canonical roster updates', async () => {
  const requests = [];
  const ui = await page('/play#teams', async (url, options) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Captain' });
    requests.push({ url, method: options.method, body: JSON.parse(options.body) });
    return new Response(null, { status: 204 });
  });
  try {
    await ui.render();
    const team = { id: 'team', name: 'Friends', captainSteamId: '76561198154367261', revision: 7, rosterLocked: false, registered: false, members: [{ steamId: '76561198154367261', gameName: 'Captain' }, { steamId: '76561198154367262', gameName: 'Friend' }], actions: { invite: true, leave: false, removeMember: true, transferCaptain: true, disband: true } };
    const snapshot = (t, time) => ({ capturedAt: time, servers: [], matches: [], tournaments: [], team: t });
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot(snapshot(team, '2026-11-15T10:00:00Z')); });
    await act(async () => button('Remove member').click());
    assert.equal(requests.length, 0);
    assert.match(document.body.textContent, /Remove Friend from Friends/);
    await act(async () => button('Confirm removal').click());
    assert.deepEqual(requests[0], { url: '/api/player/teams/team/members/76561198154367262', method: 'DELETE', body: { revision: 7 } });
    assert.equal(document.querySelectorAll('.team-members li').length, 2);
    assert.match(document.body.textContent, /Waiting for the updated team/);
    const updated = { ...team, revision: 8, members: team.members.slice(0, 1), actions: { ...team.actions, removeMember: false, transferCaptain: false } };
    await act(async () => ui.sockets[0].snapshot(snapshot(updated, '2026-11-15T10:00:01Z')));
    assert.equal(document.querySelectorAll('.team-members li').length, 1);
    await act(async () => button('Disband team').click());
    assert.equal(requests.length, 1);
    await act(async () => button('Confirm disband').click());
    assert.deepEqual(requests[1], { url: '/api/player/teams/team', method: 'DELETE', body: { revision: 8 } });
    assert.ok(document.querySelector('.public-team'));
    await act(async () => ui.sockets[0].snapshot(snapshot(null, '2026-11-15T10:00:02Z')));
    assert.match(document.body.textContent, /You are not on a team yet/);
  } finally { await ui.finish(); }
});

test('captain transfer rejects stale confirmation and locked team actions follow server snapshot', async () => {
  const requests = [];
  const ui = await page('/play#teams', async (url, options) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Captain' });
    requests.push({ url, body: JSON.parse(options.body), method: options.method });
    return new Response(null, { status: 204 });
  });
  try {
    await ui.render();
    const team = { id: 'team', name: 'Friends', captainSteamId: '76561198154367261', revision: 2, rosterLocked: false, registered: false, members: [{ steamId: '76561198154367261', gameName: 'Captain' }, { steamId: '76561198154367262', gameName: 'Friend' }], actions: { transferCaptain: true } };
    const snapshot = (t, time) => ({ capturedAt: time, servers: [], matches: [], tournaments: [], team: t });
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot(snapshot(team, '2026-11-15T10:00:00Z')); });
    await act(async () => button('Make captain').click());
    await act(async () => ui.sockets[0].snapshot(snapshot({ ...team, revision: 3 }, '2026-11-15T10:00:01Z')));
    assert.equal(button('Confirm captain transfer').disabled, true);
    assert.match(document.body.textContent, /The team changed/);
    await act(async () => button('Cancel').click());
    await act(async () => button('Make captain').click());
    await act(async () => button('Confirm captain transfer').click());
    assert.deepEqual(requests[0], { url: '/api/player/teams/team/captain', body: { revision: 3, steamId: '76561198154367262' }, method: 'PATCH' });
    await act(async () => ui.sockets[0].snapshot(snapshot({ ...team, revision: 4, rosterLocked: true, registered: true, actions: {} }, '2026-11-15T10:00:02Z')));
    assert.match(document.body.textContent, /roster is locked/);
    assert.equal(document.querySelectorAll('.team-members button').length, 0);
  } finally { await ui.finish(); }
});

test('team invite display clears with the next roster revision and controls disable on disconnect', async () => {
  const code = 'private-team-invite-code';
  const ui = await page('/play#teams', async (url) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Captain' });
    if (url === '/api/player/teams/team/invite') return response({ code });
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    await ui.render();
    const team = { id: 'team', name: 'Friends', captainSteamId: '76561198154367261', revision: 0, rosterLocked: false, registered: false, members: [{ steamId: '76561198154367261', gameName: 'Captain' }], actions: { invite: true, disband: true } };
    const snapshot = (revision, time) => ({ capturedAt: time, servers: [], matches: [], tournaments: [], team: { ...team, revision } });
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot(snapshot(0, '2026-11-15T10:00:00Z')); });
    await act(async () => button('Copy invite code').click());
    assert.match(document.querySelector('.team-invite').textContent, /private-team-invite-code/);
    await act(async () => ui.sockets[0].snapshot(snapshot(1, '2026-11-15T10:00:01Z')));
    assert.equal(document.querySelector('.team-invite'), null);
    await act(async () => ui.sockets[0].onclose());
    assert.equal(button('Copy invite code').disabled, true);
    assert.equal(button('Disband team').disabled, true);
  } finally { await ui.finish(); }
});

test('tournament withdrawal confirms the current registration and waits for the canonical entry removal', async () => {
  let selection;
  const ui = await page('/play#tournaments?tournament=cup', async (url, options) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Captain' });
    if (url === '/api/player/tournaments/cup/registration') { assert.equal(options.method, 'DELETE'); selection = JSON.parse(options.body); return new Response(null, { status: 204 }); }
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    await ui.render();
    const tournament = { id: 'cup', name: 'Friends cup', status: 'checkin', bestOf: 1, playersPerSide: 1, entries: [{ teamId: 'team', teamName: 'Friends', status: 'checked_in' }], matches: [], myTeamId: 'team', myTeamRevision: 0, myEntryStatus: 'checked_in', canWithdrawTeam: true, mapPool: [] };
    const snapshot = (cup, time) => ({ capturedAt: time, servers: [], matches: [], tournaments: [cup], team: { id: 'team', name: 'Friends', captainSteamId: '76561198154367261', members: [] } });
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot(snapshot(tournament, '2026-11-15T10:00:00Z')); });
    await act(async () => button('Withdraw team').click());
    assert.equal(selection, undefined);
    assert.match(document.body.textContent, /Registration is closed, so the team cannot rejoin/);
    await act(async () => button('Confirm withdrawal').click());
    assert.deepEqual(selection, { teamRevision: 0, teamId: 'team', entryStatus: 'checked_in', phase: 'checkin' });
    assert.match(document.body.textContent, /Your team: checked_in/);
    assert.match(document.body.textContent, /Waiting for the updated tournament/);
    await act(async () => ui.sockets[0].snapshot(snapshot({ ...tournament, entries: [], myEntryStatus: '', canWithdrawTeam: false }, '2026-11-15T10:00:01Z')));
    assert.doesNotMatch(document.body.textContent, /Your team: checked_in/);
    assert.equal(document.querySelector('.tournament-withdrawal'), null);
  } finally { await ui.finish(); }
});

test('tournament phase changes disable an open withdrawal confirmation', async () => {
  const ui = await page('/play#tournaments?tournament=cup', async (url) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Captain' });
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    await ui.render();
    const cup = { id: 'cup', name: 'Friends cup', status: 'open', bestOf: 1, playersPerSide: 1, entries: [], matches: [], myTeamId: 'team', myTeamRevision: 0, myEntryStatus: 'registered', canWithdrawTeam: true, mapPool: [] };
    const snapshot = (t, time) => ({ capturedAt: time, servers: [], matches: [], tournaments: [t] });
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot(snapshot(cup, '2026-11-15T10:00:00Z')); });
    await act(async () => button('Withdraw team').click());
    await act(async () => ui.sockets[0].snapshot(snapshot({ ...cup, status: 'checkin' }, '2026-11-15T10:00:01Z')));
    assert.equal(button('Confirm withdrawal').disabled, true);
    assert.match(document.body.textContent, /tournament or team changed/);
  } finally { await ui.finish(); }
});

test('machine resources use canonical samples and clear stale machine readings', async () => {
  const ui = await page('/manage', async () => response({}));
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token');
    await ui.render();
    const machine = { id: 'machine', name: 'Byttorrent', driver: 'managed', online: true, createServer: true, resources: { cpuPercent: 25.5, memoryTotalBytes: 8 * 1024 ** 3, memoryAvailableBytes: 2 * 1024 ** 3 } };
    const snapshot = (m, time) => ({ capturedAt: time, hosts: [], machines: [m], matches: [], players: [], maps: ['de_mirage'], tournaments: [], teams: [] });
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot(snapshot(machine, '2026-11-15T10:00:00Z')); });
    assert.equal(document.querySelector('meter[aria-label="CPU use on Byttorrent"]').getAttribute('value'), '25.5');
    assert.match(document.body.textContent, /6.0 \/ 8.0 GiB/);
    await act(async () => ui.sockets[0].onclose());
    assert.match(document.body.textContent, /Last received measurements/);
    await act(async () => ui.sockets[0].snapshot(snapshot({ ...machine, resources: { ...machine.resources, cpuPercent: null } }, '2026-11-15T10:00:01Z')));
    assert.match(document.body.textContent, /Awaiting sample/);
    assert.equal(document.querySelector('meter[aria-label="CPU use on Byttorrent"]'), null);
    await act(async () => ui.sockets[0].snapshot(snapshot({ ...machine, online: false, resources: null }, '2026-11-15T10:00:02Z')));
    assert.equal(document.querySelectorAll('.machine-resources meter').length, 0);
    assert.match(document.body.textContent, /Machine resources unavailable/);
  } finally { await ui.finish(); }
});

test('server creation displays the selected machine resources and disables submission when stale', async () => {
  const ui = await page('/manage', async () => response({}));
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token');
    await ui.render();
    const first = { id: 'a', name: 'First', driver: 'managed', online: true, createServer: true, resources: { cpuPercent: 10, memoryTotalBytes: 8 * 1024 ** 3, memoryAvailableBytes: 2 * 1024 ** 3 } };
    const second = { ...first, id: 'b', name: 'Second', resources: { ...first.resources, cpuPercent: 70 } };
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot({ capturedAt: '2026-11-15T10:00:00Z', hosts: [], machines: [first, second], matches: [], players: [], maps: ['de_mirage'], tournaments: [], teams: [] }); });
    await act(async () => button('Create server').click());
    const dialog = document.querySelector('dialog');
    const select = dialog.querySelector('select[name="machine"]');
    assert.equal(select.value, 'a');
    assert.equal(dialog.querySelector('meter[aria-label="CPU use on First"]').getAttribute('value'), '10');
    await act(async () => { select.value = 'b'; select.dispatchEvent(new ui.window.Event('change', { bubbles: true })); });
    assert.equal(dialog.querySelector('meter[aria-label="CPU use on Second"]').getAttribute('value'), '70');
    assert.equal(dialog.querySelector('meter[aria-label="CPU use on First"]'), null);
    await act(async () => ui.sockets[0].onclose());
    assert.equal(select.disabled, true);
    assert.equal(dialog.querySelector('button[type="submit"]').disabled, true);
    assert.match(dialog.textContent, /Last received measurements/);
  } finally { await ui.finish(); }
});

test('live room score updates from snapshots and expires without becoming a final result', async () => {
  const ui = await page('/play#matches', async (url) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' });
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    await ui.render();
    const room = { id: 'room', title: 'Friends match', resultCT: null, resultT: null, reportedCT: null, reportedT: null, hostId: 'host', hostName: 'Friends', map: 'de_mirage', status: 'open', loadStatus: 'complete', players: 2, ready: 2, playersPerSide: 1, ctPlayers: 1, tPlayers: 1, joined: true, mySide: 'ct', participants: [], actions: {}, matchPhase: 'live', liveScore: { setupId: 'setup', map: 'de_nuke', ct: 0, t: 0, observedAt: 1794736800000 } };
    const snapshot = (m, time) => ({ capturedAt: time, servers: [], matches: [m], tournaments: [] });
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot(snapshot(room, '2026-11-15T10:00:00Z')); });
    assert.match(document.querySelector('.room-live-score').textContent, /nuke/);
    assert.match(document.querySelector('.room-score-ct').textContent, /CT 0/);
    assert.match(document.querySelector('.room-live-score').textContent, /Current in-game sides/);
    await act(async () => ui.sockets[0].snapshot(snapshot({ ...room, liveScore: { ...room.liveScore, ct: 8, t: 5 } }, '2026-11-15T10:00:01Z')));
    assert.match(document.querySelector('.room-score-ct').textContent, /CT 8/);
    assert.doesNotMatch(document.body.textContent, /Reported final score/);
    await act(async () => ui.sockets[0].onclose());
    assert.match(document.querySelector('.room-live-score').textContent, /Last received score/);
    await act(async () => ui.sockets[0].snapshot(snapshot({ ...room, liveScore: null }, '2026-11-15T10:00:17Z')));
    assert.equal(document.querySelector('.room-live-score'), null);
    assert.match(document.body.textContent, /Current map score unavailable/);
  } finally { await ui.finish(); }
});

test('admin cancellation confirms and waits for the canonical cancelled tournament', async () => {
  let cancelled;
  let current;
  const ui = await page('/manage#tournaments?tournament=cup', async (url, options) => {
    if (url === '/api/tournaments/cup/cancel') { cancelled = JSON.parse(options.body); return new Response(null, { status: 204 }); }
    if (url === '/api/state') return response(current);
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token');
    await ui.render();
    const cup = { id: 'cup', name: 'Friends cup', status: 'checkin', canCancel: true, canCheckIn: false, canBuildBracket: false, bestOf: 1, playersPerSide: 1, entries: [], matches: [], mapPool: [] };
    current = { capturedAt: '2026-11-15T10:00:00Z', hosts: [], machines: [], matches: [], players: [], maps: [], teams: [], tournaments: [cup] };
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot(current); });
    await act(async () => button('Cancel tournament').click());
    assert.equal(cancelled, undefined);
    assert.match(document.body.textContent, /cannot be undone/);
    await act(async () => button('Confirm cancellation').click());
    assert.deepEqual(cancelled, { phase: 'checkin' });
    assert.match(document.querySelector('.tournament-card .state-pill').textContent, /checkin/);
    assert.match(document.body.textContent, /Waiting for the updated tournament/);
    current = { ...current, capturedAt: '2026-11-15T10:00:01Z', tournaments: [{ ...cup, status: 'cancelled', canCancel: false }] };
    await act(async () => ui.sockets[0].snapshot(current));
    assert.match(document.querySelector('.tournament-card .state-pill').textContent, /cancelled/);
    assert.match(document.body.textContent, /Tournament cancelled. No fixtures were started/);
    assert.equal(document.querySelector('.team-confirmation'), null);
  } finally { await ui.finish(); }
});

test('round-robin winner correction confirms and rejects a stale snapshot', async () => {
  let submitted;
  const ui = await page('/manage#tournaments?tournament=cup', async (url, options = {}) => {
    if (url === '/api/tournaments/cup/matches/fixture/correction') { submitted = JSON.parse(options.body); return new Response(null, { status: 204 }); }
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token'); await ui.render();
    const fixture = { id: 'fixture', round: 1, number: 1, status: 'complete', teamA: { teamId: 'a', teamName: 'Alpha' }, teamB: { teamId: 'b', teamName: 'Bravo' }, winnerTeamId: 'a', resultRevision: 3, canCorrectWinner: true, vetoBans: [], vetoMaps: [], mapSequence: [], vetoStep: 7 };
    const cup = { id: 'cup', name: 'Cup', format: 'round_robin', status: 'complete', bestOf: 1, playersPerSide: 1, entries: [], matches: [fixture], mapPool: [] };
    const state = { capturedAt: '2026-11-15T10:00:00Z', hosts: [], machines: [], matches: [], players: [], maps: [], teams: [], tournaments: [cup] };
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot(state); });
    await act(async () => button('Correct winner').click());
    assert.match(document.body.textContent, /Change the fixture winner to Bravo/);
    await act(async () => ui.sockets[0].snapshot({ ...state, capturedAt: '2026-11-15T10:00:01Z', tournaments: [{ ...cup, matches: [{ ...fixture, resultRevision: 4 }] }] }));
    assert.equal(button('Confirm correction').disabled, true);
    await act(async () => button('Cancel').click());
    await act(async () => button('Correct winner').click());
    await act(async () => button('Confirm correction').click());
    assert.deepEqual(submitted, { teamId: 'b', revision: 4 });
  } finally { await ui.finish(); }
});

test('management resolves a participant dispute using its current revision', async () => {
  let submitted;
  let current;
  const ui = await page('/manage#tournaments?tournament=cup', async (url, options = {}) => {
    if (url === '/api/tournaments/cup/matches/fixture/dispute') { submitted = JSON.parse(options.body); return new Response(null, { status: 204 }); }
    if (url === '/api/state') return response(current);
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token'); await ui.render();
    const match = { id: 'fixture', round: 1, number: 1, status: 'complete', teamA: { teamId: 'alpha', teamName: 'Alpha' }, teamB: { teamId: 'bravo', teamName: 'Bravo' }, winnerTeamId: 'alpha', dispute: { reason: 'Wrong score', revision: 3 }, canDispute: false, vetoBans: [], vetoMaps: [], mapSequence: [], vetoStep: 7, canVeto: false, canReportWinner: false, canOpenRoom: false };
    const cup = { id: 'cup', name: 'Friends cup', status: 'active', canCancel: false, canCheckIn: false, canBuildBracket: false, bestOf: 1, playersPerSide: 1, entries: [], matches: [match], mapPool: [] };
    current = { capturedAt: '2026-11-15T10:00:00Z', hosts: [], machines: [], matches: [], players: [], maps: [], teams: [], tournaments: [cup] };
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot(current); });
    await act(async () => button('Tournaments').click());
    await act(async () => button('View Friends cup').click());
    const textarea = document.querySelector('[aria-label="Fixture dispute"] textarea');
    Object.getOwnPropertyDescriptor(Object.getPrototypeOf(textarea), 'value').set.call(textarea, 'ě'.repeat(1001));
    await act(async () => textarea.dispatchEvent(new ui.window.Event('input', { bubbles: true })));
    assert.equal(button('Resolve dispute').disabled, true);
    assert.match(document.querySelector('[aria-label="Fixture dispute"]').textContent, /text is too long/);
    Object.getOwnPropertyDescriptor(Object.getPrototypeOf(textarea), 'value').set.call(textarea, 'Confirmed, result stands.');
    await act(async () => textarea.dispatchEvent(new ui.window.Event('input', { bubbles: true })));
    assert.equal(button('Resolve dispute').disabled, false);
    current = { ...current, capturedAt: '2026-11-15T10:00:01Z', tournaments: [{ ...cup, matches: [{ ...match, dispute: { reason: 'Updated evidence', revision: 4 } }] }] };
    await act(async () => ui.sockets[0].snapshot(current));
    assert.equal(textarea.value, 'Confirmed, result stands.');
    assert.equal(button('Resolve dispute').disabled, true);
    await act(async () => button('Discard draft and review the latest version').click());
    assert.equal(textarea.value, '');
    Object.getOwnPropertyDescriptor(Object.getPrototypeOf(textarea), 'value').set.call(textarea, 'Reviewed updated evidence.');
    await act(async () => textarea.dispatchEvent(new ui.window.Event('input', { bubbles: true })));
    await act(async () => button('Resolve dispute').click());
    assert.deepEqual(submitted, { revision: 4, resolution: 'Reviewed updated evidence.' });
  } finally { await ui.finish(); }
});

test('tournament list filters archives, opens one detail and preserves filters on return', async () => {
  const ui = await page('/manage#tournaments', async () => { throw new Error('Tournament navigation must use snapshots'); });
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token');
    await ui.render();
    const cup = { id: 'active', name: 'Current cup', status: 'active', bestOf: 1, playersPerSide: 1, entries: [], matches: [], mapPool: [] };
    const cups = [cup, { ...cup, id: 'past', name: 'Archived cup', status: 'complete' }, { ...cup, id: 'cancelled', name: 'Cancelled cup', status: 'cancelled' }];
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot({ capturedAt: '2026-11-15T10:00:00Z', hosts: [], machines: [], matches: [], players: [], maps: [], teams: [], tournaments: cups }); });
    assert.equal(document.querySelectorAll('.tournament-list-item').length, 1);
    assert.equal(document.querySelectorAll('.tournament-card').length, 0);
    const status = document.querySelector('.tournament-search select');
    await act(async () => { status.value = 'complete'; status.dispatchEvent(new ui.window.Event('change', { bubbles: true })); });
    assert.equal(button('View Archived cup').getAttribute('aria-label'), 'View Archived cup');
    await act(async () => button('View Archived cup').click());
    assert.equal(ui.window.location.hash, '#tournaments?tournament=past');
    assert.equal(document.querySelectorAll('.tournament-card').length, 1);
    assert.equal(document.querySelector('.tournament-card h2').textContent, 'Archived cup');
    assert.equal(document.activeElement.id, 'tournament-past');
    await act(async () => button('Back to tournaments').click());
    assert.equal(ui.window.location.hash, '#tournaments');
    assert.equal(document.querySelector('.tournament-search select').value, 'complete');
    assert.equal(document.activeElement.id, 'view-tournament-past');
    assert.equal(document.querySelectorAll('.tournament-list-item').length, 1);
  } finally { await ui.finish(); }
});

test('public tournament deep links open a single event and handle an unknown event', async () => {
  const ui = await page('/play#tournaments?tournament=cup', async (url) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' });
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    await ui.render();
    const cup = { id: 'cup', name: 'Friends cup', status: 'open', bestOf: 1, playersPerSide: 1, entries: [], matches: [], mapPool: [] };
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot({ capturedAt: '2026-11-15T10:00:00Z', servers: [], matches: [], tournaments: [cup, { ...cup, id: 'other', name: 'Other cup' }] }); });
    assert.equal(document.querySelectorAll('.tournament-card').length, 1);
    assert.equal(document.querySelector('.tournament-card h2').textContent, 'Friends cup');
    assert.equal(document.activeElement.id, 'tournament-cup');
    await act(async () => { ui.window.location.hash = '#tournaments?tournament=missing'; ui.window.dispatchEvent(new ui.window.HashChangeEvent('hashchange')); });
    assert.match(document.body.textContent, /not available in the current snapshot/);
    assert.equal(document.querySelectorAll('.tournament-card').length, 0);
    await act(async () => button('Back to tournaments').click());
    assert.equal(document.querySelectorAll('.tournament-list-item').length, 2);
    assert.equal(button('Tournaments').getAttribute('aria-current'), 'page');
    const search = document.querySelector('.tournament-search input');
    await act(async () => {
      Object.getOwnPropertyDescriptor(ui.window.HTMLInputElement.prototype, 'value').set.call(search, 'Other');
      search.dispatchEvent(new ui.window.Event('input', { bubbles: true }));
    });
    assert.equal(document.querySelectorAll('.tournament-list-item').length, 1);
    assert.equal(button('View Other cup').getAttribute('aria-label'), 'View Other cup');
  } finally { await ui.finish(); }
});

test('fleet navigation keeps filters, opens direct server links and never invents a current map', async () => {
  const ui = await page('/manage#servers', async () => { throw new Error('Navigation must use snapshots'); });
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token');
    await ui.render();
    const host = { id: 'one', name: 'Friends', address: '10.0.0.2', port: 27015, map: 'de_mirage', activeMap: '', desiredState: 'running', observedState: 'running', status: 'running', online: true, canObserveMap: true, driver: 'managed', machineId: 'machine', revision: 1, players: 0, maxPlayers: 10, whitelist: [], actions: {} };
    const second = { ...host, id: 'two', name: 'Practice', port: 27025, desiredState: 'stopped', observedState: 'stopped', status: 'stopped', gameBuildStatus: 'outdated' };
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot({ capturedAt: '2026-11-15T10:00:00Z', hosts: [host, second], machines: [{ id: 'machine', name: 'Byttorrent', online: true }], matches: [], players: [], maps: ['de_mirage'], presets: [] }); });
    assert.equal(document.querySelectorAll('.server-fleet-row').length, 2);
    assert.equal(document.querySelectorAll('.server-card').length, 0);
    assert.match(button('Manage Friends').textContent, /Current map unavailable/);
    assert.doesNotMatch(button('Manage Friends').textContent, /Mirage/);
    assert.match(button('Manage Friends').textContent, /0\/10 players/);
    const status = document.querySelector('.fleet-filters select');
    await act(async () => { status.value = 'attention'; status.dispatchEvent(new ui.window.Event('change', { bubbles: true })); });
    assert.equal(document.querySelectorAll('.server-fleet-row').length, 1);
    await act(async () => ui.sockets[0].snapshot({ capturedAt: '2026-11-15T10:00:01Z', hosts: [{ ...host, restartPhase: 'failed' }, second], machines: [{ id: 'machine', name: 'Byttorrent', online: true }], matches: [], players: [], maps: ['de_mirage'], presets: [] }));
    assert.equal(document.querySelectorAll('.server-fleet-row').length, 2);
    assert.match(button('Manage Friends').textContent, /Restart failed/);
    await act(async () => ui.sockets[0].snapshot({ capturedAt: '2026-11-15T10:00:02Z', hosts: [host, second], machines: [{ id: 'machine', name: 'Byttorrent', online: true }], matches: [], players: [], maps: ['de_mirage'], presets: [] }));
    assert.equal(document.querySelectorAll('.server-fleet-row').length, 1);
    await act(async () => button('Manage Practice').click());
    assert.equal(ui.window.location.hash, '#servers?server=two');
    assert.equal(document.querySelector('h1').textContent, 'Practice');
    assert.equal(document.querySelectorAll('.server-detail-grid .server-card').length, 1);
    assert.equal(document.activeElement.id, 'server-detail-two');
    await act(async () => button('Back to servers').click());
    assert.equal(document.querySelector('.fleet-filters select').value, 'attention');
    assert.equal(document.activeElement.id, 'view-server-two');
    await act(async () => { ui.window.location.hash = '#servers?server=one'; ui.window.dispatchEvent(new ui.window.HashChangeEvent('hashchange')); });
    assert.equal(document.querySelector('h1').textContent, 'Friends');
    assert.equal(button('Servers').getAttribute('aria-current'), 'page');
    await act(async () => { ui.window.location.hash = '#servers?server=missing'; ui.window.dispatchEvent(new ui.window.HashChangeEvent('hashchange')); });
    assert.match(document.body.textContent, /This server is not available/);
    assert.equal(document.querySelector('.server-detail-grid'), null);
    await act(async () => button('Back to servers').click());
    assert.equal(document.activeElement.id, 'find-server');
  } finally { await ui.finish(); }
});

test('server sections keep roster revision guards and show canonical logs on reconnect', async () => {
  const ui = await page('/manage#servers?server=server', async () => { throw new Error('No action should be submitted'); });
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token');
    await ui.render();
    const host = { id: 'server', name: 'Friends', address: '10.0.0.2', port: 27015, map: 'de_mirage', activeMap: '', desiredState: 'stopped', observedState: 'stopped', status: 'stopped', online: true, driver: 'managed', machineId: 'machine', revision: 1, whitelist: [], logTail: 'Initial server log', preset: null, actions: { start: true, changeWhitelist: true, changeMap: true, remove: true } };
    const snapshot = (server, time) => ({ capturedAt: time, hosts: [server], machines: [], matches: [], players: [], maps: ['de_mirage'], presets: [] });
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot(snapshot(host, '2026-11-15T10:00:00Z')); });
    assert.equal(button('Setup').getAttribute('aria-pressed'), 'true');
    assert.ok(document.querySelector('select[aria-label="Startup map for Friends"]'));
    await act(async () => ui.sockets[0].snapshot(snapshot({ ...host, map: 'workshop/123/aim_test', availableMaps: ['de_mirage', 'workshop/123/aim_test'] }, '2026-11-15T10:00:00Z')));
    let startup = document.querySelector('select[aria-label="Startup map for Friends"]');
    assert.equal(startup.value, 'workshop/123/aim_test');
    assert.equal(startup.selectedOptions[0].disabled, false);
    await act(async () => ui.sockets[0].snapshot(snapshot({ ...host, map: 'workshop/123/aim_test', availableMaps: ['de_mirage'], actions: { ...host.actions, start: false } }, '2026-11-15T10:00:00Z')));
    startup = document.querySelector('select[aria-label="Startup map for Friends"]');
    assert.equal(startup.value, 'workshop/123/aim_test');
    assert.equal(startup.selectedOptions[0].disabled, true);
    assert.match(startup.selectedOptions[0].textContent, /unavailable/);
    assert.equal(document.querySelector('.join-access'), null);
    await act(async () => button('Player access').click());
    assert.equal(document.querySelector('select[aria-label="Startup map for Friends"]'), null);
    assert.ok(button('Show join password'));
    await act(async () => button('Edit roster').click());
    assert.equal(button('Save roster').disabled, false);
    await act(async () => ui.sockets[0].snapshot(snapshot({ ...host, revision: 2 }, '2026-11-15T10:00:01Z')));
    assert.equal(button('Save roster').disabled, true);
    assert.equal(button('Player access').getAttribute('aria-pressed'), 'true');
    assert.match(document.body.textContent, /Server changed since editing began/);
    await act(async () => button('Logs').click());
    assert.match(document.querySelector('.server-logs pre').textContent, /Initial server log/);
    await act(async () => ui.sockets[0].snapshot(snapshot({ ...host, revision: 2, logTail: 'Updated server log' }, '2026-11-15T10:00:02Z')));
    assert.equal(document.querySelector('.server-logs pre').textContent, 'Updated server log');
    await act(async () => ui.sockets[0].onclose());
    assert.match(document.querySelector('.server-logs').textContent, /Last received logs/);
    assert.equal(button('Start server').disabled, true);
    await act(async () => button('Updates').click());
    assert.equal(button('Remove server').disabled, true);
    await act(async () => ui.sockets[0].snapshot({ ...snapshot(host, '2026-11-15T10:00:03Z'), hosts: [] }));
    assert.match(document.body.textContent, /This server is not available/);
    assert.equal(document.querySelector('.server-detail-grid'), null);
  } finally { await ui.finish(); }
});

test('team invite stays out of the Steam request and prefills the join form after sign in', async () => {
  const code = 'abcdefghijklmnopqrst123456';
  const signedOut = await page(`/play#teams?invite=${code}`, async () => response({ steamId: null, steamEnabled: true, gameName: '' }));
  try {
    await signedOut.render();
    const link = document.querySelector('.steam-sign-in');
    assert.equal(new URL(link.href).searchParams.get('returnTo'), '#teams');
    assert.equal(link.href.includes(code), false);
    link.addEventListener('click', (event) => event.preventDefault());
    await act(async () => link.click());
    assert.equal(sessionStorage.getItem('clutchbase-pending-invite'), code);
  } finally { await signedOut.finish(); }
  let joined = false;
  const signedIn = await page('/play#teams', async (url) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' });
    if (url === '/api/player/teams/join') { joined = true; return new Response(null, { status: 204 }); }
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    sessionStorage.setItem('clutchbase-pending-invite', code);
    await signedIn.render();
    await act(async () => { signedIn.sockets[0].open(); signedIn.sockets[0].snapshot({ capturedAt: '2026-11-15T10:00:00Z', servers: [], matches: [], tournaments: [], team: null }); });
    assert.equal(document.querySelector('input[name="code"]').value, code);
    assert.equal(joined, false);
    await act(async () => button('Join team').click());
    assert.equal(joined, true);
    assert.equal(sessionStorage.getItem('clutchbase-pending-invite'), null);
  } finally { await signedIn.finish(); }
});

for (const player of [true, false]) test(`${player ? 'public' : 'admin'} match search follows snapshots and retains direct links`, async () => {
  const ui = await page(player ? '/play#matches' : '/manage#matches', async (url) => {
    if (player && url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' });
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    if (!player) sessionStorage.setItem('clutchbase-token', 'test-admin-token');
    await ui.render();
    const room = { id: 'one', hostId: 'server', title: 'Friday match', hostName: 'Friends', map: 'de_mirage', status: 'open', loadStatus: '', players: 0, playersPerSide: 1, ctPlayers: 0, tPlayers: 0, ready: 0, participants: [], joined: false, actions: {}, resultCT: null, resultT: null, reportedCT: null, reportedT: null };
    const other = { ...room, id: 'two', title: 'Saturday', hostName: 'Practice', map: 'de_nuke' };
    const snapshot = (matches, capturedAt) => ({ capturedAt, servers: [], hosts: [], machines: [], players: [], maps: [], tournaments: [], teams: [], matches });
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot(snapshot([room, other], '2026-11-15T10:00:00Z')); });
    const input = document.querySelector('input[aria-label="Search match rooms"]');
    await act(async () => { Object.getOwnPropertyDescriptor(ui.window.HTMLInputElement.prototype, 'value').set.call(input, 'nuke'); input.dispatchEvent(new ui.window.Event('input', { bubbles: true })); });
    assert.equal(document.querySelector('#match-room-one'), null);
    assert.ok(document.querySelector('#match-room-two'));
    await act(async () => ui.sockets[0].snapshot(snapshot([room, other], '2026-11-15T10:00:01Z')));
    assert.equal(input.value, 'nuke');
    await act(async () => { ui.window.location.hash = '#matches?room=one'; ui.window.dispatchEvent(new ui.window.HashChangeEvent('hashchange')); });
    assert.ok(document.querySelector('#match-room-one'));
    await act(async () => button('Clear search').click());
    assert.equal(input.value, '');
  } finally { await ui.finish(); }
});

test('fixture scheduling waits for committed snapshot and rejects stale edits', async () => {
  const { TournamentSchedule } = await vite.ssrLoadModule('/src/TournamentSchedule.tsx');
  const ui = await page('/manage', async () => response({}));
  let requested;
  const fixture = { id: 'fixture', scheduleRevision: 1, scheduledAt: null, canSchedule: true };
  const draw = async (match, connected = true) => act(async () => ui.root.render(React.createElement(TournamentSchedule, { match, admin: true, connected, onSave: async (revision, scheduledAt) => { requested = { revision, scheduledAt }; return true; } })));
  try {
    await draw(fixture);
    await act(async () => button('Schedule fixture').click());
    const input = document.querySelector('input[type="datetime-local"]');
    await act(async () => { Object.getOwnPropertyDescriptor(ui.window.HTMLInputElement.prototype, 'value').set.call(input, '2026-12-01T18:00'); input.dispatchEvent(new ui.window.Event('input', { bubbles: true })); });
    await act(async () => button('Save time').click());
    assert.equal(requested.revision, 1);
    assert.ok(requested.scheduledAt.endsWith('Z'));
    assert.match(document.body.textContent, /Waiting for the updated schedule/);
    assert.match(document.body.textContent, /Not scheduled/);
    await draw({ ...fixture, scheduleRevision: 2, scheduledAt: requested.scheduledAt });
    assert.ok(document.querySelector('time'));
    await act(async () => button('Schedule fixture').click());
    await draw({ ...fixture, scheduleRevision: 3, scheduledAt: requested.scheduledAt });
    assert.equal(button('Save time').disabled, true);
    assert.match(document.body.textContent, /fixture changed/);
    await act(async () => button('Cancel').click());
    await draw({ ...fixture, scheduleRevision: 3, scheduledAt: requested.scheduledAt }, false);
    assert.equal(button('Schedule fixture').disabled, true);
  } finally { await ui.finish(); }
});

for (const player of [true, false]) test(`${player ? 'public' : 'admin'} schedule orders canonical fixtures and links to tournaments`, async () => {
  const ui = await page(player ? '/play#schedule' : '/manage#schedule', async (url) => {
    if (player && url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' });
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    if (!player) sessionStorage.setItem('clutchbase-token', 'test-admin-token');
    await ui.render();
    const match = { id: 'one', round: 1, number: 1, status: 'pending', teamA: { teamId: 'a', teamName: 'Alpha' }, teamB: { teamId: 'b', teamName: 'Bravo' }, winnerTeamId: '', vetoBans: [], vetoMaps: [], mapSequence: [], vetoStep: 0, scheduledAt: '2026-12-01T18:00:00Z' };
    const cup = { id: 'cup', name: 'Friends cup', format: 'single_elimination', status: 'active', playersPerSide: 1, bestOf: 1, entries: [], mapPool: [], matches: [match, { ...match, id: 'two', number: 2, scheduledAt: '2026-11-20T18:00:00Z' }, { ...match, id: 'done', status: 'complete' }] };
    const snapshot = (tournaments, capturedAt) => ({ capturedAt, tournaments, servers: [], hosts: [], machines: [], players: [], maps: [], teams: [], matches: [] });
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot(snapshot([cup], '2026-11-15T10:00:00Z')); });
    assert.equal(button('Schedule').getAttribute('aria-current'), 'page');
    if (player) assert.ok(document.querySelector('aside').contains(button('Schedule')));
    assert.equal(document.querySelectorAll('.fixture-list-row').length, 2);
    assert.equal(document.querySelector('.fixture-list-row time').getAttribute('datetime'), '2026-11-20T18:00:00Z');
    await act(async () => ui.sockets[0].snapshot(snapshot([{ ...cup, matches: [{ ...match, scheduledAt: null }] }], '2026-11-15T10:00:01Z')));
    assert.match(document.querySelector('.fixture-list').textContent, /No scheduled fixtures/);
    await act(async () => ui.sockets[0].snapshot(snapshot([cup], '2026-11-15T10:00:02Z')));
    await act(async () => ui.sockets[0].onclose());
    assert.match(document.querySelector('.fixture-list').textContent, /last received schedule/);
    await act(async () => document.querySelector('.fixture-list-row').click());
    assert.equal(ui.window.location.hash, '#tournaments?tournament=cup');
  } finally { await ui.finish(); }
});

test('restart confirmation follows canonical status and cancellation', async () => {
  let restartBody, cancelBody;
  let current;
  const ui = await page('/manage#servers?server=server', async (url, options) => {
    if (url === '/api/hosts/server/restart') { restartBody = JSON.parse(options.body); return response({ status: 'stopping' }, 202); }
    if (url === '/api/hosts/server') { cancelBody = JSON.parse(options.body); return response({}); }
    if (url === '/api/state') return response(current);
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token');
    await ui.render();
    const host = { id:'server', name:'Friends', address:'127.0.0.1', port:27015, map:'de_mirage', activeMap:'de_mirage', desiredState:'running', observedState:'running', status:'running', online:true, driver:'managed', machineId:'machine', revision:4, whitelist:null, players:null, maxPlayers:null, actions:{ restart:true, stop:true, start:false } };
    const snapshot = (value, time) => ({ capturedAt:time, hosts:[value], machines:[], matches:[], players:[], maps:['de_mirage'], presets:[] });
    current = snapshot(host, '2026-11-15T10:00:00Z');
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot(current); });
    await act(async () => button('Restart server').click());
    assert.equal(restartBody, undefined);
    await act(async () => ui.sockets[0].snapshot(snapshot({ ...host, revision:5 }, '2026-11-15T10:00:01Z')));
    assert.equal(button('Confirm restart').disabled, true);
    await act(async () => button('Keep running').click());
    await act(async () => button('Restart server').click());
    await act(async () => button('Confirm restart').click());
    assert.deepEqual(restartBody, { revision:5 });
    assert.doesNotMatch(document.body.textContent, /Restart requested/);
    current = snapshot({ ...host, revision:6, desiredState:'stopped', status:'stopping', restartPhase:'stopping', actions:{ restart:false, stop:false, start:false, cancelRestart:true } }, '2026-11-15T10:00:02Z');
    await act(async () => ui.sockets[0].snapshot(current));
    assert.match(document.body.textContent, /Restart requested/);
    await act(async () => button('Cancel restart').click());
    assert.deepEqual(cancelBody, { desiredState:'stopped', revision:6 });
    await act(async () => ui.sockets[0].snapshot(snapshot({ ...host, revision:7, desiredState:'stopped', status:'stopped', restartPhase:'cancelling', restartSupported:false, actions:{ restart:false, stop:false, start:false, cancelRestart:false } }, '2026-11-15T10:00:03Z')));
    assert.match(document.body.textContent, /Reconnect a restart-capable machine agent/);
    await act(async () => ui.sockets[0].onclose());
    assert.equal([...document.querySelectorAll('button')].some((item) => item.textContent === 'Cancel restart'), false);
  } finally { await ui.finish(); }
});

test('leaderboard uses server ranks and stays in the public sidebar', async () => {
  const ui = await page('/play#leaderboard', async (url) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' });
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    await ui.render();
    const leaderboard = ui.sockets.find((socket) => socket.url.searchParams.get('view') === 'player-leaderboard');
    assert.ok(leaderboard);
    assert.ok(document.querySelector('aside').contains(button('Leaderboard')));
    await act(async () => { leaderboard.open(); leaderboard.snapshot({ capturedAt: '2026-11-15T10:00:00Z', truncated: false, players: [{ rank: 1, name: 'Alpha', wins: 2, losses: 0, draws: 0, rooms: 3 }, { rank: 1, name: 'Bravo', wins: 2, losses: 1, draws: 0, rooms: 3 }] }); });
    assert.match(document.querySelector('.statistics').textContent, /1Alpha2–0–03/);
    assert.match(document.querySelector('.statistics').textContent, /1Bravo2–1–03/);
    await act(async () => leaderboard.onclose());
    assert.match(document.querySelector('.statistics').textContent, /last received snapshot/);
    await act(async () => leaderboard.snapshot({ capturedAt: '2026-11-15T10:01:00Z', truncated: false, players: [] }));
    assert.match(document.querySelector('.statistics').textContent, /No scored matches yet/);
    await act(async () => leaderboard.snapshot({ code: 401, error: 'Sign in again' }));
    assert.match(document.querySelector('.statistics').textContent, /Session expired/);
  } finally { await ui.finish(); }
});

test('management opens the leaderboard using its administrator stream', async () => {
  const ui = await page('/manage', async () => response({}));
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token');
    await ui.render();
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot({ capturedAt: '2026-11-17T10:00:00Z', hosts: [], machines: [], matches: [], players: [], maps: [], tournaments: [], teams: [] }); });
    await act(async () => button('Leaderboard').click());
    const leaderboard = ui.sockets.at(-1);
    assert.equal(leaderboard.url.searchParams.get('view'), 'leaderboard');
    await act(async () => { leaderboard.open(); leaderboard.snapshot({ capturedAt: '2026-11-17T10:00:00Z', truncated: true, players: [{ rank: 1, name: 'Ace', wins: 3, losses: 1, draws: 0, rooms: 5 }] }); });
    assert.match(document.querySelector('.statistics').textContent, /Ace3–1–05/);
    assert.match(document.querySelector('.statistics').textContent, /first 200 players/);
  } finally { await ui.finish(); }
});

test('participants can report a match and follow the private review status', async () => {
  const calls = [];
  const ui = await page('/play#matches', async (url, options) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' });
    if (url === '/api/player/matches/room/report') { calls.push(JSON.parse(options.body)); return new Response(null, { status: 201 }); }
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    await ui.render();
    const main = ui.sockets[0];
    await act(async () => { main.open(); main.snapshot({ capturedAt: '2026-11-17T10:00:00Z', servers: [], tournaments: [], matches: [{ id: 'room', hostId: 'host', title: 'Friday', hostName: 'Friends', map: 'de_mirage', status: 'open', players: 1, playersPerSide: 1, ctPlayers: 1, tPlayers: 0, ready: 0, participants: [], joined: true, myReady: false, mySide: 'ct', actions: { report: true }, resultCT: null, resultT: null, reportedCT: null, reportedT: null }] }); });
    assert.ok(document.querySelector('aside').contains(button('My reports')));
    const form = document.querySelector('.match-report-form');
    form.querySelector('textarea').value = 'There was a problem';
    await act(async () => form.dispatchEvent(new ui.window.Event('submit', { bubbles: true, cancelable: true })));
    assert.deepEqual(calls, [{ description: 'There was a problem' }]);
    await act(async () => button('My reports').click());
    const reports = ui.sockets.at(-1);
    assert.equal(reports.url.searchParams.get('view'), 'player-reports');
    await act(async () => { reports.open(); reports.snapshot({ capturedAt: '2026-11-17T10:00:01Z', truncated: false, reports: [{ id: 'r', matchId: 'room', matchTitle: 'Friday', hostName: 'Friends', description: 'There was a problem', status: 'open', revision: 1, createdAt: '2026-11-17T10:00:00Z' }] }); });
    assert.match(document.querySelector('main').textContent, /There was a problem/);
    assert.match(document.querySelector('main').textContent, /Open/);
    await act(async () => reports.snapshot({ capturedAt: '2026-11-17T10:00:02Z', truncated: false, reports: [{ id: 'r', matchId: 'room', matchTitle: 'Friday', hostName: 'Friends', description: 'There was a problem', status: 'resolved', revision: 2, createdAt: '2026-11-17T10:00:00Z' }] }));
    assert.match(document.querySelector('main').textContent, /Resolved/);
  } finally { await ui.finish(); }
});

test('report review waits for the canonical administrator snapshot', async () => {
  const calls = [];
  const ui = await page('/manage#reports', async (url, options) => {
    if (url === '/api/reports/r') { calls.push(JSON.parse(options.body)); return new Response(null, { status: 204 }); }
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token');
    await ui.render();
    const reports = ui.sockets.find((socket) => socket.url.searchParams.get('view') === 'reports');
    assert.ok(reports);
    await act(async () => { reports.open(); reports.snapshot({ capturedAt: '2026-11-17T10:00:00Z', truncated: false, reports: [{ id: 'r', matchId: 'room', matchTitle: 'Friday', hostName: 'Friends', reporterName: 'Friend', reporterSteamId: '765', description: 'Problem', status: 'open', revision: 1, createdAt: '2026-11-17T10:00:00Z' }] }); });
    await act(async () => button('Mark resolved').click());
    assert.deepEqual(calls, [{ status: 'resolved', revision: 1 }]);
    assert.ok(button('Mark resolved'));
    await act(async () => reports.snapshot({ capturedAt: '2026-11-17T10:00:01Z', truncated: false, reports: [{ id: 'r', matchId: 'room', matchTitle: 'Friday', hostName: 'Friends', reporterName: 'Friend', reporterSteamId: '765', description: 'Problem', status: 'resolved', revision: 2, createdAt: '2026-11-17T10:00:00Z' }] }));
    assert.ok(button('Reopen'));
    await act(async () => reports.onclose());
    assert.equal(button('Reopen').disabled, true);
  } finally { await ui.finish(); }
});

test('standalone map votes follow player snapshots and disable on disconnect', async () => {
  const calls = [];
  const ui = await page('/play#matches', async (url, options) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' });
    if (url === '/api/player/matches/room/map-vote') { calls.push({ method: options.method, body: options.body && JSON.parse(options.body) }); return new Response(null, { status: 204 }); }
    throw new Error(`Unexpected fetch ${url}`);
  });
  const room = { id: 'room', hostId: 'host', title: 'Friday', hostName: 'Friends', map: 'de_mirage', mapVotes: [], myMapVote: '', status: 'open', players: 1, playersPerSide: 1, ctPlayers: 1, tPlayers: 0, ready: 0, participants: [], joined: true, myReady: false, mySide: 'ct', actions: { voteMap: true }, resultCT: null, resultT: null, reportedCT: null, reportedT: null };
  const state = { capturedAt: '2026-11-17T10:00:00Z', maps: ['de_mirage', 'de_nuke'], servers: [], tournaments: [], matches: [room] };
  try {
    await ui.render(); const main = ui.sockets[0];
    await act(async () => { main.open(); main.snapshot(state); });
    await act(async () => button('nuke').click());
    assert.deepEqual(calls, [{ method: 'PUT', body: { map: 'de_nuke' } }]);
    assert.equal(button('nuke').getAttribute('aria-pressed'), 'false');
    await act(async () => main.snapshot({ ...state, capturedAt: '2026-11-17T10:00:01Z', matches: [{ ...room, mapVotes: [{ map: 'de_nuke', votes: 1 }], myMapVote: 'de_nuke' }] }));
    assert.equal(button('nuke').getAttribute('aria-pressed'), 'true');
    assert.match(document.querySelector('.match-map-vote').textContent, /1 vote/);
    await act(async () => main.onclose());
    assert.equal(button('Clear my vote').disabled, true);
  } finally { await ui.finish(); }
});

test('management applies a voted map through revision-checked room settings', async () => {
  const calls = [];
  const ui = await page('/manage#matches', async (url, options) => {
    if (url === '/api/matches/room/settings') { calls.push(JSON.parse(options.body)); return response({ readinessReset: true }); }
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token'); await ui.render();
    await act(async () => { ui.sockets[0].open(); ui.sockets[0].snapshot({ capturedAt: '2026-11-17T10:00:00Z', hosts: [], machines: [], players: [], teams: [], maps: ['de_nuke'], tournaments: [], matches: [{ id: 'room', hostId: 'host', title: 'Friday', hostName: 'Friends', map: 'de_mirage', mapVotes: [{ map: 'de_nuke', votes: 2 }], overtime: false, status: 'open', players: 2, playersPerSide: 1, ctPlayers: 1, tPlayers: 1, ready: 2, participants: [], canEdit: true, settingsRevision: 4, canLoad: false }] }); });
    await act(async () => button('Use nuke').click());
    assert.deepEqual(calls, [{ title: 'Friday', map: 'de_nuke', playersPerSide: 1, overtime: false, manualReady: false, settingsRevision: 4 }]);
    assert.match(document.querySelector('.match-card').textContent, /Mirage/);
    assert.match(document.body.textContent, /Players must mark ready again/);
  } finally { await ui.finish(); }
});

test('players see server update guidance without being blocked from trying to connect', async () => {
  const ui = await page('/play', async (url) => {
    if (url === '/api/player/session') return response({ steamId: '76561198154367261', steamEnabled: true, gameName: 'Friend' });
    throw new Error(`Unexpected fetch ${url}`);
  });
  const server = { id: 'server', name: 'Friends', address: '127.0.0.1', port: 27015, map: 'de_mirage', status: 'running', players: 0, maxPlayers: 10, canJoin: true, gameBuildStatus: 'outdated' };
  const room = { id: 'room', hostId: 'server', title: 'Friday', hostName: 'Friends', map: 'de_mirage', status: 'open', players: 2, playersPerSide: 1, ctPlayers: 1, tPlayers: 1, ready: 2, participants: [], joined: true, mySide: 'ct', myReady: true, actions: {}, loadStatus: 'complete', resultCT: null, resultT: null, reportedCT: null, reportedT: null };
  const state = { capturedAt: '2026-11-17T10:00:00Z', matches: [room], tournaments: [], servers: [server] };
  try {
    await ui.render(); const main = ui.sockets[0];
    await act(async () => { main.open(); main.snapshot(state); });
    assert.match(document.body.textContent, /CS2 server update available/);
    assert.equal(button('Copy join command').disabled, false);
    await act(async () => button('Matches').click());
    assert.match(document.querySelector('.match-card').textContent, /Connections may fail/);
    await act(async () => main.onclose());
    assert.match(document.querySelector('.match-card').textContent, /Last reported: CS2 server update available/);
    await act(async () => main.snapshot({ ...state, capturedAt: '2026-11-17T10:01:00Z', servers: [{ ...server, gameBuildStatus: 'unknown' }] }));
    assert.doesNotMatch(document.querySelector('.match-card').textContent, /CS2 server update available/);
  } finally { await ui.finish(); }
});

test('stop and update requires confirmation and follows the server snapshot', async () => {
  const calls = [];
  const host = { id: 'server', name: 'Friends', address: '127.0.0.1', port: 27015, map: 'de_mirage', activeMap: 'de_mirage', desiredState: 'running', observedState: 'running', status: 'running', online: true, lastSeen: '2026-11-17T10:00:00Z', driver: 'managed', machineId: 'machine', revision: 4, whitelist: null, gameUpdateStatus: '', gameUpdateError: '', actions: { start: false, stop: true, changeMap: false, openMatch: false, updateGame: false, stopAndUpdate: true } };
  const state = { capturedAt: '2026-11-17T10:00:00Z', hosts: [host], machines: [], matches: [], players: [], maps: [], presets: [] };
  const ui = await page('/manage#servers?server=server', async (url, options = {}) => {
    if (url === '/api/hosts/server/game-update') { calls.push(JSON.parse(options.body)); return response({ status: 'pending' }, 202); }
    if (url === '/api/state') return response(state);
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token'); await ui.render(); const socket = ui.sockets[0];
    await act(async () => { socket.open(); socket.snapshot(state); });
    await act(async () => button('Stop & update CS2').click());
    assert.match(document.body.textContent, /remains stopped afterward/);
    await act(async () => socket.snapshot({ ...state, capturedAt: '2026-11-17T10:00:01Z', hosts: [{ ...host, revision: 5 }] }));
    assert.equal(button('Confirm stop & update').disabled, true);
    await act(async () => button('Keep running').click());
    await act(async () => button('Stop & update CS2').click());
    await act(async () => button('Confirm stop & update').click());
    assert.deepEqual(calls, [{ revision: 5, stopFirst: true }]);
    assert.ok(button('Stop server'));
    await act(async () => socket.snapshot({ ...state, capturedAt: '2026-11-17T10:00:02Z', hosts: [{ ...host, revision: 6, desiredState: 'stopped', observedState: 'running', status: 'stopping', gameUpdateStatus: 'pending', actions: { ...host.actions, stop: false, stopAndUpdate: false } }] }));
    assert.match(document.body.textContent, /update will start after the agent reports the server stopped/);
    assert.equal(button('Stopping…').disabled, true);
    await act(async () => socket.snapshot({ ...state, capturedAt: '2026-11-17T10:00:03Z', hosts: [{ ...host, revision: 6, desiredState: 'stopped', observedState: 'stopped', status: 'stopped', gameUpdateStatus: 'pending', actions: { ...host.actions, stop: false, stopAndUpdate: false } }] }));
    assert.match(document.body.textContent, /CS2 update requested/);
    await act(async () => socket.snapshot({ ...state, capturedAt: '2026-11-17T10:00:04Z', hosts: [{ ...host, revision: 6, desiredState: 'stopped', observedState: 'stopped', status: 'stopped', gameUpdateStatus: 'pending', gameUpdateSupported: false, actions: { ...host.actions, stop: false, stopAndUpdate: false } }] }));
    assert.match(document.body.textContent, /agent that supports CS2 updates/);
  } finally { await ui.finish(); }
});

test('queued update cancellation follows canonical state even when the agent is offline', async () => {
  const calls = [];
  const host = { id: 'server', name: 'Friends', address: '127.0.0.1', port: 27015, map: 'de_mirage', activeMap: '', desiredState: 'stopped', observedState: 'running', status: 'offline', online: false, driver: 'managed', machineId: 'machine', revision: 5, whitelist: null, gameUpdateStatus: 'pending', gameUpdateError: '', actions: { start: false, stop: false, changeMap: false, openMatch: false, updateGame: false, cancelGameUpdate: true } };
  const state = { capturedAt: '2026-11-17T10:00:00Z', hosts: [host], machines: [], matches: [], players: [], maps: [], presets: [] };
  const ui = await page('/manage#servers?server=server', async (url, options = {}) => {
    if (url === '/api/hosts/server/game-update/cancel') { calls.push(JSON.parse(options.body)); return new Response(null, { status: 204 }); }
    if (url === '/api/state') return response(state);
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token'); await ui.render(); const socket = ui.sockets[0];
    await act(async () => { socket.open(); socket.snapshot(state); });
    assert.equal(button('Cancel queued update').disabled, false);
    await act(async () => button('Cancel queued update').click());
    assert.deepEqual(calls, [{ revision: 5 }]);
    assert.ok(button('Cancel queued update'));
    await act(async () => socket.snapshot({ ...state, capturedAt: '2026-11-17T10:00:01Z', hosts: [{ ...host, revision: 6, gameUpdateStatus: 'cancelled', actions: { ...host.actions, cancelGameUpdate: false } }] }));
    assert.match(document.body.textContent, /update cancelled before delivery.*still requested to stop/i);
    assert.equal([...document.querySelectorAll('button')].some(x => x.textContent.includes('Cancel queued update')), false);
  } finally { await ui.finish(); }
});

test('shuffle confirms the current roster and waits for committed lineups', async () => {
  const calls = [];
  const first = { steamId: '76561198154367261', gameName: 'Alpha', side: 'ct', ready: true, stats: null, reportedStats: null };
  const second = { steamId: '76561198154367262', gameName: 'Bravo', side: 't', ready: true, stats: null, reportedStats: null };
  const room = { id: 'room', hostId: 'host', title: 'Friday', hostName: 'Friends', map: 'de_mirage', status: 'open', players: 2, playersPerSide: 1, ctPlayers: 1, tPlayers: 1, ready: 2, participants: [first, second], canShuffle: true, canEdit: true, canRemovePlayer: true, settingsRevision: 4, canLoad: false };
  const state = { capturedAt: '2026-11-17T10:00:00Z', hosts: [], machines: [], players: [], teams: [], maps: [], tournaments: [], matches: [room] };
  const ui = await page('/manage#matches', async (url, options) => {
    if (url === '/api/matches/room/shuffle') { calls.push(JSON.parse(options.body)); return new Response(null, { status: 204 }); }
    throw new Error(`Unexpected fetch ${url}`);
  });
  try {
    sessionStorage.setItem('clutchbase-token', 'test-admin-token'); await ui.render(); const socket=ui.sockets[0];
    await act(async () => { socket.open(); socket.snapshot(state); });
    await act(async () => button('Shuffle sides').click());
    assert.match(document.querySelector('.team-confirmation').textContent, /ready up again/);
    await act(async () => socket.snapshot({ ...state, capturedAt: '2026-11-17T10:00:01Z', matches: [{ ...room, participants: [{ ...first, ready: false }, second], ready: 1 }] }));
    assert.equal(button('Confirm shuffle').disabled, true);
    await act(async () => button('Cancel').click());
    await act(async () => button('Shuffle sides').click());
    await act(async () => button('Confirm shuffle').click());
    assert.deepEqual(calls, [{ settingsRevision: 4, players: [{ steamId: first.steamId, side: 'ct', ready: false }, { steamId: second.steamId, side: 't', ready: true }] }]);
    assert.match(document.querySelector('.match-card').textContent, /Bravo · Ready/);
    await act(async () => socket.snapshot({ ...state, capturedAt: '2026-11-17T10:00:02Z', matches: [{ ...room, participants: [{ ...first, side: 't', ready: false }, { ...second, side: 'ct', ready: false }], ready: 0 }] }));
    assert.match(document.querySelector('.match-card').textContent, /Bravo · Not ready/);
    assert.ok(button('Shuffle sides'));
  } finally { await ui.finish(); }
});

test('rostered players open a match only when the live server allows it', async () => {
  const calls=[];
  const ui=await page('/play#matches',async(url,options)=>{
    if(url==='/api/player/session') return response({steamId:'76561198154367261',steamEnabled:true,gameName:'Friend'});
    if(url==='/api/player/matches'){calls.push(JSON.parse(options.body));return response({id:'room'},201);}
    throw new Error(`Unexpected fetch ${url}`);
  });
  const server={id:'server',name:'Friends',address:'127.0.0.1',port:27015,map:'de_mirage',status:'running',players:0,maxPlayers:10,canJoin:true,canOpenMatch:true};
  const state={capturedAt:'2026-11-17T10:00:00Z',servers:[server],matches:[],tournaments:[],maps:['de_mirage','de_nuke']};
  try{
    await ui.render();const socket=ui.sockets[0];await act(async()=>{socket.open();socket.snapshot(state)});
    assert.ok(document.querySelector('aside').contains(button('Matches')));
    const form=document.querySelector('.match-create-form');assert.ok(form);
    form.elements.hostId.value='server';form.elements.title.value='Friday';form.elements.map.value='de_nuke';form.elements.playersPerSide.value='2';form.elements.side.value='t';form.elements.manualReady.checked=true;
    await act(async()=>form.dispatchEvent(new ui.window.Event('submit',{bubbles:true,cancelable:true})));
    assert.deepEqual(calls,[{hostId:'server',title:'Friday',map:'de_nuke',playersPerSide:2,side:'t',overtime:false,manualReady:true}]);
    assert.match(document.body.textContent,/appear when the live view updates/);
    assert.match(document.body.textContent,/No matches available/);
    const room={id:'room',hostId:'server',title:'Friday',hostName:'Friends',map:'de_nuke',status:'open',players:1,playersPerSide:2,ctPlayers:0,tPlayers:1,ready:0,participants:[],joined:true,mySide:'t',myReady:false,actions:{},resultCT:null,resultT:null,reportedCT:null,reportedT:null};
    await act(async()=>socket.snapshot({...state,capturedAt:'2026-11-17T10:00:01Z',servers:[{...server,canOpenMatch:false}],matches:[room]}));
    assert.match(document.body.textContent,/Friday/);
    assert.equal(document.querySelector('.match-create-form'),null);
  } finally {await ui.finish()}
});

test('management changes player room permission using the server revision',async()=>{
  const calls=[];
  const host={id:'server',name:'Friends',address:'127.0.0.1',port:27015,map:'de_mirage',activeMap:'',desiredState:'running',observedState:'running',status:'running',online:true,driver:'managed',machineId:'machine',revision:3,whitelist:null,allowPlayerRooms:false,actions:{start:false,stop:true,changeMap:false,openMatch:false,changePlayerRooms:true}};
  const state={capturedAt:'2026-11-17T10:00:00Z',hosts:[host],machines:[],matches:[],players:[],maps:[],presets:[]};
  const ui=await page('/manage#servers?server=server',async(url,options={})=>{if(url==='/api/hosts/server'){calls.push(JSON.parse(options.body));return response({})}if(url==='/api/state')return response(state);throw new Error(`Unexpected fetch ${url}`)});
  try{
    sessionStorage.setItem('clutchbase-token','test-admin-token');await ui.render();const socket=ui.sockets[0];await act(async()=>{socket.open();socket.snapshot(state)});
    await act(async()=>button('Player access').click());await act(async()=>button('Allow players to open rooms').click());
    assert.deepEqual(calls,[{allowPlayerRooms:true,revision:3}]);assert.ok(button('Allow players to open rooms'));
    await act(async()=>socket.snapshot({...state,capturedAt:'2026-11-17T10:00:01Z',hosts:[{...host,revision:4,allowPlayerRooms:true}]}));
    assert.ok(button('Disable player room creation'));
    await act(async()=>socket.onclose());assert.equal(button('Disable player room creation').disabled,true);
  }finally{await ui.finish()}
});

test('player-created room controls follow the canonical load and close actions', async () => {
  const calls=[];
  const ui=await page('/play#matches',async(url,options)=>{
    if(url==='/api/player/session')return response({steamId:'76561198154367261',steamEnabled:true,gameName:'Friend'});
    if(url==='/api/player/matches/room/load'||url==='/api/player/matches/room/close'){calls.push({url,method:options.method});return response({status:'pending'},202)}
    throw new Error(`Unexpected fetch ${url}`);
  });
  const room={id:'room',hostId:'server',title:'Friday',hostName:'Friends',map:'de_mirage',status:'open',loadStatus:'',players:2,playersPerSide:1,ctPlayers:1,tPlayers:1,ready:2,participants:[],myRoom:true,joined:true,mySide:'ct',myReady:true,actions:{load:true,close:true},resultCT:null,resultT:null,reportedCT:null,reportedT:null};
  const state={capturedAt:'2026-11-17T10:00:00Z',servers:[],matches:[room],tournaments:[]};
  try{
    await ui.render();const socket=ui.sockets[0];await act(async()=>{socket.open();socket.snapshot({...state,matches:[{...room,loadBlockers:['server_offline','players_not_ready'],actions:{load:false,close:true}}]})});
    assert.match(document.body.textContent,/server must be online and running/i);
    assert.match(document.body.textContent,/Both sides need players/);
    assert.equal([...document.querySelectorAll('button')].some(x=>x.textContent.trim()==='Load match'),false);
    await act(async()=>socket.snapshot({...state,capturedAt:'2026-11-17T10:00:01Z',matches:[{...room,loadBlockers:[]}]}));
    assert.match(document.body.textContent,/Everyone is ready. You can load the match/);
    await act(async()=>button('Load match').click());
    assert.deepEqual(calls,[{url:'/api/player/matches/room/load',method:'POST'}]);
    assert.ok(button('Load match'),'request alone does not mark loading');
    await act(async()=>socket.snapshot({...state,capturedAt:'2026-11-17T10:00:02Z',matches:[{...room,loadStatus:'pending',actions:{load:false,close:false}}]}));
    assert.match(document.body.textContent,/Loading the MatchZy setup/);
    assert.equal([...document.querySelectorAll('button')].some(x=>x.textContent.trim()==='Load match'),false);
    await act(async()=>socket.snapshot({...state,capturedAt:'2026-11-17T10:00:03Z',matches:[room]}));
    await act(async()=>button('Close room').click());
    assert.match(document.body.textContent,/Close this unstarted room/);
    await act(async()=>socket.onclose());
    assert.equal(button('Confirm close room').disabled,true);
  }finally{await ui.finish()}
});

test('room creator edits follow server revision and wait for snapshots', async () => {
  const calls=[]; const ui=await page('/play#matches',async(url,options)=>{if(url==='/api/player/session')return response({steamId:'76561198154367261',steamEnabled:true,gameName:'Friend'});if(url==='/api/player/matches/room/settings'){calls.push(JSON.parse(options.body));return response({readinessReset:true})}throw new Error(`Unexpected fetch ${url}`)});
  const room={id:'room',hostId:'server',title:'Friday',hostName:'Friends',map:'de_mirage',overtime:false,status:'open',players:1,playersPerSide:2,ctPlayers:1,tPlayers:0,ready:1,participants:[],joined:true,mySide:'ct',myReady:true,settingsRevision:1,actions:{edit:true},resultCT:null,resultT:null,reportedCT:null,reportedT:null};
  const state={capturedAt:'2026-11-17T10:00:00Z',servers:[],matches:[room],tournaments:[],maps:['de_mirage','de_nuke']};
  try{await ui.render();const socket=ui.sockets[0];await act(async()=>{socket.open();socket.snapshot(state)});await act(async()=>button('Edit room').click());const form=document.querySelector('section[aria-label="Room settings for Friday"] form');form.elements.map.value='de_nuke';form.elements.manualReady.checked=true;await act(async()=>form.dispatchEvent(new ui.window.Event('submit',{bubbles:true,cancelable:true})));assert.equal(calls[0].settingsRevision,1);assert.equal(calls[0].map,'de_nuke');assert.equal(calls[0].manualReady,true);assert.match(document.body.textContent,/Everyone must ready up again/)}finally{await ui.finish()}
});

test('room creator applies a voted map using the current revision', async () => {
  const calls=[];const ui=await page('/play#matches',async(url,options)=>{if(url==='/api/player/session')return response({steamId:'76561198154367261',steamEnabled:true,gameName:'Friend'});if(url==='/api/player/matches/room/settings'){calls.push(JSON.parse(options.body));return response({readinessReset:true})}throw new Error(`Unexpected fetch ${url}`)});
  const room={id:'room',hostId:'server',title:'Friday',hostName:'Friends',map:'de_mirage',mapVotes:[{map:'de_nuke',votes:2}],overtime:false,manualReady:true,status:'open',players:1,playersPerSide:2,ctPlayers:1,tPlayers:0,ready:1,participants:[],joined:true,mySide:'ct',myReady:true,settingsRevision:4,actions:{edit:true},resultCT:null,resultT:null,reportedCT:null,reportedT:null};
  try{await ui.render();const socket=ui.sockets[0];await act(async()=>{socket.open();socket.snapshot({capturedAt:'2026-11-22T10:00:00Z',servers:[],matches:[room],tournaments:[],maps:['de_mirage','de_nuke']})});await act(async()=>button('Use nuke').click());assert.deepEqual(calls,[{title:'Friday',map:'de_nuke',playersPerSide:2,overtime:false,manualReady:true,settingsRevision:4}]);assert.match(document.querySelector('.match-card').textContent,/mirage/);assert.match(document.body.textContent,/Everyone must ready up again/)}finally{await ui.finish()}
});

test('room creator shuffle requires current roster confirmation and waits for the snapshot', async () => {
  const calls=[]; const ui=await page('/play#matches',async(url,options)=>{if(url==='/api/player/session')return response({steamId:'76561198154367261',steamEnabled:true,gameName:'Friend'});if(url==='/api/player/matches/room/shuffle'){calls.push(JSON.parse(options.body));return new Response(null,{status:204})}throw new Error(`Unexpected fetch ${url}`)});
  const room={id:'room',hostId:'server',title:'Friday',hostName:'Friends',map:'de_mirage',status:'open',players:2,playersPerSide:2,ctPlayers:2,tPlayers:0,ready:2,participants:[],joined:true,mySide:'ct',myReady:true,settingsRevision:1,rosterFingerprint:'first',actions:{shuffle:true},resultCT:null,resultT:null,reportedCT:null,reportedT:null};
  const state={capturedAt:'2026-11-17T10:00:00Z',servers:[],matches:[room],tournaments:[]};
  try {
    await ui.render(); const socket=ui.sockets[0]; await act(async()=>{socket.open();socket.snapshot(state)});
    await act(async()=>button('Shuffle sides').click());
    await act(async()=>socket.snapshot({...state,matches:[{...room,rosterFingerprint:'second'}]}));
    assert.equal(button('Confirm shuffle').disabled,true);
    assert.match(document.body.textContent,/room or roster changed/);
    await act(async()=>button('Cancel').click());
    await act(async()=>button('Shuffle sides').click());
    await act(async()=>button('Confirm shuffle').click());
    assert.deepEqual(calls,[{settingsRevision:1,rosterFingerprint:'second'}]);
    assert.match(document.body.textContent,/2 ready/);
    await act(async()=>socket.snapshot({...state,matches:[{...room,ready:0,ctPlayers:1,tPlayers:1,rosterFingerprint:'third'}]}));
    assert.match(document.body.textContent,/0 ready/);
    await act(async()=>socket.onclose());
    assert.equal(button('Shuffle sides').disabled,true);
  } finally { await ui.finish(); }
});

test('room creator removes a participant only after confirming the current roster', async () => {
  const calls=[]; const ui=await page('/play#matches',async(url,options)=>{if(url==='/api/player/session')return response({steamId:'76561198154367261',steamEnabled:true,gameName:'Owner'});if(url==='/api/player/matches/room/remove-player'){calls.push(JSON.parse(options.body));return new Response(null,{status:204})}throw new Error(`Unexpected fetch ${url}`)});
  const owner={gameName:'Owner',side:'ct',ready:true,stats:null,reportedStats:null};
  const friend={gameName:'Friend',side:'t',ready:true,removalToken:'target',stats:null,reportedStats:null};
  const room={id:'room',hostId:'server',title:'Friday',hostName:'Friends',map:'de_mirage',status:'open',players:2,playersPerSide:2,ctPlayers:1,tPlayers:1,ready:2,participants:[owner,friend],joined:true,mySide:'ct',myReady:true,settingsRevision:1,rosterFingerprint:'first',actions:{shuffle:true},resultCT:null,resultT:null,reportedCT:null,reportedT:null};
  const state={capturedAt:'2026-11-17T10:00:00Z',servers:[],matches:[room],tournaments:[]};
  try {
    await ui.render(); const socket=ui.sockets[0]; await act(async()=>{socket.open();socket.snapshot(state)});
    await act(async()=>button('Remove Friend from the room').click());
    assert.match(document.body.textContent,/They can rejoin/);
    await act(async()=>socket.snapshot({...state,matches:[{...room,rosterFingerprint:'second'}]}));
    assert.equal(button('Confirm removal').disabled,true);
    await act(async()=>button('Cancel').click());
    await act(async()=>button('Remove Friend from the room').click());
    await act(async()=>button('Confirm removal').click());
    assert.deepEqual(calls,[{settingsRevision:1,rosterFingerprint:'second',removalToken:'target'}]);
    assert.match(document.body.textContent,/Friend/);
    await act(async()=>socket.snapshot({...state,matches:[{...room,players:1,tPlayers:0,ready:0,participants:[{...owner,ready:false}],rosterFingerprint:undefined,actions:{}}]}));
    assert.doesNotMatch(document.body.textContent,/Remove Friend/);
  } finally { await ui.finish(); }
});
