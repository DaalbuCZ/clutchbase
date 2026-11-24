# Clutchbase project memory

Signed-in players can request a server's access through a link copied from
management. Player and management request lists update over authenticated live
views. Administrators can reject or approve a pending request; approval appends
the SteamID to the server's saved roster under a lock and retains existing
entries. It requires an online capable agent and a stopped server. Empty or
unconfigured rosters disable enforcement; approving the first player requires
explicit confirmation that enabling the whitelist can exclude other players.
Approval records intent; the agent must still apply the roster and verify
access before players can join. Rejected or revoked requests may be resubmitted.
Management can hide a revealed join password; sign-out clears revealed values
and ignores password requests that complete after that session ended.

Series match history shows configured maps and the reported series total.
Administrators can record or clear a manual score for each map using the shared
result revision; players see those records on servers they can access. Scores
refer to the teams that started CT and T on the first map, even when sides
switch. These records are independent of MatchZy's reported final-map score and
do not change tournament advancement or overall series outcomes. Recorded map
scores contribute to map performance; configured maps without records are not
counted, since they may not have been played. Earlier map scores are not
captured automatically.

For future automatic per-map capture, the inspected pinned MatchZy source sets
`currentMapFinished` at map end, publishes a map result using team scores and a
zero-based map number, then advances that number for a continuing series. The
flag is private and reset on the next live map; polling alone can miss this
window. A durable, setup-fenced event path is needed before treating individual
map scores as authoritative, including cancellation and forfeit semantics.
Source: https://github.com/shobhit-pathak/MatchZy/blob/21c278070f9ef4eddcb0baf8f860a8e007271af6/Utility.cs#L853

The management and public navigation remain in their sidebars. The sidebar can
scroll on short screens; on narrow screens its page links use a scrollable grid
so every page remains reachable without a horizontal navigation strip.
The public server list can be searched by name, map or address and filtered to
the servers currently marked joinable by the live snapshot. Filters are local
display choices and do not override the server's join action or access rules.
Public and management match rooms provide a shareable public room link and show
server-reported team capacity with a visual bar and open-slot count. Steam
sign-in retains the room destination; the recipient still needs server access.
If copying fails, the link is displayed for manual copying.
Management's player directory supports name and SteamID search, result counts,
and empty states. It filters the canonical player snapshot locally and offers
SteamID copying, with the displayed ID available if clipboard access fails.
Public match rooms summarize the next join, ready, load or connect step from the
current snapshot. Side actions and connection controls adapt to narrow screens;
browser visual verification remains outstanding.
Once every participant is ready, non-owners see that the host must load the
match. A loaded room only shows its connect command to participants in the
locked roster, with the join-password reminder beside it.
Closing an unstarted room also locks its participant roster; players cannot
remove themselves afterward, and its snapshot no longer offers a leave action.
Match snapshots provide load blocker codes for server readiness, access,
reporter support, player readiness and unsupported Workshop maps. Management
and joined players see these explanations; the server action remains the gate.
The server uses the same eligibility predicate for management snapshots and the
locked load request, and player snapshots also account for creator permission
and access. PostgreSQL integration coverage checks the audited failure cases.
Tournament fixtures now have a database-maintained result revision that changes
when participants, status or winner change. This is groundwork for a safe
correction workflow; no correction action is available yet.
Winner changes made after migration are recorded with their prior and new winner,
revision and timestamp. This record does not identify a specific administrator;
administrators can correct a round-robin winner with confirmation and the fixture
revision, recomputing completed standings without changing reported scores.
Completed single-elimination fixtures can be corrected when their immediate next
fixture is pending, has no room or veto, and still contains the old winner.
Finals and double-elimination reset finals can also be corrected, updating the
tournament winner. Started downstream fixtures and double-elimination grand
finals remain protected; broader dependency-aware correction is still incomplete.
An active double-elimination winners-bracket result can be corrected when both
winner and loser destinations still hold the expected teams and are pending with
no room or veto; both routes are checked and updated in one transaction. Bye
propagation and started destinations remain protected. A losers-bracket result
can be corrected before its winner destination starts; the transaction swaps
the advancing team and the two teams' checked-in/eliminated states.
PostgreSQL coverage checks reset-final correction and
rejects changes to the preceding grand final.
Public and management brackets label fixtures whose winner has been corrected;
the captured score remains separately visible. Management can expand the
recorded winner changes and timestamps; player snapshots omit that history.
The management team directory searches team names, members and SteamIDs, with
expandable rosters and counts from the same snapshot.
Tournament brackets show round completion counts and highlight the current
round. Events with more than three rounds include keyboard-accessible round
jump links. The layout has not yet been visually checked in a browser.
Reporter 0.3.15 checks canonical Workshop IDs and local files but explicitly
blocks Workshop match loading. The API also blocks that action and management
explains why. Real engine loading and canonical runtime identity remain required;
see docs/WORKSHOP_REPORTER.md.

Player and management match history can search by match, server, map or player
name across accessible history. Map searches include every configured series
map. Filtering and pagination run on the server.
History cards keep score summaries visible and place player scoreboards and
configured series maps in expandable sections. During reconnects the last
snapshot and open drafts remain visible with their receipt time; editing and
paging stay disabled until a fresh connection is established. Frontend build,
existing UI tests and five focused history tests pass. Browser visual inspection
of this layout remains outstanding.

## Product and user decisions

Agreed 2026-10-23; expanded 2026-11-14. Build an independent CS2 clubhouse for
playing with friends, managing servers and matches, preserving stats and
highlights, and supporting tournament workflows. The user now requests functional
breadth comparable to Auto Tournament, Ready Up and CS2 Server Manager, without
copying their implementations or restricted dependencies. Tournament features
must remain optional: users can create and keep matches without entering a
tournament. Later import supported matches played elsewhere, similar in purpose
to Leetify.

The user selected **React + TypeScript, Go, and PostgreSQL**, and authorized
implementation. Application, backend workers, and host agent live in one repo.
The user explicitly requested this Markdown memory for future chats.

GitHub repository: https://github.com/DaalbuCZ/clutchbase (private, owned by
`DaalbuCZ`). The local `origin` remote points to this repository; primary branch
is `main`. The user requested creating the repository and uploading this work.

The user also explicitly required shared UI/server state: the UI must reflect
the authoritative internal state, not maintain a second lifecycle model.
PostgreSQL-backed snapshots, server-computed statuses/actions, and revision
checks implement this requirement. Keep form drafts/loading indicators local;
never optimistically mark a server running or stopped.

UI copy preference: remove pointless subheadings, decorative eyebrow labels,
marketing taglines, and redundant headings. Keep meaningful field/status labels.
This preference is also recorded in `AGENTS.md`.

## Ownership and dependencies

Auto-Tournament, cs2-server-manager, and Ready Up were inspiration only. Their
PolyForm Noncommercial terms prompted the user to build independently. Do not
copy their source, assets, branding, or depend on their restricted forks.

Possible future integrations, not current dependencies:

- Original MatchZy: MIT. https://github.com/shobhit-pathak/MatchZy
- CounterStrikeSharp: GPLv3 core with an MIT exception for plugins/extensions and
  software referencing published .NET packages; not entirely MIT.
  https://github.com/roflmuffin/CounterStrikeSharp/blob/main/LICENSE
- Go demo parser: MIT. https://github.com/markus-wa/demoinfocs-golang

Check exact release licenses and transitive dependencies before distribution.
No license for the user's own code has been selected yet.

## Incremental roadmap

1. First control loop: register one host, show status, select an official map,
   start/stop a preinstalled CS2 service, show connection details.
2. Steam sign-in, groups and permissions; installation/provisioning, additional
   server instances, SteamID whitelists (preferred over join passwords), logs and
   useful startup/readiness checks.
3. Match/practice startup presets and standalone rooms with player participation
   and temporary CT/T sides; MatchZy single-map loading from ready rooms is
   implemented, with selectable official match maps and guarded live pause/resume
   controls. Next: Workshop maps and validated game settings. Standalone rooms
   support an explicit overtime toggle.
4. Demo recording/upload, completed matches, scoreboard and basic statistics.
5. Manual demo imports, then share-code retrieval and authorized automatic
   external match tracking. Source-aware deduplication and one analysis pipeline.
6. Highlight detection/bookmarks, then CS2 playback/capture and clip rendering
   in a separate worker. FFmpeg alone cannot render a demo.
7. Skins/loadout preferences and separately evaluated server integrations.
   Valve restrictions and server-ban risks need explicit treatment.
8. Tournament management and richer analytics, multiple hosts, safe updates and
   backups. Tournament mode is part of the new parity target, but not required
   for casual matches.

## Architecture

- `web/`: React/Vite dashboard. Calls the same-origin `/api` endpoints.
- `backend/`: Go HTTP API, PostgreSQL persistence, transactional versioned migrations.
- `agent/`: outbound Go agent and `clutchbase-host` CLI. Managed mode installs
  CS2 and generates systemd user services; the existing-service mode remains.
  Machine mode connects once and controls multiple UI-created instances. Legacy
  single-instance registrations/agents remain supported separately.
- `internal/protocol/`: shared map allowlist and agent/API types.
- PostgreSQL stores desired state and the last observed process state separately.
- Management uses an administrator token; Steam players use HttpOnly sessions
  with read-only snapshots filtered by their saved server whitelist.
  Each machine (or legacy instance)
  gets its own random agent token; only its hash is retained in the database.
- Agent reports observed state and receives desired state, map and numeric game
  port. Paths and executables are configured locally, never sent as shell strings.
- Offline is derived from heartbeat age. Running requires a successful local
  game query and, for configured whitelists, fresh runtime evidence. Neither
  proves authenticated human clients can join.

## First-slice boundaries

No staff/group permission administration, public deployment, cloud VM provisioning,
shared game installation, RCON, in-game ready-up/veto/score controls, statistics,
demos, skins, tournament administration, or highlight renderer yet. Managed installs require SteamCMD, OS
runtime libraries, and a working user systemd session supplied by the operator. Each instance has
a full isolated install. Changing the startup map requires a stopped or
uninstalled server; loading a match room can switch the live MatchZy map.
Simulation is explicitly labeled and does not launch CS2. Keep API/frontend on
loopback locally; use HTTPS and proper user authentication before public access.

## Verified implementation (2026-10-23)

The first control loop is implemented: host registration, separate hashed agent
credentials, PostgreSQL-backed workspace snapshots, server-derived lifecycle and
actions, revision-protected commands, map selection, and simulation/systemd agent
drivers. The UI has been simplified according to the user's copy preference.

Verified with Go race tests (including a real PostgreSQL 18 integration test),
`go vet`, a TypeScript/Vite production build, Compose configuration validation,
and browser registration/map/start/stop checks using the simulated agent. The
browser showed starting/running/stopping/stopped transitions from API snapshots
and no console warnings/errors. At this initial checkpoint, real CS2/systemd
operation and the Docker image build had not yet been exercised. The later
remote test below supersedes the CS2/systemd limitation.

Current development preview uses a disposable PostgreSQL cluster under `/tmp`
and one explicitly simulated host. It is not a production deployment or a real
game server. Follow README.md to configure durable local credentials/storage.

