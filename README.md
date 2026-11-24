# Clutchbase

An independent CS2 server control room built with React, Go, and PostgreSQL.
The product decisions and longer-term roadmap live in [docs/PROJECT.md](docs/PROJECT.md).

## Server management

- Connect a Linux machine once, then create multiple servers from the dashboard.
- Manage servers at `/manage`; Steam players see permitted servers at `/play`.
- Receive canonical updates over authenticated WebSockets instead of browser polling.
- Allocate non-overlapping game/client/TV ports automatically, or select a game port.
- View a server-authoritative workspace snapshot, process status, and heartbeat freshness.
- View machine CPU and memory readings alongside storage; unknown/stale readings remain labeled.
- Search the fleet by server, address, map or machine and filter by reported status.
- Prove joinability with an agent-local game query; running servers show player counts.
- Show a redacted tail of the service journal and install log on each server card.
- Select an official startup map while stopped and request start/stop.
- Request a guarded restart on a running managed server without an open match.
- Stop and update a running managed server in one confirmed request when no match is active.
- Edit SteamID64 rosters on stopped installed fleet servers with an updated agent.
- Remove stopped fleet servers after agent confirmation, retaining their installations.
- Persist desired and observed state separately in PostgreSQL.
- Automatically install missing CS2 files through SteamCMD on first Start.
- Create isolated instances with a local CLI, including private configs and systemd services.
- Install/update, start/stop, list instances, and inspect installation logs.
- Connect an outbound Go agent in managed, simulation, or existing-service mode.
- Reject stale edits using host revisions, and reject invalid lifecycle actions on the API.

One machine agent manages the instances created on it through the dashboard.
Existing single-instance registrations and agents remain supported. Provisioning
installs CS2 on an existing Linux machine; it does not create cloud machines or
identify individual players. Each instance currently needs its own
full installation.
No Auto-Tournament/Ready Up code or runtime is used.

Deploy the API before a restart-capable agent: older APIs reject the new agent
capability field. Restart awaits the matching stop confirmation before starting;
real CS2 restart testing is still needed.
Stop & update waits for the agent to report that the server has stopped before
requesting the CS2 download. The server remains stopped afterward; if the agent
disconnects, the request stays pending until an update-capable agent reconnects.
Before the update command is sent, an administrator can cancel the queued
update, even if the agent is offline. Cancellation leaves the stop request in
place. Once sent, the update must finish or fail before the server can start.

## Quick start with Docker Compose

1. Copy `.env.example` to `.env`. Generate **two separate** secrets using
   `openssl rand -hex 32`; set `CLUTCHBASE_ADMIN_TOKEN` and `POSTGRES_PASSWORD`.
   Use a hex password to avoid URL escaping in the development connection string.
2. Run `docker compose up --build -d`.
3. Open http://127.0.0.1:8080/manage and enter `CLUTCHBASE_ADMIN_TOKEN`. In Komodo, find it in the stack's Environment settings; this is separate from the machine-agent token.
4. Choose **Connect machine**, enter a name and its game hostname/IP, and save
   the displayed agent command privately. The agent credential is shown once.
5. Build the agent with Go 1.25+ (`make build`) and run that command on the Linux
   game machine. Install SteamCMD and required OS libraries first. Use your HTTPS
   API origin for remote machines; loopback HTTP works for local development.
6. Once the machine is online, choose **Create server**, select a machine and map,
   and leave the port blank for automatic allocation. Each instance reserves its
   game port and the next two ports. Choose **Install & start** on its card to
   download and launch CS2. No additional agent setup is needed per server.

Add `--simulate` to the displayed `--machine` command to test several independent
server controls without downloading games. Keep its `--root` separate from real
instances; simulation does not inspect actual services.
Simulation is explicitly labeled in the dashboard. It does not launch a game,
test UDP connectivity, or make the displayed game address joinable.

The agent token is shown only once and its hash is stored in PostgreSQL. Keep the
command in a private password manager. Administrators can rotate a machine token;
save its replacement and restart that machine's agent with the new value. Rotation
marks the machine and its servers offline until the replacement agent reports,
without stopping their CS2 processes. Use environment variables rather than
command-line arguments for credentials.

## Player access

