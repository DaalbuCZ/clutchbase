# Remote provisioning test — 2026-10-23

Tested over SSH on Ubuntu 26.04.1 LTS, Linux x86-64, using a real user systemd
session and the host's installed SteamCMD. No simulation or fake commands were
used for these checks. The existing CSM agent and CS2 server were left running;
their original process IDs were preserved.

| Check | Result |
| --- | --- |
| Fresh Steam app 730 download and validation | Passed; about 73 GB downloaded |
| Cancel installation from CLI supervision | Passed; process exited, partial files stayed unready |
| Cancel and resume through the real API/agent | Passed; heartbeat stayed online and state returned to uninstalled |
| Block local CLI mutations while an agent owns the instance | Passed |
| Generate/load a real systemd user unit | Passed after fixing WorkingDirectory quoting |
| Start and stop through the API | Passed; desired and observed states converged |
| Query live CS2 over UDP on the SSH host | Passed; expected hostname, app 730, Dust2 and then Mirage |
| Repeat Start without restarting the process | Passed; process ID unchanged |
| Reject map changes while running | Passed with HTTP 409 |
| Stop, change map, restart | Passed; game query and API both reported Mirage |
| Stop agent without stopping its running game | Passed |
| Reject an update of a running game | Passed; game process unchanged |
| Update a stopped instance and start through standalone CLI | Passed |
| Final stop and cleanup | Passed; game PID zero, test UDP listener absent, test agent stopped |

Two implementation changes were required:

- `WorkingDirectory=` must contain an unquoted path. It does not use the same
  word-unquoting rules as `ExecStart=`. Installation now also checks systemd's
  `LoadState=loaded` before writing its installed marker.
- CS2 resets game cvars while loading a map. Managed instances now put settings
  in `server.cfg`, which the engine executes after the reset. The startup
  `clutchbase.cfg` loads settings, supplies any configured GSLT, and starts the
  map. Map commands stay out of `server.cfg` to avoid recursive map loads.

Regression tests were added for both changes. The full Go race suite, real
PostgreSQL integration tests, `go vet`, frontend production build, and local
binary builds passed afterward.

UDP queries from the development machine to the game port timed out even after
the server answered queries locally. No firewall or routing rules were changed.
An actual player connection, password authentication, and public internet
operation with a GSLT were not tested. The instance used an anonymous Steam
server login, for which the engine reported LAN-only player connections.

