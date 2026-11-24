# Preinstalled Workshop maps

Managed agents now report per-instance discovery through the Go protocol and
PostgreSQL-backed API. This is an integration slice, not verified CS2 Workshop
compatibility. No content is downloaded or installed by this feature.

A managed instance scans regular `.bsp` files directly under
`server/game/csgo/maps/workshop/<published-file-id>/` in its locally configured
installation. Map directory ancestors, published-file directories and files must
not be symlinks. Missing installations or map directories return a successful
empty scan; other errors omit discovery rather than report an empty scan.
Identifiers are sorted; paths never leave the agent.

Canonical identifiers are `workshop/<published-file-id>/<map>`. The ID is 1–20
ASCII decimal digits without a leading zero; the map name is 1–128 lowercase
ASCII letters, digits, underscores or hyphens without `.bsp`. Traversal,
whitespace, command delimiters and control characters are rejected.

## API and frontend contract

Each managed `Report`, including an entry in `MachineReport.servers`, can carry
`workshopMaps: string[]`. A successful scan sends an array, including `[]` for no
maps. Omission or JSON null means unsupported/failed discovery. The Go field is
`*[]string`, preserving the empty/absent distinction. Simulated and existing-service
agents omit the field and cannot submit discovery. Reports allow at most 128
unique canonical Workshop IDs and 8,192 total identifier bytes per instance.
The machine heartbeat body limit is 2 MiB; existing per-report bounds still apply.
An excessive local scan is omitted, never silently truncated.

`GET /api/state` and its live snapshots expose `hosts[].availableMaps: string[]`.
Authorized player state and live snapshots expose the same array on
`servers[].availableMaps`. It always contains the official maps, followed by
that instance's Workshop identifiers only while both the host heartbeat and
server-recorded scan receipt are less than 20 seconds old. Empty, absent, failed
or expired discovery yields official maps only. Missing instance reports also
invalidate discovery, without refreshing that instance's heartbeat. Foreign or
invalid reports reject the entire heartbeat transaction. Discovery contains no
host paths or credentials.

Use the selected host's `availableMaps` for startup and standalone room map
choices. Global `maps` remains the official pool; tournament pools and map votes
remain official-only. Keep saved draft selections visible if no longer available,
but do not present them as authorized. Reconcile forms from canonical snapshots;
never infer availability or lifecycle state from a previous successful command.

Startup changes still use `PATCH /api/hosts/{id}` with `map` and the current
`revision`; Start uses `desiredState: "running"` and `revision`. Standalone room
creation accepts a canonical `map` through the existing management/player create
endpoint. Standalone room settings use the existing settings endpoint with
`map`, `settingsRevision`, title, size and overtime fields. Malformed IDs receive
400. A syntactically valid map absent from the owning host's fresh inventory
receives 409. Existing permission, revision and action requirements also apply.

Availability is checked under the host lock in the same transaction as command
and room authorization. Room loading rechecks availability, and heartbeat
issuance rechecks every map in a saved match setup. A missing setup map fails
that pending load instead of sending it. Start/restart actions are unavailable
for a missing startup map. If its previously saved Workshop map is no longer
available at heartbeat issuance, the response sends Stop with a safe official
fallback; it preserves the saved map and desired state in PostgreSQL. This is a
fail-closed reconciliation fence, and can stop an active Workshop server after
failed discovery. Restore discovery or select an available map to recover.

Managed agents independently validate syntax and rescan the local installation
before writing startup configuration or a match setup. A stale API response
cannot authorize a removed file. The existing-service runner remains
official-only; simulation never claims managed file discovery.

## Compatibility limits

No real CS2 Workshop installation, storage layout, startup command or MatchZy
load has been verified. Discovery recognizes only the specified `.bsp` layout;
other formats/layouts are not discovered. Startup configuration uses the existing
`map <canonical-id>` command and requires real-engine verification. Files can
still change after a scan; local checks reduce stale-report risk but do not lock
Valve content against external mutation.

Reporter 0.3.15 validates canonical IDs and checks installed files, but explicitly
rejects Workshop loading before config writes or commands. The API suppresses
Workshop room load actions and rejects load requests with an explicit blocker.
Discovery does not establish a working load capability. See WORKSHOP_REPORTER.md
for upstream evidence and the required real-engine checks.

The current C# ClutchbaseAccess reporter independently restricts match requests,
MatchZy config construction and runtime map evidence to official maps. This
Go/SQL slice does not modify that reporter. Workshop room settings can persist,
but Workshop match loads will be rejected by that reporter until its validators
and engine loading semantics are updated and verified. A2S/runtime observations
must report a canonical ID to identify a Workshop map; an unrecognized engine
map name remains unavailable. Deploy the API before the updated agent because
older APIs reject the new report field. Older agents omit discovery, which
clears Workshop availability and prevents new Workshop starts/loads.