Players open `/play` and choose **Sign in with Steam**. OpenID verifies their
SteamID directly with Steam; no Steam Web API key is required. No Steam password
passes through Clutchbase. Sessions use private HttpOnly, SameSite cookies, expire
after 24 hours and are removed on logout. Logging in never grants management
access. The server filters every player snapshot against its saved SteamID roster.
A roster removal or expired/revoked session also updates an open player stream.

From a server's **Player access** settings, an administrator can copy an access
request link. A friend can sign in and submit the request; management reviews it
under **Access requests**. Approval requires a stopped server and an online,
capable agent. An empty roster disables whitelist enforcement, so approving the
first player requires explicit confirmation that enabling it can exclude other
players. Approval adds the player to the saved roster; the agent must still
apply and verify it.

Set `CLUTCHBASE_ORIGIN` to the exact browser origin, without a path. It defaults
to `http://127.0.0.1:8080`. For Vite development set it to
`http://127.0.0.1:5173`; use that same hostname consistently. Public origins must
use HTTPS. Steam callbacks and logout checks use this fixed configured origin,
not forwarded request headers. Keep reverse proxies configured accordingly.

The player page shows only permitted server names, addresses, maps, availability
and player counts. It does not expose machine registrations, logs, complete
rosters, tokens or lifecycle controls. Players copy the connection command and
use the join password shared by their host; passwords stay on the game machine.
Simulated servers stay labelled and cannot offer a join action. When management
opens a room on a running MatchZy Match server, rostered players can join a
1v1–5v5 room, choose a CT/T side, switch before ready, leave, and mark ready
from `/play`. Public and management pages show separate CT/T lineups, readiness,
side capacity and open slots. PostgreSQL enforces side capacity. Management
chooses an official map when opening a room; MatchZy changes the live map when
the room is loaded.
When the host's fresh Steam build check detects an update, players see a warning
on the server and match cards. The warning is advisory: Clutchbase cannot inspect
the player's installed client build. An unavailable or expired check is unknown.
With equal team sizes, the generated MatchZy setup treats joining the assigned
side as ready by default, so the in-game match starts automatically after its
roster joins. Room owners can opt into MatchZy's manual `.ready` flow for
balanced rooms. Uneven team sizes always use manual `.ready` so the smaller side
cannot trigger an early start; wait for the larger side to join before readying
in CS2. The opt-in requires a fresh capability report from the current agent
and running reporter; older or offline hosts remain blocked from loading it.
Match rooms require an updated machine agent that reports the current game map.
An administrator may opt in to player-created standalone rooms for each
managed server. A rostered player can then open a room and choose their side;
its creator can edit or shuffle it, remove participants, load it once both sides
are ready, or close it before loading. Removed participants can join again.
This permission is off by default and can be revoked by management.
When a room is loaded, its map, sides and player names are captured for that
request; later profile edits do not change the setup being sent to the server.
Management can close rooms and view side counts. Side choices
are local to a room. Managers can also switch a running MatchZy server between
Match and Practice while it is outside an active match; the dashboard waits for
the loaded plugin to confirm the runtime mode.
Before a standalone room is loaded, its participants can vote for an official
map. Everyone with access sees the totals, and management or the creator can choose a voted
map; changing the room map clears ready checks. Votes are advisory and do not
change the game until the match is loaded.
Management can also shuffle two or more players into sides of similar size.
The shuffle is random, not skill-based, and clears ready checks. Tournament
rosters remain fixed by their fixtures.
During an active MatchZy match, management can request a pause or resume when
the runtime reports that action is available. The dashboard and player page
show MatchZy's observed live or pause-requested state.
Steam display names and avatars are not fetched. Players can set an in-game name
shown to other permitted players in room rosters. Staff roles and account
administration are later slices.

## Reports

Players can report a problem from a match they joined and follow its status in
**My reports**. Administrators review reports under **Reports** and can mark
them resolved or reopen them. A report is visible only to its author and
administrators; resolving it does not change the match or penalize a player.
One report per participant and match is accepted.

## Statistics

Management has a Statistics page; signed-in players have My statistics in the
sidebar. Player totals cover only their own rooms on currently permitted
servers. The page shows score and K/D/A coverage, marks missing stats unavailable,
and uses confirmed series results for BO3/BO5 outcomes. K/D/A captures can be
partial or from the final map only; these totals are not a rating.
Map performance includes manually recorded series map scores as well as
single-map rooms. Unrecorded series maps do not count as appearances.