## Automatic provisioning (2026-10-23)

The user requested an independent CSM-like tool to spin up servers automatically.
`clutchbase-host` now creates named local instances, installs Steam app 730,
generates private configs/systemd user units, starts/stops, updates stopped
instances, lists state, reads install logs, and retrieves join passwords.
Multiple instances use separate directories and non-overlapping port ranges.
The tool never invokes sudo or downloads code from Auto-Tournament projects.

`clutchbase-agent --instance NAME` creates a missing local config from the
registered game port, installs missing CS2 files on Start, and reports
uninstalled/installing/error/actual service state through the existing snapshot.
Installation runs in a cancellable background job so heartbeats continue.
Successful installation alone does not start a game: the latest desired state
is reconciled afterward. Failed downloads stay unready and can be retried.
One controller lease prevents local CLI mutations or another agent from fighting
the dashboard while linked. Secrets stay in host files, not PostgreSQL or logs.

Tests cover isolation, port collisions, private credentials, generated commands,
service ownership, blocked live updates, cancellation/process-group cleanup,
retry after partial downloads, controller leases, background provisioning,
registration/port matching, and authoritative API installation states. Game
downloads and systemd commands use stand-ins in the automated tests; they are
now complemented by the real remote smoke test below.

## Real remote verification (2026-10-23)

The user authorized testing over SSH on their Linux host. The actual SteamCMD
download/validation, cancellation/resume, linked agent/API state, real systemd
launch/stop, Dust2-to-Mirage map change, local A2S query, idempotent Start, CLI
ownership lock, live-update rejection, and stopped update/start all passed.
Existing CSM/CS2 process IDs were preserved. Full details and retained instance
location are in `docs/REMOTE_TEST.md`. Never store the supplied SSH password.

The test found and fixed quoted WorkingDirectory paths rejected by systemd, and
game settings needing reapplication after CS2's map-load cvar reset. Installation
now verifies unit LoadState before marking ready; managed server.cfg reapplies
settings without map commands. Regression and PostgreSQL tests passed afterward.

Development-machine UDP queries timed out; no firewall was changed. Human player
connections, password authentication, public GSLT operation, and the Docker
image build remain unverified. CS2's A2S password flag was false, matching a
known upstream reporting issue; do not treat that flag as an authentication test.
The test game/agent were stopped and temporary tunnel/agent credentials removed;
the approximately 69 GiB isolated installation is retained for reuse.

## UI provisioning and access preference (2026-10-23)

The user requested whitelist-based access instead of passwords and creating new
servers from the UI, allowing the next step that follows the roadmap. This slice
implements machine registration plus multiple UI-created instances with one
outbound machine agent. The user's preferred final access model is a SteamID
whitelist, not shared join passwords. Passwords remain until a verified game-side
whitelist enforcement mechanism exists; a database list alone cannot restrict
CS2 connections.

The UI connects a machine once, shows heartbeat/capability state, and creates
servers with an official map and automatic or explicit game port. Creation is
stopped until Install & start; state/actions still come from PostgreSQL and
observed reports. Reserved three-port ranges are allocated under a machine row
lock, including concurrent requests. Machines and legacy servers keep separate
hashed credentials; a machine heartbeat may only report its own instances.
Failed/omitted reports do not falsely mark another instance ready or online.

The new machine agent derives private local instance names from server IDs,
keeps a controller lease per instance, reuses cancellable managed installations,
and reports siblings independently. Global SteamCMD locking serializes downloads.
A process-wide GSLT is rejected in machine mode because each public instance
needs its own token. No public fleet onboarding or GSLT management UI is provided.
Legacy installations are retained, not automatically adopted or duplicated.
There is no delete/move operation. Up to 64 instances per machine are allowed by
the protocol; disk, CPU, networking and memory remain operator responsibilities.

Versioned transactional migrations preserve the previous unversioned database,
including desired state, revisions, map selections and legacy credentials. Tests
cover upgrades, repeated migration, auth boundaries, transactional rejection of
foreign reports, concurrent port allocation, independent instance reconciliation,
installation cancellation, sibling failure recovery, and controller leases.
Go race tests, vet, real PostgreSQL integration tests and the TypeScript/Vite
production build pass. Browser checks use a clearly labeled simulated machine;
this new multi-instance path has not yet been tested against multiple real CS2
processes. Earlier single-instance remote CS2 verification still applies only to
the previously tested managed runner.

## Player visibility (2026-10-23)

The agent proves joinability instead of inferring it from systemd. On each
heartbeat it sends one A2S_INFO query to the loopback game port, so host
firewalls that block remote queries do not matter. A valid answer reports
running with player counts; an active service that does not answer keeps the
existing starting projection. Managed instances query their configured port;
the legacy existing-service runner learns the registered port from desired
state. Simulation is unchanged and stays labeled. Observed running therefore
means the game answers queries, which the service-process meaning never proved.

Player counts and a log tail persist in PostgreSQL (third migration; nullable
players/max_players, log_tail text). Reports without counts clear them, so a
stopped or stale report cannot leave phantom players behind. Server cards show
the count while running and a capped tail of the service journal and
install.log. Lines that could echo join passwords or GSLT tokens are redacted
on the host before anything is sent, keeping secrets out of PostgreSQL and the
UI. The A2S password flag stays untrusted per the known upstream defect.

Tests use in-process fake A2S responders (modern, obsolete and challenge
replies, silence), redaction and truncation cases, and the real PostgreSQL
integration suite covering persistence, validation limits and clearing. The
API/browser path was verified with a synthetic running report showing 5/12
players and the log tail on a card. The agent query was then verified against
real CS2 on the retained remote installation: the probe parsed the engine's
reply on the first attempt (2/64 with idle bots, queryable about 19 seconds
after the start request). A password-authenticated human connection and public
GSLT operation remain unverified and need a friend with CS2.

Next: the enforced SteamID allowlist. Original MatchZy has documented whitelist
support (one SteamID64 per line in whitelist.cfg and
matchzy_whitelist_enabled_default), but needs Metamod and CounterStrikeSharp.
Evaluate/pin compatible independently licensed releases, verify loaded plugins
and allowed/denied real client joins, and fail closed before replacing
passwords. Reference: https://shobhit-pathak.github.io/MatchZy/configuration/ .
Dashboard Steam sign-in/groups waits until the UI is shared with other people.
The Docker image build and a delete operation for stopped fleet instances
remain small open items.

## Dependency security update (2026-10-23)

Upgraded pgx from v5.7.6 to v5.9.2 to address the GitHub dependency alerts.
The updated driver no longer depends on golang.org/x/crypto; go mod tidy removes
that module and its affected versions from the dependency graph. A follow-up
Go vulnerability scan identified two x/text issues; x/text is updated to v0.41.0,
with its required x/sync v0.22.0. The minimum Go version is now 1.25; the existing
Docker builder uses Go 1.26.

Verified the security-only change in an isolated copy of committed code because
other application edits were in progress in the shared workspace. Go race tests,
real PostgreSQL integration tests, vet, frontend type checking/build and all three
binary builds pass. govulncheck v1.9.0 reports no vulnerabilities.

## SteamID whitelist preparation (2026-10-23)

The next access-control slice now has an opt-in host command:
`clutchbase-host whitelist --file FILE NAME`, with `--disable` for an explicit
return to password-only access. It configures an operator-installed original
MatchZy stack on an already installed, stopped managed instance. No plugins are
downloaded or installed automatically and no existing instance was modified by
this development work. Dashboard whitelist controls are not present yet.

The host validates up to 256 unique public individual SteamID64 values, persists
them in private local configuration, and regenerates MatchZy's whitelist and
managed override on Start. Agent leases and operation locks prevent conflicting
CLI mutations. Custom override files are preserved by refusing to replace them.
The join password stays mandatory. Missing core plugin files or the Metamod
search-path entry blocks Start; an interrupted roster write is retried before
launch. Disabling remains possible after a failed update removes the installed
marker, and interrupted disables are repaired before launch.

`docs/ACCESS.md` records exact candidate archives, SHA-256 hashes, upstream
licenses, installation steps and a real-client test procedure. MatchZy 0.9.2
targets .NET 10 and references CounterStrikeSharp.API 1.0.376; the selected
CounterStrikeSharp 1.0.378 instructions require Metamod 2.x build 1467 or newer.
The candidate Metamod is 2.0 build 1483, not its incompatible stable 1.12 line.
The pinned MatchZy implementation kicks unlisted players on full connection;
it does not reject the initial network connection. No player receives admin
privileges merely by being in the roster.

This prepares whitelist testing, not passwordless or fail-closed enforcement.
On-disk prerequisite checks cannot detect plugin load failure, unloads, runtime
whitelist toggles or a malicious administrator. A runtime access gate and
canonical enforcement reports must precede dashboard roster controls and removal
of password fallback. Real allowed/denied authenticated client joins, plugin
loading on the retained remote CS2 build and bundled dependency redistribution
review remain unverified. No third-party binaries are vendored in this repository.

Validation: Go race tests (including real PostgreSQL integration), vet, frontend
type checking/production build and all three binary builds pass. Focused tests
cover invalid/duplicate/oversized rosters, stopped-only edits, controller leases,
private files, custom config ownership, password preservation, regeneration,
missing loader entries and disable recovery. Plugin behavior itself is not
proven by the stand-in files used in these automated tests.

## Runtime whitelist watchdog (2026-10-25)

Added the independent first-party `ClutchbaseAccess` CounterStrikeSharp plugin
and `clutchbase-guard` Go supervisor. The optional plugin builds with .NET 10
against the separately installed, pinned CounterStrikeSharp API 378. It checks
the loaded original MatchZy 0.9.2 instance's whitelist flag, everyone-is-admin
flag and the roster file digest. Its adapter uses a pinned private CSS plugin
manager field because the per-plugin dependency-injection scope does not expose
the manager; unknown API versions or layouts do not verify.

Private atomic reports bind a random launch nonce, game process group, engine
PID, roster hash and timestamp. The supervisor starts with a 90-second grace
period and, after verification, stops the process group when evidence is invalid
or more than five seconds old. Shutdown allows ten seconds before SIGKILL.
It remains under systemd when the outbound agent exits. Failure latches until
Stop so the agent cannot repeatedly restart an unverified game. Stop also resets
systemd's failed state; a real startup test exposed the need for that recovery.

The host whitelist command copies the guard from beside the host/agent binaries
into the private instance. The plugin must be installed separately. Existing
configured whitelists need reconfiguration before their next managed Start.
No RCON listener or arbitrary browser command/path was added. Passwords remain
mandatory during startup, detection and shutdown; this watchdog is not a
passwordless admission gate and cannot protect against a trusted local operator.

A fourth transactional database migration persists agent-reported access status.
Cards render the canonical snapshot's password/pending/verified/blocked state;
stale hosts project access as unknown. Dashboard roster editing remains pending.

