# SteamID access setup

This is an opt-in host-side test path. The dashboard still uses join passwords.
Clutchbase configures MatchZy's SteamID list on a stopped managed instance.
The Clutchbase Access plugin reports runtime evidence to a local watchdog;
the dashboard shows the agent's reported verification state. Passwords remain
enabled during startup and the watchdog detection window.

## Reference releases

Reviewed on 2026-10-23. These are pinned candidates for a real-server test,
not a claim that their combination has passed CS2 client tests.

| Component | Official Linux archive | SHA-256 |
| --- | --- | --- |
| Metamod 2.0 build 1483 | [mmsource-2.0.0-git1483-linux.tar.gz](https://github.com/alliedmodders/metamod-source/releases/download/2.0.0.1483/mmsource-2.0.0-git1483-linux.tar.gz) | `0fdcccaa369937ed7f521c78462314b5266f39c1d65a84e4c2d3847324cd7776` |
| CounterStrikeSharp 1.0.378, including runtime | [counterstrikesharp-with-runtime-linux-1.0.378.zip](https://github.com/roflmuffin/CounterStrikeSharp/releases/download/v1.0.378/counterstrikesharp-with-runtime-linux-1.0.378.zip) | `b7fcbd99619ab17e32b07a6af2bd5179d83e2c7a4c045e33cbd11e5492c87b7d` |
| Original MatchZy 0.9.2, plugin only | [MatchZy-0.9.2.zip](https://github.com/shobhit-pathak/MatchZy/releases/download/0.9.2/MatchZy-0.9.2.zip) | `8618294d1cfd1f951eeedc2e0b97b0780435cc62df9f9b0afe6738900ea71d88` |

Hashes come from GitHub's release-asset metadata and all three archives were
hashed locally. Their Linux loader paths and .NET runtime metadata were
inspected without executing the plugins. MatchZy's
[project at 0.9.2](https://github.com/shobhit-pathak/MatchZy/blob/0.9.2/MatchZy.csproj)
targets .NET 10 and references CounterStrikeSharp.API 1.0.376.
[CounterStrikeSharp's pinned instructions](https://github.com/roflmuffin/CounterStrikeSharp/blob/v1.0.378/docfx/docs/guides/getting-started.md)
require Metamod **2.x build 1467 or newer** and recommend the bundled runtime.
Metamod's latest stable 1.12 release does not satisfy that requirement.
The host may also need ICU and the usual game runtime libraries.

Original [MatchZy 0.9.2 is MIT](https://github.com/shobhit-pathak/MatchZy/blob/0.9.2/LICENSE).
[Metamod uses the zLib/libpng license](https://github.com/alliedmodders/metamod-source/blob/master/LICENSE.txt).
[CounterStrikeSharp 1.0.378 uses GPLv3 with an MIT exception for plugins,
extensions and software referencing published .NET packages](https://github.com/roflmuffin/CounterStrikeSharp/blob/v1.0.378/LICENSE).
That exception does not relicense its core. Archives also contain runtime and
third-party libraries with their own terms. Clutchbase does not vendor or
redistribute these binaries; review bundled notices and source obligations
before distributing an image containing them. No restricted Auto-Tournament or
Ready Up dependency is involved.

## Configure a stopped instance

1. Stop the game through its current controller, wait until it is stopped, then
   stop the linked agent. The CLI refuses changes while an agent holds its lease.
2. Back up the private instance configuration. Install CS2 with
   `clutchbase-host install NAME` if necessary.
3. Download the three archives above and compare `sha256sum` results with the
   pinned hashes. Extract Metamod, then CounterStrikeSharp, then MatchZy into the
   intended stopped instance's `server/game/csgo/`. Keep all configs, gamedata
   and libraries. Use an isolated instance without unrelated plugins.
4. With a .NET 10 SDK, build the first-party status plugin using the pinned
   CounterStrikeSharp archive's `addons/counterstrikesharp/api` directory:

   ```bash
   make access-plugin CSSHARP_DIR=/absolute/path/to/counterstrikesharp/api
   ```

   Copy `plugins/ClutchbaseAccess/bin/Release/net10.0/` files into the instance's
   `server/game/csgo/addons/counterstrikesharp/plugins/ClutchbaseAccess/`.
   Run `make build` and keep `clutchbase-guard` beside the host and agent binaries.
   The whitelist command copies this watchdog into the private instance directory.
5. In `server/game/csgo/gameinfo.gi`, insert `Game csgo/addons/metamod` inside
   `SearchPaths`, immediately after `Game_LowViolence csgo_lv`, following the
   CounterStrikeSharp instructions. Tabs and spaces are accepted. SteamCMD can
   remove this entry during an update; repair it before starting again.
6. Create a text file with one public individual SteamID64 per line. The command
   below replaces MatchZy's `whitelist.cfg` and owns `config_override.cfg`;
   existing custom override files are refused.

```bash
./bin/clutchbase-host whitelist --file /absolute/path/players.txt friday
./bin/clutchbase-host start friday
```

Use the same global `--root` as creation. Fleet instances use the local name
shown in the dashboard's join-password instructions. IDs must be unique, with
at most 256 entries, and persist in private `instance.json`. An empty file is
rejected. No SteamID grants administrator privileges.

Each managed start rewrites the roster and enables
`matchzy_whitelist_enabled_default`, with `matchzy_everyone_is_admin false`.
The join password stays in `server.cfg`. Missing core plugin files, a missing
Metamod loader entry or a missing fallback password blocks that start. These
are local prerequisite checks. A systemd-supervised watchdog then requires fresh
runtime evidence within 90 seconds of startup. Once verified, evidence older
than five seconds, a disabled whitelist, a changed roster or an unloaded plugin
causes it to stop the whole game process group. It waits up to ten seconds for
graceful shutdown before killing remaining processes. This works even when the
outbound agent is stopped. Failure latches until Stop, preventing a restart loop.
Restart the agent after configuration to resume dashboard control. Existing
configured whitelists must be reconfigured once to install the watchdog before
their next managed Start.

Each launch has a random nonce and process-group binding. The first-party plugin
checks the loaded original MatchZy 0.9.2 instance's actual whitelist flag,
everyone-is-admin flag and a SHA-256 digest of the roster read by MatchZy.
It uses a version-pinned adapter for CounterStrikeSharp API 378's plugin manager;
unknown host/plugin versions or layouts do not verify. Reports stay in private
local files; the API receives only pending, verified or blocked state. Offline
snapshots project access as unknown. No RCON listener is added.

To return explicitly to password-only access, stop the game and agent, then:

```bash
./bin/clutchbase-host whitelist --disable friday
```

This disables the managed MatchZy override and clears its roster, including
after a failed game update has removed the installed marker. It does not
uninstall plugins or remove the password. Do not hand-edit the managed files
for persistent settings.

## Required real-client checks

Record the exact CS2 build and plugin versions. From a locally operated server
console, confirm `meta list` lists CounterStrikeSharp and `css_plugins list`
lists MatchZy with no load errors. Check the runtime whitelist setting;
merely reading its config file is insufficient.

Use two real authenticated Steam accounts, with only the first in the roster:

1. The listed account with the correct password can join and play.
2. The unlisted account with the correct password is kicked. The pinned
   [MatchZy event handler](https://github.com/shobhit-pathak/MatchZy/blob/0.9.2/EventHandlers.cs)
   checks on full connection: this is a post-connect kick, not rejection of the
   initial network connection.
3. A wrong password is rejected for either account.
4. Repeat after restart, map change, a stopped roster edit, practice/match mode
   transitions and plugin/game updates.
5. Remove a plugin file or the Metamod loader entry while stopped, and prove
   managed Start refuses. Separately test plugin load failures and unloading.

The watchdog bounds detection and shutdown time; it cannot prevent every
connection during startup, a five-second evidence window or shutdown. A trusted
local operator can also forge files or modify the processes. Passwordless access
still needs a game-side admission gate and real allowed/denied client evidence.
MatchZy administrators can toggle its whitelist; this now fails runtime
verification and stops the game when detected. Fleet rosters can be edited in
the dashboard after installing this stack and stopping the server. The owning
agent must acknowledge the saved roster before Start is offered; this confirms
configuration only. Runtime verification still comes from the watchdog after
launch. An empty dashboard roster disables the whitelist and keeps the password.
Local roster configuration is preserved until the first dashboard save, after
which PostgreSQL owns the roster. Do not remove password fallback on the strength of an A2S reply or
the runtime verification label.

The real engine smoke-test results, failure injection and retained test-instance
state are recorded in [REMOTE_TEST.md](REMOTE_TEST.md). Those checks verify the
watchdog path; the authenticated human-client checks above are still required.