The Leaderboard ranks players by recorded wins, with ties sharing a rank. Player
views count only matches on servers they can currently access; management sees
retained history from all servers. Unscored rooms count toward room coverage but
do not affect ranking. This is a wins table, not a skill rating.

## Tournaments

Public and management pages offer a searchable tournament list, status filters
and individual event links (`#tournaments?tournament=ID`). Archived events remain
available through the filters. Steam sign-in preserves the selected public page,
room or tournament link.

Management can create optional single-elimination, double-elimination or round-robin
tournaments in BO1, BO3 or BO5. Double elimination uses winners and losers
brackets and a reset final if the lower-bracket finalist wins the grand final.
Teams register and check in through the public sidebar. Round
robin pairs every checked-in team once, opens rooms one round at a time, and
ranks teams by fixture wins. Captains can withdraw before fixtures start; once
registration closes, withdrawing prevents rejoining. Administrators can cancel registration or check-in before fixtures exist; history is retained and that tournament releases its roster locks. Equal wins share a rank; tied first place has no
unique champion or automatic playoff. Match history shows reported and manual
scores and player K/D/A separately. For series, administrators can record
individual map scores in history; these manual records do not change tournament
results. Earlier map scores are not captured automatically, and configured maps
may remain unplayed if a series ends early. Administrators can choose an
unresolved fixture winner and correct a completed winner with a revision-checked
confirmation. Round-robin standings are recalculated. Elimination corrections
are available for supported finals and when affected downstream slots remain
pending with no room or veto. Started fixtures and automatic bye propagation
remain protected; the recorded match score is not changed by a correction.

## Local development

Requirements: Go 1.25+, Node.js 24+, npm, PostgreSQL 18 (or Compose for the database).

```bash
cp .env.example .env
# Fill both secrets and put your POSTGRES_PASSWORD into DATABASE_URL.
# Change CLUTCHBASE_ORIGIN to http://127.0.0.1:5173 for Vite.
docker compose up -d postgres
npm ci --prefix web
make dev-api
# In another terminal:
make dev-web
```

Open http://127.0.0.1:5173/manage. Vite proxies `/api` to the Go API on port 8080. For a
production frontend build, `make web` and run the API with
`CLUTCHBASE_WEB_DIR=web/dist` and `CLUTCHBASE_ORIGIN=http://127.0.0.1:8080`
(or your HTTPS public origin). The API defaults to loopback.

```bash
make build
make test
# Enable real PostgreSQL integration tests (an isolated temporary schema is used):
TEST_DATABASE_URL='postgres://clutchbase:YOUR_PASSWORD@127.0.0.1:5432/clutchbase?sslmode=disable' go test -race ./...
```

The frontend tests include an opt-in API integration check. Run it only against a
**disposable** API and database: it changes the test player's in-game name. Seed a
player session and a whitelisted server named `Integration server` with an open
room named `Integration match`, then set `CLUTCHBASE_UI_TEST_URL`,
`CLUTCHBASE_UI_TEST_ADMIN`, and `CLUTCHBASE_UI_TEST_PLAYER` (the session cookie
value) when running `npm test --prefix web`. Without all three, this check skips;
the other UI tests still run. The integration test exercises HTTP and live
WebSocket snapshots. To also test server creation and start/stop, connect a
simulated machine agent to that disposable API and set
`CLUTCHBASE_UI_TEST_SIMULATED=1`. This creates a simulated server; it does not
authenticate with Steam or launch CS2.

