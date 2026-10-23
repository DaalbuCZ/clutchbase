# Clutchbase

An independent CS2 server control room built with React, Go, and PostgreSQL.
The product decisions and longer-term roadmap live in [docs/PROJECT.md](docs/PROJECT.md).

## What works in this first slice

- Register a host and receive its dedicated agent credential once.
- View a server-authoritative workspace snapshot, process status, and heartbeat freshness.
- Select an official startup map while stopped and request start/stop.
- Persist desired and observed state separately in PostgreSQL.
- Connect an outbound Go agent in simulation or systemd user-service mode.
- Reject stale edits using host revisions, and reject invalid lifecycle actions on the API.

One host registration represents one existing server/service. This does not yet
install CS2, create cloud machines, or verify game-query readiness/player counts.
No Auto-Tournament/Ready Up code or runtime is used.

## Quick start with Docker Compose

1. Copy `.env.example` to `.env`. Generate **two separate** secrets using
   `openssl rand -hex 32`; set `CLUTCHBASE_ADMIN_TOKEN` and `POSTGRES_PASSWORD`.
   Use a hex password to avoid URL escaping in the development connection string.
2. Run `docker compose up --build -d`.
3. Open http://127.0.0.1:8080 and enter your administrator token.
4. Register a host. The game hostname/IP and port are connection details; the
   agent controls the locally configured unit, so ensure those details match it.
5. Build the agent with Go 1.24+ (`make build`). Copy the command displayed after
   registration and run it from the repository root, on the same machine as the
   API. Its `--simulate` flag demonstrates the control loop without CS2.

Simulation is explicitly labeled in the dashboard. It does not launch a game,
test UDP connectivity, or make the displayed game address joinable.

The agent token is shown only once and its hash is stored in PostgreSQL. Keep the
command in a private password manager; no token rotation/recovery UI exists yet.
Use environment variables rather than command-line arguments for credentials.

## Local development

Requirements: Go 1.24+, Node.js 24+, npm, PostgreSQL 18 (or Compose for the database).

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

The initial schema is embedded and applied idempotently on startup. Before any
future schema changes, introduce versioned migrations; `CREATE TABLE IF NOT
EXISTS` is only the initial bootstrap, not an upgrade mechanism.

## Connect an actual CS2 server

Use a Linux account with an already installed, working dedicated server. This
release intentionally leaves installation, Steam server tokens, firewall rules,
runtime libraries, and server passwords to that host's operator.

1. Adapt `deploy/clutchbase-cs2.service`, including paths, game port and any
   server launch options. Copy it to
   `~/.config/systemd/user/clutchbase-cs2.service`.
2. Run `systemctl --user daemon-reload`. Keep this service stopped while linking.
3. Make sure the game account can write `game/csgo/cfg/clutchbase.cfg`. The agent
   atomically writes a validated `map de_...` command there before starting the
   service. The service must consume it through `+exec clutchbase.cfg`.
4. Build/copy `bin/clutchbase-agent` onto the game host. Set the three environment
   variables from registration, then run:

```bash
./bin/clutchbase-agent \
  --unit clutchbase-cs2.service \
  --config /home/cs2/server/game/csgo/cfg/clutchbase.cfg
```

Run the agent as the same unprivileged user that owns the game service, never as
root. User services need a functioning user systemd session; configure lingering
if you need them to survive logout. Do not enable the game service at boot if
you want Clutchbase to own its lifecycle. Inspect logs with
`journalctl --user -u clutchbase-cs2.service`.

Remote agents require an HTTPS API URL. Plain HTTP is accepted only for loopback
development. Agents make outbound requests; do not expose RCON or a host shell.
The backend never sends shell commands, unit names, filesystem paths, or secrets
to a host. Those are configured by the host operator.

## State ownership

PostgreSQL is the source of truth. `GET /api/state` returns one canonical snapshot:
hosts, maps, effective statuses, revisions, and allowed actions. The React UI
stores that snapshot and temporary form/loading state; it does not optimistically
change server lifecycle state. It refreshes every five seconds and after commands.
Older responses are discarded using the snapshot timestamp. Failed refreshes
disable controls until the API is reachable again.

`PATCH /api/hosts/{id}` requires the snapshot's `revision`. A transaction locks
the row, validates revision and current actions, and increments the revision on
acceptance. A stale client gets HTTP 409 and reloads state. Requesting start sets
desired state to running; it does not overwrite observed state. The next agent
report confirms process state. No heartbeat for 20 seconds projects an offline
status with no actions. Agent reports cannot change desired state.

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
