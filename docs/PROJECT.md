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
   server instances, passwords, logs and useful startup/readiness checks.
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
- `backend/`: Go HTTP API, PostgreSQL persistence, embedded initial schema.
- `agent/`: Go executable making outbound authenticated heartbeats. One locally
  configured systemd user service per registered host in this first slice.
- `internal/protocol/`: shared map allowlist and agent/API types.
- PostgreSQL stores desired state and the last observed process state separately.
- The browser uses an administrator token. Each host gets its own random agent
  token; only its hash is retained in the database.
- Agent reports observed state and receives desired state. Hostnames and service
  commands are never interpreted as shell strings.
- Offline is derived from heartbeat age. Running means service process running,
  not verified game-query readiness or player counts.

## First-slice boundaries

No Steam authentication, public/multiuser deployment, CS2 installer, RCON, live
match plugin, statistics, demos, skins, or highlight renderer yet. The real agent
controls an existing systemd user service and writes a map config consumed by
that service. Map changes require a stopped server. Simulation is explicitly
labeled and does not launch CS2. Keep API/frontend on loopback locally; use HTTPS
and proper user authentication before exposing to other people.

## Verified implementation (2026-10-23)

The first control loop is implemented: host registration, separate hashed agent
credentials, PostgreSQL-backed workspace snapshots, server-derived lifecycle and
actions, revision-protected commands, map selection, and simulation/systemd agent
drivers. The UI has been simplified according to the user's copy preference.

Verified with Go race tests (including a real PostgreSQL 18 integration test),
`go vet`, a TypeScript/Vite production build, Compose configuration validation,
and browser registration/map/start/stop checks using the simulated agent. The
browser showed starting/running/stopping/stopped transitions from API snapshots
and no console warnings/errors. Real CS2/systemd operation and the Docker image
build have not been exercised in this environment.

Current development preview uses a disposable PostgreSQL cluster under `/tmp`
and one explicitly simulated host. It is not a production deployment or a real
game server. Follow README.md to configure durable local credentials/storage.