Embedded, versioned migrations run transactionally on startup under a database
advisory lock. The first migration adopts the original unversioned schema; the
second adds machines without replacing existing server rows or credentials; the
third adds player-count and log-tail columns; the fourth persists runtime access
status; the fifth adds acknowledged fleet removal and agent capability tracking; the
sixth adds dashboard roster intent, applied hashes and agent capability tracking;
the seventh adds committed-change notifications; the eighth adds Steam players,
private sessions and single-use login state; the ninth adds preset intent,
agent acknowledgements, runtime mode and capability tracking; the tenth adds
standalone match rooms and player readiness; the eleventh adds temporary
CT/T side selection within each room; the twelfth adds guarded live mode command
acknowledgements; the thirteenth adds match room side capacity; the fourteenth
adds player-selected in-game names; the fifteenth adds MatchZy setup requests and
runtime acknowledgements; the sixteenth adds live pause/resume requests and
runtime state; the seventeenth records map-observation capability; the
eighteenth captures match-setup requests and safely interrupts older pending
requests that did not have a saved snapshot; later migrations add safe match
expiry/cancellation, results/history/player stats, staged reporter and game
updates, and, in migration 31, persistent teams and tournaments.
Back up durable databases before upgrading. Add subsequent schema changes as
new migrations rather than editing already-applied migrations.

## Automatically provision CS2

Use a Linux x86-64 host with SteamCMD, CS2 runtime libraries and a working user
systemd session. Install SteamCMD through your distribution or Valve's official
instructions first. The manager runs as the game-owning user and never invokes
sudo, changes the firewall, or installs OS packages. If SteamCMD is not on PATH,
pass `--steamcmd /absolute/path/to/steamcmd.sh` before the CLI command or on the
agent. Ensure your host has sufficient disk space for a **full CS2 installation
per instance**; shared game files are not implemented yet.

```bash
make build
./bin/clutchbase-host create --port 27015 --map de_mirage friday
./bin/clutchbase-host start friday
./bin/clutchbase-host list
./bin/clutchbase-host logs friday
./bin/clutchbase-host stop friday
# Wait for list to show stopped before updating:
./bin/clutchbase-host update friday

# Another isolated server on the same machine:
./bin/clutchbase-host create --port 27025 --map de_nuke saturday
./bin/clutchbase-host start saturday
```

`start` downloads and validates Steam app 730 if it is not installed, creates a
systemd user service, and requests launch through `game/cs2.sh`. It is safe to
repeat. `install NAME` downloads/prepares without launching. `update NAME` uses
SteamCMD without routine validation and refuses to update a running, starting,
or stopping instance. No scheduled automatic updates or plugins are installed automatically.

The initial download may take a long time. Inspect `logs NAME` in another
terminal. Output is retained in the instance's private `install.log`. A failed or
cancelled download is resumable; it does not receive an installed marker. Ctrl+C
cancels the SteamCMD process group. Status comes from actual operations and
systemd, not an estimated download percentage.

Instances live in `~/.local/share/clutchbase/instances/NAME/`. Use
`clutchbase-host --root /absolute/path COMMAND ...` and the same agent `--root`
to choose a different disk. Names must be unique within your user systemd session,
even across roots. Each instance reserves the game port plus the following two
ports for client/TV use; the manager rejects overlaps in its own root and enables
strict game port binding. It does not reserve sockets against unrelated software.

Creation generates a private join password and prints it once. Retrieve it later
with `clutchbase-host credentials NAME`. For custom values, load
`CLUTCHBASE_SERVER_PASSWORD` and optional `CLUTCHBASE_GSLT` into the host's private
environment **before creation**. A public server needs its own valid Steam game
server login token and host networking configured by its operator; do not reuse
one token across instances. The manager rejects duplicate tokens within a root.
Passwords and GSLTs stay in private host files, are never sent to the API, and
are excluded from `list` output. Do not commit instance directories or logs.

Managed instances own their generated `server.cfg` and `clutchbase.cfg`. Settings
in `server.cfg` are reapplied after CS2 resets game variables on a map load;
`clutchbase.cfg` contains startup commands and the selected map. Both are
regenerated on install/update and managed start. Do not hand-edit those files
for persistent customization.

For dashboard control, connect a machine and use its displayed command:

```bash
# Set CLUTCHBASE_API_URL, CLUTCHBASE_MACHINE_ID and CLUTCHBASE_AGENT_TOKEN first.
./bin/clutchbase-agent --machine
```

Create instances through the UI after its agent connects. Local names are
`s-` followed by the lowercase server ID. Each has its own controller lease,
configuration, systemd service, and generated join password. The API sends only
server IDs, allowed maps, numeric ports, and desired states. Paths and commands
are derived locally. The agent serializes SteamCMD downloads through the existing
root lock while continuing to report individual instance state. Creation leaves
a server stopped; its card becomes actionable after the first instance report.
Automatic port selection starts at 27015 and searches in increments of ten.
A machine supports up to 64 registered instances; this is a protocol bound, not
a claim that its hardware can run 64 games. Unrelated software and alternate DNS
names for the same physical machine can still cause port conflicts.

