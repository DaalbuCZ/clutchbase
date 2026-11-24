# Workshop reporter compatibility

ClutchbaseAccess 0.3.15 recognizes canonical Workshop identifiers and independently
checks their local discovery files. Workshop match loading remains explicitly
blocked. This is a safety/diagnostics change, not working Workshop gameplay.
Official-map behavior, setup fences, whitelist verification and fixed commands
remain supported on the pinned CounterStrikeSharp API 378 / original MatchZy
0.9.2 stack. No restricted fork code or additional dependencies were used.

## Reporter behavior

`MapPolicy` applies the same identifier grammar as Go:
`workshop/<published-file-id>/<map>`, with 1–20 ASCII decimal ID digits, no leading
zero, and 1–128 lowercase ASCII letters, digits, underscores or hyphens for the
map. It rejects extensions, whitespace, Unicode digits, delimiters, traversal,
absolute paths, extra components and bare numeric IDs. Syntax is not availability.

Before building a setup, every map in its 1/3/5-map sequence is validated. For a
Workshop entry the reporter independently checks
`Server.GameDirectory/csgo/maps/workshop/<id>/<map>.bsp`. The browser and request
supply only the identifier; the installation root comes from CounterStrikeSharp.
This matches the current Go discovery layout; it does not claim the file is a
valid CS2 map. Even a regular file containing arbitrary fixture bytes passes the
file-type check and is then rejected by the compatibility fence.

Linux `openat` walks directory descriptors from `/` with `O_NOFOLLOW`,
`O_DIRECTORY` and `O_PATH`; `statx(AT_EMPTY_PATH)` verifies the final descriptor
is a regular file. Symlinks in the trusted root's ancestors are also rejected.
Missing/unreadable components, directories, FIFOs, devices and symlink files do
not authorize loading. Relative roots and roots containing `.` or `..` are
rejected. Missing statx support fails closed. Open descriptors prevent symlink
redirection during the walk, but do not freeze content against changes by a
trusted local operator. No content is opened for execution, installed or downloaded.

A valid Workshop setup with missing/unsafe files writes a failed match result.
An installed Workshop setup writes a failed result with this explicit blocker:

> Workshop match loading is unavailable: installed-file discovery does not prove CS2 loading or canonical runtime map identity

Both failures occur before writing the MatchZy setup file, claiming the issuance
fence or issuing a game command. Mixed official/Workshop series are also blocked,
including when Workshop appears after the first map. A canonical Workshop first
map no longer silently times out at the old official-only request check.
Malformed requests may still be discarded by the existing request boundary.
A failed request retains existing backend failed-load/cancellation rules; this
change does not authorize automatic retries or erase issuance markers.

Live score evidence remains official-only. Numeric IDs, bare Workshop names and
canonical Workshop strings produce no live score. Final-score capture also
requires an official observed map and an entirely official request sequence.
Neither configured intent nor `loadedConfigFile` supplies a Workshop runtime
identity. No Workshop capability is advertised; existing `canLoadMatch` and
series capability continue to describe official-map operation.

## Upstream evidence

Inspected original MatchZy tag `0.9.2`, commit
`21c278070f9ef4eddcb0baf8f860a8e007271af6`:

