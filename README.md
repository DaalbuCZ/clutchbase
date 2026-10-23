# Clutchbase

An independent CS2 server control room built with React, Go, and PostgreSQL.
The product decisions and longer-term roadmap live in [docs/PROJECT.md](docs/PROJECT.md).

## Server management

- Connect a Linux machine once, then create multiple servers from the dashboard.
- Allocate non-overlapping game/client/TV ports automatically, or select a game port.
- View a server-authoritative workspace snapshot, process status, and heartbeat freshness.
- Select an official startup map while stopped and request start/stop.
- Persist desired and observed state separately in PostgreSQL.
- Automatically install missing CS2 files through SteamCMD on first Start.
- Create isolated instances with a local CLI, including private configs and systemd services.
- Install/update, start/stop, list instances, and inspect installation logs.
- Connect an outbound Go agent in managed, simulation, or existing-service mode.
- Reject stale edits using host revisions, and reject invalid lifecycle actions on the API.

One machine agent manages the instances created on it through the dashboard.
Existing single-instance registrations and agents remain supported. Provisioning
installs CS2 on an existing Linux machine; it does not create cloud machines or
verify game-query readiness/player counts. Each instance currently needs its own
full installation.
No Auto-Tournament/Ready Up code or runtime is used.

## Quick start with Docker Compose

1. Copy `.env.example` to `.env`. Generate **two separate** secrets using
   `openssl rand -hex 32`; set `CLUTCHBASE_ADMIN_TOKEN` and `POSTGRES_PASSWORD`.
   Use a hex password to avoid URL escaping in the development connection string.
2. Run `docker compose up --build -d`.
3. Open http://127.0.0.1:8080 and enter your administrator token.
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
command in a private password manager; no token rotation/recovery UI exists yet.
Use environment variables rather than command-line arguments for credentials.

## Local development

Requirements: Go 1.25+, Node.js 24+, npm, PostgreSQL 18 (or Compose for the database).

```bash
cp .env.example .env
# Fill both secrets and put your POSTGRES_PASSWORD into DATABASE_URL.
docker compose up -d postgres
npm ci --prefix web
make dev-api
# In another terminal:
make dev-web
```

Open http://127.0.0.1:5173. Vite proxies `/api` to the Go API on port 8080. For a
production frontend build, `make web` and run the API with
`CLUTCHBASE_WEB_DIR=web/dist`. The API defaults to loopback.

```bash
make build
make test
# Enable real PostgreSQL integration tests (an isolated temporary schema is used):
TEST_DATABASE_URL='postgres://clutchbase:YOUR_PASSWORD@127.0.0.1:5432/clutchbase?sslmode=disable' go test -race ./...
```

Embedded, versioned migrations run transactionally on startup under a database
advisory lock. The first migration adopts the original unversioned schema; the
second adds machines without replacing existing server rows or credentials.
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
or stopping instance. No scheduled automatic updates or plugins are installed.

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

For a fleet instance, the card's **Join password** details show the local command
to retrieve its password. Whitelists are not enforced yet; password protection
remains enabled. Public fleet operation still requires an individual GSLT in
each instance's private local configuration and suitable networking. Machine
mode refuses a process-wide `CLUTCHBASE_GSLT` to prevent accidental reuse across
instances. Stop the agent before changing private instance configuration; never
edit generated game configs for persistent settings. Steam sign-in and a
verified server-side SteamID whitelist are the next access-control slice.

Legacy single-instance agents remain supported:

```bash
# Set CLUTCHBASE_HOST_ID instead of CLUTCHBASE_MACHINE_ID for this mode.
./bin/clutchbase-agent --instance friday
```

If `friday` does not exist locally, the agent creates its config using the
registered game port. Legacy registrations stay attached to their original agent;
there is no automatic adoption into machine mode, delete, or move operation yet.
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
change server lifecycle state. It refreshes every five seconds and after commands.
Older responses are discarded using the snapshot timestamp. Failed refreshes
disable controls until the API is reachable again.

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
shutdown. A service reported running may still be loading CS2. A stopped request
can take the service's configured timeout to complete. If the agent restarts
while the service is already running, the active map is unknown until a managed
start; the configured startup map is not proof of the currently loaded map.

## Deployment boundary

This is a private, single-administrator development slice. The administrator token
is stored in browser sessionStorage for the current tab; it is not Steam sign-in
or a multiuser permission system. Both Compose ports bind to loopback. Add proper
authentication, authorization, HTTPS, backups and operational hardening before
exposing it as a public service. Skins, matches, demos, external imports and
highlight rendering are on the roadmap, not implemented controls.

No license has been selected for the original Clutchbase source yet. Dependencies
retain their own licenses; this repository does not include Valve game assets.

Real SSH/SteamCMD/systemd smoke-test results and remaining network/player-test
limits are recorded in [docs/REMOTE_TEST.md](docs/REMOTE_TEST.md).