Updated machine agents offer **Remove server** for online stopped or uninstalled
instances. Confirm on the card; it remains visible as removing until the agent
acknowledges completion. The agent checks the local service is inactive, removes
its owned systemd unit and moves the installation into `ROOT/retired/LOCAL_NAME/`.
Private configs, game files and logs are retained; removal does not reclaim disk
space. Ports and machine capacity are released only after acknowledgement.
Retries survive lost responses and agent restarts. A database tombstone prevents
stale reports from resurrecting the registration. Older agents keep removal
disabled until updated. Restore/purge controls are not provided.

For a managed fleet instance, open **Player access** on its server page and
choose **Show join password**. The machine agent must be online. Administrators
can copy or hide the revealed password; signing out clears it. Password protection
remains enabled, including with a configured whitelist. Public fleet operation still requires an individual GSLT in
each instance's private local configuration and suitable networking. Machine
mode refuses a process-wide `CLUTCHBASE_GSLT` to prevent accidental reuse across
instances. Stop the agent before changing private instance configuration; never
edit generated game configs for persistent settings. An opt-in host-side
`whitelist --file FILE NAME` command configures an operator-installed MatchZy
stack on a stopped instance, while retaining password protection. See
[SteamID access setup](docs/ACCESS.md) for pinned reference releases, setup and
required real-client checks. The Clutchbase Access plugin and independent local
watchdog verify runtime settings and stop a game when evidence is lost. Cards
show the agent's access verification state through the canonical API snapshot.
On a stopped, installed fleet server, expand **SteamID whitelist** and choose
**Edit roster**. Save up to 256 unique public individual SteamID64 values, one
per line. The required plugins must already be installed on the machine; saving
a roster does not download them. An empty roster explicitly disables the
whitelist while retaining the join password. Start stays blocked until the
owning agent confirms it applied the saved roster. This acknowledgement confirms
configuration, not runtime enforcement or real-client admission. The access
status separately reports watchdog verification after launch.

The dashboard takes ownership of roster configuration only after the first save;
servers without a dashboard roster keep their local CLI configuration. Stop the
agent before making local configuration changes. Once a roster is managed here,
the agent reapplies PostgreSQL's roster, retries partial writes and refuses live
roster changes. Older agents receive Stop for these instances and cannot enable
Start; deploy the API before upgrading agents. Simulated agents acknowledge the
flow without enforcing player access. Legacy instances keep the local whitelist
command. Passwordless admission remains future work.

Legacy single-instance agents remain supported:

```bash
# Set CLUTCHBASE_HOST_ID instead of CLUTCHBASE_MACHINE_ID for this mode.
./bin/clutchbase-agent --instance friday
```

If `friday` does not exist locally, the agent creates its config using the
registered game port. Legacy registrations stay attached to their original agent;
there is no automatic adoption into machine mode or dashboard removal/move for them.
Do not register the same existing server again under a machine.
During installation, heartbeats continue and the dashboard shows `installing`.
Stop cancels provisioning. After a failed install, request Stop and then Start to
retry. Once files are ready, the agent checks the latest desired state before
launching, so a cancelled Start does not independently launch a game later.
While linked, local CLI install/start/stop/update commands are blocked by the
agent's controller lease; use the dashboard for lifecycle changes. Stop the agent
first to return control to the standalone CLI. Listing/logs/credentials remain
available. Game processes remain managed by systemd if the agent exits.

Logs for the game process are in
`journalctl --user -u clutchbase-friday.service`. Configure user lingering if
agents/services need to survive logout. The manager does not enable game
services at boot or touch existing service files owned by another configuration.

## Connect an existing service

For a server you already manage yourself, the original driver is still available:

```bash
./bin/clutchbase-agent \
  --unit clutchbase-cs2.service \
  --config /home/cs2/server/game/csgo/cfg/clutchbase.cfg
```

Adapt `deploy/clutchbase-cs2.service` if needed. The service must consume the map
config through `+exec clutchbase.cfg`. This mode does not install or update files.
Run it as the same unprivileged user that owns the service.

