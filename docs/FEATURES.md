# Management and player workflows

The user requests functional breadth comparable to Auto Tournament, Ready Up and
CS2 Server Manager, delivered over independent implementation slices. Official
product documentation is used to identify workflows; no source, styling, assets
or restricted integrations are copied. Clutchbase keeps its React/Go/PostgreSQL
architecture and prioritizes playing with friends. Tournament features are now
in scope but remain optional for standalone matches.

| Slice | Management | Player | State |
| --- | --- | --- | --- |
| Access and live state | Server controls and access request review under `/manage`; administrator token; canonical WebSocket snapshots | Steam sign-in, session/logout, access requests, permitted server list and join command under `/play` | Implemented foundation; real Steam sign-in and permitted-server view confirmed by user; enabling a first whitelist requires explicit management confirmation |
| Match and practice setup | Standalone 1v1–5v5 rooms with editable titles, official maps and sizes before loading; startup presets; guarded live MatchZy Match/Practice switching; load one-map setups from ready rooms; confirmed end-match request | Whitelisted players join CT/T sides with server-enforced capacity, switch until ready, leave, mark ready, see CT/T lineups with capacity bars and open-slot counts, view setup/end state and copy a connect command from a loaded match when the canonical server snapshot says it is ready | Partial: setup and end request delivery are implemented, live CS2 setup loading and map switching passed the recorded smoke test; end still needs live verification; Workshop maps and validated game settings remain next |
| Live match operation | Setup-fenced MatchZy pause/resume and end without a winner; configure a player pause-vote threshold | Rostered players can vote to pause a live setup, see the canonical vote count, mark room readiness in the web UI and follow live, overtime and ended states | Partial: balanced lineups auto-ready by default and can opt into MatchZy `.ready`; uneven lineups retain manual in-game ready to avoid early starts. Pause votes are revision-checked and queue the existing fenced pause command at the configured threshold; administrators can still pause directly. Final score capture exists, but earlier series map scores, pause budgets and round recovery remain gaps. Real CS2 verification is required. |
| Players and permissions | Signed-in player directory and selection in server roster drafts; staff roles and moderation/report review | Player-set in-game name; profile, account connections and personal preferences | Partial: access uses saved SteamID rosters; staff roles and moderation remain planned |
| Completed matches | Paginated live history, automatic final score and partial player K/D/A reports, manual corrections and manual per-map series scores; demo and replay processing | Live history, reported and manual scores and K/D/A for permitted servers | Partial: reported stats cover players available at capture and need live CS2 verification; automatic earlier-map scores, detailed statistics and demos remain planned |
| Imports and analysis | Manual demo imports, deduplication, analysis jobs and supported external match sources | Imported match history and statistics | Planned |
| Ratings and statistics | Live player summary table, score/stat coverage and a leaderboard by recorded wins | Private My statistics and server-scoped leaderboard sidebar pages | Partial: room summaries, coverage, single-map and manually recorded series map performance, and wins rankings are implemented; skill ratings, automatic multi-map performance and detailed analysis remain planned |
| Playing together | Opt in per server to player-created rooms; random side shuffle for standalone rooms | Rostered players can create rooms, and their creators can edit, shuffle, remove participants, load ready matches or close unstarted rooms | Partial: player-created rooms, creator controls, random side shuffle and persistent teams are implemented; parties, queueing, matchmaking and skill-based balancing remain planned |
| Community | Private match-report review with revision-checked resolve/reopen | Match participants can report a problem and follow their own report | Partial: reports are implemented; player search, friends, penalties and notification history remain planned |
| Highlights and replay | Recorder/worker settings, processing queues, bookmarks and clip review | Personal highlights, replay/watch pages and clip browsing | Planned |
| Server operations | Search servers by name, address, map or machine; filter by canonical status and attention conditions; fleet health, updates, plugin installation/version checks, credentials, backups, logs and recovery | Accurate availability and connection guidance | Partial: lifecycle/provisioning/logs/whitelists/removal, fleet search/status filters, actionable offline/error/update/access attention summary, available machine storage, live machine CPU/memory readings with freshness and unknown-state handling, stopped-server staged reporter installation/recovery with per-server installed/staged SHA-256 inventory, guarded managed-server restart and administrator machine-token rotation are implemented; file fingerprints do not prove runtime loading, and restart needs real CS2 verification |
| Integration and system settings | Steam account settings, optional Discord/email integrations, diagnostics and deployment | Account linking and notification preferences | Planned |
| Loadouts and inventory | Independently evaluated inventory/drop/settings workflows | Loadout/skin preferences | Conditional on Valve rules, licensing and server-ban evaluation |
| Teams and tournaments | Team roster view, BO1/BO3/BO5 configuration, map-pool, registration/check-in, single- and double-elimination brackets or round-robin fixtures grouped by round, stage progress, linked match rooms, guarded winner correction and dispute review | Persistent five-player teams, signup/check-in, confirmed withdrawal, player fixture disputes and administrator cancellation before fixtures, server-authoritative map bans/picks/side choice, public bracket, series map order and per-team map score | Seven-map veto and MatchZy multi-map BO1/BO3/BO5 series are implemented. Administrators can correct round-robin winners and supported elimination results with revision checks; started downstream fixtures and bye propagation block correction. Fixture participants can dispute reported results and administrators can record a review decision. Round robin provides one fixture per pairing, server-projected standings and shared ranks on equal wins. Double elimination includes a reset final when required. Informational fixture scheduling is implemented. Swiss, playoffs and broader tournament administration remain gaps |

