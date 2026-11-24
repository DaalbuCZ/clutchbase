import { useEffect, useRef, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import { APIError, connection, request, presetName, presetStatus, tournamentMatchProgress, tournamentSeriesDecided, matchRoomStatus } from "./api";
import type { Host, Machine, MatchRoom, Workspace } from "./api";
import { connectLive } from "./live";
import { PlayerApp } from "./PlayerApp";
import { MatchHistory } from "./MatchHistory";
import { Statistics } from "./Statistics";
import { Leaderboard } from "./Leaderboard";
import { Reports } from "./Reports";
import { MachineResources } from "./MachineResources";
import { ReporterInventory } from "./ReporterInventory";
import { AdminServerFleet } from "./AdminServerFleet";
import { PlayerScore } from "./PlayerScore";
import { PlayerDirectory } from "./PlayerDirectory";
import { loadBlockerLabels } from "./LoadBlockers";
import { TeamDirectory } from "./TeamDirectory";
import { MatchLineups } from "./MatchLineups";
import { ShareRoom } from "./ShareRoom";
import { RoomReopen } from "./RoomReopen";
import { ShareAccess } from "./ShareAccess";
import { AccessRequests } from "./AccessRequests";
import { MatchShuffle } from "./MatchShuffle";
import { MatchMapVote } from "./MatchMapVote";
import { PauseVoting } from "./PauseVoting";
import { TournamentDispute } from "./TournamentDispute";
import { TournamentCorrection } from "./TournamentCorrection";
import { RoomLiveScore } from "./RoomLiveScore";
import { MatchFilter } from "./MatchFilter";
import { TournamentBracket } from "./TournamentBracket";
import { TournamentSchedule } from "./TournamentSchedule";
import { FixtureSchedule } from "./FixtureSchedule";
import { TournamentProgress } from "./TournamentProgress";
import { TournamentPicker } from "./TournamentPicker";
import { TournamentCancellation } from "./TournamentCancellation";
import { TournamentStandings } from "./TournamentStandings";
import { useView } from "./view";

const managementViews = ["overview", "servers", "matches", "tournaments", "schedule", "history", "players", "teams", "stats", "leaderboard", "reports", "access-requests"] as const;

function Mark() {
  return (
    <span className="mark" aria-hidden="true">
      c<span>↗</span>
    </span>
  );
}
function mapName(map: string) {
  return map.replace(/^de_/, "").replace(/^./, (s) => s.toUpperCase());
}
function matchLoadBlockers(match: MatchRoom, host?: Host): string[] {
  if (match.loadBlockers) return loadBlockerLabels(match.loadBlockers).length ? loadBlockerLabels(match.loadBlockers) : ["Waiting for the server to confirm that loading is safe."];
  const blockers: string[] = [];
  if (match.map.startsWith("workshop/")) blockers.push("Workshop match loading is not supported by the current reporter.");
  if (!host) blockers.push("Waiting for the server snapshot.");
  else {
    if (!host.online || host.status !== "running") blockers.push("The server must be online and running.");
    if (host.activePreset !== "match") blockers.push("Switch the server to Match mode.");
    if (host.accessStatus !== "verified") blockers.push("Wait for server access verification.");
    if (["setup", "live", "paused"].includes(host.matchPhase)) blockers.push("End the current MatchZy match before loading another setup.");
    else if (!host.canLoadMatch) blockers.push("The MatchZy setup reporter is not ready.");
    if (!host.canObserveMap || !host.activeMap) blockers.push("The current map is unavailable; update the machine agent to report it.");
    if (match.manualReady && !host.supportsManualReady) blockers.push("Update the host agent and reporter to use in-game ready.");
  }
  if (match.players === 0) blockers.push("At least one player must join the room.");
  if (match.ctPlayers === 0) blockers.push("At least one player must join CT.");
  if (match.tPlayers === 0) blockers.push("At least one player must join T.");
  if (match.ready < match.players) blockers.push(`${match.players - match.ready} player${match.players - match.ready === 1 ? "" : "s"} still need to ready up.`);
  return blockers.length ? blockers : ["Waiting for the server to confirm that loading is safe."];
}
function storageSize(bytes: number) {
  const unit = bytes >= 1024 ** 4 ? "TiB" : "GiB";
  return `${(bytes / (unit === "TiB" ? 1024 ** 4 : 1024 ** 3)).toFixed(1)} ${unit}`;
}
const message = (error: unknown) =>
  error instanceof Error ? error.message : "Something went wrong";

function hostAttentionReasons(host: Host): string[] {
  const reasons: string[] = [];
  if (host.status === "offline") reasons.push("Agent offline");
  if (host.status === "error") reasons.push("Server error");
  if (host.gameUpdateStatus === "failed") reasons.push("CS2 update failed");
  if (host.gameBuildStatus === "outdated") reasons.push("CS2 update available");
  if (host.reporterUpdateStatus === "failed") reasons.push("Plugin update failed");
  if (host.restartPhase === "failed") reasons.push("Restart failed");
  if (host.accessStatus === "blocked") reasons.push("Access verification blocked");
  return reasons;
}

export function App() {
  return window.location.pathname === "/manage" ? <ManagementApp /> : <PlayerApp />;
}

function ManagementApp() {
  const [token, setToken] = useState(
    () => localStorage.getItem("clutchbase-token") || sessionStorage.getItem("clutchbase-token") || "",
  );
  const [snapshot, setSnapshot] = useState<Workspace | null>(null);
  const session = useRef(token);
  session.current = token;
  const hosts = snapshot?.hosts || [];
  const maps = snapshot?.maps || [];
  const machines = snapshot?.machines || [];
  const matches = snapshot?.matches || [];
  const [matchFilter, setMatchFilter] = useState<"open" | "closed" | "all">("open");
  const [matchSearch, setMatchSearch] = useState("");
  const players = snapshot?.players || [];
  const [matchHostID, setMatchHostID] = useState("");
  const [createMachineID, setCreateMachineID] = useState("");
  const provisioningMachine = machines.find((machine) => machine.id === createMachineID && machine.createServer) || machines.find((machine) => machine.createServer);
  const [serverSearch, setServerSearch] = useState("");
  const [serverStatusFilter, setServerStatusFilter] = useState("all");
  const matchHosts = hosts.filter((host) => host.actions.openMatch);
  const selectedMatchHost = matchHosts.find((host) => host.id === matchHostID) || matchHosts[0];
  const accept = (next: Workspace) =>
    setSnapshot((previous) =>
      !previous || next.capturedAt >= previous.capturedAt ? next : previous,
    );
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [live, setLive] = useState(false);
  const unavailable = !live;
  const [adding, setAdding] = useState<"machine" | "server" | "match" | "tournament" | null>(null);
  const [busy, setBusy] = useState("");
  const [winnerSelections, setWinnerSelections] = useState<Record<string, string>>({});
  const [confirmRemoval, setConfirmRemoval] = useState("");
  const [confirmRestart, setConfirmRestart] = useState<{ id: string; revision: number } | null>(null);
  const [confirmUpdate, setConfirmUpdate] = useState<{ id: string; revision: number } | null>(null);
  const [confirmCloseMatch, setConfirmCloseMatch] = useState("");
  const [confirmEndMatch, setConfirmEndMatch] = useState("");
  const [editingMatch, setEditingMatch] = useState<MatchRoom | null>(null);
  const [editingResult, setEditingResult] = useState<MatchRoom | null>(null);
  const [view, setView, selectedEntityID] = useView(managementViews, "overview");
  const selectedRoomID = view === "matches" ? selectedEntityID : "";
  const selectedServerID = view === "servers" ? selectedEntityID : "";
  const selectedServer = hosts.find((host) => host.id === selectedServerID);
  const [serverSection, setServerSection] = useState("setup");
  const previousServer = useRef("");
  useEffect(() => {
    setServerSection("setup");
    setConfirmRemoval("");
    setConfirmUpdate(null);
    setRoster(null);
  }, [selectedServerID]);
  useEffect(() => {
    if (view !== "servers") return;
    if (selectedServerID) {
      previousServer.current = selectedServerID;
      const card = document.getElementById(`server-detail-${selectedServerID}`);
      card?.focus(); card?.scrollIntoView?.({ block: "start" });
    } else if (previousServer.current) {
      (document.getElementById(`view-server-${previousServer.current}`) || document.getElementById("find-server"))?.focus();
    }
  }, [view, selectedServerID, !!selectedServer]);
  const selectedTournamentID = view === "tournaments" ? selectedEntityID : "";
  const selectedTournament = snapshot?.tournaments?.find((tournament) => tournament.id === selectedTournamentID);
  useEffect(() => {
    if (view === "tournaments" && selectedTournament) {
      const card = document.getElementById(`tournament-${selectedTournamentID}`);
      card?.focus(); card?.scrollIntoView?.({ block: "start" });
    }
  }, [view, selectedTournamentID, !!selectedTournament]);
  const selectedRoom = matches.find((match) => match.id === selectedRoomID);
  const openMatches = matches.filter((match) => match.status === "open");
  const closedMatches = matches.filter((match) => match.status === "closed");
  const filteredMatches = matchFilter === "open" ? openMatches : matchFilter === "closed" ? closedMatches : matches;
  const visibleMatches = (selectedRoomID ? matches.filter((match) => match.id === selectedRoomID || filteredMatches.includes(match)) : filteredMatches).filter((match) => match.id === selectedRoomID || `${match.title} ${match.hostName} ${match.map} ${(match.participants || []).map((p) => p.gameName).join(" ")}`.toLocaleLowerCase().includes(matchSearch.trim().toLocaleLowerCase()));

  useEffect(() => {
    if (view === "matches" && selectedRoom) setMatchFilter(selectedRoom.status);
  }, [view, selectedRoomID, selectedRoom?.status]);
  useEffect(() => {
    if (view !== "matches" || !selectedRoomID || !selectedRoom) return;
    const card = document.getElementById(`match-room-${selectedRoomID}`);
    card?.focus();
    card?.scrollIntoView?.({ block: "center" });
  }, [view, selectedRoomID, matchFilter, !!selectedRoom]);
  const [resultError, setResultError] = useState("");
  const [matchEditError, setMatchEditError] = useState("");
  const currentEditedMatch = matches.find((match) => match.id === editingMatch?.id);
  const canSaveMatch = !!currentEditedMatch?.canEdit && currentEditedMatch.settingsRevision === editingMatch?.settingsRevision;
  const currentResultMatch = matches.find((match) => match.id === editingResult?.id);
  const canSaveResult = !!currentResultMatch?.canEditResult && currentResultMatch.resultRevision === editingResult?.resultRevision;
  const [roster, setRoster] = useState<{ id: string; revision: number; text: string } | null>(null);
  const [notice, setNotice] = useState("");
  const [joinPasswords, setJoinPasswords] = useState<Record<string, string>>({});
  const [loadingPassword, setLoadingPassword] = useState("");
  const passwordSession = useRef(0);
  const [credentials, setCredentials] = useState<{
    machine: Machine;
    agentToken: string;
    rotated?: boolean;
  } | null>(null);
  const [rotationTarget, setRotationTarget] = useState<Machine | null>(null);
  const [rotationError, setRotationError] = useState("");

  function signOut() {
    passwordSession.current += 1;
    session.current = "";
    localStorage.removeItem("clutchbase-token");
    sessionStorage.removeItem("clutchbase-token");
    setToken("");
    setSnapshot(null);
    setCredentials(null);
    setJoinPasswords({});
    setLoadingPassword("");
    setRotationTarget(null);
    setAdding(null);
    setEditingMatch(null);
    setEditingResult(null);
    setView("servers");
    setConfirmRemoval("");
    setRoster(null);
  }
  async function refresh() {
    const next = await request<Workspace>(token, "/state");
    if (session.current === token) accept(next);
  }
  useEffect(() => {
    if (!token) return;
    setLoading(true);
    setLive(false);
    return connectLive<Workspace>("manage", token,
      (next) => {
        accept(next);
        setLoading(false);
        setError("");
      },
      (online) => {
        setLive(online);
        if (!online) {
          setLoading(false);
          setError("Live connection lost. Reconnecting…");
        }
      },
      () => {
        signOut();
        setError("Administrator token rejected. Check CLUTCHBASE_ADMIN_TOKEN in the Komodo stack environment, then try again.");
      },
    );
  }, [token]);
  useEffect(() => {
    if (!notice) return;
    const t = setTimeout(() => setNotice(""), 4000);
    return () => clearTimeout(t);
  }, [notice]);

  async function update(host: Host, patch: Record<string, string | string[] | boolean>, remove = false) {
    setBusy(host.id);
    setError("");
    try {
      await request(token, `/hosts/${host.id}`, {
        method: remove ? "DELETE" : "PATCH",
        body: JSON.stringify({ ...patch, revision: host.revision }),
      });
      setConfirmRemoval("");
      setRoster(null);
      await refresh();
    } catch (err) {
      if (err instanceof APIError && err.status === 409) {
        try {
          await refresh();
        } catch {
          /* Preserve the original action error. */
        }
      }
      setError(message(err));
    } finally {
      setBusy("");
    }
  }
  async function restart(host: Host, revision: number) {
    setBusy(host.id); setError("");
    try {
      await request(token, `/hosts/${host.id}/restart`, { method: "POST", body: JSON.stringify({ revision }) });
      setConfirmRestart(null);
      await refresh();
    } catch (err) {
      if (err instanceof APIError && err.status === 409) { try { await refresh(); } catch { /* Keep the original error. */ } }
      setError(message(err));
    } finally { setBusy(""); }
  }
  async function switchMode(host: Host, mode: "match" | "practice") {
    setBusy(host.id);
    setError("");
    try {
      await request(token, `/hosts/${host.id}/mode`, {
        method: "POST",
        body: JSON.stringify({ mode, revision: host.revision }),
      });
      await refresh();
    } catch (err) {
      if (err instanceof APIError && err.status === 409) {
        try { await refresh(); } catch { /* Preserve the action error. */ }
      }
      setError(message(err));
    } finally { setBusy(""); }
  }

  async function installReporter(host: Host) {
    setBusy(host.id); setError("");
    try {
      await request(token, `/hosts/${host.id}/reporter-update`, { method: "POST", body: JSON.stringify({ revision: host.revision }) });
      await refresh();
    } catch (err) { setError(message(err)); }
    finally { setBusy(""); }
  }
  async function updateGame(host: Host, stopFirst = false, revision = host.revision) {
    setBusy(host.id); setError("");
    try {
      await request(token, `/hosts/${host.id}/game-update`, { method: "POST", body: JSON.stringify({ revision, ...(stopFirst ? { stopFirst: true } : {}) }) });
      setConfirmUpdate(null);
      await refresh();
    } catch (err) { setError(message(err)); }
    finally { setBusy(""); }
  }
  async function cancelGameUpdate(host: Host) {
    setBusy(host.id); setError("");
    try { await request(token, `/hosts/${host.id}/game-update/cancel`, { method: "POST", body: JSON.stringify({ revision: host.revision }) }); await refresh(); }
    catch (err) { if (err instanceof APIError && err.status === 409) { try { await refresh(); } catch { /* Keep the original error. */ } } setError(message(err)); }
    finally { setBusy(""); }
  }
  async function controlMatch(host: Host, action: "pause" | "resume") {
    setBusy(host.id); setError("");
    try {
      await request(token, `/hosts/${host.id}/match-control`, {
        method: "POST", body: JSON.stringify({ action, revision: host.revision }),
      });
      await refresh();
    } catch (err) {
      if (err instanceof APIError && err.status === 409) {
        try { await refresh(); } catch { /* Preserve the action error. */ }
      }
      setError(message(err));
    } finally { setBusy(""); }
  }
  async function configurePauseVoting(match: MatchRoom, threshold: number) {
    setBusy(match.id); setError("");
    try {
      await request(token, `/matches/${match.id}/pause-voting`, { method: "PATCH", body: JSON.stringify({ threshold, revision: match.controlRevision }) });
      await refresh();
    } catch (err) {
      if (err instanceof APIError && err.status === 409) { try { await refresh(); } catch { /* Keep the original error. */ } }
      setError(message(err));
    } finally { setBusy(""); }
  }
  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text);
      setNotice("Copied to clipboard");
    } catch {
      setError(
        "Clipboard access is unavailable. Select and copy the displayed text.",
      );
    }
  }
  async function showJoinPassword(host: Host) {
    const requestSession = passwordSession.current;
    setLoadingPassword(host.id);
    setError("");
    try {
      const result = await request<{ password: string }>(token, `/hosts/${host.id}/join-password`);
      if (passwordSession.current === requestSession) setJoinPasswords((current) => ({ ...current, [host.id]: result.password }));
    } catch (err) {
      if (passwordSession.current === requestSession) setError(message(err));
    } finally {
      if (passwordSession.current === requestSession) setLoadingPassword("");
    }
  }
  async function addMachine(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    setBusy("create");
    setError("");
    try {
      const result = await request<{ machine: Machine; agentToken: string }>(
        token,
        "/machines",
        {
          method: "POST",
          body: JSON.stringify({
            name: form.get("name"),
            address: form.get("address"),
          }),
        },
      );
      setCredentials(result);
      setAdding(null);
      await refresh();
    } catch (err) {
      setError(message(err));
    } finally {
      setBusy("");
    }
  }

  async function rotateMachineToken(machine: Machine) {
    setBusy(`rotate:${machine.id}`);
    setRotationError("");
    try {
      const result = await request<{ agentToken: string }>(token, `/machines/${machine.id}/rotate-token`, { method: "POST" });
      if (session.current !== token) return;
      setRotationTarget(null);
      setCredentials({ machine, agentToken: result.agentToken, rotated: true });
    } catch (err) {
      setRotationError(`${message(err)}. If the agent disconnects, the request may have succeeded; replace the token again to get a new one.`);
    } finally {
      setBusy("");
    }
  }

  async function addServer(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    setBusy("create");
    setError("");
    try {
      await request<Host>(token, `/machines/${form.get("machine")}/servers`, {
        method: "POST",
        body: JSON.stringify({ name: form.get("name"), map: form.get("map"), port: Number(form.get("port")) }),
      });
      setAdding(null);
      await refresh();
    } catch (err) { setError(message(err)); }
    finally { setBusy(""); }
  }

  async function addMatch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    setBusy("create"); setError("");
    try {
      await request<MatchRoom>(token, "/matches", { method: "POST", body: JSON.stringify({ hostId: form.get("host"), title: form.get("title"), map: form.get("map") || "", playersPerSide: Number(form.get("playersPerSide")), overtime: form.get("overtime") !== null, manualReady: form.get("manualReady") !== null }) });
      setAdding(null); await refresh();
    } catch (err) { setError(message(err)); }
    finally { setBusy(""); }
  }

  async function addTournament(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); const form = new FormData(event.currentTarget); setBusy("create"); setError("");
    try { await request(token, "/tournaments", { method: "POST", body: JSON.stringify({ name: form.get("name"), playersPerSide: Number(form.get("playersPerSide")), bestOf: Number(form.get("bestOf")), format: form.get("format"), mapPool: form.getAll("mapPool") }) }); setAdding(null); await refresh(); }
    catch (err) { setError(message(err)); } finally { setBusy(""); }
  }

  async function tournamentAction(path: string, options: RequestInit) {
    setBusy(path); setError("");
    try { await request(token, path, options); await refresh(); return true; }
    catch (err) { setError(message(err)); return false; } finally { setBusy(""); }
  }

  async function closeMatch(match: MatchRoom) {
    setBusy(match.id); setError("");
    try {
      await request(token, `/matches/${match.id}`, { method: "PATCH", body: JSON.stringify({ status: "closed", confirmLoaded: match.loadStatus === "complete" && confirmCloseMatch === match.id }) });
      setConfirmCloseMatch("");
      await refresh();
    } catch (err) { setError(message(err)); }
    finally { setBusy(""); }
  }

  async function saveResult(match: MatchRoom, scores: { ct: number; t: number } | null) {
    setBusy(match.id); setResultError("");
    try {
      await request(token, `/matches/${match.id}/result`, { method: "PATCH", body: JSON.stringify({ revision: match.resultRevision, ...(scores || { clear: true }) }) });
      setEditingResult(null);
      await refresh();
    } catch (err) {
      if (err instanceof APIError && err.status === 409) { try { await refresh(); } catch { /* Keep the original error. */ } }
      setResultError(message(err));
    } finally { setBusy(""); }
  }

  async function endMatch(match: MatchRoom) {
    const host = hosts.find((item) => item.id === match.hostId);
    if (!host) return;
    setBusy(match.id); setError("");
    try { await request(token, `/matches/${match.id}/end`, { method: "POST", body: JSON.stringify({ revision: host.revision }) }); setConfirmEndMatch(""); await refresh(); }
    catch (err) { setError(message(err)); }
    finally { setBusy(""); }
  }

  async function cancelMatchLoad(match: MatchRoom) {
    const host = hosts.find((item) => item.id === match.hostId);
    if (!host) return;
    setBusy(match.id); setError("");
    try { await request(token, `/matches/${match.id}/cancel-load`, { method: "POST", body: JSON.stringify({ revision: host.revision }) }); await refresh(); }
    catch (err) { setError(message(err)); }
    finally { setBusy(""); }
  }

  async function loadMatch(match: MatchRoom) {
    setBusy(match.id); setError("");
    try { await request(token, `/matches/${match.id}/load`, { method: "POST" }); await refresh(); }
    catch (err) { setError(message(err)); }
    finally { setBusy(""); }
  }

  async function removeRoomPlayer(match: MatchRoom, steamId: string) {
    setBusy(match.id); setError("");
    try {
      await request(token, `/matches/${match.id}/players/${steamId}`, { method: "DELETE" });
      await refresh();
      setNotice("Player removed from the room. Remaining players must mark ready again.");
    } catch (err) { setError(message(err)); }
    finally { setBusy(""); }
  }

  async function chooseVotedMap(match: MatchRoom, map: string) {
    setBusy(match.id); setError("");
    try {
      const result = await request<{ readinessReset: boolean }>(token, `/matches/${match.id}/settings`, { method: "PATCH", body: JSON.stringify({ title: match.title, map, playersPerSide: match.playersPerSide, overtime: match.overtime, manualReady: !!match.manualReady, settingsRevision: match.settingsRevision }) });
      setNotice(result.readinessReset ? "Map selected. Players must mark ready again." : "Map selected.");
    } catch (err) { setError(message(err)); }
    finally { setBusy(""); }
  }
  async function saveMatchSettings(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!editingMatch) return;
    const form = new FormData(event.currentTarget);
    setBusy(editingMatch.id); setMatchEditError("");
    try {
      const result = await request<{ readinessReset: boolean }>(token, `/matches/${editingMatch.id}/settings`, {
        method: "PATCH",
        body: JSON.stringify({ title: form.get("title"), map: form.get("map"), playersPerSide: Number(form.get("playersPerSide")), overtime: form.get("overtime") !== null, manualReady: form.get("manualReady") !== null, settingsRevision: editingMatch.settingsRevision }),
      });
      setEditingMatch(null);
      setNotice(result.readinessReset ? "Room settings saved. Players must mark ready again." : "Room settings saved.");
      await refresh();
    } catch (err) { setMatchEditError(message(err)); }
    finally { setBusy(""); }
  }

  if (!token)
    return (
      <main className="login">
        <div className="login-card">
          <Mark />
          <h1>Manage Clutchbase</h1>
          <a className="small muted" href="/play">Back to Play</a>
          <form
            onSubmit={(event) => {
              event.preventDefault();
              const value = String(
                new FormData(event.currentTarget).get("token") || "",
              ).trim();
              if (value) {
                setError("");
                setSnapshot(null);
                setLoading(true);
                localStorage.setItem("clutchbase-token", value);
                sessionStorage.removeItem("clutchbase-token");
                setToken(value);
              }
            }}
          >
            <label>
              Deployment administrator token
              <input
                name="token"
                type="password"
                required
                autoComplete="off"
                placeholder="CLUTCHBASE_ADMIN_TOKEN"
              />
            </label>
            <button className="primary" type="submit">
              Open control room <span>↗</span>
            </button>
          </form>
          <p className="small muted">Find <code>CLUTCHBASE_ADMIN_TOKEN</code> in the Komodo stack’s Environment settings. The machine-agent token is different. This token stays in this browser until you sign out.</p>
          {error && (
            <p className="error" role="alert">
              {error}
            </p>
          )}
        </div>
      </main>
    );

  const running = hosts.filter(
    (h) => h.status === "running",
  ).length;
  const attentionHosts = hosts.filter((host) => hostAttentionReasons(host).length > 0);
  const disconnectedMachines = machines.filter((machine) => !machine.online && !!machine.lastSeen);
  const filteredHosts = hosts.filter((host) => {
    const query = serverSearch.trim().toLocaleLowerCase();
    const machineName = machines.find((machine) => machine.id === host.machineId)?.name || "";
    const matchesSearch = !query || [host.name, host.address, host.map, host.activeMap, machineName]
      .some((value) => value.toLocaleLowerCase().includes(query));
    const matchesStatus = serverStatusFilter === "all"
      || (serverStatusFilter === "online" && host.status === "running")
      || (serverStatusFilter === "stopped" && (host.status === "stopped" || host.status === "uninstalled"))
      || (serverStatusFilter === "progress" && ["installing", "starting", "stopping", "removing"].includes(host.status))
      || (serverStatusFilter === "attention" && hostAttentionReasons(host).length > 0);
    return matchesSearch && matchesStatus;
  });
  const agentCommand = credentials
    ? `CLUTCHBASE_API_URL='${import.meta.env.DEV ? "http://127.0.0.1:8080" : window.location.origin}' \\\nCLUTCHBASE_MACHINE_ID='${credentials.machine.id}' \\\nCLUTCHBASE_AGENT_TOKEN='${credentials.agentToken}' \\\n./bin/clutchbase-agent --machine`
    : "";
  const activeMatches = matches.filter((match) => match.status === "open" && match.loadStatus === "complete" && !match.endedAt && !match.setupUnloaded);
  const waitingMatches = matches.filter((match) => match.status === "open" && match.loadStatus === "" && !match.canLoad);
  const requiresAttention = attentionHosts.length + disconnectedMachines.length;

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <a className="brand" href="/play" aria-label="Clutchbase home">
          <Mark />
          <span>
            clutchbase<span className="brand-dot">.</span>
          </span>
        </a>
        <a className="nav-item" href="/play">Play</a>
        <nav aria-label="Management pages">
          <button className={`nav-item nav-button ${view === "overview" ? "selected" : ""}`} aria-current={view === "overview" ? "page" : undefined} onClick={() => setView("overview")}>Overview</button>
          <button className={`nav-item nav-button ${view === "servers" ? "selected" : ""}`} aria-current={view === "servers" ? "page" : undefined} onClick={() => setView("servers")}>Servers <span className="count">{hosts.length}</span></button>
          <button className={`nav-item nav-button ${view === "matches" ? "selected" : ""}`} aria-current={view === "matches" ? "page" : undefined} onClick={() => setView("matches")}>Matches <span className="count">{matches.length}</span></button>
          <button className={`nav-item nav-button ${view === "tournaments" ? "selected" : ""}`} aria-current={view === "tournaments" ? "page" : undefined} onClick={() => setView("tournaments")}>Tournaments <span className="count">{snapshot?.tournaments?.length || 0}</span></button>
          <button className={`nav-item nav-button ${view === "schedule" ? "selected" : ""}`} aria-current={view === "schedule" ? "page" : undefined} onClick={() => setView("schedule")}>Schedule</button>
          <button className={`nav-item nav-button ${view === "history" ? "selected" : ""}`} aria-current={view === "history" ? "page" : undefined} onClick={() => setView("history")}>Match history</button>
          <button className={`nav-item nav-button ${view === "players" ? "selected" : ""}`} aria-current={view === "players" ? "page" : undefined} onClick={() => setView("players")}>Players <span className="count">{players.length}</span></button>
          <button className={`nav-item nav-button ${view === "teams" ? "selected" : ""}`} aria-current={view === "teams" ? "page" : undefined} onClick={() => setView("teams")}>Teams <span className="count">{snapshot?.teams?.length || 0}</span></button>
          <button className={`nav-item nav-button ${view === "stats" ? "selected" : ""}`} aria-current={view === "stats" ? "page" : undefined} onClick={() => setView("stats")}>Statistics</button>
          <button className={`nav-item nav-button ${view === "leaderboard" ? "selected" : ""}`} aria-current={view === "leaderboard" ? "page" : undefined} onClick={() => setView("leaderboard")}>Leaderboard</button>
          <button className={`nav-item nav-button ${view === "reports" ? "selected" : ""}`} aria-current={view === "reports" ? "page" : undefined} onClick={() => setView("reports")}>Reports</button>
          <button className={`nav-item nav-button ${view === "access-requests" ? "selected" : ""}`} aria-current={view === "access-requests" ? "page" : undefined} onClick={() => setView("access-requests")}>Access requests</button>
        </nav>
        <button className="text-button logout" onClick={signOut}>
          ↪ Sign out
        </button>
      </aside>
      <div className="workspace">
        <main className="content" id="servers">
          {error && <div className="error banner" role="alert">{error}</div>}
          {view === "overview" && <section className="control-overview" aria-labelledby="overview-heading">
            <div className="page-heading">
              <div><h1 id="overview-heading">Control room</h1><p className="small muted">{live ? "Live status from the server" : "Reconnecting to server state…"}{snapshot && ` · Updated ${new Date(snapshot.capturedAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`}</p></div>
              <div className="heading-actions"><button className="secondary" disabled={loading || unavailable} onClick={() => setAdding("machine")}>Connect machine</button><button className="primary" disabled={loading || unavailable || !machines.some((machine) => machine.createServer)} onClick={() => setAdding("server")}>＋ Create server</button></div>
            </div>
            <div className="overview-metrics" aria-label="Current server state">
              <button className="overview-metric" onClick={() => setView("servers")}><span>Servers running</span><strong>{running}<small> / {hosts.length}</small></strong><em>View server fleet →</em></button>
              <button className="overview-metric" onClick={() => { setMatchFilter("open"); setView("matches"); }}><span>Live matches</span><strong>{activeMatches.length}</strong><em>{matches.filter((match) => match.status === "open").length} open rooms · View matches →</em></button>
              <button className={`overview-metric ${requiresAttention ? "overview-metric-alert" : ""}`} onClick={() => { setServerSearch(""); setServerStatusFilter("attention"); setView("servers"); }}><span>Needs attention</span><strong>{requiresAttention}</strong><em>{requiresAttention ? "Review servers and agents →" : "All clear"}</em></button>
              <button className="overview-metric" onClick={() => setView("tournaments")}><span>Tournaments</span><strong>{snapshot?.tournaments?.filter((tournament) => tournament.status === "active").length || 0}</strong><em>{snapshot?.teams?.length || 0} teams · View tournaments →</em></button>
            </div>
            <div className="overview-columns">
              <section className="overview-panel" aria-labelledby="match-activity-heading">
                <div className="overview-panel-heading"><div><h2 id="match-activity-heading">Match activity</h2><p className="small muted">Ready rooms and loaded games from the current snapshot.</p></div><button className="text-button" onClick={() => { setMatchFilter("open"); setView("matches"); }}>All matches →</button></div>
                {matches.filter((match) => match.status === "open").slice(0, 5).map((match) => <button className="overview-match" key={match.id} onClick={() => setView("matches", match.id)}>
                  <span className={`overview-match-state ${match.loadStatus === "complete" && !match.setupUnloaded ? "is-live" : match.loadStatus === "failed" ? "is-error" : ""}`} aria-hidden="true" />
                  <span className="overview-match-main"><strong>{match.title}</strong><small>{match.hostName} · {mapName(match.map)} · CT {match.ctPlayers}/{match.playersPerSide} · T {match.tPlayers}/{match.playersPerSide}</small></span>
                  <span className="overview-match-action">{match.setupUnloaded ? "Setup ended" : match.canLoad ? "Ready to load" : match.loadStatus === "complete" ? "Loaded" : match.loadStatus === "failed" ? "Load failed" : "Room open"}<b aria-hidden="true">›</b></span>
                </button>)}
                {matches.filter((match) => match.status === "open").length === 0 && <div className="overview-empty"><strong>No open match rooms</strong><p>Open a room when a Match server is ready for your group.</p><button className="secondary" disabled={!live || matchHosts.length === 0} onClick={() => setAdding("match")}>＋ Open match room</button></div>}
                {waitingMatches.length > 0 && <p className="overview-note">{waitingMatches.length} room{waitingMatches.length === 1 ? " is" : "s are"} waiting for players or server readiness.</p>}
              </section>
              <section className="overview-panel" aria-labelledby="operations-heading">
                <div className="overview-panel-heading"><div><h2 id="operations-heading">Operations</h2><p className="small muted">Your fleet at a glance.</p></div><button className="text-button" onClick={() => setView("servers")}>Manage servers →</button></div>
                {machines.slice(0, 4).map((machine) => <div className="overview-machine" key={machine.id}><span className={`overview-machine-dot ${machine.online ? "is-online" : ""}`} /><span><strong>{machine.name}</strong><small>{machine.online ? "Agent connected" : machine.lastSeen ? "Agent disconnected" : "Waiting for agent"}</small></span><span className="small muted">{hosts.filter((host) => host.machineId === machine.id).length} servers</span><MachineResources machine={machine} live={live} compact /></div>)}
                {machines.length === 0 && <div className="overview-empty"><strong>Connect your first machine</strong><p>One agent manages the CS2 servers on a Linux host.</p><button className="primary" disabled={!live} onClick={() => setAdding("machine")}>Connect machine</button></div>}
                {requiresAttention > 0 && <button className="overview-attention" onClick={() => { setServerSearch(""); setServerStatusFilter("attention"); setView("servers"); }}><strong>{requiresAttention} item{requiresAttention === 1 ? "" : "s"} need attention</strong><span>See the reported issue and next action →</span></button>}
              </section>
            </div>
          </section>}
          {view === "servers" && <>
          <div className="page-heading">
            <div>
              <h1>{selectedServerID ? selectedServer?.name || "Server" : "Servers"}</h1>
              <p className="small muted" role="status">{live ? "Live server state" : loading ? "Connecting to server state…" : "Connection lost · showing last received server state"}</p>
            </div>
            <div className="heading-actions">
              <button className="secondary" disabled={loading || unavailable} onClick={() => setAdding("machine")}>Connect machine</button>
              <button className="primary" disabled={loading || unavailable || !machines.some((m) => m.createServer)} onClick={() => setAdding("server")}>＋ Create server</button>
            </div>
          </div>
          {selectedServerID && <div className="server-detail-nav"><button className="secondary" onClick={() => setView("servers")}>← Back to servers</button></div>}
          {!selectedServerID && <>
          <div className="stats">
            <div>
              <span className="stat-label">Servers</span>
              <strong>{hosts.length.toString().padStart(2, "0")}</strong>
            </div>
            <div>
              <span className="stat-label">Running</span>
              <strong>
                {running.toString().padStart(2, "0")}
                <i className="tiny-dot" />
              </strong>
            </div>
            <div>
              <span className="stat-label">Agents online</span>
              <strong>
                {(machines.filter((m) => m.online).length + hosts.filter((h) => !h.machineId && h.online).length).toString()
                  .padStart(2, "0")}
              </strong>
            </div>
            <div>
              <span className="stat-label">Servers needing attention</span>
              <button className="stat-action" type="button" disabled={attentionHosts.length === 0} onClick={() => setServerStatusFilter("attention")}>
                <strong>{attentionHosts.length.toString().padStart(2, "0")}</strong><span>{attentionHosts.length ? "View servers →" : "All clear"}</span>
              </button>
            </div>
          </div>
          {(attentionHosts.length > 0 || disconnectedMachines.length > 0) && <section className="attention-panel" aria-labelledby="attention-heading">
            <div className="attention-panel-head"><div><h2 id="attention-heading">Needs attention</h2><p className="small muted">Reported issues from the latest machine heartbeat.</p></div>{attentionHosts.length > 0 && <button className="secondary" onClick={() => { setServerSearch(""); setServerStatusFilter("attention"); }}>Show servers</button>}</div>
            {attentionHosts.length > 0 && <ul>{attentionHosts.slice(0, 5).map((host) => <li key={host.id}><button className="text-button" onClick={() => setView("servers", host.id)}>{host.name}</button><span>{hostAttentionReasons(host).join(" · ")}</span></li>)}</ul>}
            {attentionHosts.length > 5 && <p className="small muted">And {attentionHosts.length - 5} more servers.</p>}
            {disconnectedMachines.length > 0 && <p className="small attention-agent-warning">{disconnectedMachines.map((machine) => machine.name).join(", ")} · {disconnectedMachines.length === 1 ? "machine agent is" : "machine agents are"} disconnected.</p>}
          </section>}
          {machines.length > 0 && (
            <div className="machine-list">
              {machines.map((m) => (
                <div className="machine-row" key={m.id}>
                  <div><strong>{m.name}</strong> <span className="muted">{m.address}</span></div>
                  <span className="small muted">{m.online ? (m.driver === "simulated" ? "Simulated agent connected" : "Agent connected") : m.lastSeen ? "Agent disconnected" : "Waiting for agent"}</span>
                  {m.online && m.driver === "managed" && <span className="small muted">{m.storage ? `${storageSize(m.storage.availableBytes)} available of ${storageSize(m.storage.totalBytes)} · server storage` : "Server storage unavailable"}</span>}
                  <MachineResources machine={m} live={live} />
                  <button className="text-button" disabled={unavailable || !!busy} onClick={() => { setRotationError(""); setRotationTarget(m); }}>Replace agent token</button>
                </div>
              ))}
            </div>
          )}
          {hosts.length > 0 && <div className="fleet-filters" role="search" aria-label="Filter servers">
            <label>Find server<input id="find-server" type="search" value={serverSearch} onChange={(event) => setServerSearch(event.target.value)} placeholder="Name, address, map or machine" /></label>
            <label>Status<select value={serverStatusFilter} onChange={(event) => setServerStatusFilter(event.target.value)}>
              <option value="all">All statuses</option><option value="online">Online</option><option value="stopped">Stopped or needs install</option><option value="progress">In progress</option><option value="attention">Needs attention</option>
            </select></label>
            <span className="small muted" role="status">{filteredHosts.length} of {hosts.length} servers</span>
          </div>}
          </>}
          {loading ? (
            <div className="empty" role="status">
              Connecting to your control room…
            </div>
          ) : selectedServerID && !selectedServer ? (
            <p role="status">This server is not available in the current snapshot.</p>
          ) : !selectedServerID && hosts.length === 0 ? (
            <div className="empty">
              <div className="empty-icon">＋</div>
              <p className="muted">
                {machines.some((m) => m.createServer) ? "Create a server on your connected machine. CS2 installs when you start it." : "Connect a Linux machine once, then create servers here."}
              </p>
              <button className="primary" disabled={unavailable} onClick={() => setAdding(machines.some((m) => m.createServer) ? "server" : "machine")}>
                {machines.some((m) => m.createServer) ? "Create server" : "Connect machine"} <span>↗</span>
              </button>
            </div>
          ) : !selectedServerID && filteredHosts.length === 0 ? (
            <div className="empty"><p className="muted">No servers match these filters.</p><button className="text-button" onClick={() => { setServerSearch(""); setServerStatusFilter("all"); }}>Clear search and filters</button></div>
          ) : !selectedServerID ? <AdminServerFleet hosts={filteredHosts} machines={machines} live={live} onSelect={(id) => setView("servers", id)} /> : (
            <div className="server-detail-grid">
              {(selectedServer ? [selectedServer] : []).map((host) => {
                const state = host.status;
                const activeRoom = matches.find((match) => match.hostId === host.id && match.status === "open" && match.loadStatus === "complete" && !match.endedAt && !match.setupUnloaded);
                const openRoom = matches.find((match) => match.hostId === host.id && match.status === "open");
                const observing = host.online && host.desiredState === "running" && (host.observedState === "running" || host.observedState === "starting");
                const displayMap = observing ? (host.canObserveMap || host.driver === "simulated" ? host.activeMap : "") : host.map;
                const transitioning =
                  state === "starting" ||
                  state === "stopping" ||
                  state === "installing";
                const stateName: Record<string, string> = {
                  uninstalled: "Needs install",
                  installing: "Installing",
                  starting: "Starting",
                  running: "Online",
                  stopping: "Stopping",
                  stopped: "Stopped",
                  offline: "Agent offline",
                  error: "Error",
                  removing: "Removing",
                };
                return (
                  <article className="server-card" id={`server-detail-${host.id}`} tabIndex={-1} key={host.id}>
                    <div className="server-body">
                      <div className="server-card-head">
                        <div className="server-title">
                          <span className="small muted">{machines.find((machine) => machine.id === host.machineId)?.name || "Server status"}</span>
                          {host.driver === "simulated" && <span className="simulation">SIMULATED</span>}
                        </div>
                        <span className={`state-pill state-${state}`}><i className="tiny-dot" />{stateName[state] || state}</span>
                      </div>
                      <p className="server-meta">{displayMap ? mapName(displayMap) : "Current map unavailable"}{host.players != null && host.maxPlayers != null && state === "running" ? ` · ${host.players}/${host.maxPlayers} players` : ""}</p>
                      <div className="connection">
                        <code>{connection(host)}</code>
                        <button
                          aria-label={`Copy connection for ${host.name}`}
                          className="text-button"
                          onClick={() =>
                            void copy(`connect ${connection(host)}`)
                          }
                        >
                          Copy ↗
                        </button>
                      </div>
                      {host.gameUpdateStatus === "pending" && <p className="small muted" role="status">{host.observedState === "stopped" ? "CS2 update requested. Waiting for the machine to finish." : "Stop requested. The CS2 update will start after the agent reports the server stopped."} Server changes are unavailable until the update finishes.</p>}
                      {host.gameUpdateStatus === "pending" && host.online && host.gameUpdateSupported === false && <p className="small error" role="status">Reconnect a machine agent that supports CS2 updates to finish this request.</p>}
                      {host.gameUpdateStatus === "cancelled" && <p className="small muted" role="status">CS2 update cancelled before delivery. The server is still requested to stop.</p>}
                      {host.reporterUpdateStatus === "pending" && <p className="small muted" role="status">Updating the Clutchbase Access plugin… Server changes are unavailable until the update finishes.</p>}
                      {host.gameUpdateStatus === "failed" && <p className="small error" role="alert">CS2 update failed: {host.gameUpdateError}</p>}
                      {host.gameBuildStatus === "outdated" && <p className="small error" role="status">CS2 update available{host.gameBuildInstalled && host.gameBuildLatest ? ` · build ${host.gameBuildInstalled} → ${host.gameBuildLatest}` : ""}. {host.actions.updateGame ? "Update CS2 before starting the server." : "Stop the server, then update CS2 before starting it again."}</p>}
                      {host.error && <p className="error" role="alert">{host.error}</p>}
                      {host.removing && <p className="small muted">Removal pending. Installation files will be retained on the machine.</p>}
                      {host.restartPhase && <p className="small muted" role="status">{{ stopping: "Restart requested · waiting for the server to stop", starting: "Restarting · waiting for the server to start", cancelling: "Cancelling restart · waiting for the server to stop", complete: "Restart completed", cancelled: "Restart cancelled", failed: "Restart failed · stop requested" }[host.restartPhase]}</p>}
                      {(host.restartPhase === "stopping" || host.restartPhase === "starting" || host.restartPhase === "cancelling") && !host.restartSupported && <p className="small muted" role="status">Restart is paused. Reconnect a restart-capable machine agent to finish or cancel it.</p>}
                      {openRoom && <button className="server-room-link" onClick={() => setView("matches", openRoom.id)}><span><strong>{openRoom.title}</strong><small>{matchRoomStatus(openRoom).label} · {openRoom.players} joined · {openRoom.ready} ready</small></span><span aria-hidden="true">→</span></button>}
                      <div className="server-footer primary-server-actions">
                        <span className="small muted">
                          {!host.lastSeen ? "Waiting for agent" : !host.online ? "Agent disconnected" : transitioning ? `Requested: ${host.desiredState}` : "Agent connected"}
                        </span>
                        <div className="server-action-buttons">
                          {host.actions.restart && <button className="secondary" disabled={busy === host.id || unavailable} onClick={() => setConfirmRestart({ id: host.id, revision: host.revision })}>Restart server</button>}
                          {host.actions.cancelRestart && <button className="secondary" disabled={busy === host.id || unavailable} onClick={() => void update(host, { desiredState: "stopped" })}>Cancel restart</button>}
                          {host.actions.cancelGameUpdate && <button className="secondary" disabled={busy === host.id || unavailable} onClick={() => void cancelGameUpdate(host)}>Cancel queued update</button>}
                          {host.actions.stopAndUpdate && <button className="secondary" disabled={busy === host.id || unavailable} onClick={() => setConfirmUpdate({ id: host.id, revision: host.revision })}>Stop &amp; update CS2</button>}
                          {host.actions.updateGame && <button className="secondary" disabled={busy === host.id || unavailable} onClick={() => void updateGame(host)}>{host.gameUpdateStatus === "failed" ? "Retry CS2 update" : "Update CS2"}</button>}
                          <button className={host.desiredState === "running" ? "secondary" : "primary"}
                            disabled={(!host.actions.start && !host.actions.stop) || busy === host.id || unavailable}
                            onClick={() => void update(host, { desiredState: host.desiredState === "running" ? "stopped" : "running" })}>
                            {host.removing ? "Removing…" : busy === host.id ? "Saving…" : host.actions.stop ? "Stop server" : state === "stopping" ? "Stopping…" : state === "uninstalled" ? "Install & start" : "Start server"}
                          </button>
                        </div>
                      </div>
                      {confirmRestart?.id === host.id && <section className="team-confirmation" aria-label={`Restart ${host.name}`}>
                        <p>Restart {host.name}? Connected players will be disconnected.</p>
                        {(confirmRestart.revision !== host.revision || !host.actions.restart) && <p className="error" role="alert">The server changed. Close this confirmation and review its current state.</p>}
                        <div className="heading-actions"><button className="secondary" disabled={unavailable || busy === host.id || confirmRestart.revision !== host.revision || !host.actions.restart} onClick={() => void restart(host, confirmRestart.revision)}>Confirm restart</button><button className="secondary" disabled={busy === host.id} onClick={() => setConfirmRestart(null)}>Keep running</button></div>
                      </section>}
                      {confirmUpdate?.id === host.id && <section className="team-confirmation" aria-label={`Stop and update ${host.name}`}>
                        <p>Stop {host.name} and update CS2? Connected players will be disconnected. The server remains stopped afterward. You can cancel the update until it is sent to the agent.</p>
                        {(confirmUpdate.revision !== host.revision || !host.actions.stopAndUpdate) && <p className="error" role="alert">The server changed. Close this confirmation and review its current state.</p>}
                        <div className="heading-actions"><button className="secondary" disabled={unavailable || busy === host.id || confirmUpdate.revision !== host.revision || !host.actions.stopAndUpdate} onClick={() => void updateGame(host, true, confirmUpdate.revision)}>Confirm stop &amp; update</button><button className="secondary" disabled={busy === host.id} onClick={() => setConfirmUpdate(null)}>Keep running</button></div>
                      </section>}
                      <div className="server-section-picker" role="group" aria-label="Server sections">
                        {[["setup", "Setup"], ["access", "Player access"], ["updates", "Updates"], ["logs", "Logs"]].map(([id, label]) => <button key={id} type="button" aria-pressed={serverSection === id} onClick={() => setServerSection(id)}>{label}{id === "updates" && host.reporterUpdateAvailable ? " · Available" : ""}</button>)}
                      </div>
                      <section className="server-section settings-content" aria-label={`${{ setup: "Setup", access: "Player access", updates: "Updates", logs: "Logs" }[serverSection]} for ${host.name}`}>
                      {serverSection === "updates" && <>
                      {host.machineId && host.driver === "managed" ? <section className="maintenance-panel" aria-label={`Updates for ${host.name}`}>
                        <div className="maintenance-item">
                          <div><strong>CS2 server files</strong>
                          {host.gameUpdateStatus === "pending" ? <p className="small muted" role="status">{host.observedState === "stopped" ? "Update requested. Waiting for the machine to finish." : "Waiting for the machine to stop the server before updating."} Starting stays disabled until it finishes.</p> :
                              host.gameUpdateStatus === "complete" ? <p className="small muted" role="status">Last update completed.</p> :
                              host.gameUpdateStatus === "cancelled" ? <p className="small muted" role="status">Update cancelled before delivery. The server is still requested to stop.</p> :
                              host.gameUpdateStatus === "failed" ? <p className="small error" role="alert">Update failed: {host.gameUpdateError}</p> :
                              <p className="small muted">Runs while the server is stopped.</p>}
                          </div>
                        </div>
                        <div className="maintenance-item">
                          <div><strong>Clutchbase Access plugin</strong>
                            {host.reporterUpdateAvailable ? <p className="small muted">A supported plugin is staged. Install or update it while the server is stopped; runtime verification follows the next start.</p> :
                              host.reporterUpdateStatus === "pending" ? <p className="small muted" role="status">Installation requested. Waiting for the machine to confirm.</p> :
                              host.reporterUpdateStatus === "failed" ? <p className="small error" role="alert">Installation failed: {host.reporterUpdateError || "The machine could not install the plugin."}</p> :
                              host.reporterUpdateStatus === "complete" ? <p className="small muted" role="status">Installed. Runtime verification follows the next start.</p> :
                              <p className="small muted">No plugin update is staged.</p>}
                          </div>
                          {host.reporterUpdateAvailable && <button className="secondary" disabled={!host.actions.updateReporter || busy === host.id || unavailable} onClick={() => void installReporter(host)}>Install / update plugin</button>}
                        </div>
                        <ReporterInventory inventory={host.reporterInventory ?? { status: "unknown", installedHash: "", stagedHash: "", checkedAt: null }} />
                        {!host.actions.updateGame && host.gameUpdateStatus !== "pending" && <p className="small muted">{host.reporterUpdateStatus === "pending" ? "Wait for the plugin update to finish before updating CS2." : "CS2 updates are available on stopped servers when the connected machine agent supports them."}</p>}
                      </section> : <p className="small muted">Updates require a managed server on a connected machine.</p>}
                      </>}
                      {serverSection === "logs" && <div className="server-logs">{host.logTail && host.driver !== "simulated" ? <><p className="small muted">{live ? "Recent logs from the latest heartbeat." : "Last received logs · connection lost."}</p><pre>{host.logTail}</pre></> : <p className="small muted">{host.driver === "simulated" ? "Simulation does not produce CS2 logs." : "No recent logs received from this server."}</p>}</div>}
                      {serverSection === "access" && <>
                      <section className="join-access"><h2>Share access request link</h2><p className="small muted">Send this link to a player. It reveals the server name only after they sign in and submit a request.</p><ShareAccess id={host.id} title={host.name} /></section>
                      {host.machineId && host.driver === "managed" && <section className="join-access"><h2>Player match rooms</h2><p className="small muted">{host.allowPlayerRooms ? "Rostered players may open standalone rooms on this server." : "Only management can open rooms on this server."} The room creator can edit the room, load a ready match or close an unstarted room.</p><button className="secondary" disabled={!host.actions.changePlayerRooms || busy === host.id || unavailable} onClick={() => void update(host, { allowPlayerRooms: !host.allowPlayerRooms })}>{host.allowPlayerRooms ? "Disable player room creation" : "Allow players to open rooms"}</button></section>}
                      {host.accessStatus && (
                        <p className="small muted">
                          {{ password: "Join password required", pending: "Whitelist awaiting verification · password required", verified: "Whitelist runtime verified · password required", blocked: "Whitelist verification failed", unknown: "Access status unavailable" }[host.accessStatus]}
                        </p>
                      )}
                      {host.machineId && host.driver !== "simulated" && (
                        <section className="join-access">
                          <h2>Join password</h2>
                          {joinPasswords[host.id] ? (
                            <div className="password-reveal">
                              <code>{joinPasswords[host.id]}</code>
                              <button className="secondary" onClick={() => void copy(joinPasswords[host.id])}>Copy</button>
                              <button className="secondary" onClick={() => setJoinPasswords((current) => { const next = { ...current }; delete next[host.id]; return next; })}>Hide password</button>
                            </div>
                          ) : (
                            <button className="secondary" disabled={loadingPassword === host.id || !host.online} onClick={() => void showJoinPassword(host)}>
                              {loadingPassword === host.id ? "Loading…" : "Show join password"}
                            </button>
                          )}
                          <p className="small muted">Visible to administrators only. Share it with players who need to join.</p>
                        </section>
                      )}
                      {host.machineId && (
                        <section className="join-access">
                          <h2>SteamID whitelist{host.whitelist?.length ? ` (${host.whitelist.length})` : ""}</h2>
                          <p className="small muted">One SteamID64 per line, up to 256. An empty list disables the whitelist. Players still need the join password. Roster changes are applied while the server is stopped. {host.driver === "simulated" && "Simulation does not enforce player access."}</p>
                          {host.whitelist !== null && (
                            <p className="small muted" role="status">{host.whitelistApplied ? (host.driver === "simulated" ? "Roster applied in simulation" : "Roster applied on machine") : "Roster awaiting agent confirmation · Start blocked"}</p>
                          )}
                          {roster?.id === host.id ? (
                            <form onSubmit={(event) => {
                              event.preventDefault();
                              void update({ ...host, revision: roster.revision }, { whitelist: roster.text.split(/\s+/).filter(Boolean) });
                            }}>
                              <label>SteamID64 values<textarea aria-label={`SteamID64 values for ${host.name}`} rows={6} maxLength={8192} value={roster.text} onChange={(event) => setRoster({ ...roster, text: event.target.value })} /></label>
                              {players.length > 0 && <label>Add a signed-in player<select aria-label={`Add a signed-in player to ${host.name}`} value="" onChange={(event) => {
                                const steamId = event.target.value;
                                if (steamId) setRoster({ ...roster, text: [...roster.text.split(/\s+/).filter(Boolean), steamId].join("\n") });
                              }}><option value="">Select a player</option>{players.filter((player) => !roster.text.split(/\s+/).includes(player.steamId)).map((player) => <option key={player.steamId} value={player.steamId}>{player.gameName || "Unnamed player"} · {player.steamId}</option>)}</select></label>}
                              {roster.revision !== host.revision && <p className="small muted">Server changed since editing began. Cancel and reopen to use the latest roster.</p>}
                              <div className="heading-actions">
                                <button className="secondary" type="button" disabled={busy === host.id} onClick={() => setRoster(null)}>Cancel</button>
                                <button className="primary" type="submit" disabled={!host.actions.changeWhitelist || !live || busy === host.id || roster.revision !== host.revision}>{busy === host.id ? "Saving…" : "Save roster"}</button>
                              </div>
                            </form>
                          ) : <>
                            {host.whitelist?.length ? <pre>{host.whitelist.join("\n")}</pre> : <p className="small muted">{host.whitelist === null ? "Managed on the machine until a roster is saved here." : "Saved roster is empty · password required"}</p>}
                            <button className="secondary" disabled={!host.actions.changeWhitelist || busy === host.id || unavailable} onClick={() => setRoster({ id: host.id, revision: host.revision, text: host.whitelist?.join("\n") || "" })}>Edit roster</button>
                            {!host.actions.changeWhitelist && <p className="small muted">Roster editing requires a stopped, installed server and an updated connected agent.</p>}
                          </>}
                        </section>
                      )}
                      {!host.machineId && <p className="small muted">Player access is managed on this server's host.</p>}
                      </>}
                      {serverSection === "setup" && <>
                      {observing && host.driver === "managed" && !host.canObserveMap && <p className="small muted">Update the machine agent to report the current map and open match rooms.</p>}
                      {host.machineId && (
                        <label className="map-label">
                          Startup preset
                          <select aria-label={`Startup preset for ${host.name}`} value={host.preset ?? ""} disabled={!host.actions.changePreset || busy === host.id || unavailable} onChange={(event) => void update(host, { preset: event.target.value })}>
                            {(snapshot?.presets || []).map((preset) => <option key={preset} value={preset}>{presetName(preset)}</option>)}
                          </select>
                          {host.preset !== null && <span className="small muted">{!host.presetApplied ? "Preset awaiting agent confirmation · Start blocked" : host.driver === "simulated" ? "Preset applied in simulation" : presetStatus(host.presetStatus) || "Host default applied"}</span>}
                          {!host.actions.changePreset && host.preset === null && <span className="small muted">Preset selection requires a stopped server and an updated agent.</span>}
                        </label>
                      )}
                      {host.machineId && (host.activePreset === "match" || host.activePreset === "practice" || host.modeCommandStatus !== "") && (
                        <div className="map-label">
                          Live mode
                          <div className="heading-actions">
                            {(["match", "practice"] as const).map((mode) => (
                              <button key={mode} className={host.activePreset === mode ? "primary" : "secondary"}
                                aria-pressed={host.activePreset === mode}
                                disabled={!host.actions.switchMode || busy === host.id || unavailable || host.activePreset === mode}
                                onClick={() => void switchMode(host, mode)}>
                                {mode === "match" ? "Match" : "Practice"}
                              </button>
                            ))}
                          </div>
                          <span className="small muted" role="status">
                            {host.modeCommandStatus === "pending" ? `Switching to ${presetName(host.modeCommandMode)}…` :
                              host.modeCommandStatus === "failed" || host.modeCommandStatus === "cancelled" ? host.modeCommandError || "Mode switch did not complete" :
                              host.activePreset ? `Runtime verified · ${presetName(host.activePreset)}` : "Live switching requires an updated agent and MatchZy outside an active match."}
                          </span>
                        </div>
                      )}
                      {(host.matchPhase === "live" || host.matchPhase === "paused" || host.matchControlStatus === "pending" || host.matchControlStatus === "failed") && (
                        <div className="map-label">
                          Match
                          <span className="small muted" role="status">
                            {host.matchControlStatus === "pending" ? `${host.matchControlAction === "pause" ? "Requesting pause" : host.matchControlAction === "end" ? "Ending match" : "Requesting resume"}…` :
                              host.matchControlStatus === "failed" ? host.matchControlError || "Match control did not complete" :
                              host.matchPhase === "paused" ? "MatchZy pause requested or active" : host.matchPhase === "live" ? "Match live" : "Match state unavailable"}
                          </span>
                          {activeRoom ? <button className="secondary" onClick={() => setView("matches", activeRoom.id)}>Open match controls →</button> : <div className="heading-actions">
                            <button className="secondary" disabled={!host.actions.pauseMatch || busy === host.id || unavailable} onClick={() => void controlMatch(host, "pause")}>Pause match</button>
                            <button className="secondary" disabled={!host.actions.resumeMatch || busy === host.id || unavailable} onClick={() => void controlMatch(host, "resume")}>Resume match</button>
                          </div>}
                        </div>
                      )}
                      <label className="map-label">
                        Startup map
                        <select
                          aria-label={`Startup map for ${host.name}`}
                          value={host.map}
                          disabled={
                            !host.actions.changeMap ||
                            busy === host.id ||
                            unavailable
                          }
                          onChange={(event) =>
                            void update(host, { map: event.target.value })
                          }
                        >
                          {!(host.availableMaps || maps).includes(host.map) && <option value={host.map} disabled>{mapName(host.map)} · unavailable</option>}
                          {(host.availableMaps || maps).map((map) => (
                            <option key={map} value={map}>
                              {mapName(map)}
                            </option>
                          ))}
                        </select>
                      </label>
                      </>}
                      {serverSection === "updates" && <>
                      {host.actions.remove && confirmRemoval !== host.id && (
                        <button className="text-button" disabled={busy === host.id || unavailable} onClick={() => setConfirmRemoval(host.id)}>
                          Remove server
                        </button>
                      )}
                      {host.actions.remove && confirmRemoval === host.id && (
                        <div role="group" aria-label={`Remove ${host.name}`}>
                          <p className="small muted">Remove {host.name}? Its installation will be retained on the machine.</p>
                          <div className="heading-actions">
                            <button className="secondary" disabled={busy === host.id} onClick={() => setConfirmRemoval("")}>Cancel</button>
                            <button className="secondary" disabled={busy === host.id || unavailable} onClick={() => void update(host, {}, true)}>Confirm removal</button>
                          </div>
                        </div>
                      )}
                      </>}
                      </section>
                    </div>
                  </article>
                );
              })}
            </div>
          )}
          </>}
          {view === "matches" && <section className="match-rooms" id="matches" aria-labelledby="matches-heading">
            <div className="page-heading"><h1 id="matches-heading">Matches</h1><button className="primary" title="Requires a running Match server, verified roster and an updated agent" disabled={loading || unavailable || matchHosts.length === 0} onClick={() => setAdding("match")}>＋ Open match</button></div>
            {!loading && matches.length === 0 && !selectedRoomID && <p className="muted">No matches available.</p>}
            {snapshot && selectedRoomID && !selectedRoom && <div className="empty" role="status"><p>This match room is not in the current snapshot. It may be outside the recent rooms.</p><button className="secondary" onClick={() => setView("history")}>View match history</button></div>}
            {matches.length > 0 && <><MatchFilter value={matchFilter} onChange={(filter) => { setMatchFilter(filter); if (selectedRoomID) setView("matches"); }} openCount={openMatches.length} closedCount={closedMatches.length} label="Filter match rooms" /><label>Search match rooms<input aria-label="Search match rooms" type="search" value={matchSearch} onChange={(event) => setMatchSearch(event.target.value)} /></label>{matchSearch && <button type="button" className="secondary" onClick={() => setMatchSearch("")}>Clear search</button>}</>}
            {matches.length >= 50 && <p className="small muted">Showing active and unresolved rooms and up to 50 recent closed rooms.</p>}
            {matches.length > 0 && visibleMatches.length === 0 && <p className="muted" role="status">{matchSearch ? "No match rooms match your search." : `No ${matchFilter} match rooms.`} Full match history is available from the Match history page.</p>}
            <div className="server-grid match-grid">{visibleMatches.map((match) => {
              const matchHost = hosts.find((host) => host.id === match.hostId);
              const loadBlockers = matchLoadBlockers(match, matchHost);
              const roomStatus = matchRoomStatus(match);
              return <article className="server-card match-card" id={`match-room-${match.id}`} tabIndex={-1} key={match.id}>
              <div className="server-body"><div className="server-card-head"><div><div className="server-title"><h2>{match.title}</h2></div><p className="server-meta">{match.hostName} · {mapName(match.map)} · {match.overtime ? "Overtime on" : "Overtime off"}{match.manualReady && " · In-game ready required"}</p></div><span className={`state-pill state-${roomStatus.state}`}>{roomStatus.label}</span></div>
                <p className="match-summary">{match.players} joined <span aria-hidden="true">·</span> {match.ready} ready</p>
                <ShareRoom id={match.id} title={match.title} />
                <RoomLiveScore score={match.liveScore} connected={live} active={match.status === "open" && match.loadStatus === "complete" && !match.setupUnloaded && (matchHost?.matchPhase === "live" || matchHost?.matchPhase === "paused")} />
                <details className="admin-match-details"><summary>Room settings and votes</summary><div>
                <MatchMapVote votes={match.mapVotes || []} maps={[]} current={match.map} canChoose={match.canEdit} connected={live} busy={busy === match.id || unavailable} onChoose={(map) => void chooseVotedMap(match, map)} />
                {(match.canConfigurePauseVoting || match.pauseVotes > 0) && <PauseVoting threshold={match.pauseVoteThreshold} votes={match.pauseVotes} canConfigure={match.canConfigurePauseVoting} connected={live} busy={busy === match.id || unavailable} onConfigure={(threshold) => void configurePauseVoting(match, threshold)} />}
                </div></details>
                {match.canLoad && <div className="match-ready-to-load"><span><strong>Ready to load</strong><small>Players have marked ready. Loading locks the room roster.</small></span><button className="primary" disabled={busy === match.id || unavailable} onClick={() => void loadMatch(match)}>{busy === match.id ? "Loading…" : "Load MatchZy match"}</button></div>}
                {match.status === "open" && match.loadStatus === "" && !match.canLoad && !match.loadBlockedByFailure && <div className="match-preflight" aria-label="Requirements to load this match">
                  <strong>Can’t load yet</strong><ul>{loadBlockers.map((blocker) => <li key={blocker}>{blocker}</li>)}</ul>
                </div>}
                <details className="admin-match-details" open><summary>Players · {match.players} joined, {match.ready} ready</summary><div>
                <MatchLineups participants={match.participants} playersPerSide={match.playersPerSide} ctPlayers={match.ctPlayers} tPlayers={match.tPlayers} renderPlayer={(participant) => <div className="match-player" key={participant.steamId}>
                  <PlayerScore participant={participant} matchId={match.id} revision={match.resultRevision} token={token} canEdit={match.canEditResult} online={live} showReady={match.status === "open"} compact />
                  {match.canRemovePlayer && <button className="text-button" disabled={busy === match.id || unavailable} aria-label={`Remove ${participant.gameName} ending ${participant.steamId.slice(-4)} from the room`} onClick={() => void removeRoomPlayer(match, participant.steamId)}>Remove</button>}
                </div>} />
                {match.canRemovePlayer && <p className="small muted">Removing a player only changes this room; they can join again.</p>}
                <MatchShuffle match={match} token={token} connected={live} />
                </div></details>
                {match.loadStatus === "pending" && <p className="small muted" role="status">Loading MatchZy setup…</p>}
                {match.endedAt && <p className="small muted" role="status">MatchZy ended the match without recording a winner</p>}
                {match.resultCT != null && match.resultT != null && <p role="status">Manual result · started CT {match.resultCT} – {match.resultT} started T</p>}
                {match.reportedCT != null && match.reportedT != null && <p role="status">{match.seriesMatch ? `Reported final map · CT ${match.reportedCT} – ${match.reportedT} T` : `Reported final score · started CT ${match.reportedCT} – ${match.reportedT} started T`}</p>}
                {match.loadStatus === "complete" && !match.setupUnloaded && <p className="small muted" role="status">MatchZy confirmed this setup was loaded · players are locked to their sides</p>}
                {match.setupUnloaded && <p role="status">No match setup is currently loaded on this server. Close the room when ready; a score can be entered afterward.</p>}
                {match.loadStatus === "complete" && match.status === "open" && !match.setupUnloaded && (!match.manualReady && match.ctPlayers === match.tPlayers
                  ? <p className="small muted">Balanced rosters auto-ready in MatchZy when players join their assigned sides.</p>
                  : <p className="small muted">{match.ctPlayers === match.tPlayers ? "Players use MatchZy’s in-game .ready command before starting." : "Uneven rosters use MatchZy’s in-game .ready command. Wait until everyone on the larger side has joined first."}</p>)}
                {match.loadStatus === "failed" && <p className="small error" role="status">{match.loadError || "MatchZy did not load the match setup."}</p>}
                {match.loadBlockedByFailure && <p className="small muted">A failed match load needs recovery before another match can be loaded on this server.</p>}
                {match.cancelStatus === "pending" && <p className="small muted" role="status">Waiting for the host to cancel the failed request…</p>}
                {match.cancelStatus === "failed" && <p className="small error" role="status">{match.cancelError || "The failed request could not be cancelled."}</p>}
                {match.cancelStatus === "complete" && <p className="small muted" role="status">Failed request cancelled. Players must mark ready again.</p>}
                {match.canCancelLoad && <div className="server-footer"><button className="secondary" disabled={busy === match.id || unavailable} onClick={() => void cancelMatchLoad(match)}>Cancel failed load</button></div>}
                {matchHost && (matchHost.matchPhase === "live" || matchHost.matchPhase === "paused" || matchHost.matchControlStatus === "pending" || matchHost.matchControlStatus === "failed") && <div className="match-admin-controls">
                  <div><strong>Match controls</strong><p className="small muted" role="status">{matchHost.matchControlStatus === "pending" ? `${matchHost.matchControlAction === "pause" ? "Pausing" : "Resuming"}…` : matchHost.matchControlStatus === "failed" ? matchHost.matchControlError || "Match control failed" : matchHost.matchPhase === "paused" ? "Match paused" : "Match live"}</p></div>
                  <div className="heading-actions"><button className="secondary" disabled={!matchHost.actions.pauseMatch || busy === matchHost.id || unavailable} onClick={() => void controlMatch(matchHost, "pause")}>Pause</button><button className="secondary" disabled={!matchHost.actions.resumeMatch || busy === matchHost.id || unavailable} onClick={() => void controlMatch(matchHost, "resume")}>Resume</button></div>
                </div>}
                <div className="admin-match-actions">
                <RoomReopen id={match.id} title={match.title} revision={match.settingsRevision} allowed={!!match.canReopen} connected={live} token={token} />
                {match.canEnd && <div className="server-footer">{confirmEndMatch !== match.id ? <button className="secondary" disabled={busy === match.id || unavailable} onClick={() => setConfirmEndMatch(match.id)}>End MatchZy match</button> : <><span className="small">End this match without a winner?</span><button className="secondary" disabled={busy === match.id || unavailable} onClick={() => void endMatch(match)}>Confirm end match</button><button className="secondary" onClick={() => setConfirmEndMatch("")}>Cancel</button></>}</div>}
                {matchHost?.matchControlAction === "end" && matchHost?.matchControlStatus === "pending" && <p className="small muted" role="status">Waiting for MatchZy to confirm the match has ended…</p>}
                {match.canEditResult && <div className="server-footer"><button className="secondary" disabled={busy === match.id || unavailable} onClick={() => { setEditingResult(match); setResultError(""); }}>{match.resultCT == null ? "Record result" : "Edit result"}</button></div>}
                {match.status === "open" && <div className="server-footer">
                  {match.canEdit && <button className="secondary" disabled={busy === match.id || unavailable} onClick={() => { setEditingMatch(match); setMatchEditError(""); }}>Edit room</button>}
                  {match.canClose && confirmCloseMatch !== match.id && <button className="secondary" disabled={busy === match.id || unavailable} onClick={() => match.loadStatus === "complete" ? setConfirmCloseMatch(match.id) : void closeMatch(match)}>{busy === match.id ? "Saving…" : "Close match room"}</button>}
                  {match.canClose && confirmCloseMatch === match.id && <><span className="small">Closing the room does not end the MatchZy match or stop the server.</span><button className="secondary" disabled={busy === match.id || unavailable} onClick={() => void closeMatch(match)}>Confirm close room</button><button className="secondary" disabled={busy === match.id} onClick={() => setConfirmCloseMatch("")}>Cancel</button></>}
                </div>}
                </div>
              </div>
              </article>;
            })}</div>
          </section>}
          {view === "history" && <MatchHistory token={token} onClose={() => setView("matches")} />}
          {view === "stats" && <Statistics token={token} />}
          {view === "leaderboard" && <Leaderboard token={token} />}
          {view === "reports" && <Reports token={token} />}
          {view === "schedule" && <><div className="page-heading"><h1>Schedule</h1></div><FixtureSchedule tournaments={snapshot?.tournaments || []} connected={live} loading={!snapshot} onOpen={(id) => setView("tournaments", id)} /></>}
          {view === "tournaments" && <section className="match-rooms" aria-labelledby="tournaments-heading">
            <div className="page-heading"><h1 id="tournaments-heading">Tournaments</h1><button className="primary" disabled={!live} onClick={() => setAdding("tournament")}>＋ Create tournament</button></div>
            <TournamentPicker tournaments={snapshot?.tournaments || []} loading={!snapshot} selectedID={selectedTournamentID} onSelect={(id) => setView("tournaments", id)} onBack={() => setView("tournaments")} />
            <div className="server-grid">{(selectedTournament ? [selectedTournament] : []).map((tournament) => <article className="server-card match-card tournament-card" id={`tournament-${tournament.id}`} tabIndex={-1} key={tournament.id}><div className="server-body">
              <div className="server-card-head"><div><h2>{tournament.name}</h2><p className="server-meta">{tournament.playersPerSide}v{tournament.playersPerSide} · {tournament.entries.length} {tournament.entries.length === 1 ? "team" : "teams"}</p></div><span className={`state-pill ${tournament.status === "active" ? "state-running" : "state-stopped"}`}>{tournament.status}</span></div>
              <TournamentProgress tournament={tournament} />
              <TournamentStandings tournament={tournament} />
              <div className="machine-list">{tournament.entries.map((entry) => <div className="machine-row" key={entry.teamId}><strong>{entry.teamName}</strong><span className="small muted">{entry.status} · {entry.members}/{tournament.playersPerSide} players</span></div>)}</div>
              <p className="small muted">{tournament.format === "round_robin" ? "Round robin" : tournament.format === "double_elimination" ? "Double elimination" : "Single elimination"} · Best of {tournament.bestOf} · {tournament.mapPool.map(mapName).join(" · ")}</p>
              {tournament.canCheckIn && <button className="secondary" disabled={!live || busy.includes(tournament.id)} onClick={() => void tournamentAction(`/tournaments/${tournament.id}/checkin`, { method: "POST" })}>Close registration and start check-in</button>}
              {tournament.status === "open" && tournament.entries.length < 2 && <p className="small muted">Registration stays open until at least two teams sign up.</p>}
              {tournament.canBuildBracket && <button className="primary" disabled={!live || busy.includes(tournament.id)} onClick={() => void tournamentAction(`/tournaments/${tournament.id}/bracket`, { method: "POST" })}>{tournament.format === "round_robin" ? "Generate fixtures" : "Generate bracket"}</button>}
              <TournamentCancellation tournament={tournament} connected={live} busy={busy.includes(tournament.id)} onCancel={(phase) => tournamentAction(`/tournaments/${tournament.id}/cancel`, { method: "POST", body: JSON.stringify({ phase }) })} />
              {tournament.matches.length > 0 && <TournamentBracket format={tournament.format} matches={tournament.matches} label={`${tournament.name} ${tournament.format === "round_robin" ? "fixtures" : "bracket"}`} renderMatch={(match) => {
                const progress = tournamentMatchProgress(match);
                const seriesDecided = tournamentSeriesDecided(tournament.bestOf, match.teamAScore, match.teamBScore);
                return <div className={`bracket-match bracket-match-${progress.key}`} key={match.id}>
                <div className="bracket-match-head"><span className="small muted">Match {match.number}</span><span className={`bracket-state bracket-state-${progress.key}`}>{progress.label}</span></div>
                <div className={`bracket-team ${match.winnerTeamId && match.winnerTeamId === match.teamA?.teamId ? "bracket-team-winner" : ""}`}>{match.teamA?.teamName || (match.status === "skipped" ? "Not needed" : match.status === "bye" ? "Bye" : "Waiting")}{match.winnerTeamId === match.teamA?.teamId && " ✓"}</div>
                <div className={`bracket-team ${match.winnerTeamId && match.winnerTeamId === match.teamB?.teamId ? "bracket-team-winner" : ""}`}>{match.teamB?.teamName || (match.status === "skipped" ? "Not needed" : match.status === "bye" ? "Bye" : "Waiting")}{match.winnerTeamId === match.teamB?.teamId && " ✓"}</div>
                {match.winnerCorrected && <p className="small muted" role="status">Winner corrected by management; recorded scores remain unchanged.</p>}
                {!!match.winnerHistory?.length && <details><summary>Winner change history</summary><ul>{match.winnerHistory.map((change) => <li key={change.revision}>{change.previousWinner || "No winner"} → {change.newWinner || "No winner"} · <time dateTime={change.changedAt}>{new Date(change.changedAt).toLocaleString()}</time></li>)}</ul></details>}
                <TournamentSchedule match={match} admin connected={live} busy={busy.includes(match.id)} onSave={(revision, scheduledAt) => tournamentAction(`/tournaments/${tournament.id}/matches/${match.id}/schedule`, { method: "PATCH", body: JSON.stringify({ revision, scheduledAt }) })} />
                <TournamentDispute dispute={match.dispute} admin connected={live} busy={busy.includes(match.id)} onResolve={(revision, resolution) => tournamentAction(`/tournaments/${tournament.id}/matches/${match.id}/dispute`, { method: "PATCH", body: JSON.stringify({ revision, resolution }) })} />
                <TournamentCorrection match={match} connected={live} busy={busy.includes(match.id)} roundRobin={tournament.format === "round_robin"} onCorrect={(teamId, revision) => tournamentAction(`/tournaments/${tournament.id}/matches/${match.id}/correction`, { method: "PATCH", body: JSON.stringify({ teamId, revision }) })} />
                {match.reportedCT != null && match.reportedT != null && <p className="small muted" role="status">{tournament.bestOf === 1 ? `MatchZy final · started CT ${match.reportedCT} – ${match.reportedT} started T` : `Final map · CT ${match.reportedCT} – ${match.reportedT} T`}</p>}
                {match.canReportWinner && (tournament.bestOf === 1 ? match.reportedCT != null && match.reportedCT === match.reportedT : !seriesDecided) && <p className="small muted" role="status">The series result is unresolved. Choose which team advances below.</p>}
                {match.resultCT != null && match.resultT != null && <p className="small muted" role="status">Manual score · started CT {match.resultCT} – {match.resultT} started T</p>}
                {match.teamAScore != null && match.teamBScore != null && <p className="small muted" role="status">Series · {match.teamA?.teamName} {match.teamAScore}–{match.teamBScore} {match.teamB?.teamName} maps</p>}
                {match.vetoStep < 7 && match.teamA && match.teamB && <p className="small muted">{match.vetoBans.length} bans · {match.vetoMaps.length} map picks · {match.vetoTurnTeam ? `${match.vetoTurnTeam} to ${match.vetoStep < 6 ? ((tournament.bestOf === 3 && match.vetoStep >= 2 && match.vetoStep <= 3) || (tournament.bestOf === 5 && match.vetoStep >= 2 && match.vetoStep <= 5) ? "pick a map" : "ban a map") : "choose starting side"}` : "Veto waiting"}</p>}
                {match.vetoBans.length > 0 && <p className="small muted">Banned: {match.vetoBans.map(mapName).join(" → ")}</p>}
                {match.vetoMaps.length > 0 && <p className="small muted">Picked: {match.vetoMaps.map(mapName).join(" → ")}</p>}
                {match.mapSequence.length > 0 && <p className="small muted">Series maps: {match.mapSequence.map(mapName).join(" → ")}</p>}
                {match.vetoStep === 7 && <p className="small muted">Team B starts {match.teamBSide?.toUpperCase()}</p>}
                {match.map && <p className="small muted">{mapName(match.map)}{match.roomId ? ` · Room ${match.roomStatus}` : " · Veto selected"}</p>}
                {match.roomId && <button className="text-button" onClick={() => setView("matches", match.roomId)}>View match room →</button>}
                {match.canOpenRoom && <form className="tournament-room-form" onSubmit={(event) => { event.preventDefault(); const form = new FormData(event.currentTarget); void tournamentAction(`/tournaments/${tournament.id}/matches/${match.id}/room`, { method: "POST", body: JSON.stringify({ hostId: form.get("hostId") }) }); }}>
                  <label>Server<select name="hostId" required defaultValue=""><option value="">Choose a server…</option>{matchHosts.filter((host) => tournament.bestOf === 1 || host.supportsMatchSeries).map((host) => <option key={host.id} value={host.id}>{host.name}</option>)}</select></label>
                  <p className="small muted">The veto selected {match.map}. The room will use this map and the team starting sides.</p>
                  <button className="primary" disabled={!live || !matchHosts.length || busy.includes(match.id)}>{busy.includes(match.id) ? "Opening…" : "Open match room"}</button>
                  {!matchHosts.some((host) => tournament.bestOf === 1 || host.supportsMatchSeries) && <span className="small muted">{tournament.bestOf === 1 ? "No running Match server with verified access is ready." : "Update the host agent and Clutchbase Access reporter on a Match server to enable BO3/BO5."}</span>}
                </form>}
                {match.canReportWinner && <form className="tournament-winner-form" onSubmit={(event) => {
                  event.preventDefault();
                  const teamId = winnerSelections[match.id];
                  if (teamId) void tournamentAction(`/tournaments/${tournament.id}/matches/${match.id}/winner`, { method: "PATCH", body: JSON.stringify({ teamId }) });
                }}>
                  <label>{tournament.format === "round_robin" ? "Fixture winner" : "Team to advance"}<select aria-label={`Team to advance from round ${match.round} match ${match.number}`} value={winnerSelections[match.id] || ""} onChange={(event) => setWinnerSelections((current) => ({ ...current, [match.id]: event.target.value }))}>
                    <option value="">Choose a winner…</option>{[match.teamA,match.teamB].filter((entry) => entry !== null).map((entry) => <option key={entry!.teamId} value={entry!.teamId}>{entry!.teamName}</option>)}
                  </select></label>
                  <p className="small muted">{tournament.format === "round_robin" ? "Confirming records a fixture win in the standings." : "Confirming advances this team into the next round."}</p>
                  <button className="secondary" type="submit" disabled={!winnerSelections[match.id] || !live || busy.includes(match.id)}>Confirm winner</button>
                </form>}
              </div>;
              }} />}
            </div></article>)}</div>
          </section>}
          {view === "players" && <section className="match-rooms" id="players" aria-labelledby="players-heading">
            <div className="page-heading"><h1 id="players-heading">Players</h1></div>
            <p className="small muted">Players appear here after signing in with Steam. Add them to a server roster to grant access.</p>
            <PlayerDirectory players={players} />
          </section>}
          {view === "teams" && <section className="match-rooms" aria-labelledby="teams-heading"><div className="page-heading"><h1 id="teams-heading">Teams</h1></div><p className="small muted">Captains manage their own teams from the public page. Invite codes expire after 24 hours; roster size is capped at five.</p>{snapshot ? <TeamDirectory teams={snapshot.teams || []} /> : <p role="status">Loading teams…</p>}</section>}
          {view === "access-requests" && <AccessRequests token={token} />}
        </main>
      </div>
      {notice && (
        <div className="toast" role="status">
          ✓ {notice}
        </div>
      )}
      {editingMatch && <Dialog onClose={() => { if (busy !== editingMatch.id) setEditingMatch(null); }}>
        <h2 id="modal-title">Edit match room</h2>
        <form onSubmit={(event) => void saveMatchSettings(event)}>
          <label>Match name<input autoFocus name="title" required maxLength={80} defaultValue={editingMatch.title} /></label>
          <label>Match map<select name="map" required defaultValue={editingMatch.map}>{maps.map((map) => <option key={map} value={map}>{mapName(map)}</option>)}</select></label>
          <label>Room size<select name="playersPerSide" defaultValue={editingMatch.playersPerSide}>{[1, 2, 3, 4, 5].map((size) => <option key={size} value={size}>{size}v{size}</option>)}</select></label>
          <label className="overtime-choice"><input type="checkbox" name="overtime" defaultChecked={editingMatch.overtime} /> Enable overtime</label>
          <label className="overtime-choice"><input type="checkbox" name="manualReady" defaultChecked={!!editingMatch.manualReady} /> Require in-game ready</label>
          <p className="small muted">Players use .ready in CS2 before starting. This requires an updated host agent and reporter; uneven teams always use in-game ready.</p>
          <p className="small muted">Changing the map, room size, overtime or in-game ready setting clears everyone's ready checks. The room must still have space for all joined players. The server changes map when you load the setup.</p>
          {!canSaveMatch && <p className="error" role="alert">The room changed. Close and reopen the editor to use its current settings.</p>}
          {matchEditError && <p className="error" role="alert">{matchEditError}</p>}
          <button className="primary" type="submit" disabled={busy === editingMatch.id || !live || !canSaveMatch}>{busy === editingMatch.id ? "Saving…" : "Save room settings"}</button>
        </form>
      </Dialog>}
      {editingResult && <Dialog onClose={() => { if (busy !== editingResult.id) setEditingResult(null); }}>
        <h2 id="modal-title">Record match result</h2>
        <p className="small muted">Enter the final score for each team by its starting side. This is a manual record and does not end or change the game.</p>
        <form onSubmit={(event) => { event.preventDefault(); const data = new FormData(event.currentTarget); void saveResult(editingResult, { ct: Number(data.get("ct")), t: Number(data.get("t")) }); }}>
          <label>Team that started CT<input autoFocus name="ct" type="number" min="0" max="99" step="1" required defaultValue={editingResult.resultCT ?? ""} /></label>
          <label>Team that started T<input name="t" type="number" min="0" max="99" step="1" required defaultValue={editingResult.resultT ?? ""} /></label>
          {!canSaveResult && <p className="error" role="alert">The result changed. Close and reopen the editor to use its current state.</p>}
          {resultError && <p className="error" role="alert">{resultError}</p>}
          <div className="heading-actions"><button className="primary" type="submit" disabled={busy === editingResult.id || !live || !canSaveResult}>{busy === editingResult.id ? "Saving…" : "Save result"}</button>{editingResult.resultCT !== null && <button type="button" className="secondary" disabled={busy === editingResult.id || !live || !canSaveResult} onClick={() => void saveResult(editingResult, null)}>Clear result</button>}</div>
        </form>
      </Dialog>}
      {rotationTarget && <Dialog onClose={() => { if (busy !== `rotate:${rotationTarget.id}`) setRotationTarget(null); }}>
        <h2 id="modal-title">Replace agent token for {rotationTarget.name}?</h2>
        <p className="muted">The current token stops working immediately. This machine and its servers will show offline until you restart its agent with the new token. Running CS2 processes will keep running.</p>
        <p className="small muted">Save the new token when it appears. It will only be shown once.</p>
        {rotationError && <p className="error" role="alert">{rotationError}</p>}
        <div className="heading-actions"><button className="primary" disabled={!live || !!busy} onClick={() => void rotateMachineToken(rotationTarget)}>{busy ? "Replacing…" : "Replace token"}</button><button className="secondary" disabled={!!busy} onClick={() => setRotationTarget(null)}>Cancel</button></div>
      </Dialog>}
      {(adding || credentials) && (
        <Dialog
          onClose={() => {
            if (busy !== "create") {
              setAdding(null);
              setCredentials(null);
            }
          }}
        >
          {credentials ? (
            <>
              <h2 id="modal-title">{credentials.rotated ? "Restart agent on" : "Connect"} {credentials.machine.name}</h2>
              <p className="muted">
                {credentials.rotated ? "Replace the agent token in your existing service configuration and restart the agent. The command below uses the new token." : "Run this once on your Linux machine, then create servers from this dashboard."} The agent token is shown once; keep it private.
              </p>
              <pre>{agentCommand}</pre>
              <button
                className="primary"
                onClick={() => void copy(agentCommand)}
              >
                Copy agent command
              </button>
              <p className="small muted">
                For a remote host, use your HTTPS API address. Install SteamCMD
                first. Add --simulate to try the controls without downloading CS2.
                Each real server needs a full CS2 installation. See README.md for host setup.
              </p>
            </>
          ) : adding === "match" ? (
            <>
              <h2 id="modal-title">Open a match room</h2>
              <p className="muted">Players on the server’s saved SteamID roster can join and mark ready. This opens a room; it does not start a MatchZy match.</p>
              <form onSubmit={(event) => void addMatch(event)}>
                <label>Match name<input autoFocus name="title" required maxLength={80} placeholder="Friday night match" /></label>
                <label>Running Match server<select name="host" required value={selectedMatchHost?.id || ""} onChange={(event) => setMatchHostID(event.target.value)}>{matchHosts.map((host) => <option key={host.id} value={host.id}>{host.name} · {mapName(host.activeMap)}</option>)}</select></label>
                <label>Match map<select key={selectedMatchHost?.id} name="map" defaultValue="" disabled={!selectedMatchHost?.canObserveMap}><option value="">Current server map{selectedMatchHost ? ` · ${mapName(selectedMatchHost.activeMap)}` : ""}</option>{maps.map((map) => <option key={map} value={map}>{mapName(map)}</option>)}</select></label>
                {selectedMatchHost?.canObserveMap ? <p className="small muted">Loading the match switches the live server to the selected map.</p> : <p className="small muted">Update the machine agent to choose a different match map.</p>}
                <label>Room size<select name="playersPerSide" defaultValue="5">{[1, 2, 3, 4, 5].map((size) => <option key={size} value={size}>{size}v{size}</option>)}</select></label>
                <label className="overtime-choice"><input type="checkbox" name="overtime" /> Enable overtime</label>
                <label className="overtime-choice"><input type="checkbox" name="manualReady" /> Require in-game ready</label>
                <p className="small muted">Players use .ready in CS2 before starting. This requires an updated host agent and reporter; uneven teams always use in-game ready.</p>
                <p className="small muted">Each CT/T side can hold up to the selected number of players.</p>
                {error && <p className="error" role="alert">{error}</p>}
                <button className="primary" type="submit" disabled={busy === "create" || matchHosts.length === 0}>{busy === "create" ? "Opening…" : "Open match room →"}</button>
              </form>
            </>
          ) : adding === "tournament" ? (
            <>
              <h2 id="modal-title">Create tournament</h2>
              <p className="muted">Create a team tournament. Players register their teams before check-in; the bracket is generated from teams that confirm.</p>
              <form onSubmit={(event) => void addTournament(event)}>
                <label>Tournament name<input autoFocus name="name" required maxLength={80} placeholder="Saturday cup" /></label>
                <label>Players per team<select name="playersPerSide" defaultValue="5">{[1,2,3,4,5].map((size) => <option key={size} value={size}>{size}v{size}</option>)}</select></label>
                <label>Tournament format<select name="format" defaultValue="single_elimination"><option value="single_elimination">Single elimination</option><option value="double_elimination">Double elimination · two losses to be eliminated</option><option value="round_robin">Round robin · every team plays every other team</option></select></label>
                <label>Series format<select name="bestOf" defaultValue="1"><option value="1">Best of 1</option><option value="3">Best of 3</option><option value="5">Best of 5</option></select></label>
                <fieldset className="map-pool-field"><legend>Map veto pool · choose exactly 7 maps</legend><div className="map-choice-grid">{maps.map((map, index) => <label key={map}><input type="checkbox" name="mapPool" value={map} defaultChecked={index < 7} />{mapName(map)}</label>)}</div></fieldset>
                {error && <p className="error" role="alert">{error}</p>}
                <button className="primary" type="submit" disabled={busy === "create" || unavailable}>{busy === "create" ? "Creating…" : "Create tournament →"}</button>
              </form>
            </>
          ) : adding === "server" ? (
            <>
              <h2 id="modal-title">Create server</h2>
              <p className="muted">Select a machine and startup map. After creation, use Install &amp; start to download and launch CS2.</p>
              <form onSubmit={(event) => void addServer(event)}>
                <label>Server name<input autoFocus name="name" required maxLength={80} placeholder="Friday night crew" /></label>
                <label>Machine<select name="machine" required value={provisioningMachine?.id || ""} disabled={unavailable || busy === "create"} onChange={(event) => setCreateMachineID(event.target.value)}>{machines.filter((m) => m.createServer).map((m) => <option key={m.id} value={m.id}>{m.name}{m.driver === "simulated" ? " (simulated)" : ""}</option>)}</select></label>
                {provisioningMachine && <MachineResources machine={provisioningMachine} live={live} />}
                <label>Startup map<select name="map" required>{maps.map((m) => <option key={m} value={m}>{mapName(m)}</option>)}</select></label>
                <label>Game port<input name="port" type="number" min={1024} max={65533} placeholder="Automatic" /></label>
                <p className="small muted">Leave the port blank to allocate it automatically. The next two ports are reserved for client and TV traffic.</p>
                {error && <p className="error" role="alert">{error}</p>}
                <button className="primary" type="submit" disabled={busy === "create" || unavailable || !machines.some((m) => m.createServer)}>{busy === "create" ? "Creating…" : "Create server →"}</button>
              </form>
            </>
          ) : (
            <>
              <h2 id="modal-title">Connect machine</h2>
              <p className="muted">
                Register a Linux machine, then run its agent. One agent manages all servers you create on it.
              </p>
              <form onSubmit={(event) => void addMachine(event)}>
                <label>
                  Machine name
                  <input
                    autoFocus
                    name="name"
                    required
                    maxLength={80}
                    placeholder="Home server"
                  />
                </label>
                <label>
                  Game hostname or IP
                  <input
                    name="address"
                    required
                    maxLength={253}
                    placeholder="play.example.com"
                  />
                </label>
                {error && (
                  <p className="error" role="alert">
                    {error}
                  </p>
                )}
                <button
                  className="primary"
                  type="submit"
                  disabled={busy === "create"}
                >
                  {busy === "create" ? "Registering…" : "Register machine →"}
                </button>
              </form>
            </>
          )}
        </Dialog>
      )}
    </div>
  );
}

function Dialog({
  children,
  onClose,
}: {
  children: ReactNode;
  onClose: () => void;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const dialog = ref.current!;
    dialog.showModal();
    return () => dialog.close();
  }, []);
  return (
    <dialog
      ref={ref}
      className="modal"
      aria-labelledby="modal-title"
      onCancel={(event) => {
        event.preventDefault();
        onClose();
      }}
    >
      <button
        className="close text-button"
        aria-label="Close dialog"
        onClick={onClose}
      >
        ✕
      </button>
      {children}
    </dialog>
  );
}