A2S reported its password-protection flag as false. This is not a reliable
authentication check in CS2: [Valve issue #3993](https://github.com/ValveSoftware/csgo-osx-linux/issues/3993)
describes the same reporting defect even with `sv_password` set. The test does
not claim that a query flag proves password authentication works.

The approximately 69 GiB installation is retained, stopped, under
`~/.local/share/clutchbase-test-20261023`. Its instance is `cb-smoke-20261023`,
using game port 27235 and reserved client/TV ports 27236–27237. The generated
game service is `clutchbase-cb-smoke-20261023.service`, disabled at boot. Tools
are in that root's `bin/` directory. Private instance credentials remain on the
host; temporary API-agent credentials and the SSH forwarding session were
removed after testing. No SSH password is stored in this repository.

## Game-query probe — 2026-10-23

The agent's A2S_INFO implementation was verified against the real engine on the
retained installation. The opt-in `TestRealServerQuery` probe (CS2_PROBE_PORT,
built as a static test binary) queried 127.0.0.1:27235 while
`clutchbase-cb-smoke-20261023.service` ran. The real reply parsed on the first
attempt with no parser changes: 2/64 players (idle bots, no human connection),
stable across repeated runs. The server answered queries about 19 seconds after
the start request, which bounds the expected starting window on that machine.
The probe binary was removed from the host and the instance was stopped again;
the SSH and join passwords remain unstored.

A sample of the real service journal showed the map-load console output the
dashboard log tail collects; no password echo appeared in that sample. A human
password-authenticated connection and public GSLT operation remain unverified.

## Runtime whitelist smoke test — 2026-10-25

Tested the retained isolated installation with Metamod 2.0 build 1483,
CounterStrikeSharp API 378, original MatchZy 0.9.2 (reported build `b1ddce5`),
and the independently built Clutchbase Access 0.1.0 plugin and Go watchdog.
The three upstream archives matched the pinned SHA-256 hashes in
[ACCESS.md](ACCESS.md). No restricted CSM or Ready Up code was used.

| Check | Result |
| --- | --- |
| Real plugin loading and per-launch runtime evidence | Passed; loaded MatchZy reported enabled whitelist and the expected roster |
| Missing runtime evidence at startup | Passed on the initial test; the watchdog stopped the game after the startup deadline |
| Real API/agent Start | Passed; canonical snapshot reached running/verified in about 20 seconds on the repeat run, with 0 players |
| Change the actual roster file while running | Passed; runtime verification failed, the game stopped, and the API reported error/blocked |
| Keep polling after watchdog failure | Passed; the agent did not restart the blocked game |
| Stop, change Mirage to Nuke, Start | Passed; failed state/latch cleared, roster was regenerated and fresh runtime evidence verified |
| Stop the outbound agent | Passed; the game and independent watchdog remained running |
| Expire evidence with the outbound agent stopped | Passed; the watchdog stopped the real game and latched the failure |
| Remove the first-party status plugin while stopped | Passed; managed Start refused; file restored afterward |
| Remove the Metamod search-path entry while stopped | Passed; managed Start refused; file restored afterward |
| Preserve the existing unrelated CS2 process | Passed; original engine PID 2797195 remained unchanged |
| Final cleanup | Passed; isolated service inactive with MainPID 0, test agent/API stopped, temporary database schema and SSH forwarding removed |

The first live launch exposed CounterStrikeSharp's per-plugin DI scope not
providing `IPluginManager`. The adapter now uses the pinned API 378 application's
plugin-manager field; unknown layouts/versions cannot verify. The watchdog
correctly refused that failed launch. A second recovery issue was fixed:
`systemctl stop` alone left the service failed, so Stop now also resets the
failed systemd state. Regression tests cover that recovery.

The end-to-end API test began on 2026-10-23 and was interrupted after detecting
roster tampering. On 2026-10-25 the retained service was confirmed stopped with
its failure latch and no test agent running; the complete test was then repeated
successfully. Go race tests, real PostgreSQL integration tests, vet, frontend
type checking/build, all four Go binaries and the warning-free .NET plugin build
and adapter checks had passed before interruption. The repeat used those same
binaries and a fresh disposable PostgreSQL cluster/schema.

Updated host/agent/guard tools remain in the retained root's `bin/` directory.
The optional plugin stack remains installed, stopped, with a synthetic single-ID
test roster; set the intended players before using it. The last tested map was
Nuke. Private original configuration backups remain under `access-test/original`
on that host. No host/admin/agent credential was added to the repository or logs.

These tests prove runtime observation and bounded watchdog shutdown, not client
admission. Real allowed/denied authenticated Steam joins, wrong-password behavior,
public GSLT operation and multiple real fleet instances remain unverified.
Password fallback stays enabled. Startup grace, evidence expiry and graceful
shutdown leave detection windows; this is not a passwordless fail-closed gate.


## Running prototype for client tests — 2026-10-26

A separate fleet root, `~/.local/share/clutchbase-prototype`, now contains an
isolated copy of the retained game installation. SteamCMD successfully validated
app 730 before launch. The new instance is
`s-vvz3jtdoyc6oe2eidnitsk2xcm`, using game port 27245 and reserved adjacent ports
27246–27247. Its agent runs as `clutchbase-prototype-agent.service`, with private
credentials and an SSH loopback tunnel to the local API. No SSH password was
saved, and the existing original CS2 PID 2797195 remained unchanged.

Both user-supplied SteamIDs were saved from the dashboard. The real machine
agent acknowledged the roster before Start was enabled. The game reached
running/verified on Mirage with 0/64 players before human testing. UDP A2S_INFO
from the development computer succeeded against the host's direct Tailscale
address and timed out against its SSH/LAN address. The user then disabled
Tailscale, which changed the client route to direct LAN. UDP queries succeeded
at `10.0.0.2:27245`, the user confirmed a successful CS2 join, and the engine
reported one player. The canonical dashboard join address now uses LAN. The
agent's SSH forward was reconnected over LAN without restarting the game. UFW
reported inactive; no firewall configuration was changed.

The first installed/stopped heartbeat exposed `reset-failed` failing after
systemd unloaded a never-started inactive unit. Stop now avoids inactive service
mutations and resets failure only for failed services. The new regression,
existing failure recovery tests, Go race/PostgreSQL integration suite, vet,
frontend build and all binary builds passed; the fixed binaries were deployed
before the successful real dashboard roster/start flow.

Unlike earlier smoke tests, this game and its agent are intentionally left
running for the user's CS2 testing. Local prototype service names, connection
commands, private join password and storage paths are recorded in ignored
`.prototype/` files. Anonymous Steam login reports LAN-only connections, so
one allowed human join was confirmed by the user over LAN with Tailscale off.
Wrong-password and unlisted-player rejection, the second allowed account and
public GSLT operation remain to be tested. The prototype services are transient
and not configured to start at boot.

After the player joined, the old combined log report reached 4116 bytes and the
API rejected heartbeats above its 4096-byte cap. The separator now has its own
budget, and UTF-8 capping prevents JSON replacement characters expanding reports.
New regressions and all required checks passed. The updated agent was deployed
and restarted while the actual game PID remained 3342416 and the original
unrelated PID remained 2797195. The player's game process was not restarted.

## Startup presets (2026-10-26)

Reused stopped `cb-smoke-20261023` on port 27235. Staged the independently built
ClutchbaseAccess 0.2.0 reporter beside the new host binary, selected Practice,
and launched the existing pinned stack. A2S readiness and fresh runtime evidence
reported `practice`. Stopped the test, waited for systemd shutdown, selected Match
and repeated: readiness and runtime evidence reported `match`. The test was
stopped again and its managed preset cleared to Host default afterward.
The unrelated CS2 PID 2797195 and active prototype PID 3342416 were preserved.
No human practice commands or complete competitive match were exercised.

## Ready-room MatchZy setup deployment (2026-10-26)

Migration fifteen and the new API were applied to the local prototype. The updated
machine agent was deployed and restarted. ClutchbaseAccess 0.3.1 was staged with
a backup of the previous DLL and hot-reloaded through the configured
CounterStrikeSharp plugin watcher. The API now reports the real prototype server
as running, whitelist-verified, in Match mode and able to load a match setup.
The management bundle is served with the Load MatchZy action. Both CS2 PIDs
(2797195 and 3416627) remained unchanged through API/agent restarts and plugin
hot reload. A real generated match has deliberately not been loaded yet because
that action changes the live game state; complete a ready room and trigger it
from management when ready to exercise the gameplay path.

## Local dashboard recovery (2026-10-27)

The transient local services had stopped. The existing database was backed up
before migration sixteen, and the PostgreSQL and API services were started again.
The dashboard reports the prototype server offline. SSH to the game host timed
out, so its current CS2 and agent state is unknown and the pause-control agent
and reporter have not been deployed there. Real match loading, pause and resume
remain unverified on CS2.

The local API was later upgraded through migration seventeen after another
private database backup. It serves official match-map selection and requires the
updated agent's live map observation before opening or loading a room. A new SSH
attempt to the host still timed out, so the new agent has not been deployed and
real map switching remains unverified.

Migration eighteen was applied to the local prototype after confirming there
were no pending match loads and taking a private database backup. The local API
is healthy and reports the prototype server offline. A subsequent SSH attempt
to the game host timed out, so the updated agent has not been deployed and the
snapshot handoff has not yet been verified against a real CS2 match.

## Prototype host reconnection (2026-10-27)

The saved CS2 host was reachable again. The existing loopback reverse tunnel was
restored and the updated machine agent was installed with a private rollback
copy. Restarting the agent left the separate CS2 service active with the same
main process. The API subsequently reported a running server, verified access,
Match mode and the observed Mirage map. ClutchbaseAccess 0.3.5 was staged beside
the host tools; it was not installed into the active game plugin directory. No
match load, cancellation, live plugin upgrade or player join was exercised in
this step. The SSH control-master tunnel must be recreated after disconnects.

## Prototype reconnection (2026-10-31)

The local database and API were restarted from their retained files, and the
existing host agent was reconnected through its loopback SSH tunnel. The API
reported the CS2 service running with verified Match mode and Mirage observed;
the game service retained its existing main process. A read-only Source query
received a response from `100.89.5.109:27245`. No UDP response was received from
`10.0.0.2:27245`; the workstation's current route for that address uses
Tailscale. A Source query does not prove a player can join. Tailscale and the
running game were not changed during this check.

The reporter-update API and host agent were deployed after a private database
backup. The server remained running with the same game process, and its snapshot
showed a staged reporter update with installation disabled while running. The
actual stopped-server installation and subsequent plugin startup remain untested.

## Retained prototype smoke test (2026-11-12)

On byttorrent over LAN, the retained prototype instance was initially stopped,
with its agent inactive. Its existing host tool started the isolated instance on
port 27245. The current game-query test binary queried real CS2 on loopback and
parsed a 0/64 reply. Fresh ClutchbaseAccess runtime evidence reported verified
whitelist status, Match mode, and match-load capability; the game was supervised
by its guard. The test did not load a match or exercise a human connection.

The instance was stopped afterward and systemd reported inactive with MainPID 0.
The two unrelated CS2 processes that were present at the start retained their PIDs
throughout. The test binary, SSH control connection and supplied authentication
secret were removed. The dashboard/agent control loop and the staged reporter
were not tested in this run.

## MatchZy setup through the real control loop (2026-11-12)

After backing up the retained prototype database, the current API and current
machine agent were run temporarily against the isolated byttorrent instance.
The retained desired state was running, so the reconnected agent started CS2;
the canonical snapshot reached running, verified whitelist, Match mode and
Mirage. A clearly labeled test room was created for Nuke. Two temporary sessions
for rostered SteamIDs joined CT and T and marked ready via the player API; no
human players were connected. The load request returned accepted, then the
canonical room reported MatchZy setup complete and the agent observed `de_nuke`
while whitelist verification remained active. This verifies setup acknowledgement
and live map switching, not a played match, pause/resume or automatic results.

Stop was requested through the API; the snapshot converged to stopped and the
systemd service was inactive with MainPID 0. The temporary test room, sessions
and untouched test-only profile were removed. The temporary agent, API, database
and SSH tunnel were stopped; unrelated CS2 PIDs 8683 and 13090 stayed unchanged.
The database now requests the instance stopped, matching its original physical
state before this test. The private backup remains in `.prototype/`.