Reference scope is based on published product workflows: tournament formats,
team check-in, browser map veto, server allocation, match rooms, demo/stat/2D
replay, highlights, ratings, leaderboards and matchmaking; Ready Up match
operation and server-side administration; and CSM machine/server lifecycle,
updates, plugin versions and health. This is a multi-slice target, not a claim of
current parity. Reference billing, license activation, restricted plugin
compatibility and private APIs are not dependencies; equivalent needs must use
independent Clutchbase integrations. See the upstream feature overviews:
https://docs.autotournament.gg/ and
https://docs.autotournament.gg/cs2/server-manager/updates .

Each slice needs usable backend behavior and enforced permissions before its
controls appear. Tournament membership is never required for a standalone match.
Real client evidence is required before removing join-password fallback.

## First slice boundaries

Management remains protected by the existing administrator token. Steam users
receive player sessions; sign-in does not make anyone an administrator. Players
can see only servers and match rooms whose saved dashboard whitelist contains
their SteamID. They can join/leave open rooms and update their own readiness.
Joining a server is offered only when the canonical server state is running,
its roster is acknowledged and runtime whitelist evidence is verified. Passwords
and other credentials remain on the game host. Players use the password shared
by their host; the application does not retrieve or distribute it.

State is pushed over authenticated same-origin WebSockets. PostgreSQL triggers
notify listeners after committed changes, and server timers project heartbeat
and player-session expiry without browser polling. Agent heartbeats remain the
existing outbound HTTP protocol. Future agent push needs its own reconciliation
and compatibility slice.

The current implementation includes persistent teams, single-elimination
tournaments and round-robin tournaments with standings. MatchZy owns in-game
`.ready`; Clutchbase can opt balanced standalone rooms into that flow, while its
room readiness controls remain on the web page. In-game veto/score controls,
social features, profile enrichment, staff roles and public deployment
configuration remain parity gaps, not out-of-scope product decisions.
Steam names/avatars may be added later through an optional Steam Web API key.

Startup presets configure original MatchZy 0.9.2 on an installed, stopped server.
Match selects its PUG/warmup flow; Practice selects practice mode. A ready room
can load a one-map MatchZy setup with its SteamID roster, chosen sides and player
names. The host plugin confirms the exact local setup file is active before the
room reports success. Managers can request MatchZy end that loaded setup without
a winner; the room closes only after the host reports that setup unloaded. Neither
action has been validated in a live CS2 match.

Match rooms are stored independently of tournaments and attach to a running,
managed server with verified Match mode and a saved roster. Management can open
one room per server and close it. A room holds one to five players on each side.
Eligible players choose an in-match CT/T side, can change it until ready, and can
leave or set their ready flag. PostgreSQL serializes joins and side changes so a
side cannot exceed capacity. The player page shows participant and ready counts
by side. These are temporary sides within a room, not persistent teams. Players
can set the in-game name shown to other permitted room members without a Steam
Web API key. Management must load the setup explicitly after everyone is ready.
While loading or after confirmation, player roster changes are locked. A failed
load blocks further match loads on that server until it is confirmed as loaded
or an eligible request is cancelled. Issued requests created by the updated
plugin can be cancelled after a later supervised server launch; legacy and
otherwise uncertain requests remain blocked. Loading a MatchZy setup can change
the live map. The selected map, player IDs, sides and names are captured when a
load is requested and kept unchanged for retries and room display.

Snapshots keep open and unresolved rooms and the currently active setup, plus
the 50 most recent other rooms. The separate paginated history contains closed
rooms with confirmed loaded setups and a player scoreboard table. Reported and
manual K/D/A remain separately labeled.

Before a load is requested, managers can edit the room title, official map and
size. Shrinking a side below its current player count is rejected. Map, size
or overtime changes clear every player's ready check; changing only the title preserves
readiness. Concurrent edits use the room's settings revision to prevent an old
editor from overwriting newer choices. Managers can remove a participant from
the room before loading; the remaining ready checks are cleared. Removal from a
room does not change the server's SteamID roster or ban the player.
Participants in standalone rooms can also vote for an official map; the live
snapshot shows anonymous totals and each player's own vote. Management can
select any voted map through the revision-checked room settings. Votes are
advisory and do not replace tournament vetoes.
Player joins, departures and side changes also clear other ready checks. The
server supplies the actions available to each player in its live snapshot.

Managers can switch the loaded MatchZy 0.9.2 runtime between Match and Practice
while `matchStarted` is false. The first-party reporter accepts only fixed mode
requests, runs the corresponding MatchZy mode command, and reports the actual
runtime mode back through the agent. This control is unavailable when access or
plugin evidence is stale, the plugin version is unknown, or a match has started.

When fresh verified evidence shows no setup loaded after a room's setup was
previously loaded, the room shows that observation. The manager can close it and
enter a known result, but an unloaded setup alone does not prove how it ended.

ClutchbaseAccess generates MatchZy ready thresholds from the smaller side of the
room roster, rather than the room's maximum capacity or host defaults. In
uneven matches the larger side can become ready before every rostered player
has joined, so players must follow the server's in-game `.ready` flow. Balanced
standalone rooms auto-ready by default; room owners can opt into manual in-game
ready. That option is sent in the captured setup and can load only when the
current agent heartbeat confirms support from the running reporter. The updated
reporter requires stopped-server installation and the manual-ready path still
needs live CS2 verification.