Remote agents require an HTTPS API URL. Plain HTTP is accepted only for loopback
development. Agents make outbound requests; do not expose RCON or a host shell.
Paths, executables and service names are chosen locally by the operator. The API
sends only server IDs, validated desired state, map and numeric game port.

## State ownership

PostgreSQL is the source of truth. `GET /api/state` returns one canonical snapshot:
server instances, connected machines, maps, effective statuses, revisions, and
allowed actions. The React UI
stores that snapshot and temporary form/loading state; it does not optimistically
change server lifecycle state. The browser receives an initial WebSocket snapshot, then updates on committed
PostgreSQL changes and server-computed heartbeat expiry. Commands still use
revision-protected HTTP endpoints and can request a fresh snapshot afterward.
There is no periodic browser state fetch. Older responses are discarded using
the snapshot timestamp. A disconnected or silent live channel disables controls
until it reconnects and receives canonical state. Agent heartbeats still use HTTP
every five seconds; browser WebSockets do not change game reconciliation.

`GET /api/live` upgrades a same-origin WebSocket. Management authenticates in its
first message, keeping tokens out of URLs; player streams use their HttpOnly
session cookie. PostgreSQL LISTEN/NOTIFY drives committed-change updates. Each
live tab uses a dedicated listener connection outside the HTTP query pool; the
API allows 64 concurrent live connections. Use a shared fan-out before larger
deployments. Reverse proxies must support WebSocket upgrades and long-lived
connections. Vite's API proxy already forwards upgrades.

`PATCH /api/hosts/{id}` requires the snapshot's `revision`. A transaction locks
the row, validates revision and current actions, and increments the revision on
acceptance. A stale client gets HTTP 409 and reloads state. Provisioning adds observed
`uninstalled` and `installing` states to this same state model; no separate
frontend provisioning state is kept. Requesting start sets
desired state to running; it does not overwrite observed state. The next agent
report confirms process state. No heartbeat for 20 seconds projects an offline
status with no actions. Agent reports cannot change desired state. Machine heartbeats return all of that
machine's desired instances; individual reports cannot write another machine's
servers. Omitted instance reports do not refresh their last-seen timestamps.
`POST /api/machines/{id}/servers` locks the machine row while checking capacity
and reserving ports, including concurrent creation requests.

Agent reconciliation is idempotent and retries on subsequent heartbeats. The
agent leaves an already running game process alone during an API outage or agent
shutdown. A report of running means the agent's loopback game query was
answered, so the engine accepts connections; while the service is active but
the query is unanswered, the card shows starting. Player counts come from the
same query and clear with the next report that does not carry them. The query
targets 127.0.0.1 only, so host firewalls blocking remote A2S traffic do not
affect it. Agent reports also include a capped log tail of the service journal
and install log; lines that could echo join passwords or GSLT tokens are
redacted on the host before anything is sent. A stopped request can take the
service's configured timeout to complete. Updated agents read the active map
from the game query, including after an agent restart or MatchZy map change.
If the reported map is not supported or the agent is older, the current map is
shown as unavailable. The configured startup map remains a separate setting.

Managed machine agents report space available to their user on the filesystem
containing their storage root. This is shown only while the report is fresh;
separate instance mounts, quotas and concurrent downloads can change usable space.

## Deployment boundary

This remains a private development deployment with a single administrator. The administrator token
is stored in browser localStorage until sign out; the management sign-in screen points to the deployment setting where it is configured. It is separate from Steam player sign-in. Steam player sessions grant
read-only server views and limited match-room participation, not administrator
privileges or staff roles. Both Compose ports bind to loopback. Add proper
authentication, authorization, HTTPS, backups and operational hardening before
exposing it as a public service. Live match operation, results, demos, external
imports and highlight rendering remain on the roadmap. Standalone rooms remain
available without joining a team or tournament. See [functional slices](docs/FEATURES.md).

## Teams and tournaments

