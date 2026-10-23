# Clutchbase project memory

## Product and user decisions

Agreed 2026-10-23. Build an independent private CS2 clubhouse, primarily a server
manager: spin up a server for friends, manage maps and matches, preserve stats and
highlights. Later import supported matches played elsewhere, similar in purpose
to Leetify. Tournaments must not be required to create or retain a match.

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
3. Match/practice presets, Workshop maps, rosters, ready-up, pauses and overtime.
4. Demo recording/upload, completed matches, scoreboard and basic statistics.
5. Manual demo imports, then share-code retrieval and authorized automatic
   external match tracking. Source-aware deduplication and one analysis pipeline.
6. Highlight detection/bookmarks, then CS2 playback/capture and clip rendering
   in a separate worker. FFmpeg alone cannot render a demo.
7. Skins/loadout preferences and separately evaluated server integrations.
   Valve restrictions and server-ban risks need explicit treatment.
8. Richer analytics, multiple hosts, safe updates, backups and optional tournaments.

## Architecture

- `web/`: React/Vite dashboard. Calls the same-origin `/api` endpoints.
- `backend/`: Go HTTP API, PostgreSQL persistence, transactional versioned migrations.
- `agent/`: outbound Go agent and `clutchbase-host` CLI. Managed mode installs
  CS2 and generates systemd user services; the existing-service mode remains.
  Machine mode connects once and controls multiple UI-created instances. Legacy
  single-instance registrations/agents remain supported separately.
- `internal/protocol/`: shared map allowlist and agent/API types.
- PostgreSQL stores desired state and the last observed process state separately.
- The browser uses an administrator token. Each machine (or legacy instance)
  gets its own random agent token; only its hash is retained in the database.
- Agent reports observed state and receives desired state, map and numeric game
  port. Paths and executables are configured locally, never sent as shell strings.
- Offline is derived from heartbeat age. Running means service process running,
  not verified game-query readiness or player counts.

## First-slice boundaries

No Steam authentication, public/multiuser deployment, cloud VM provisioning,
shared game installation, RCON, live match plugin, statistics, demos, skins, or
highlight renderer yet. Managed installs require SteamCMD, OS runtime libraries,
and a working user systemd session supplied by the operator. Each instance has
a full isolated install. Map changes require a stopped or uninstalled server.
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

Next: Steam sign-in/groups and an enforced SteamID allowlist. Original MatchZy
has documented whitelist support (one SteamID64 per line in whitelist.cfg and
matchzy_whitelist_enabled_default), but needs Metamod and CounterStrikeSharp.
Evaluate/pin compatible independently licensed releases, verify loaded plugins
and allowed/denied real client joins, and fail closed before replacing passwords.
Reference: https://shobhit-pathak.github.io/MatchZy/configuration/ .

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