Go race tests, real PostgreSQL migration/report tests, vet, frontend type checking
and production build, all four Go binaries, and a warning-free .NET plugin build
and assert-based adapter checks passed on 2026-10-23. Automated tests cover
launch/process/roster binding, expiry, invalid evidence, startup failure,
cancellation, watchdog loss, restart latching, Stop recovery and stale snapshots.
Real CS2 loaded the pinned stack and produced verified evidence; an initial
plugin load failure correctly triggered the startup timeout. The complete real
API/agent test was repeated on 2026-10-25: roster tampering stopped and latched
the game, Stop/map-change/Start recovered with fresh evidence, and expired
evidence stopped the game even after the outbound agent exited. Missing plugin
and Metamod loader entries also refused Start. The isolated game is stopped,
temporary API/agent credentials and forwarding are removed, and the original
unrelated CS2 process was preserved. Details are in `docs/REMOTE_TEST.md`.

Allowed/denied authenticated human joins, wrong-password behavior and public
GSLT operation remain unverified. The existing real fleet/multi-instance and
dependency redistribution limitations still apply. Keep password fallback until
a game-side admission gate and real client tests justify replacing it.

## Stopped fleet server removal (2026-10-25)

The dashboard now offers Remove server for online stopped/uninstalled fleet
instances whose machine agent advertises removal support. An inline confirmation
states that installation files are retained. The revision-protected DELETE
request marks the canonical row as removing with no lifecycle actions. The card
does not disappear optimistically; ports and capacity remain reserved until the
owning agent acknowledges completion. Legacy instances and older agents do not
offer removal.

The agent keeps its controller lease while checking the local service is
inactive and verifying systemd unit ownership. It removes the inactive owned
unit, reloads systemd and atomically moves the installation to the private
`ROOT/retired/LOCAL_NAME/` directory, preserving configs, credentials, game files
and logs. Local creation and per-instance operation locks prevent races with
provisioning and port allocation. Retained directories are never overwritten.
The controller lease is released only after the API omits the acknowledged ID.
Unexpected omissions of live controllers remain errors.

A fifth migration adds removal capability, pending state and a database
tombstone. Tombstones make repeated acknowledgements safe after a lost response
and prevent stale normal reports from reviving removed entries. Machine-row
locking serializes acknowledgement, removal requests and new port allocation;
host revision checks arbitrate concurrent Start and Remove. Downgrading an agent
does not discard an already pending request. Agent restart after the local move
can safely repeat its acknowledgement.

Go race tests, real PostgreSQL migration/integration tests, vet, frontend type
checking/build and all four binary builds pass. Coverage includes live/offline
removal rejection, ownership/lease/operation locks, retained-file integrity,
restart/retry recovery, foreign report rollback, stale revisions, capability
fallback, capacity/port release and concurrent lifecycle requests. A live local
API plus clearly simulated fleet agent also completed the request/acknowledgement
flow while preserving a sibling registration. Browser inspection verified the
initial removal controls; a native-dialog automation failure prompted the inline
confirmation, but the final browser confirmation flow could not be fully retested
in that browser session. No real CS2 instance was removed for this slice.

Removal retains disk usage and a database tombstone. Automatic restore, permanent
file purge, legacy removal and machine removal remain unsupported. The existing
human-client whitelist and real multi-instance compatibility limits still apply.


## Dashboard SteamID roster editing (2026-10-26)

Stopped, installed fleet servers now expose a SteamID64 roster editor when their
machine agent advertises support. Up to 256 unique public individual accounts
are validated by shared Go code at both API and agent boundaries. PostgreSQL
stores nullable roster intent: null preserves existing host-side configuration;
an explicit empty roster disables the whitelist while keeping password protection.
The sixth transactional migration preserves existing instances and credentials.

Roster saves require administrator authentication, a current revision and the
canonical stopped action. They cannot be combined with Start. Start remains
blocked until an online capable owning agent reports the exact applied roster
hash. An older agent receives stopped intent for dashboard-managed rosters, even
when the stored desired lifecycle is running. The card shows pending/applied
configuration separately from runtime whitelist verification. The browser keeps
only an editing draft and reloads canonical state after saving.

The managed agent uses its existing instance lease and stopped-only whitelist
mutation, refuses custom override ownership, preserves credentials, and retries
partial file writes before acknowledging a roster. Matching running rosters are
checked without rewriting the launch nonce, so heartbeat reconciliation does not
invalidate watchdog evidence. On restart, acknowledgements are reconstructed
from local configuration/files and fresh desired intent. Plugin prerequisites
and the guard are still operator-installed; no third-party download or shell/path
input was added. Simulated agents label their acknowledgement as simulation and
do not enforce player access. Legacy instances retain local CLI roster controls.

Validation includes Go race tests with real PostgreSQL integration, vet, frontend
type checking/production build and all four binary builds. Tests cover canonical
roster persistence, explicit disable, revision/capability/authentication gates,
concurrent Start versus roster saves, older agents, partial-write recovery,
restart reconciliation, live-edit refusal and launch-nonce preservation. Browser
checks use a disposable API/database and a clearly simulated agent.

The real authenticated allowed/denied client tests, public GSLT checks and real
multi-instance limits remain unchanged. Password fallback remains mandatory.
Dashboard roster editing does not prove real-client enforcement. The Docker image
build remains an open validation item.


## Running client-test prototype (2026-10-26)

The user requested a working prototype for real CS2 client testing. A local
production frontend/API now runs on loopback port 8080, with a private durable
PostgreSQL cluster on port 55438. Local transient user services are named
`clutchbase-prototype-api`, `clutchbase-prototype-postgres` and
`clutchbase-prototype-tunnel`. Private credentials, database files, join commands
and operational notes live in ignored `.env` and `.prototype/`; the SSH password
is not saved. These services are not enabled at boot. The SSH tunnel and this
computer must remain online for the remote machine agent to reach the API.

The game host has a separate `~/.local/share/clutchbase-prototype` root and
`clutchbase-prototype-agent` user service. A new fleet instance uses an isolated
copy of the retained test game's files, validated successfully by SteamCMD.
Validation reset the Metamod search-path entry; it was restored in the new copy.
The existing retained instance and unrelated original CS2 process were preserved.
The two user-supplied SteamIDs were saved through the dashboard, acknowledged by
the real agent, and loaded with verified runtime watchdog evidence on Mirage.
The new game is intentionally left running for the user's client tests.

With Tailscale enabled, the host's direct tailnet address answered real UDP A2S
queries while its LAN address did not. After the user disabled Tailscale, the
route changed to direct LAN and UDP succeeded at `10.0.0.2:27245`. The user then
confirmed a successful CS2 join, and fresh engine queries reported one player.
The canonical dashboard address and private join instructions now use that LAN
address. The agent tunnel was reconnected over LAN without restarting the game.
No firewall setting was changed; keep the current Tailscale routing disabled
while testing through LAN. The engine uses anonymous Steam login and warns of
LAN-only connections; public GSLT operation and wrong-password/unlisted-player
rejection remain unverified. Exact access instructions are in private
`.prototype/join.txt` and `.prototype/README.md`.

This deployment exposed an inactive Stop regression: systemd can unload a
never-started unit before `reset-failed`, causing stopped reconciliation and
roster application to fail. Stop now queries actual service state, leaves an
inactive service stopped, and only resets failure for a failed service. It
still clears the watchdog latch. A regression test covers repeated Stop on an
inactive/unloaded unit; existing failed-watchdog recovery tests also pass.
Go race tests, real PostgreSQL integration tests, vet, frontend type checking/
build and all four binary builds passed after the fix. The new binaries were
deployed and the real stopped/roster/start flow then succeeded.

A second live issue appeared once the journal and installation tails were both
full: their 21-byte separator pushed the combined report to 4116 bytes, above
the API's 4096-byte limit, so valid game heartbeats were rejected. The install
tail now reserves space for the separator. Log capping also preserves valid
UTF-8 so JSON encoding cannot expand split characters beyond the byte budget.
Regression tests cover full combined tails, Unicode truncation and redaction.
The full race/PostgreSQL/vet/frontend/binary checks passed again. Updated agent
and host binaries were deployed without restarting the game or changing its
watchdog launch evidence. The local tunnel service now supervises the health of
the existing authenticated LAN SSH forward; no SSH password is persisted.


## Management/player workflows and WebSockets (2026-10-26)

The user requested Auto-Tournament's functional breadth in several slices, using
its management/player workflows as a starting point. Functional inventory and
slice order are recorded in `docs/FEATURES.md`. Only route/navigation
behavior was used as reference; no restricted implementation, styling, assets or
runtime was copied. The app remains independently implemented in this repository.

The first slice separates `/manage` from `/play` (also the default root view).
Management retains administrator-token control. Steam OpenID verifies player
identities with a fixed HTTPS provider and configured application origin, using
single-use hashed login state, nonce/signed-field checks and a provider-side
verification request. Hashed 24-hour sessions use HttpOnly/SameSite cookies and
Secure cookies for HTTPS. Steam players never inherit administrator rights.
No Steam Web API key is required; names/avatars are not fetched in this slice.

Player HTTP and live snapshots expose only servers containing that SteamID in
the saved dashboard whitelist. They exclude machine records, management actions,
logs, errors, other SteamIDs and credentials. Join availability requires canonical
running state, acknowledged roster and verified runtime access. Administrators
can reveal a managed server's join password in its card; the agent reports it
over its authenticated heartbeat and the API keeps it only in short-lived memory.
The password is absent from PostgreSQL, logs, workspace snapshots and player APIs.

Browser polling is replaced by same-origin WebSockets using the ISC-licensed
`github.com/coder/websocket` v1.8.15 dependency. Administrator credentials are
sent in the first frame, not query parameters; player streams use their session
cookie. New migrations seven/eight add committed-change notification triggers,
players, sessions and login state. PostgreSQL notifications publish fresh canonical
snapshots; server-side timers publish heartbeat/session expiry. Logout, roster
removal and session expiry revoke player stream access. Reconnect backoff and
transport liveness checks disable controls while the connection is unavailable.
Agent heartbeats remain outbound HTTP every five seconds and game processes are
not restarted by this browser transport change.

Each live connection owns a dedicated PostgreSQL listener outside the query pool,
with a 64-connection process limit. Use shared fan-out before larger deployments.
`CLUTCHBASE_ORIGIN` defaults to loopback port 8080; HTTPS is required for public
origins and the configured origin controls callbacks and logout validation.
Steam sign-in does not by itself make this a hardened public deployment.


Validation for this slice passed Go race tests, real PostgreSQL integration tests,
vet, frontend type checking/build, a frontend WebSocket reconnect/liveness test,
all four binary builds and Compose configuration validation. Integration coverage
includes direct committed database pushes, rollback silence, offline expiry,
player roster revocation, session expiry, cross-origin rejection, Steam callback
replay and forged assertions, logout CSRF, and player/admin access separation.
The user completed real Steam sign-in on the deployed `/play` page and confirmed
that the permitted CS2 server was visible. Browser inspection confirmed the
player saw join information and one connected player without management fields;
`/manage` authenticated independently and showed the same canonical game state.
The CS2 process stayed running. Password fallback, public deployment hardening,
staff/group administration, and all later functional slices remain separate.

## Match and Practice startup presets (2026-10-26)

The next vertical slice adds finite Host default / Match / Practice presets to
stopped fleet servers and the standalone host CLI. Nondefault modes require an
installed original MatchZy stack. Match selects MatchZy 0.9.2's PUG/warmup
startup, Practice its practice startup. No match record, teams, tournaments,
Workshop maps, or dashboard ready-up/veto/phase controls are added in this slice.