Signed-in players can create or join a five-player team at `/play` → **My team**.
Captains can generate a 24-hour invite code, remove members, transfer captaincy,
and disband a team. Team edits require confirmation and the current roster
revision. Checked-in tournament rosters are locked; disbanding waits until all
registrations finish. Disbanding preserves historical fixture names. At `/manage` → **Tournaments**,
an administrator creates a 1v1–5v5 event, closes registration to start check-in,
and generates a single-elimination bracket from checked-in teams. Captains
register and check in their team in the public **Tournaments** page. Byes advance
automatically, and fenced MatchZy final scores advance the winner; an admin can
resolve tied or unreported results. Tournament and team
state lives in PostgreSQL and appears in live snapshots. Administrators can open
a linked match room on an eligible Match server; players join its roster, ready
up and copy a connect command after the setup loads and the server is ready. See
[functional slices](docs/FEATURES.md) for the remaining parity gaps.

Management lists players after they sign in with Steam and can add them to a
server's roster draft. Saving a roster requires an eligible stopped server;
signing in does not itself grant server access.

For a closed room whose MatchZy setup was loaded, management can record, correct
or clear a manual score for the teams that started CT and T. Authorized players
see that score marked as manual. This record does not end the game.

**View match history** opens paginated, live records of closed rooms whose setup
was loaded. Managers can correct scores there, including older rooms. Players
see history only for servers that currently permit their SteamID; managers also
retain read-only history from removed servers.

For participants in closed loaded rooms, management can also enter or correct
manual kills, deaths and assists. An absent stat line means it has not been
recorded. These numbers are not fetched from CS2.

With ClutchbaseAccess 0.3.8 and a current agent, a supervised MatchZy 0.9.2
match that finishes normally can report its final score automatically. Reports
are retried until accepted and displayed separately from manual corrections.
The reporter must observe the match live and its completed state; an abrupt exit
or missing runtime evidence can prevent capture. Reported K/D/A are a partial
snapshot of rostered players available to the server at capture; disconnected
players may be absent and resets can affect counters. This has not been verified
in a live CS2 match.

No license has been selected for the original Clutchbase source yet. Dependencies
retain their own licenses; this repository does not include Valve game assets.

Real SSH/SteamCMD/systemd smoke-test results and remaining network/player-test
limits are recorded in [docs/REMOTE_TEST.md](docs/REMOTE_TEST.md).

## Match and Practice presets

On an installed, stopped fleet server, select **Startup preset**: Match, Practice,
or Host default. Match uses original MatchZy 0.9.2's PUG/warmup startup; Practice
uses its practice startup. Save confirmation comes from the owning agent before
Start is enabled. Changing a mode requires stopping the game. Host default clears
the managed startup mode; existing local settings remain untouched until the first save.

Install the pinned third-party stack described in [access setup](docs/ACCESS.md)
first. Build `plugins/ClutchbaseAccess` with .NET 10 and the pinned
CounterStrikeSharp API, then place `ClutchbaseAccess.dll`, `presets.version` and
`mode-control.version` inside `ClutchbaseAccess/` beside both host/agent executables. Selecting a mode
copies this first-party reporter into the stopped instance. It never updates an
active game's plugin. No third-party stack is downloaded by this control.

When the connected agent reports a different staged reporter, management shows
**Install staged reporter**. Stop the server first, then use this action to
install the staged first-party build. Start remains blocked until the agent
confirms the requested file; runtime verification occurs after the next start.

Reporter 0.3.6 sizes MatchZy's in-game ready threshold to the smaller side of a
room, allowing small matches in a larger room. For uneven sides, wait until the
larger side has joined before readying. This behavior requires the new reporter
to be installed and remains unverified in a live match.

The standalone equivalent is `clutchbase-host preset --mode practice NAME`
(or `match` / `default`). Stop the linked agent before using mutating CLI commands.
For an unlinked instance, `clutchbase-host reporter NAME` reports the staged and
installed reporter fingerprints and whether an update is available. With the
game stopped, `clutchbase-host reporter --install NAME` installs the staged build;
it can also restore a missing reporter when the remaining plugin stack is ready.
This checks on-disk files. The next game start still needs runtime verification.
Runtime verification reads the loaded MatchZy fields through fresh whitelist
watchdog evidence; password-only servers without that evidence show an unknown
runtime mode. Configuration acknowledgement alone does not prove active mode.
Older agents cannot start instances with managed preset intent. Simulation only
acknowledges configuration. Match records, Workshop maps and dashboard match
operation remain later slices; tournament and persistent-team features remain
planned but are not required for casual matches.
