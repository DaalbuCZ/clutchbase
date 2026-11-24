import test from 'node:test';
import assert from 'node:assert/strict';
import { JSDOM } from 'jsdom';
import { createServer } from 'vite';
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';

const origin = process.env.CLUTCHBASE_UI_TEST_URL;
const admin = process.env.CLUTCHBASE_UI_TEST_ADMIN;
const player = process.env.CLUTCHBASE_UI_TEST_PLAYER;

test('public and management pages connect to the API and receive live state', { skip: !origin || !admin || !player }, async () => {
  const vite = await createServer({ configFile: false, root: new URL('..', import.meta.url).pathname, server: { middlewareMode: true, hmr: false, ws: false }, appType: 'custom' });
  const { App } = await vite.ssrLoadModule('/src/App.tsx');
  const nativeFetch = globalThis.fetch;
  async function mount(path) {
    const dom = new JSDOM('<div id="root"></div>', { url: `${origin}${path}` });
    const { window } = dom;
    window.document.cookie = `clutchbase-player=${player}; Path=/api`;
    window.HTMLDialogElement.prototype.showModal = function () { this.open = true; };
    window.HTMLDialogElement.prototype.close = function () { this.open = false; };
    window.sessionStorage.setItem('clutchbase-token', admin);
    Object.assign(globalThis, { window, document: window.document, HTMLElement: window.HTMLElement, FormData: window.FormData, sessionStorage: window.sessionStorage, WebSocket: window.WebSocket, IS_REACT_ACT_ENVIRONMENT: true });
    globalThis.fetch = (url, options = {}) => nativeFetch(new URL(url, origin), { ...options, headers: { ...options.headers, Cookie: `clutchbase-player=${player}`, Origin: origin } });
    const root = createRoot(document.getElementById('root'));
    await act(async () => root.render(React.createElement(App)));
    return async () => { await act(async () => root.unmount()); window.close(); };
  }
  async function until(check) {
    for (let i = 0; i < 500; i++) {
      if (check()) return;
      await act(async () => { await new Promise((resolve) => setTimeout(resolve, 50)); });
    }
    assert.fail(`UI did not reach expected state: ${document.body.textContent}`);
  }
  const button = (name) => [...document.querySelectorAll('button')].find((item) => item.textContent.includes(name));
  let close;
  try {
    const anonymous = await nativeFetch(`${origin}/api/player/state`);
    assert.equal(anonymous.status, 401);
    const managementWithPlayer = await nativeFetch(`${origin}/api/state`, { headers: { Cookie: `clutchbase-player=${player}` } });
    assert.equal(managementWithPlayer.status, 401);
    const steam = await nativeFetch(`${origin}/api/auth/steam`, { redirect: 'manual' });
    assert.equal(steam.status, 302);
    const steamURL = new URL(steam.headers.get('location'));
    assert.equal(steamURL.origin, 'https://steamcommunity.com');
    assert.equal(steamURL.searchParams.get('openid.realm'), `${origin}/`);
    assert.ok(steamURL.searchParams.get('openid.return_to').startsWith(`${origin}/api/auth/steam/callback?state=`));
    const reset = await nativeFetch(`${origin}/api/player/profile`, { method: 'PATCH', headers: { 'Content-Type': 'application/json', Cookie: `clutchbase-player=${player}`, Origin: origin }, body: JSON.stringify({ gameName: 'Integration player' }) });
    assert.equal(reset.status, 200);
    close = await mount('/play');
    await until(() => document.body.textContent.includes('Integration server'));
    await act(async () => button('Matches').click());
    await until(() => document.body.textContent.includes('Integration match'));
    await act(async () => button('Profile').click());
    const input = document.querySelector('input');
    assert.equal(input.value, 'Integration player');
    await act(async () => input.form.dispatchEvent(new window.Event('submit', { bubbles: true, cancelable: true })));
    await until(() => document.body.textContent.includes('In-game name saved'));
    await close();
    close = await mount('/manage');
    await until(() => document.body.textContent.includes('Integration server'));
    await act(async () => button('Matches').click());
    assert.match(document.body.textContent, /Integration match/);
    await act(async () => button('Players').click());
    const update = await nativeFetch(`${origin}/api/player/profile`, { method: 'PATCH', headers: { 'Content-Type': 'application/json', Cookie: `clutchbase-player=${player}`, Origin: origin }, body: JSON.stringify({ gameName: 'Updated friend' }) });
    assert.equal(update.status, 200);
    await until(() => document.body.textContent.includes('Updated friend'));
    await act(async () => button('Servers').click());
    assert.equal(button('Connect machine').disabled, false);
    if (process.env.CLUTCHBASE_UI_TEST_SIMULATED === '1') {
      await until(() => button('Create server') && !button('Create server').disabled);
      await act(async () => button('Create server').click());
      const form = document.querySelector('dialog form');
      form.elements.name.value = `Integration fleet ${Date.now()}`;
      await act(async () => form.dispatchEvent(new window.Event('submit', { bubbles: true, cancelable: true })));
      await until(() => [...document.querySelectorAll('.server-card')].some((card) => card.textContent.includes('Integration fleet')));
      const card = [...document.querySelectorAll('.server-card')].find((item) => item.textContent.includes('Integration fleet'));
      const action = () => [...card.querySelectorAll('button')].find((item) => item.textContent.includes('Start server') || item.textContent.includes('Stop server'));
      await until(() => action() && !action().disabled && action().textContent.includes('Start server'));
      await act(async () => action().click());
      await until(() => card.querySelector('.badge')?.textContent.includes('running'));
      assert.match(card.textContent, /SIMULATED/);
      await act(async () => action().click());
      await until(() => card.querySelector('.badge')?.textContent.includes('stopped'));
    }
  } finally {
    await close?.();
    globalThis.fetch = nativeFetch;
    await vite.close();
  }
});