Migration nine persists nullable desired preset, agent acknowledgement and actual
runtime mode, plus machine capability. Null preserves existing local settings;
an explicit empty preset clears the managed mode. Revision-protected stopped-only
updates cannot be combined with Start. The agent retries partial writes, and the
canonical Start action waits for an exact acknowledgement. Older agents receive
Stop for managed preset intent. Player join actions additionally require verified
runtime mode for explicit Match/Practice. Browser state continues over WebSockets.

The first-party ClutchbaseAccess 0.3.2 reporter reads the pinned original plugin's
public runtime fields, bound to fresh verified whitelist watchdog evidence.
Configuration files or a saved preset never prove actual mode. Password-only
servers without whitelist evidence show unknown runtime mode. Simulation confirms
configuration only. Both access policy and mandatory passwords are preserved.
The built reporter DLL and capability markers are staged beside host/agent
binaries; selecting a startup preset copies them into an instance only while
stopped. Live mode requests use a private fixed-schema file and only the MatchZy
Match/Practice commands. The reporter acknowledges the actual mode. It refuses
requests when `matchStarted` is true, and never accepts browser-provided console
commands. No third-party plugins are downloaded.

Validation passed Go race tests, vet, real PostgreSQL integration, frontend
transport tests/type checking/production build, four Go binary builds, and .NET
reporter build/runtime-field tests. The retained isolated remote CS2 instance
started in Practice and then Match, answered A2S, and emitted fresh verified
runtime evidence for each selected mode. It was stopped afterward. These tests
do not verify human practice commands or a completed match. The prototype API and
machine agent were updated; the active prototype and unrelated CS2 processes kept
their original process IDs. Browser inspection showed the running server, one
player, verified whitelist and disabled startup selector. The live instance keeps
Host default; its reporter upgrade is staged beside binaries for stopped setup.
A private database backup was taken before applying migration nine.

## Standalone match rooms (2026-10-26)

A standalone MatchZy room and participation slice adds room creation/closure to
management and join/leave/side/ready state to the Steam player page. Rooms attach only
to a running managed server with verified Match mode and an explicitly saved
whitelist. Players discover only rooms for servers that include their SteamID.
Managers can explicitly load a one-map MatchZy setup from a ready room. The host
agent transports a finite room ID, map, selected sides and player names; the
first-party plugin validates it, writes the local fixed-schema config and runs
only the fixed `matchzy_loadmatch` action. The room stays pending until MatchZy
reports that exact generated config loaded. Player roster mutations lock while
pending or loaded; the original failed-load retry is superseded below. Loading can change the active
map.
Rooms can opt into manual ready, which disables MatchZy's automatic player-count
ready threshold. The setting persists when room settings or the voted map change.
Loading such a room requires the server's current reporter evidence to advertise
manual-ready support; older or offline agents remain blocked. Legacy settings
updates that omit the option preserve its saved value.
Player mutations require a live Steam session and same-origin request. Player
state continues to arrive through the existing WebSocket stream. Migration ten
stores rooms and participant readiness; migration eleven adds temporary side
selection; migration twelve records guarded live mode commands; migration thirteen
adds 1v1–5v5 room capacity; migration fourteen stores player-selected in-game
names. Joins and side changes lock the room row, so concurrent
requests cannot exceed a side's selected capacity. Guarded MatchZy Match/Practice mode switching is also an operator
action when the verified runtime reports no active match. Migration fifteen adds
the setup request state, agent capability and MatchZy runtime acknowledgement.

Players set their own in-game display name from `/play`. It is shown only within
rooms they can already access, and is not represented as a verified Steam name.
After MatchZy confirms a setup loaded, the player match card shows a copyable
`connect` command only when the same canonical server snapshot marks that server
ready for joining. This uses the existing server join gate and does not introduce
a separate client-side server lifecycle state.

## Persistent teams and tournament brackets (2026-11-14)

Players can create a persistent five-player team, invite members with an expiring
code, view the roster, and leave as a non-captain. Only one team per Steam player
is enforced. Admins see team rosters and can create team tournaments from 1v1 to
5v5. Captains register and check in; administrators then generate a seeded
single-elimination bracket from checked-in teams and record each winner. Bye
rounds advance automatically. Team, registration, check-in, bracket and winner
state is stored in PostgreSQL and published through the same canonical live
snapshots as server and match state. Player tournament snapshots expose team
names and statuses, and the member's own team roster only. Admins can open a
fixture as a linked match room on an eligible Match server. The API verifies
each selected SteamID against the server's saved whitelist and seeds the
tournament-sized roster onto CT/T; players then use the existing public
match-room ready-up flow. The selected official map and room link are visible
in the public and management brackets. Each tournament stores a seven-map
pool and can use BO1, BO3 or BO5. A BO1 fixture uses six bans and one side pick.
BO3 uses two bans, two ordered map picks, two bans and a decider; BO5 uses two
bans, four ordered picks and a decider. PostgreSQL checks the acting team's
membership and turn before accepting each veto action. The player page shows
whether the next action is a ban or pick. The fixture map sequence is passed to
the pinned MatchZy setup as one series; map sides alternate across maps.

When Clutchbase accepts the linked room's fenced MatchZy final score, it maps
BO1 results by starting side and BO3/BO5 series totals to the fixture teams,
records the winner and advances the bracket in PostgreSQL. Public and management
brackets show reported series totals under the team names. A tied or unavailable
series result remains pending for an administrator.
Advancement runs after the heartbeat transaction commits, and the machine result
is acknowledged only after both the score and bracket update persist.

This is the first tournament slice, not feature parity: no double elimination,
Swiss/round-robin formats, scheduling, disputes, or captain transfer/disband
workflow yet. Standalone matches continue to work independently. MatchZy
multi-map setup and series scores are based on the pinned 0.9.2 source/config
contract and still need a real multi-map CS2 match verification. Go PostgreSQL
integration tests cover invitation, signup, check-in, two-team completion, and
3/5-team bye advancement.

## MatchZy setup loading (2026-10-26)

Migration fifteen adds a pending/complete/failed load state per room and a finite
agent capability. A manager can load only when both sides have players, everyone
is ready, and the server reports fresh MatchZy 0.9.2 Match mode with no setup
already loaded. The Go API constructs a fixed-schema command from the room's
map, sides, SteamIDs and names; browsers send only the room ID. The agent writes
the request into its private instance config area. The plugin validates the
request, generates a one-map MatchZy config and runs the fixed
`matchzy_loadmatch` command. It acknowledges only when MatchZy reports the
generated file as loaded. Room roster changes are serialized against loading and
locked while pending or loaded. The original retry behavior is superseded by the
failed-load restriction described below. MatchZy loading
may change the live map; the user must explicitly trigger it. No persistent
teams, tournaments, veto controls, or score reporting are included.

## Live match pause control (2026-10-27)

Migration sixteen persists a bounded pause/resume request and the most recently
verified MatchZy phase. An administrator may request pause only when the loaded
MatchZy match is live, or resume when MatchZy reports its pause flag. The agent
uses a private fixed-schema request and the plugin issues only MatchZy's fixed
admin pause or unpause command. It acknowledges after MatchZy's runtime flag
changes; the browser keeps rendering the server snapshot while the request is
pending. MatchZy's pause flag can represent a queued pause, so it is not evidence
that the game engine has already stopped the round. The player page exposes the
reported live or pause-requested state only for servers the player can access.
The real pause and resume path has not yet been exercised in a live match.

## Official match maps and observation (2026-10-27)

Managers can choose an official map when opening a room; omitting the selection
uses the server's observed current map. The room retains that map and the agent
delivers it as a bounded MatchZy setup. Loading the room can change a running
server's map. Updated agents read the actual map from A2S_INFO each heartbeat,
so a live map change or agent restart does not substitute the startup setting.
Unrecognized map names are not projected as supported maps. Migration seventeen
records the machine's map-observation capability; opening or loading rooms now
requires that capability and a current supported map. This prevents an older
agent's remembered startup map from being used as an observed runtime map.
Real MatchZy map switching has not yet been exercised on CS2.

## Match setup snapshots (2026-10-27)

Migration eighteen stores each generated MatchZy setup with its command ID in
PostgreSQL. Machine heartbeats resend that exact map, capacity, SteamID roster,
sides and player names even when profiles change while the request is pending.
Management and player room views display the captured names while the setup is
pending or confirmed. An attempted retry would create a new snapshot. The agent recognizes an
identical replay before rechecking MatchZy availability and rejects a reused ID
with changed data. An invalid stored setup fails that room without blocking the
machine's heartbeat. Upgrading marks older pending requests without snapshots as
failed rather than reconstructing their payload. Further loading after failure is
now blocked pending safe recovery. Older completed rooms have no saved
snapshot and continue to show current profile names. No real CS2 match load has
yet verified this path.

## Closing match rooms (2026-10-27)

Room closure does not end a MatchZy match or stop the game server. The manager
must acknowledge this before closing a room whose setup was confirmed loaded;
the API enforces the same condition and does not allow a room to close while a
load is pending. Room views describe the load acknowledgement as a past event,
not evidence that the match is still running. Automatic detection of naturally completed matches remains future work.


## Ending a loaded match (2026-10-27)

An administrator can request that MatchZy end a Clutchbase-loaded match without
a winner. The button requires fresh verified evidence identifying the exact
loaded setup, a current server revision, and explicit confirmation. The agent
checks the same setup identifier; the reporter issues only the fixed
`get5_endmatch` command and records issuance before execution. PostgreSQL keeps
the request pending until the reporter confirms that MatchZy unloaded the setup;
only then is the matching room closed. An older agent or reporter cannot offer
this action. This path has not yet been tested against a live CS2 server, and
natural match completion is not automatically detected.

## Editing room settings (2026-10-27)

Migration twenty adds a settings revision to match rooms. Before the first load
request, managers can edit a room's title, official map and size from 1v1 to 5v5.
The API locks the room, rejects a stale editor, and prevents shrinking either
side below its joined player count. Map or size changes clear all readiness
checks in the same transaction; title changes preserve them. Editing settings
does not send a game command. The chosen map is applied by the existing explicit
load action. Settings stay locked after any load request, including a failed
load, because an unconfirmed request may still have affected the game.

Player snapshots now include the confirmed end timestamp so players can
distinguish an administrator-ended match from a room that was only closed.

Managers can remove a participant from an open room before any load request.
The operation locks the room and clears the remaining players' ready checks so
they can confirm the changed roster. It does not revoke server access or prevent
the removed player from joining the room again. Management snapshots identify
participants by SteamID; player room snapshots do not disclose those IDs.

New MatchZy setup snapshots include their original request timestamp. The
updated agent passes that same timestamp to the plugin and refuses a request
more than 60 seconds old or more than five seconds in the future; it also
rejects reuse of a command ID with a changed timestamp. This prevents an
updated agent from making a delayed new request fresh again. Earlier snapshots
without timestamps retain their previous behavior for compatibility, and older
agents ignore the new field. Recovery for protected requests is described below.
Keep the application and host clocks synchronized. This path has not been
verified against a live CS2 server.