- Its internal `ChangeMap` parses a signed .NET `long`. Numeric strings invoke
  `host_workshop_map`; other strings require `Server.IsMapValid` before
  `changelevel`. A canonical identifier does not enter the numeric branch.
  The protocol's 20-digit syntax also exceeds signed Int64 for some IDs.
  The same numeric command branch exists in its admin map/reload paths.
  [Utility.cs](https://github.com/shobhit-pathak/MatchZy/blob/21c278070f9ef4eddcb0baf8f860a8e007271af6/Utility.cs#L1017),
  [ConsoleCommands.cs](https://github.com/shobhit-pathak/MatchZy/blob/21c278070f9ef4eddcb0baf8f860a8e007271af6/ConsoleCommands.cs#L575).
- Match config validation accepts a nonempty map array without proving that its
  entries load. Setup schedules a map change, starts warmup and sets
  `isMatchSetup`; this acknowledgement does not prove a successful map change.
  Series transitions reuse `ChangeMap`. Mapping a canonical identifier to its
  numeric ID would discard the map component without establishing which local
  file the engine uses or whether it downloads content.
  [MatchManagement.cs](https://github.com/shobhit-pathak/MatchZy/blob/21c278070f9ef4eddcb0baf8f860a8e007271af6/MatchManagement.cs#L375),
  [series transition](https://github.com/shobhit-pathak/MatchZy/blob/21c278070f9ef4eddcb0baf8f860a8e007271af6/Utility.cs#L987).
- CounterStrikeSharp 378 exposes `Server.MapName` directly from the native map
  name call. The wrapper provides no canonical Workshop ID/file binding.
  [Server.cs](https://github.com/roflmuffin/CounterStrikeSharp/blob/v1.0.378/managed/CounterStrikeSharp.API/Server.cs#L71).
- Valve's FAQ describes subscription and client downloads, but does not establish
  the dedicated server's local file layout, command completion, or canonical
  runtime identity. Valve Developer Community's dedicated-server page returned
  HTTP 403 during this investigation; it was not treated as verified evidence.
  [Valve Workshop FAQ](https://www.counter-strike.net/workshop/workshopfaq),
  [unavailable dedicated-server reference](https://developer.valvesoftware.com/wiki/Counter-Strike_2/Dedicated_Servers).

## Integration requirements and live verification

Deploy the API before a discovery-capable agent, as described in
[WORKSHOP_MAPS.md](WORKSHOP_MAPS.md). Build this reporter with .NET 10 and the
pinned CSS API, stage its DLL and existing version-marker files beside host/agent
binaries, then install through the stopped-server reporter update flow. Reporter
0.3.15 does not make Workshop startup or rooms playable. Fresh `availableMaps`
discovery is insufficient to offer a working Workshop match-load action. A
separate backend/agent capability gate is required before advertising that action;
this plugin-scoped change adds no Go protocol field or frontend behavior.

Before removing the blocker, establish and test all of the following:

1. A real CS2-installed Workshop layout and content identity, including the
   relationship between published ID, selected map name and loaded package.
   The current `.bsp` discovery contract is unverified for CS2. Update Go discovery
   and reporter verification together if the actual layout differs.
2. A locally verified load mechanism for both startup and MatchZy BO1/BO3/BO5,
   with explicit policy for engine-initiated downloads. Verify supported numeric
   bounds and reject ambiguous multiple maps sharing a published ID if loading
   by ID cannot select between them. Do not simply forward the canonical path
   or strip its map component.
3. Fresh engine evidence identifying the canonical map, bound to the active
   setup, process/launch and local installation. Capture actual `Server.MapName`
   and A2S values. Never reconstruct identity from desired startup/room state or
   an ambiguous bare name. Test wrong IDs, same-name maps, stale observations,
   failed changes and plugin reloads. Require this evidence before acknowledging
   Workshop loading or publishing scores/results.
4. A versioned reporter → guard/agent → API capability and runtime-map contract,
   failing closed for older reporters, missing evidence and downgrades. Keep
   PostgreSQL authoritative and issue only fixed commands from validated values.
5. Real client joins and a completed match with scores, plus series transitions,
   watchdog continuity, failure recovery and official-map regressions. File
   fixtures and config acknowledgements cannot establish these results.

Automated checks cover grammar boundaries, missing/removed files, symlinks at
all directory levels and at the file, directories/FIFOs in place of files,
mixed series, explicit capability rejection and unavailable Workshop scores.
No live CS2 Workshop command, download, map transition, client join or result
was exercised. No running game or deployed instance was modified.

Validation on 2026-11-23: reporter build against the locally staged CSS API
completed with zero warnings/errors; the assert-based C# suite passed using the
installed `mcr.microsoft.com/dotnet/sdk:10.0-alpine` image. `go test -race ./...`
passed with a separate disposable PostgreSQL cluster supplied through
`TEST_DATABASE_URL`; frontend `npm run build --prefix web` passed its TypeScript
check and Vite build. The disposable database was stopped afterward. The parent
integration should update `docs/PROJECT.md` and the compatibility section of
`docs/WORKSHOP_MAPS.md`; those shared documents were outside this change's scope.