## Failed load safety (2026-10-27)

Migration twenty-one records whether a machine reports support for setup
expiry and whether a particular new request was made for an expiry-aware agent.
Such a request is not sent to an older agent if the machine later downgrades.
Earlier requests remain marked as legacy because their delivery cannot be
proven to have enforced expiry. A failed load blocks further match loads on
that server, even if its room is closed. Normal room creation and server controls
remain available. Recovery for new fenced requests is described below; legacy
and possibly issued requests remain blocked.

After a timeout, the backend can restore a room to the loaded state if the
running host reports that exact setup as active through verified MatchZy
evidence, even if the earlier result still says it failed. A stale result or
an absent/different active setup does not clear the failure.
Real CS2 late-acknowledgement behavior remains unverified.

ClutchbaseAccess 0.3.4 adds durable per-setup issuance and cancellation markers.
It records issuance before sending `matchzy_loadmatch` and refuses to issue the
same setup again after a plugin reload. A private cancellation request can fence
a setup only when no issuance marker exists and MatchZy is verified unloaded;
an issued or uncertain setup remains blocked. Markers are kept on disk so a
later replay cannot undo the fence.

Migration twenty-two connects this fence to the agent and management UI. A new
request is eligible for cancellation only when it was created with expiry and
fence support, and a fresh host report confirms the updated agent and plugin.
After a manager requests cancellation, the room stays blocked until the same
request and setup IDs are acknowledged and MatchZy reports an idle, loadable
runtime. The room roster is unlocked and remaining ready checks are cleared in
one transaction. If the command may already have been issued, cancellation is
rejected and the block remains. Legacy loads cannot use this recovery path.
The host plugin must remain installed without an out-of-band downgrade while
these requests are in flight. This workflow has not been tested on live CS2.

ClutchbaseAccess 0.3.5 records the supervisor's per-launch nonce in new issuance
markers. It checks that the game is in the supervisor's process group before
using that nonce. A cancellation can fence a previously issued request only
after a different verified supervised launch and while MatchZy is unloaded;
same-launch requests remain blocked. Empty issuance markers from 0.3.4 lack a
launch identity and remain blocked even after restart. Cancellation markers
stay durable across launches, so an old request cannot be reissued by the
updated plugin. Restart remains a manager action and is never automatic. This
path has not been validated in live CS2.

## Prototype host reconnection (2026-10-27)

The previously authorized host at `10.0.0.2` was reachable again. Its agent was
updated and restarted, while the separate running CS2 service kept the same
main process. Its SSH reverse tunnel to the local API was restored; the API
reports the server online with verified access and the observed `de_mirage`
map. ClutchbaseAccess 0.3.5 and the updated host tools are staged beside the
agent. The running game still uses its previous plugin: installing the staged
reporter requires a later stopped-server maintenance action. Match loading,
end and cancellation with the new reporter have not been verified in live CS2.
The SSH tunnel is a control-master connection and must be recreated if it drops
or the workstation restarts.

## Player room actions (2026-10-31)

Player snapshots now include the allowed join, side, ready and leave actions;
the player page uses those server projections and disables them when its live
connection is unavailable. When a player joins, leaves or changes sides before
loading, other ready checks are cleared in the same room transaction. Repeated
joins, leaves or side selections that make no roster change preserve readiness.
This keeps readiness tied to the current lineup without changing server access.

## Staged reporter installation (2026-10-31)

Migration twenty-three adds a stopped-server request to install the staged
first-party Clutchbase Access reporter. The agent reports fingerprints of the
staged and installed bundles; the administrator can request the specific
staged fingerprint only while the game is stopped. PostgreSQL keeps the command
pending and blocks Start and conflicting changes until the agent confirms the
same installed fingerprint. The agent checks the service state and staged bytes
again under its local operation lock, so a changed bundle is rejected. It copies
only fixed local plugin files; browsers supply no host paths or shell commands.
The action updates the on-disk reporter. Runtime verification still happens on
the next game start, and this installation workflow has not been exercised on a
live CS2 host.

On 2026-10-31 the retained local database and API were restarted and the host
agent tunnel reconnected. The server reported running with verified access and
Mirage observed. The tailnet address answered a Source query; the saved LAN
address did not answer while the workstation routed it through Tailscale. No
new player join or live MatchZy test was performed.

The reporter-update API and agent were subsequently deployed to the prototype
after a database backup. The agent reported a staged update while the running
server correctly offered no install action; its game process remained unchanged
during the agent restart. Installation itself remains untested on the host.

## Player directory (2026-10-31)

The administrator snapshot includes Steam IDs and the in-game names of players
who have signed in. Management displays the directory and can add a known player
to a server roster draft without copying the ID. Saving still uses the existing
revision check and stopped-server whitelist rules; signing in alone does not
grant access. Player snapshots do not expose the directory. Name changes and
new sign-ins reach management through the existing live notifications.
The updated API and frontend were deployed to the local prototype; its snapshot
showed the signed-in player while the game remained running.

## Match room visibility (2026-10-31)

Management and player snapshots retain every open room, pending or failed load,
and confirmed active setup, plus the 50 most recent other rooms. This prevents
older actionable rooms from disappearing as match history grows. Player history
is selected only from servers whose current roster permits that player.

## Manual match results (2026-10-31)

Migration twenty-four stores an optional manually entered final score for each
team's starting side. An administrator can record, correct or clear it on a
closed room whose MatchZy setup was confirmed loaded. A result revision prevents
stale edits. Manager and authorized player snapshots show the score as manual;
it does not end a game or assert that MatchZy reported a winner. Natural match
completion, automatically collected scores, full history and statistics remain
unimplemented. Recording a result while a game is active only changes the record.
The migration and dashboard were deployed to the local prototype after a private
database backup; the game remained running. No live result was entered.

## Live match history (2026-10-31)

Migration twenty-five indexes closed rooms with confirmed loaded setups. A
separate paginated WebSocket view shows these rooms, their captured participants
and any manual score. Management can correct scores on retained servers even
after the room leaves the recent snapshot; records from removed servers remain
visible to management without editing. Player history follows the current
server-roster access rules and excludes removed servers. Pagination uses the
creation timestamp and room ID together; each page updates live on changes.
Automatic results, demo processing and player statistics are still unimplemented.
The history view and migration were deployed to the local prototype after a
private database backup; the game remained running. The browser was signed out,
so the history interface has not yet been checked there with real match records.

## MatchZy readiness for small rooms (2026-10-31)

ClutchbaseAccess 0.3.6 derives both `players_per_team` and
`min_players_to_ready` from the smaller room side when writing a MatchZy setup.
This addresses 1v1 and other partially filled rooms: the pinned MatchZy 0.9.2
uses `players_per_team` for ordinary ready checks and `min_players_to_ready`
for forced or join-based readiness. See the pinned [ready logic](https://github.com/shobhit-pathak/MatchZy/blob/0.9.2/ReadySystem.cs).
For uneven sides the larger side can satisfy readiness before every rostered
player joins; both dashboards explain that limitation. The room's web readiness
gates loading but does not itself set MatchZy's per-player readiness.

The new reporter was built and staged beside the host tools, with the previous
staged file retained as a rollback copy. The dashboard reports a staged update;
the running CS2 process was not interrupted and still uses its previous plugin.
The new behavior requires installing the staged reporter while the server is
stopped and has not been tested in live CS2. MatchZy's team-capacity behavior
with unequal rosters also needs live verification.

## Machine storage capacity (2026-10-31)

Migration twenty-six adds an optional machine filesystem capacity report. A
managed agent reads total space and bytes available to its user at its own
storage root. Management shows this only for a fresh online report; older agents,
inaccessible filesystems and disconnected machines do not show a stale value.
Simulation does not report the machine's real storage. This is an indication,
not an install guarantee: per-instance mounts, quotas, downloads and other
processes can change available capacity. No host path is sent to the browser.
After a private database backup, the API and host agent were updated. The real
host reported capacity in the management snapshot, and the CS2 service stayed
running with the same process during the agent restart.

## Unloaded match setup visibility (2026-10-31)

If an open room previously loaded successfully and a fresh verified MatchZy
report says the server is idle with no setup loaded, manager and player snapshots
show that fact. Managers can close the room and enter a manual result if known.
The projection disappears when evidence is stale, unavailable, or conflicts with
an active or pending operation. It does not close the room automatically or
claim a normal finish, since a reset or another external action can also unload
the setup. Automatic completion and score collection remain future work.
The API and dashboard were updated on the local prototype without restarting
CS2. The prototype had no room to exercise this observation live.

## Manual player scoreboard (2026-10-31)

Migration twenty-seven adds optional manually entered kills, deaths and assists
for participants in closed rooms with confirmed loaded setups. Managers can
enter, correct or clear each player's line from recent rooms or paginated
history. The room's result revision serializes scoreboard and score edits;
stale changes are rejected. Management and authorized player views receive the
same labeled manual data through their live snapshots. A missing line is not a
zero. This does not collect game stats or prove a match was played; MatchZy's
automatic event ingestion and complete scoreboards remain future work.
The local prototype was updated after a private database backup while CS2 stayed
running. No real player stats were entered or verified in a browser.

## Automatic final scores (2026-10-31)

Migration twenty-eight stores a reported final score separately from manual
corrections. ClutchbaseAccess 0.3.7 observes the pinned MatchZy 0.9.2 runtime:
it must first see the exact fenced setup live, then see both MatchZy's
`currentMapFinished` and `seriesEnded` flags while the same setup and verified
whitelist are active. It maps the current CS2 team scores back to the starting
teams and writes a private durable record. This relies on the pinned MatchZy
runtime layout; unknown versions or missing evidence produce no result.

The machine agent retries each retained record until PostgreSQL accepts the same
setup and score, then keeps a local acknowledgement. The API accepts only an
owning machine's fenced setup, stores the first score idempotently, closes the
room, and leaves any manual result untouched and separately labeled. An
unacknowledged result blocks instance retirement, but never blocks ordinary
start/stop reconciliation. The reporter can miss a result if the process exits
or evidence is lost before it observes the completed state; it does not claim
coverage for forced ends, forfeits, or player statistics. This path has not yet
been verified with a real CS2 match.
After a private database backup, the API and host agent were updated and the
0.3.7 reporter was staged with a retained rollback copy. The running CS2 process
was unchanged, and its snapshot still reports verified access. The staged
reporter must be installed on a stopped server before it can capture results.

## Reported player statistics (2026-10-31)

Migration twenty-nine stores reported K/D/A independently from manual lines.
ClutchbaseAccess 0.3.8 reads the same CS2 `MatchStats` counters used by the
pinned MatchZy 0.9.2 implementation when it captures a verified final score. It
includes only valid rostered players available to the server at that moment;
disconnected players can be absent, and missing stats do not mean zero. The UI
labels reported and manual lines separately. This is a partial game snapshot,
not a complete record of all participants.

The result journal uses an on-disk format that older agents cannot mistake for a
score-only record. Updated agents wait for explicit API capability before
sending player stats, reset that capability after connection failures, and
retain records until an exact full-payload acknowledgement. The API validates
roster membership, rejects conflicting retries, and accepts older score-only
retries. These paths have automated coverage but have not been exercised during
a real CS2 match; player counters can be absent or affected by game resets.
The API and host agent were updated after a private database backup, and the
0.3.8 reporter was staged with a rollback copy. The CS2 process remained
running; installing the reporter still requires stopped-server maintenance.

## Public navigation and match eligibility (2026-11-12)

The public sidebar switches between servers, matches, match history and profile;
management is separately authenticated and has its own servers, matches, history,
and players views. URL fragments preserve the selected view across reloads and
browser navigation. These views share the live
server snapshot, and connection loss disables mutation controls. Management now
receives each server's match-room creation action from the API, including whether
an open room already exists. An ordinary action error no longer disables all
management controls while the live connection remains healthy.

## Real CS2 smoke test (2026-11-12)

On byttorrent, the retained prototype was started with its existing host tool.
The current A2S parser read a real 0/64 CS2 response, and fresh runtime evidence
reported a verified whitelist and Match mode. The isolated instance was stopped
afterward; the two unrelated CS2 processes were unchanged. This did not exercise
the dashboard, match loading, a human connection, or staged reporter installation.
See [REMOTE_TEST.md](REMOTE_TEST.md).

## Real MatchZy setup and map switch (2026-11-12)

With a private prototype backup, the current API and machine agent controlled
real CS2 on byttorrent. A test room with two temporary rostered sessions became
ready; MatchZy acknowledged its Nuke setup, and the agent observed the live map
change from Mirage to Nuke while access remained verified. The API Stop request
converged to stopped and unrelated CS2 processes were preserved. Temporary test
data and services were removed. No human match, pause/resume or automatic result
was exercised. See [REMOTE_TEST.md](REMOTE_TEST.md).

## Byttorrent stack for testing (2026-11-12)

A Docker Compose stack was started directly on byttorrent from the workspace
snapshot staged under `~/.local/share/clutchbase-stack`. It publishes the app on
host port 8888 with an independent PostgreSQL volume initialized from
a private backup of the former local prototype. The old local database is not
replicated; subsequent changes belong to the new database. The administrator
credential was retained; database and machine-agent credentials remain separate.
The machine agent is an enabled user service, using the existing prototype root.
The isolated game was started through the API and reached running/verified Match
mode on Mirage. Two unrelated CS2 processes were unchanged.

The configured browser origin is `https://cs3.krychlic.com`; Steam's
callback and logout validation use that origin. Tailscale can reach the published
port 8888 through the host network interfaces. The Docker
Compose stack was started directly because the new Komodo stack entry's file-mode
configuration could not be saved through browser automation; that Komodo entry
is not yet managing the running stack. The dashboard's registered game address
remains its prior LAN address. At setup, a UDP Source query from the workstation
reached port 27245 through the host's Tailscale IP but not its LAN address; this
does not prove a player can join. Real Steam sign-in through the new endpoint and
human play remain to be tested. The configured HTTPS origin needs a reverse proxy
on port 443 to the app's HTTP host port 8888, with WebSocket upgrades enabled. See
`deploy/compose.komodo.yaml` for the staged
Compose definition; its paths and initial database restore file are host-specific.

## UI usability and parity target (2026-11-14)

The management and player server lists now prioritize server name, canonical
status, map/player count, connection address, and the server-supplied primary
action. The management sign-in screen identifies `CLUTCHBASE_ADMIN_TOKEN`, tells
Komodo users to copy it from the stack's Environment settings and distinguishes
it from the machine-agent token. Management can search servers by name, address, map or machine and filter
by the current canonical status. Fleet overview highlights offline/error
servers, failed updates, blocked access verification and disconnected machine
agents, with a direct filter to affected servers. These local view filters do not alter server
state. Management match cards explain why loading is unavailable using current
server, runtime, map, player and ready-check snapshots. Public match cards show a next-step hint derived from the server's
available join/ready actions and setup state. Join CT/T controls sit inside
their lineup panels; a joined player gets switch/ready/leave controls inside
their own side. Both public and management match
pages default to open rooms and can filter to closed rooms or all rooms; recorded
results remain on the separate history page. Open match cards show CT and T
lineups in separate panels with readiness,
side capacity and open slots on both public and management pages. These panels
render the canonical match snapshot; they do not assign sides or mark players
ready locally. Tournament cards span the content width so fixtures have room for
round columns and veto controls in both pages; narrow screens scroll the bracket
horizontally. Round headings include their match counts. Fixture badges show
waiting, veto, live, result-resolution, bye or completed state from the server
snapshot, and completed winners are highlighted. Veto actions remain attached to
the canonical fixture. Administrators now choose a team and
explicitly confirm before changing bracket advancement. Map presets, access
controls, plugin maintenance, and logs are grouped under each server's settings
disclosure. An available CS2 update action sits beside Start; detailed status
and the first-party Clutchbase Access plugin update are in settings. The UI shows
agent-reported pending/completed/failed state and offers actions only when the
canonical snapshot allows them. Join passwords remain available to
administrators through the authenticated in-app reveal flow.

The user expanded the product target from friend-server workflows to functional
breadth comparable to Auto Tournament, Ready Up and CSM. `docs/FEATURES.md` tracks
that target and the remaining gaps. In particular, broader plugin version management, Workshop maps, actual in-game
ready-up/veto, tournament/team flows, demos/replays, highlights, ratings
and broader matchmaking remain incomplete. React/CSS changes from this pass are
local and have not been deployed to byttorrent. The production deployment
therefore will not show these interface changes until its stack is updated.
Frontend build passes; browser-host visual verification was unavailable in this
session.

## In-game join readiness (2026-11-14)

ClutchbaseAccess 0.3.11 includes `matchzy_ready_mode` and `mp_overtime_enable` in each generated setup's
fixed `cvars` object. Equal CT/T rosters use join-based readiness: joining the
assigned side readies a player, and MatchZy starts after both full sides have
joined. Uneven rosters keep manual `.ready`; the lower `min_players_to_ready`
threshold could otherwise let the smaller side start while players are still
joining the larger side. The management and player pages give the instruction
that matches the roster sizes.

This uses the original MatchZy 0.9.2 setting and does not add arbitrary console
commands or mark a player ready optimistically. The plugin builder assertion
checks emitted config for equal and uneven rosters. This change is not installed
on byttorrent and has not been verified in a real CS2 match.

## Configurable match overtime (2026-11-14)

Standalone rooms now store an explicit overtime choice in PostgreSQL, include it
in admin and player snapshots, and carry it through the fenced setup request to
the first-party plugin. New rooms default to off. Admins can change the setting
before loading; changing overtime does not clear player readiness. The plugin
emits only the fixed `mp_overtime_enable` cvar with `0` or `1`, alongside the
existing ready-mode cvar. Migration thirty-four adds the room setting. This
change has not been deployed on byttorrent or verified in a live CS2 match.

## Management control room (2026-11-14)

Management now opens on an overview built from the current canonical WebSocket
snapshot: running server count, active matches, fleet attention, tournaments,
recent open rooms and connected machine agents. Each summary links to its
existing management page; it does not keep or infer a separate server lifecycle.
The public player pages remain in their sidebar as requested. Match lineups
show CT/T ready counts derived from the participants in the canonical snapshot.
Direct room links can open closed rooms from history. The overview interaction,
readiness counts, and direct links are covered by frontend tests and production
build. Browser visual verification and deployment to byttorrent remain
outstanding.

## Tournament stage visibility (2026-11-14)

Admin and player tournament cards now show registration, check-in and bracket
stages, with snapshot-derived progress counts and the current next step. Player
check-in continues to display the registered state until PostgreSQL publishes
the updated canonical snapshot. BO3/BO5 formats now include a server-authority
veto sequence and native MatchZy multi-map setup. The host agent and Clutchbase
Access reporter advertise series support before management offers those servers.
Scheduling, disputes and real multi-map CS2 verification remain open.

## CS2 update availability (2026-11-14)

The managed host agent checks SteamCMD public app info for app 730 outside the
five-second heartbeat, at most once per hour per machine. It compares the public
branch build ID with each instance's local Steam manifest and reports both IDs
and the observation time. PostgreSQL stores the observation; the server projects
`current`, `outdated`, or `unknown` into snapshots. Lookup failures, missing
manifests, stale or offline agents, and active installs yield `unknown`. The
existing Update game action remains stopped-only and requires an administrator;
update detection never starts, stops, or restarts a game. Steam's public branch
metadata can lag release availability, and real CS2 compatibility still requires
a live host test.

## Tournament room readiness (2026-11-14)

Tournament fixture rooms accept only their assigned team roster. The player
snapshot suppresses join, leave, and side-switch actions for those rooms, while
assigned players can still ready and unready before loading. The join endpoint
also rejects other whitelisted players, including when a side has space. The
database-backed tournament flow covers these restrictions and readiness.

## Machine credential recovery (2026-11-14)

Administrators can rotate a machine-agent credential. The replacement is shown
once and only its hash is stored. Rotation immediately makes the old token
invalid and clears the machine and server heartbeat freshness in the same
transaction; an in-flight heartbeat rechecks its credential while holding the
machine lock. Restart the agent with the replacement token to reconnect. Running
CS2 processes are not stopped by the rotation. Requests already committed before
rotation can still be delivered to an agent; stop an untrusted agent separately.

The staged reporter action is also available on a stopped managed server when
the installed reporter is missing. The agent still verifies the staged bundle,
game installation, plugin prerequisites and resulting installed fingerprint.

## Standalone reporter maintenance (2026-11-14)

The host CLI can inspect staged and installed Clutchbase Access fingerprints with
`reporter NAME` and install the staged reporter with `reporter --install NAME`.
The install uses the existing agent lease, stopped-server check, operation lock and
staged fingerprint verification. It can restore a missing reporter while the
remaining required plugin stack is present. The reported fingerprints describe
files on disk; runtime verification remains separate. The dashboard also offers
recovery when a staged reporter is valid and the installed reporter is missing.
The standalone path and recovery were checked with host and manager Go tests; they
have not been exercised on a live CS2 host.

## Match and fleet interaction updates (2026-11-14)

Public server cards link directly to their open room; management server cards
link back to the active room. Match filters no longer reset from live snapshots
while a player is using them. Management can replace a machine agent token after
confirmation; the replacement appears once, and the operator must restart that
agent with it. Match cards use responsive widths, larger readable CT/T lineups,
snapshot-derived setup status and a prominent server-authorized Load action or
its preflight guidance. Player next-step copy reflects canonical loading, loaded
and failure states. Public navigation stays in the sidebar. All room state and
offered actions still come from the server snapshot. Frontend build and tests
pass; browser visual verification remains unavailable in this environment.

## Match control setup fencing (2026-11-15)

Pause and resume requests now capture the active MatchZy setup ID. The API
delivers them only while an updated agent and Clutchbase Access 0.3.12 report
support and the same setup remains active. The agent checks the ID before writing
the request; the plugin checks it again immediately before issuing the command
and before confirming completion. A changed setup fails the pending request.
Older agents or reporters do not offer pause or resume. Existing in-flight
unfenced commands may already have reached an older plugin when upgrading.
Player pause controls are not yet implemented. This behavior has not been
verified in a live CS2 match.

## Live player statistics (2026-11-15)

Management has a **Statistics** page and signed-in players have **My statistics**
in the public sidebar. The shared responsive page consumes canonical `stats` and
`player-stats` live snapshots and handles connecting, reconnecting and expired
sessions. The API limits public results to the player's own statistics from
currently permitted, nonremoved servers; management sees retained results.
Manual K/D/A and single-map score corrections take precedence. Series W/L use
reported series map wins only when they prove a completed series; unresolved
series remain outside scored coverage. Admin summary counts are player
appearances and outcomes, not distinct matches. Coverage is scored rooms/rooms
and stats rooms/rooms. Reported K/D/A may be partial or final-map-only; null values
are unavailable, not zero. These totals are not Elo or a rating. Completeness is
limited to the retained server result.

## Round-robin tournaments and scorecards (2026-11-15)

Migration thirty-eight adds a tournament format, defaulting existing tournaments
to single elimination. Management can also choose round robin. Its circle
schedule pairs every checked-in team once and gives a team at most one fixture
per round; an odd team count has rest rounds without artificial wins. Rooms
open one round at a time. Each fixture retains the existing BO1/BO3/BO5 veto,
room, roster and result workflow. Automatic reported results and manual
resolution both update the same standings. The tournament completes after all
fixtures finish. Rank uses fixture wins; equal wins share a rank, and tied first
place leaves the unique champion unset. There is no playoff or head-to-head
tiebreaker yet. Both public and management pages render server-supplied
standings; public navigation remains in the sidebar.

History now uses match cards with a player scoreboard table, separate reported
and manual K/D/A columns, and the existing authorized score editors. These are
local changes, not deployed to byttorrent. Browser visual verification and real
CS2 round-robin play remain outstanding.

## Player match pause requests (2026-11-15)

Rostered players can request pause or resume from an open, confirmed loaded
match room. The player must still belong to that room and its server whitelist.
The endpoint checks same-origin player authentication, the server revision,
fresh managed runtime evidence, current action capability and that the room's
setup ID is still active. It uses the existing setup-fenced control path.
Whitelisted spectators, old rooms and stale snapshots cannot control another
match. Each request is serialized with management commands; pending commands
suppress further controls. The UI waits for canonical snapshots and distinguishes
a request from the observed MatchZy phase. All rostered players may request both
actions; team votes, captain-only policy and per-team pause budgets are not
implemented. Live CS2 and visual browser verification remain outstanding.

## Team roster management (2026-11-15)

Migration thirty-nine adds team revisions and archival disbanding. Captains can
remove another member, transfer captaincy to another current member, or disband
a team outside unfinished tournament registrations. Confirmation controls render
server-supplied actions, send the snapshot revision and wait for the canonical
roster update. A changed revision disables an open confirmation. Old captains
lose permission immediately after transfer. Captain roster edits and transfers
revoke outstanding invites; displayed invite codes clear on team/revision or
session changes. Disbanding releases memberships and retains the team row so
historical fixture names and winners survive.

Checked-in teams cannot change their roster or captain until that tournament
finishes; existing invites and the leave endpoint enforce the same restriction.
Registration without check-in still allows roster repairs, but prevents disbanding.
Captains can withdraw registration before fixtures start. Team row locks serialize joins,
invites and edits, with eligibility rechecked after acquiring the lock. Concurrent
old-revision captain edits can commit only one change. Database and frontend
tests cover these policies; these local changes have not been deployed or
visually checked in a browser.

## Tournament withdrawal (2026-11-15)

Captains can withdraw a registered or checked-in team while the tournament is
open or checking in, before fixtures exist. The confirmation identifies the
current team revision, registration status and phase; a changed team, phase or entry rejects
the request. It explains that closed registration prevents rejoining. The UI
keeps showing the entry until the committed snapshot removes it. Withdrawal
bumps the team revision and releases only that tournament's roster lock; other
checked-in registrations continue locking the team. Registration, check-in,
withdrawal and fixture generation lock the tournament before the team, and
counts are read after locks are acquired. A withdrawal racing with seeding
either removes the entrant before generation or is rejected after generation.
Active fixtures and historical results are never removed by this action.
Database race and UI tests cover these rules. Deployment and browser visual
verification remain outstanding.

## Machine CPU and memory visibility (2026-11-15)

Migration forty stores optional machine CPU and memory readings. Deploy the
API before the updated agent; older API versions reject unknown report fields. The managed
agent samples Linux `/proc/stat` counters and `MemTotal`/`MemAvailable`; CPU
uses a delta between successive samples, excluding duplicate guest counters.
The first, reset, malformed or excessively old CPU sample remains unknown.
Missing memory evidence yields no resource report; simulation never invents
telemetry. Heartbeats validate bounds, PostgreSQL retains the observation,
and offline snapshots clear resources. Agents without the field clear prior
readings rather than leaving stale capacity visible.

Management renders CPU and memory meters on the overview, machine list and
selected machine in the create-server dialog. Browser connection loss labels
retained values as the last received measurements. These machine-wide kernel
readings do not represent per-server usage or cgroup limits, do not predict
player capacity, and never start/stop games or block lifecycle actions. There
is no telemetry history or alerting yet. Local parser, protocol, database and UI
tests cover sampling, validation, freshness and unknown values. Deployment and
browser visual verification remain outstanding.

## Live current-map score (2026-11-15)

Clutchbase Access 0.3.13 reports current-map CT/T scores only during verified
live or paused MatchZy setups, with a setup ID, map and observation timestamp.
Finished maps, warmup, unknown layouts and unavailable team entities yield no
score. The agent requires the score to belong to the same fresh evidence and
match its independently queried map. Migration forty-one stores the observation
on the host. Both player and management room snapshots expose it only for the
same confirmed loaded setup, matching current map, fresh observation and valid
runtime state. Scores expire after fifteen seconds through the WebSocket timer
even if no database write occurs. Older reporters clear previous observations.

Room cards label these as current in-game CT/T sides, which can differ from
starting-side lineups after side swaps. Browser disconnection labels the last
received score. Live observations never close a room, persist a final score or
advance a tournament. Deploy the API before the agent and reporter. Parser,
database, WebSocket and UI tests cover identity, map, freshness, zero scores and
public access. Live CS2 verification, deployment and browser visual checks remain
outstanding. The reporter compiles against the pinned CounterStrikeSharp 1.0.378
API and its C# runtime-evidence and match-fence checks pass in the .NET 10 SDK.

## Prestart tournament cancellation (2026-11-15)

Migration forty-two adds cancelled tournaments. Administrators can cancel
registration or check-in only while no fixtures exist, after confirming the
current phase. The action shares the tournament lock with seeding; either
cancellation wins before generation, or it is rejected once fixtures start.
Registration history and team names remain stored. Cancelled tournaments stop
locking team rosters; other unfinished registrations retain their own locks.
Player snapshots offer no signup/check-in/withdrawal/veto action for cancelled
events, and progress explicitly shows cancellation instead of a completed
bracket. Games and active tournaments cannot be cancelled by this endpoint.
Database race tests and frontend confirmation tests cover the behavior. These
changes are local; deployment and browser visual verification remain open.

## Tournament navigation (2026-11-15)

Public and management tournament pages show a searchable list instead of expanding
every bracket. Current events are the default; completed and cancelled events
remain available through status filters. Search includes tournament and entrant
team names. Opening an event renders only its detail, with a shareable
`#tournaments?tournament=ID` route. Returning preserves the filters and restores
keyboard focus. Unknown or no-longer-visible IDs show an unavailable message.
Public navigation stays in the sidebar. Filters and routes select what is shown;
all event state and permitted actions still come from canonical snapshots.

Frontend interaction tests cover filtering, direct links, focus and snapshot-based
actions. Deployment and browser visual verification remain outstanding.

## Sign-in return navigation (2026-11-15)

Migration forty-three stores a public return destination alongside each hashed,
one-use Steam login state. Sign-in links carry the selected page and room or
tournament ID, so shared links work after authentication. Only known public
fragments and bounded identifiers are accepted; external URLs, management
paths, duplicate destinations and malformed query parameters fall back to
`/play`. The fixed configured origin remains authoritative. Callback query
parameters cannot replace the saved destination, and the destination is consumed
in the same transaction that creates the session. Existing pending logins retain
the default `/play` return path. This adds navigation, not access: room and event
permissions still come from the authenticated snapshots. Database race tests and
frontend tests pass; real Steam/browser round-trip verification remains open.

## Server fleet navigation (2026-11-15)

Management shows a compact searchable fleet and opens individual server details
at `#servers?server=ID`. Setup, player access, updates and logs have separate
sections. Returning to the fleet preserves filters and restores keyboard focus;
missing servers show an unavailable state. Outdated CS2 builds appear in Needs
attention. Controls and status continue to use server snapshots, including
revision guards and disabled actions during disconnection. Frontend interaction
tests cover navigation, stale roster edits, logs and connection loss. Browser
visual verification and deployment remain outstanding.

## Server restart foundation (2026-11-15)

The agent has a tested request-ID-fenced restart transition model and private,
atomically replaced, synced state-file helpers. A stopped
acknowledgement must precede start authorization, and cancellation prevents
further advancement. Per-instance file locking now serializes competing local
request IDs, and repeated begins for a completed ID return the stored result.
Migration forty-five adds a constrained PostgreSQL restart request record per
server; migration forty-six stores the machine's restart capability. The managed
runner now handles fenced stop, start and cancellation commands,
reports persisted phases after process recovery, and refuses an ordinary running
desired state while a restart is unresolved. Start requires the same stopped
request ID and an explicit start command. Updated managed fleet agents advertise
restart support; management confirms the request and shows the server-reported
phase, with a cancel action. Deploy the API before the updated agent because old
APIs reject the new capability field. Actual CS2 restart and browser visual
verification remain outstanding.
Cancellation is persisted even when it arrives before the stop command; a
delayed command with that request ID cannot advance the cancelled request. The
agent also requests a stop on cancellation, including after start was issued.
The heartbeat transaction now checks matching request IDs and
observed stopped/running states before advancing a restart, and keeps a
cancellation stopped. An administrator's stop request now marks any pending
restart as cancelling under the same host/request locks; conflicting edits are
rejected. Capable managed agents receive only the matching fixed restart phase;
older or downgraded agents receive a stopped desired state while a request is
unresolved. A revision-guarded administrator endpoint records stop intent only
for a fresh capable machine without a conflicting room or maintenance. Local
tests cover API acknowledgement fencing, downgrade behavior, agent recovery,
duplicate starts, cancellation and dashboard snapshot handling. They use a fake
service; they do not establish real CS2 compatibility.
An opt-in process integration test builds the agent and drives a restart through
the HTTP API and PostgreSQL, with fake service controls and game-query replies.
Run it with `CLUTCHBASE_RESTART_PROCESS_TEST=1` and `TEST_DATABASE_URL` set;
it does not test a real CS2 process or the deployed host.
After an agent process restart, a persisted stopped acknowledgement is withheld
until the current whitelist and preset have been re-applied. If a machine is
downgraded to an agent without restart support while a request is unresolved,
the request remains paused and the dashboard asks for a capable agent to return;
an old agent cannot confirm cancellation or authorize a new start.
If the pre-start checks fail after the stopped acknowledgement, the API sends
an explicit cancellation before releasing the agent's stopped fence. Failure
text states that a stop was requested until the host reports its actual state.

## Tournament fixture scheduling (2026-11-15)

Migration forty-four stores an optional scheduled time and schedule revision on
each fixture. Administrators can set, change or clear it while the tournament is
active and the fixture is pending. Changes lock tournament and fixture rows and
reject stale revisions. Public and management brackets show the committed time
in the viewer's local timezone; the editor waits for a new snapshot and disables
stale or disconnected edits. Times are informational and never start a server
or match. Tests cover authorization, validation, stale edits, public projection
and frontend snapshot behavior. Deployment and visual browser checks remain open.
The public and management sidebars also expose a schedule across active
tournaments, ordered by the committed fixture times. Pending fixtures remain
listed even after their scheduled time has passed; the clock does not infer
match completion. The list labels a disconnected snapshot and links back to
the tournament. Steam sign-in preserves the public schedule route.

## Match room discovery (2026-11-15)

Public and management match pages search the current snapshot by room, server,
map and player name alongside the existing open/closed filter. Search persists
across snapshot updates; a linked room remains visible even when filtered out.
The public navigation remains in the sidebar. These changes are local and need
browser visual verification and deployment.

## Map performance (2026-11-15)

Statistics include per-player map breakdowns for closed, confirmed-loaded
single-map rooms and manually recorded series map scores. Single-map results use
manual-score precedence and retain unscored rooms in coverage. Series maps are
counted only when manually recorded; rooms without any counted map appearance
are shown as excluded. Player views retain current server-whitelist filtering;
management sees retained history.
Database integration and frontend tests cover exclusions, access and display.
Automatic per-map series results, real CS2 verification and deployment remain outstanding.
The summary and map breakdown are read within one repeatable-read, read-only
transaction so concurrent result or whitelist changes cannot produce inconsistent
counts within a single statistics snapshot.

## Shareable team invitations (2026-11-15)

Captains can copy an invitation link alongside the existing invite code. The
recipient sees a prefilled join form and must submit it explicitly. The code is
kept in the URL fragment and, across Steam sign-in, in the same tab's session
storage; it is sent to the join endpoint only on submission. Successful joins
clear the stored code. Existing expiry, revision and roster checks remain
authoritative. Frontend tests cover sign-in preservation and explicit joining.
Real Steam and browser visual checks and deployment remain outstanding.

## Wins leaderboard (2026-11-17)

Management and player sidebars expose a live leaderboard ranked by recorded
wins, with equal wins sharing a rank. Player results include only closed,
confirmed-loaded rooms on servers they can currently access; administrators
also see retained history on removed servers. Unscored rooms count toward room
coverage but cannot place a player. Manual single-map corrections and confirmed
series outcomes follow the existing statistics rules. The table is limited to
200 players and does not claim to measure skill. Access changes and session
expiry update the live view. Browser visual checks and deployment remain open.

Changing a standalone room's overtime setting now clears its participants' ready
checks in the same transaction as the settings update, just like a map or size
change. A title-only edit continues to preserve readiness.

## Match reports (2026-11-17)

Migration forty-seven adds private reports for match participants. A player
currently permitted on the server can submit one report per match, and can
follow its open/resolved status in the sidebar even if server access is later
revoked. Administrators see the reporter identity and can resolve or reopen a
report with a revision check. The current report action comes from the player
snapshot; submissions and reviews update the live views after commit. Reports
do not apply penalties or change results. Views show at most 100 reports with
open reports first. Visual browser verification and deployment remain open.

## Standalone match map votes (2026-11-17)

Migration forty-eight stores one official-map vote per participant in an open
standalone room. Votes are private except for anonymous tallies and the voter's
own choice; departing participants' votes are deleted and votes from players no
longer on the server roster are excluded. Voting ends on load or closure and is
unavailable in tournament rooms. Management can select a voted map using the
existing revision-guarded settings action, which clears readiness on a map
change. Votes never automatically change the room or running game. Browser
visual verification, deployment and real CS2 testing remain open.

## Player update guidance (2026-11-17)

Player server and room cards now show the host's fresh CS2 build mismatch as an
advisory warning. They do not expose installed or latest build IDs and do not
block connection attempts, because the player's client build is unknown. A
server-side live timer expires the observation after two hours even without a
heartbeat; disconnected clients label retained warnings as last reported.
The public `cs3.krychlic.com` page and health endpoint responded during local
verification, but that does not identify the deployed revision or verify a CS2
connection. SSH key authentication to byttorrent was unavailable. This change
has not been deployed or visually verified.

## Maintenance exclusion (2026-11-17)

While a CS2 or Clutchbase Access update is pending, the server snapshot and
API reject a second update, starting, removal, and map, roster or preset edits.
The maintenance handlers and other host mutations lock the host before reading
related request state, so a request that waited for a concurrent update sees its
committed status. Existing matching agent acknowledgements still determine when
maintenance completes. Database tests exercise the waiting-lock race; this does
not establish real CS2 update compatibility. Deployment remains outstanding.

## Stop and update CS2 (2026-11-17)

Management can confirm a stop-and-update request for a running capable managed
server when no room or unresolved match command conflicts. The API records the
stopped desired state and the pending update atomically under the host lock. It
does not send the existing update command until the agent reports stopped, and
does not restart automatically. A disconnected or downgraded agent leaves the
request pending; a failed update remains stopped and can be retried. Confirmation
is revision guarded. Migration forty-nine records when an update command is
issued. An administrator can cancel a queued update before issuance, including
while the agent is offline; the stopped desired state remains unchanged. A
heartbeat and cancellation serialize on the host lock, and issuance is committed
before the response is sent. Previously pending requests are conservatively
marked issued on migration. Once issued, an update must finish or fail. The
process integration test uses fake service controls and SteamCMD;
deployment and real CS2 update verification remain outstanding.

## Standalone room shuffle (2026-11-17)

Management can randomly redistribute two to ten participants into sides whose
sizes differ by at most one, before a standalone room is loaded. The confirmation
captures the settings revision and each participant's side and ready status;
the API locks the room, rechecks the complete roster, and rejects stale requests.
The committed shuffle clears all ready checks and leaves map votes intact.
Tournament fixture rosters remain fixed. This balances numbers, not skill.
Browser visual verification, deployment and real CS2 testing remain open.

## Player-created rooms (2026-11-17)

Migration fifty adds a per-managed-server opt-in, disabled by default, for
rostered Steam players to open standalone match rooms. Management changes it
with the host revision. A player creation request checks the permission and
current roster under the host lock, requires the roster to be acknowledged by
the agent, checks the server and match conditions, then creates the room and joins the creator's selected side in
one transaction. The public snapshot supplies whether the action is available.
Disabling the setting prevents new player-created rooms, not existing rooms.
Visual browser verification, deployment and real CS2 testing remain open.

Migration fifty-one records the creator of new player-created rooms. While the
server opt-in and roster access remain valid and the creator is still in the
room, that creator can load a fully ready standalone match using the existing
MatchZy safety checks, or close it before loading. Disabling the opt-in revokes
these controls; management retains access. Older rooms have no inferred creator.
The player UI waits for committed snapshots rather than assuming load success.
Room creators can apply a voted map directly from the tally using the same
revision-checked settings action; changing the map clears readiness.
Creators can also edit their room's name, map, size and overtime before loading.
The API rechecks permission under the host and room locks and rejects stale
settings revisions. Rule changes clear readiness. Visual and real CS2 checks
remain outstanding.

Creators can also shuffle two or more participants in their own standalone
rooms. The server sends an opaque confirmation for the displayed roster and
rechecks it together with the room revision and creator permission under lock.
The shuffle balances player counts randomly and clears ready checks; it does
not disclose participants' Steam IDs to the public page or balance by skill.
The same creator can remove another participant from an unstarted room after
confirming the current roster. Removal clears remaining ready checks and that
participant's map vote, but does not revoke server access or prevent rejoining.
The server rechecks room ownership, server opt-in and roster under lock; public
snapshots expose opaque removal tokens only to the eligible creator.

Migration fifty-two fixes team and tournament notifications to carry the database
schema expected by WebSocket listeners. Team and tournament changes now reach
both player and management streams immediately after commit.

## Pause voting, tournament disputes and plugin inventory (2026-11-23)

Open match rooms store a pause-vote threshold from one to ten. Players who are
joined to the active setup can each cast one vote. The vote count and whether
the signed-in player has voted come from the canonical snapshot. Reaching the
threshold queues the existing setup-fenced pause command; it does not mark the
match paused until the reporter confirms the observed phase. Administrators can
still pause directly and can change the threshold with the server revision.
Votes clear when the active setup or observed phase changes, a pause request is
issued, or the threshold changes. Live CS2 pause behavior remains unverified.

Players who participated in a reported tournament fixture may submit one
dispute. Only those participants and administrators see it. Administrators
record a revision-checked review decision; that decision does not change the
fixture result or bracket. This is a review record, not an automatic correction
or appeals workflow.

The management Updates panel shows per-server Clutchbase Access installed and
staged SHA-256 fingerprints with freshness and comparison state. Fingerprints
describe files on disk and do not prove the plugin loaded. The existing stopped-
server staged update flow remains authoritative. Full Go tests with PostgreSQL,
frontend tests and production build pass; real CS2/plugin runtime behavior is
still unverified.

Managed agents report bounded preinstalled Workshop map discovery with canonical
identifiers and per-host freshness. Startup selection uses the host's canonical
availableMaps list and labels saved missing maps unavailable. The API and agent
recheck installed availability before commands; missing discovery can stop a
saved Workshop startup map while retaining database intent. Tournament pools
remain official-only. The C# reporter still rejects Workshop match maps, so room
pickers remain official-only. Real CS2 Workshop layout and loading are unverified;
see docs/WORKSHOP_MAPS.md for deployment order and compatibility limits.

## Double-elimination tournaments (2026-11-23)

Tournament creation now supports double elimination alongside the existing
single-elimination and round-robin formats. Match rows persist bracket stages,
winner and loser destinations, and the source of each advancing slot. The
server resolves byes and second-loss eliminations inside the result transaction.
The grand final starts with the winners-bracket champion and losers-bracket
champion; if the latter wins, a reset final is activated. Administrators and
players receive the same canonical bracket snapshot, with the management view
showing winners, losers, grand final and reset stages. Existing match rooms,
score reporting and scheduling continue to apply to each fixture.

PostgreSQL integration coverage exercises 3, 4, 5 and 8 teams, bye propagation,
two-loss elimination, reset-final activation and competing result submissions.
Go tests and frontend build/UI tests pass. Browser visual inspection and real
CS2 match compatibility remain unverified.

An administrator can correct a double-elimination grand final that was won by
the upper-bracket champion when its skipped reset remains untouched. The server
reopens the tournament, restores both finalists and activates the reset in the
same transaction; the original reported score remains unchanged. Corrections
that would overwrite a played or otherwise changed reset remain blocked.
The reverse correction also finishes an active tournament and skips its reset
when that fixture has not started; both directions are revision protected.
