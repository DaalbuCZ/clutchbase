import { useEffect, useRef, useState } from "react";
import type { FormEvent } from "react";
import { connection, request, presetName, presetStatus, tournamentMatchProgress, matchRoomStatus } from "./api";
import type { PlayerMatch, PlayerServer, PlayerWorkspace, Tournament } from "./api";
import { connectLive } from "./live";
import { MatchHistory } from "./MatchHistory";
import { Statistics } from "./Statistics";
import { Leaderboard } from "./Leaderboard";
import { Reports } from "./Reports";
import { PublicTeam } from "./PublicTeam";
import { TournamentWithdrawal } from "./TournamentWithdrawal";
import { PlayerScore } from "./PlayerScore";
import { MatchLineups } from "./MatchLineups";
import { ShareRoom } from "./ShareRoom";
import { RoomReopen } from "./RoomReopen";
import { PlayerAccessRequests } from "./PlayerAccessRequests";
import { PlayerRoomSettings } from "./PlayerRoomSettings";
import { PlayerRoomShuffle } from "./PlayerRoomShuffle";
import { PlayerRoomRemove } from "./PlayerRoomRemove";
import { MatchMapVote } from "./MatchMapVote";
import { PauseVoting } from "./PauseVoting";
import { TournamentDispute } from "./TournamentDispute";
import { loadBlockerLabels } from "./LoadBlockers";
import { RoomLiveScore } from "./RoomLiveScore";
import { MatchFilter } from "./MatchFilter";
import { TournamentBracket } from "./TournamentBracket";
import { TournamentSchedule } from "./TournamentSchedule";
import { FixtureSchedule } from "./FixtureSchedule";
import { TournamentProgress } from "./TournamentProgress";
import { TournamentPicker } from "./TournamentPicker";
import { TournamentStandings } from "./TournamentStandings";
import { useView, viewHash } from "./view";
import "./PlayerExperience.css";

const playerViews = ["servers", "matches", "tournaments", "schedule", "teams", "history", "profile", "stats", "leaderboard", "reports", "access"] as const;

type PlayerTournamentCardProps = {
  tournament: Tournament;
  live: boolean;
  busy: boolean;
  teamName?: string;
  onAction: (path: string, selection?: { map?: string; side?: "ct" | "t" }) => void;
  onWithdraw: (id: string, selection: { teamRevision: number; teamId: string; entryStatus: string; phase: string }) => Promise<boolean>;
  onViewMatches: (roomId: string) => void;
  onDispute: (tournamentID: string, matchID: string, reason: string) => Promise<boolean>;
};

function mapLabel(map: string) {
  return map.replace(/^de_/, "").replace(/^./, (letter) => letter.toUpperCase());
}

function matchNextStep(match: PlayerMatch, server: PlayerServer | undefined, live: boolean): string {
  if (!live) return "Reconnecting. Actions will return when the live connection is restored.";
  if (match.status === "closed") return "Room closed.";
  if (match.loadStatus === "pending") return "Match loading. Sides are locked until it finishes.";
  if (match.setupUnloaded) return "Match ended. The host can close the room.";
  if (match.loadStatus === "failed") return "Match load failed. Ask the host to retry.";
  if (match.loadStatus === "complete") return !match.joined ? "Match loaded. Its roster is locked." : server?.canJoin ? "Connect below, then join your assigned side in CS2." : "Match loaded. Waiting for the server to become ready.";
  if (!match.joined) return match.actions.joinCT || match.actions.joinT ? "Join an open CT or T slot below." : "Room full or server unavailable.";
  if (match.actions.ready) return "Ready up below, or switch sides first.";
  if (match.actions.load) return "Everyone is ready. You can load the match below.";
  if (match.actions.unready) return match.players > 0 && match.ready === match.players && match.ctPlayers > 0 && match.tPlayers > 0 ? "Everyone is ready. Waiting for the host to load the match when the server is ready." : "Ready. Waiting for the others.";
  if (match.actions.switchSide) return "Choose your side below, then ready up.";
  return "Waiting for the host to load the match.";
}

function PlayerTournamentCard({ tournament, live, busy, teamName, onAction, onWithdraw, onViewMatches, onDispute }: PlayerTournamentCardProps) {
  return <article className="server-card match-card tournament-card" id={`tournament-${tournament.id}`} tabIndex={-1}>
    <div className="server-body public-tournament-body">
      <div className="server-card-head">
        <div><h2>{tournament.name}</h2><p className="server-meta">{tournament.format === "round_robin" ? "Round robin" : tournament.format === "double_elimination" ? "Double elimination" : "Single elimination"} · Best of {tournament.bestOf} · {tournament.playersPerSide}v{tournament.playersPerSide} · {tournament.entries.length} {tournament.entries.length === 1 ? "team" : "teams"}</p></div>
        <span className={`state-pill ${tournament.status === "active" ? "state-running" : "state-stopped"}`}>{tournament.status}</span>
      </div>
      <TournamentProgress tournament={tournament} />
      <div className="public-tournament-actions">
      {tournament.canRegisterTeam && <button className="primary" disabled={!live || busy} onClick={() => onAction(`/player/tournaments/${tournament.id}/register`)}>Register {teamName}</button>}
      {tournament.canCheckInTeam && <button className="primary" disabled={!live || busy} onClick={() => onAction(`/player/tournaments/${tournament.id}/checkin`)}>Check in {teamName}</button>}
      <TournamentWithdrawal tournament={tournament} teamName={teamName} live={live} busy={busy} onWithdraw={onWithdraw} />
      {tournament.myEntryStatus && <p className="small muted">{tournament.status === "cancelled" ? "Archived team entry" : "Your team"}: {tournament.myEntryStatus}</p>}
      </div>
      <TournamentStandings tournament={tournament} />
      {tournament.entries.length > 0 && <details className="public-tournament-entries"><summary>Teams ({tournament.entries.length})</summary><div className="machine-list">{tournament.entries.map((entry) => <div className="machine-row" key={entry.teamId}><strong>{entry.teamName}</strong><span className="small muted">{entry.status}</span></div>)}</div></details>}
      {tournament.matches.length > 0 && <TournamentBracket format={tournament.format} matches={tournament.matches} label={`${tournament.name} ${tournament.format === "round_robin" ? "fixtures" : "bracket"}`} renderMatch={(match) => {
        const active = !!match.teamA && !!match.teamB && match.status === "pending";
        const progress = tournamentMatchProgress(match);
        const remainingMaps = tournament.mapPool.filter((map) => !match.vetoBans.includes(map) && !match.vetoMaps.includes(map));
        const picking = (tournament.bestOf === 3 && match.vetoStep >= 2 && match.vetoStep <= 3) || (tournament.bestOf === 5 && match.vetoStep >= 2 && match.vetoStep <= 5);
        return <div className={`bracket-match bracket-match-${progress.key}`} key={match.id}>
          <div className="bracket-match-head"><span className="small muted">Match {match.number}</span><span className={`bracket-state bracket-state-${progress.key}`}>{progress.label}</span></div>
          <div className={`bracket-team ${match.winnerTeamId && match.winnerTeamId === match.teamA?.teamId ? "bracket-team-winner" : ""}`}>{match.teamA?.teamName || (match.status === "skipped" ? "Not needed" : match.status === "bye" ? "Bye" : "Waiting")}{match.winnerTeamId === match.teamA?.teamId && " ✓"}</div>
          <div className={`bracket-team ${match.winnerTeamId && match.winnerTeamId === match.teamB?.teamId ? "bracket-team-winner" : ""}`}>{match.teamB?.teamName || (match.status === "skipped" ? "Not needed" : match.status === "bye" ? "Bye" : "Waiting")}{match.winnerTeamId === match.teamB?.teamId && " ✓"}</div>
          {match.winnerCorrected && <p className="small muted" role="status">Winner corrected by management; recorded scores remain unchanged.</p>}
          <TournamentSchedule match={match} connected={live} />
          {match.reportedCT != null && match.reportedT != null && <p className="small muted" role="status">{tournament.bestOf === 1 ? `MatchZy final · started CT ${match.reportedCT} – ${match.reportedT} started T` : `Final map · CT ${match.reportedCT} – ${match.reportedT} T`}</p>}
          {match.teamAScore != null && match.teamBScore != null && <p className="small muted" role="status">Series · {match.teamA?.teamName} {match.teamAScore}–{match.teamBScore} {match.teamB?.teamName} maps</p>}
          {match.status === "pending" && tournament.bestOf === 1 && match.reportedCT != null && match.reportedCT === match.reportedT && <p className="small muted" role="status">The reported score is tied. Management must choose which team advances.</p>}
          {match.resultCT != null && match.resultT != null && <p className="small muted" role="status">Manual score · started CT {match.resultCT} – {match.resultT} started T</p>}
          <TournamentDispute dispute={match.dispute} canDispute={!!match.canDispute} connected={live} busy={busy} onSubmit={(reason) => onDispute(tournament.id, match.id, reason)} />
          {active && <section className="veto-panel" aria-label={`Map veto for round ${match.round} match ${match.number}`}>
            <div className="veto-heading"><strong>Map veto</strong><span className="small muted">{match.vetoStep < 6 ? `${match.vetoBans.length} bans · ${match.vetoMaps.length} picks` : match.vetoStep === 6 ? "Choose starting side" : "Complete"}</span></div>
            {match.vetoBans.length > 0 && <p className="small muted">Banned: {match.vetoBans.map(mapLabel).join(" → ")}</p>}
            {match.vetoMaps.length > 0 && <p className="small muted">Picked: {match.vetoMaps.map(mapLabel).join(" → ")}</p>}
            {match.vetoStep < 7 && <p className="small muted">{match.vetoTurnTeam ? `${match.vetoTurnTeam} to ${match.vetoStep < 6 ? `${picking ? "pick" : "ban"} a map` : "choose their starting side"}` : "Waiting for the next team"}</p>}
            {match.canVeto && match.vetoStep < 6 && <div className="veto-map-grid">{remainingMaps.map((map) => <button key={map} className="secondary" disabled={!live || busy} onClick={() => onAction(`/player/tournaments/${tournament.id}/matches/${match.id}/veto`, { map })}>{picking ? "Pick" : "Ban"} {mapLabel(map)}</button>)}</div>}
            {match.canVeto && match.vetoStep === 6 && <div className="veto-map-grid"><button className="secondary" disabled={!live || busy} onClick={() => onAction(`/player/tournaments/${tournament.id}/matches/${match.id}/veto`, { side: "ct" })}>Start CT</button><button className="secondary" disabled={!live || busy} onClick={() => onAction(`/player/tournaments/${tournament.id}/matches/${match.id}/veto`, { side: "t" })}>Start T</button></div>}
            {match.vetoStep === 7 && <p className="small" role="status">{match.mapSequence.map(mapLabel).join(" → ")} · Team B starts {match.teamBSide?.toUpperCase()}. Waiting for the host to open the server room.</p>}
          </section>}
          {match.roomId && <><p className="small muted">Match room · {match.roomStatus || "pending"}{match.map && ` · ${mapLabel(match.map)}`}</p><button className="secondary" onClick={() => onViewMatches(match.roomId!)}>View match room</button></>}
        </div>;
      }} />}
      {tournament.winnerTeamId && <p role="status">Champion: {tournament.entries.find((entry) => entry.teamId === tournament.winnerTeamId)?.teamName || "Champion decided"}</p>}
    </div>
  </article>;
}

export function PlayerApp() {
  const fragmentInvite = window.location.hash.startsWith("#teams?") ? new URLSearchParams(window.location.hash.split("?", 2)[1]).get("invite") || "" : "";
  const initialInvite = fragmentInvite || (window.location.hash === "#teams" ? sessionStorage.getItem("clutchbase-pending-invite") || "" : "");
  const [identity, setIdentity] = useState<{ steamId: string | null; steamEnabled: boolean; gameName: string } | null>(null);
  const [sessionAttempt, setSessionAttempt] = useState(0);
  const [profileName, setProfileName] = useState("");
  const [savingName, setSavingName] = useState(false);
  const [profileNotice, setProfileNotice] = useState("");
  const [snapshot, setSnapshot] = useState<PlayerWorkspace | null>(null);
  const [live, setLive] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busyMatch, setBusyMatch] = useState("");
  const [confirmPlayerClose, setConfirmPlayerClose] = useState("");
  const [creatingRoom, setCreatingRoom] = useState(false);
  const [busyTeam, setBusyTeam] = useState(false);
  const [teamInvite, setTeamInvite] = useState("");
  const teamVersion = useRef("");
  teamVersion.current = `${identity?.steamId || ""}:${snapshot?.team?.id || ""}:${snapshot?.team?.revision ?? ""}`;
  const [matchFilter, setMatchFilter] = useState<"open" | "closed" | "all">("open");
  const [matchSearch, setMatchSearch] = useState("");
  const [serverSearch, setServerSearch] = useState("");
  const [joinableOnly, setJoinableOnly] = useState(false);
  const [view, setView, selectedEntityID] = useView(playerViews, "servers");
  const selectedRoomID = view === "matches" ? selectedEntityID : "";
  const selectedTournamentID = view === "tournaments" ? selectedEntityID : "";
  const selectedTournament = snapshot?.tournaments?.find((tournament) => tournament.id === selectedTournamentID);
  useEffect(() => {
    if (view === "tournaments" && selectedTournament) {
      const card = document.getElementById(`tournament-${selectedTournamentID}`);
      card?.focus(); card?.scrollIntoView?.({ block: "start" });
    }
  }, [view, selectedTournamentID, !!selectedTournament]);
  const playerMatches = snapshot?.matches || [];
  const visibleServers = (snapshot?.servers || []).filter((server) =>
    (!joinableOnly || server.canJoin) && `${server.name} ${server.map || ""} ${connection(server)}`.toLocaleLowerCase().includes(serverSearch.trim().toLocaleLowerCase())
  );
  const selectedRoom = playerMatches.find((match) => match.id === selectedRoomID);
  const openMatches = playerMatches.filter((match) => match.status === "open");
  const closedMatches = playerMatches.filter((match) => match.status === "closed");
  const filteredMatches = matchFilter === "open" ? openMatches : matchFilter === "closed" ? closedMatches : playerMatches;
  const visibleMatches = (selectedRoomID ? playerMatches.filter((match) => match.id === selectedRoomID || filteredMatches.includes(match)) : filteredMatches).filter((match) => match.id === selectedRoomID || `${match.title} ${match.hostName} ${match.map} ${(match.participants || []).map((p) => p.gameName).join(" ")}`.toLocaleLowerCase().includes(matchSearch.trim().toLocaleLowerCase()));
  useEffect(() => {
    let cancelled = false;
    void request<{ steamId: string | null; steamEnabled: boolean; gameName: string }>("", "/player/session")
      .then((value) => { if (!cancelled) { setIdentity(value); setProfileName(value.gameName); } })
      .catch(() => { if (!cancelled) setError("The API is unavailable."); });
    return () => { cancelled = true; };
  }, [sessionAttempt]);
  useEffect(() => {
    if (!identity?.steamId) return;
    return connectLive<PlayerWorkspace>("player", undefined,
      (next) => {
        setSnapshot((previous) => !previous || next.capturedAt >= previous.capturedAt ? next : previous);
        setError("");
      },
      (online) => {
        setLive(online);
        if (!online) setError("Live connection lost. Reconnecting…");
      },
      () => {
        setIdentity((previous) => ({ steamId: null, steamEnabled: previous?.steamEnabled ?? true, gameName: "" }));
        setProfileName("");
        setSnapshot(null);
        setView("servers");
        setError("Your session expired. Sign in with Steam again.");
      },
    );
  }, [identity?.steamId]);
  async function signOut() {
    try {
      await request("", "/player/logout", { method: "POST" });
      setIdentity((previous) => ({ steamId: null, steamEnabled: previous?.steamEnabled ?? true, gameName: "" }));
      setSnapshot(null);
      setProfileName("");
      setLive(false);
      setView("servers");
      setError("");
    } catch { setError("Sign out failed. Try again."); }
  }
  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text);
      setNotice("Connection command copied. Use the join password shared by the host.");
    } catch { setError("Clipboard unavailable. Select and copy the displayed address."); }
  }
  async function saveProfile(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSavingName(true); setError(""); setProfileNotice("");
    try {
      const result = await request<{ gameName: string }>("", "/player/profile", {
        method: "PATCH", body: JSON.stringify({ gameName: profileName }),
      });
      setIdentity((previous) => previous ? { ...previous, gameName: result.gameName } : previous);
      setProfileName(result.gameName); setProfileNotice("In-game name saved");
    } catch (err) { setError(err instanceof Error ? err.message : "Could not save your name"); }
    finally { setSavingName(false); }
  }
  async function createRoom(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    setCreatingRoom(true); setError("");
    try {
      await request("", "/player/matches", { method: "POST", body: JSON.stringify({ hostId: data.get("hostId"), title: data.get("title"), map: data.get("map") || "", playersPerSide: Number(data.get("playersPerSide")), side: data.get("side"), overtime: data.get("overtime") !== null, manualReady: data.get("manualReady") !== null }) });
      form.reset(); setNotice("Room opened. It will appear when the live view updates.");
    } catch (err) { setError(err instanceof Error ? err.message : "Could not open match room"); }
    finally { setCreatingRoom(false); }
  }
  async function matchAction(match: PlayerMatch, action: "join" | "leave" | "ready" | "side", joinSide?: "ct" | "t") {
    setBusyMatch(match.id); setError("");
    const side = action === "side" ? (match.mySide === "ct" ? "t" : "ct") : "";
    const route = action === "side" ? "side" : action;
    try {
      await request("", `/player/matches/${match.id}/${route}`, action === "ready"
        ? { method: "PATCH", body: JSON.stringify({ ready: !match.myReady }) }
        : action === "side" ? { method: "PATCH", body: JSON.stringify({ side }) }
        : { method: "POST", ...(action === "join" ? { body: JSON.stringify({ side: joinSide }) } : {}) });
    } catch (err) { setError(err instanceof Error ? err.message : "Match action failed"); }
    finally { setBusyMatch(""); }
  }
  async function manageOwnRoom(match: PlayerMatch, action: "load" | "close") {
    setBusyMatch(match.id); setError("");
    try { await request("", `/player/matches/${match.id}/${action}`, { method: "POST" }); setConfirmPlayerClose(""); }
    catch (err) { setError(err instanceof Error ? err.message : "Could not update match room"); }
    finally { setBusyMatch(""); }
  }
  async function voteMap(match: PlayerMatch, map: string) {
    setBusyMatch(match.id); setError("");
    try { await request("", `/player/matches/${match.id}/map-vote`, map ? { method: "PUT", body: JSON.stringify({ map }) } : { method: "DELETE" }); }
    catch (err) { setError(err instanceof Error ? err.message : "Could not save map vote"); }
    finally { setBusyMatch(""); }
  }
  async function chooseVotedMap(match: PlayerMatch, map: string) {
    if (!match.actions.edit || !match.settingsRevision) return;
    setBusyMatch(match.id); setError("");
    try {
      const result = await request<{ readinessReset: boolean }>("", `/player/matches/${match.id}/settings`, { method: "PATCH", body: JSON.stringify({ title: match.title, map, playersPerSide: match.playersPerSide, overtime: match.overtime, manualReady: !!match.manualReady, settingsRevision: match.settingsRevision }) });
      setNotice(result.readinessReset ? "Map selected. Everyone must ready up again." : "Map selected.");
    } catch (err) { setError(err instanceof Error ? err.message : "Could not select map"); }
    finally { setBusyMatch(""); }
  }
  async function submitReport(event: FormEvent<HTMLFormElement>, match: PlayerMatch) {
    event.preventDefault();
    const form = event.currentTarget;
    const description = String(new FormData(form).get("description") || "");
    setBusyMatch(match.id); setError("");
    try { await request("", `/player/matches/${match.id}/report`, { method: "POST", body: JSON.stringify({ description }) }); form.reset(); setNotice("Report submitted. You can follow it in My reports."); }
    catch (err) { setError(err instanceof Error ? err.message : "Could not submit report"); }
    finally { setBusyMatch(""); }
  }
  async function controlMatch(match: PlayerMatch, action: "pause" | "resume") {
    setBusyMatch(match.id); setError("");
    try {
      await request("", `/player/matches/${match.id}/control`, { method: "POST", body: JSON.stringify({ action, revision: match.controlRevision }) });
    } catch (err) { setError(err instanceof Error ? err.message : "Match control failed"); }
    finally { setBusyMatch(""); }
  }
  async function teamAction(path: string, options: RequestInit): Promise<boolean> {
    const requestedTeamVersion = teamVersion.current;
    setBusyTeam(true); setError("");
    try {
      const result = await request<{ code?: string }>("", path, options);
      if (result?.code && requestedTeamVersion === teamVersion.current) {
        setTeamInvite(result.code);
        try { await navigator.clipboard.writeText(result.code); setNotice("Invite generated and copied. It expires in 24 hours."); }
        catch { setNotice("Invite generated. Copy the displayed code; it expires in 24 hours."); }
      }
      return true;
    } catch (err) { setError(err instanceof Error ? err.message : "Team action failed"); return false; }
    finally { setBusyTeam(false); }
  }
  useEffect(() => { setTeamInvite(""); }, [snapshot?.team?.id, snapshot?.team?.revision, identity?.steamId]);
  async function submitTeam(event: FormEvent<HTMLFormElement>, joining: boolean) {
    event.preventDefault(); const form = new FormData(event.currentTarget);
    const success = await teamAction(joining ? "/player/teams/join" : "/player/teams", { method: "POST", body: JSON.stringify(joining ? { code: form.get("code") } : { name: form.get("name") }) });
    if (joining && success) { sessionStorage.removeItem("clutchbase-pending-invite"); if (window.location.hash.startsWith("#teams?")) window.history.replaceState(null, "", "#teams"); }
  }
  async function publicTournamentAction(path: string) { await teamAction(path, { method: "POST" }); }
  async function submitVeto(path: string, selection?: { map?: string; side?: "ct" | "t" }) {
    await teamAction(path, { method: "POST", ...(selection ? { body: JSON.stringify(selection) } : {}) });
  }
  async function submitTournamentDispute(tournamentID: string, matchID: string, reason: string) {
    return teamAction(`/player/tournaments/${tournamentID}/matches/${matchID}/dispute`, { method: "POST", body: JSON.stringify({ reason }) });
  }
  const shownView = identity?.steamId || view === "access" ? view : "servers";
  useEffect(() => {
    if (shownView === "matches" && selectedRoom) setMatchFilter(selectedRoom.status);
  }, [shownView, selectedRoomID, selectedRoom?.status]);
  useEffect(() => {
    if (shownView !== "matches" || !selectedRoomID || !selectedRoom) return;
    const card = document.getElementById(`match-room-${selectedRoomID}`);
    card?.focus();
    card?.scrollIntoView?.({ block: "center" });
  }, [shownView, selectedRoomID, matchFilter, !!selectedRoom]);
  return (
    <div className="app-shell public-shell">
      <aside className="sidebar">
        <a className="brand" href="/play"><span className="mark" aria-hidden="true">c<span>↗</span></span><span>clutchbase.</span></a>
        <nav aria-label="Public pages">
          <button className={`nav-item nav-button ${shownView === "servers" ? "selected" : ""}`} aria-current={shownView === "servers" ? "page" : undefined} onClick={() => setView("servers")}>Servers {snapshot && <span className="count">{snapshot.servers.length}</span>}</button>
          {identity?.steamId && <>
            <button className={`nav-item nav-button ${shownView === "matches" ? "selected" : ""}`} aria-current={shownView === "matches" ? "page" : undefined} onClick={() => setView("matches")}>Matches {snapshot && <span className="count">{snapshot.matches.length}</span>}</button>
            <button className={`nav-item nav-button ${shownView === "tournaments" ? "selected" : ""}`} aria-current={shownView === "tournaments" ? "page" : undefined} onClick={() => setView("tournaments")}>Tournaments {snapshot && <span className="count">{snapshot.tournaments?.length || 0}</span>}</button>
            <button className={`nav-item nav-button ${shownView === "teams" ? "selected" : ""}`} aria-current={shownView === "teams" ? "page" : undefined} onClick={() => setView("teams")}>My team {snapshot?.team && <span className="count">{snapshot.team.members.length}</span>}</button>
            <button className={`nav-item nav-button ${shownView === "history" ? "selected" : ""}`} aria-current={shownView === "history" ? "page" : undefined} onClick={() => setView("history")}>Match history</button>
            <button className={`nav-item nav-button ${shownView === "schedule" ? "selected" : ""}`} aria-current={shownView === "schedule" ? "page" : undefined} onClick={() => setView("schedule")}>Schedule</button>
            <button className={`nav-item nav-button ${shownView === "stats" ? "selected" : ""}`} aria-current={shownView === "stats" ? "page" : undefined} onClick={() => setView("stats")}>My statistics</button>
            <button className={`nav-item nav-button ${shownView === "leaderboard" ? "selected" : ""}`} aria-current={shownView === "leaderboard" ? "page" : undefined} onClick={() => setView("leaderboard")}>Leaderboard</button>
            <button className={`nav-item nav-button ${shownView === "reports" ? "selected" : ""}`} aria-current={shownView === "reports" ? "page" : undefined} onClick={() => setView("reports")}>My reports</button>
            <button className={`nav-item nav-button ${shownView === "access" ? "selected" : ""}`} aria-current={shownView === "access" ? "page" : undefined} onClick={() => setView("access")}>Access requests</button>
            <button className={`nav-item nav-button ${shownView === "profile" ? "selected" : ""}`} aria-current={shownView === "profile" ? "page" : undefined} onClick={() => setView("profile")}>Profile</button>
          </>}
        </nav>
        <a className="nav-item" href="/manage">Manage</a>
        {identity?.steamId && <button className="text-button logout" onClick={() => void signOut()}>↪ Sign out</button>}
      </aside>
      <div className="workspace">
        <main className="content" id="play">
          <div className="page-heading"><h1>{shownView === "servers" ? "Servers" : shownView === "matches" ? "Matches" : shownView === "tournaments" ? "Tournaments" : shownView === "schedule" ? "Schedule" : shownView === "teams" ? "My team" : shownView === "history" ? "Match history" : shownView === "stats" ? "My statistics" : shownView === "leaderboard" ? "Leaderboard" : shownView === "reports" ? "My reports" : shownView === "access" ? "Access requests" : "Profile"}</h1>{identity?.steamId && <span className="small muted">SteamID {identity.steamId}</span>}</div>
          {error && <p className="error banner" role="alert">{error}</p>}
          {notice && <p className="small muted" role="status">{notice}</p>}
          {profileNotice && <p className="small muted" role="status">{profileNotice}</p>}
          {identity?.steamId && shownView === "profile" && <form id="profile" className="profile-form" onSubmit={(event) => void saveProfile(event)}>
            <label>In-game name<input value={profileName} onChange={(event) => setProfileName(event.target.value)} required minLength={1} maxLength={32} placeholder="Name friends see in room rosters" /></label>
            <button className="secondary" type="submit" disabled={savingName || !live}>{savingName ? "Saving…" : "Save in-game name"}</button>
            <p className="small muted">This name appears in match room rosters. It is not fetched from Steam.</p>
          </form>}
          {identity?.steamId && shownView === "teams" && <section className="match-rooms" aria-label="Team management">
            {!snapshot ? <p role="status">Loading your team…</p> : !snapshot.team ? <div className="empty"><p>You are not on a team yet.</p><div className="server-grid"><article className="server-card"><div className="server-body"><h2>Create a team</h2><form onSubmit={(event) => void submitTeam(event, false)}><label>Team name<input name="name" required maxLength={48} placeholder="Raven Five" /></label><button className="primary" disabled={!live || busyTeam}>{busyTeam ? "Saving…" : "Create team"}</button></form></div></article><article className="server-card"><div className="server-body"><h2>Join a team</h2><form onSubmit={(event) => void submitTeam(event, true)}><label>Invite code<input name="code" defaultValue={initialInvite} required minLength={20} autoComplete="off" placeholder="Paste an invite code" /></label><button className="secondary" disabled={!live || busyTeam}>{busyTeam ? "Joining…" : "Join team"}</button></form></div></article></div></div> : <PublicTeam key={snapshot.team.id} team={snapshot.team} steamId={identity.steamId} live={live} busy={busyTeam} invite={teamInvite} onAction={teamAction} />}
          </section>}
          {identity?.steamId && shownView === "tournaments" && <section className="match-rooms" aria-label="Tournaments">
            <TournamentPicker tournaments={snapshot?.tournaments || []} loading={!snapshot} selectedID={selectedTournamentID} onSelect={(id) => setView("tournaments", id)} onBack={() => setView("tournaments")} />
            {snapshot && <div className="server-grid">{(selectedTournament ? [selectedTournament] : []).map((tournament) => <PlayerTournamentCard key={tournament.id} tournament={tournament} live={live} busy={busyTeam} teamName={snapshot.team?.name} onAction={(path, selection) => void submitVeto(path, selection)} onWithdraw={(id, selection) => teamAction(`/player/tournaments/${id}/registration`, { method: "DELETE", body: JSON.stringify(selection) })} onViewMatches={(roomId) => setView("matches", roomId)} onDispute={submitTournamentDispute} />)}</div>}
          </section>}
          {shownView === "servers" && (!identity ? <div className="empty">{error ? <><p>Unable to load your session.</p><button className="secondary" onClick={() => { setError(""); setSessionAttempt((attempt) => attempt + 1); }}>Try again</button></> : <p role="status">Loading your session…</p>}</div> : !identity.steamId ? (
            <div className="empty">
              <p>Sign in to see the servers you have access to.</p>
              {identity.steamEnabled ? <a className="primary steam-sign-in" href={`/api/auth/steam?returnTo=${encodeURIComponent(initialInvite ? "#teams" : viewHash(view, selectedEntityID))}`} onClick={() => { if (fragmentInvite) sessionStorage.setItem("clutchbase-pending-invite", fragmentInvite); }}>Sign in with Steam ↗</a> : <p className="muted">Steam sign-in is not configured on this instance.</p>}
              <p className="small muted">Server administrators can use <a href="/manage">Manage</a>.</p>
            </div>
          ) : !snapshot ? <div className="empty" role="status">Connecting to your servers…</div> : snapshot.servers.length === 0 ? (
            <div className="empty"><p>No servers have been shared with your SteamID yet.</p><p className="small muted">Ask the host to add your SteamID to a server’s whitelist.</p></div>
          ) : <><div className="fleet-filters" role="search" aria-label="Filter servers">
            <label>Search servers<input type="search" value={serverSearch} onChange={(event) => setServerSearch(event.target.value)} placeholder="Name, map or address" /></label>
            <label>Availability<select value={joinableOnly ? "joinable" : "all"} onChange={(event) => setJoinableOnly(event.target.value === "joinable")}><option value="all">All servers</option><option value="joinable">Ready to join</option></select></label>
            <span className="small muted" role="status">{visibleServers.length} of {snapshot.servers.length} servers</span>
          </div>{visibleServers.length === 0 && <p role="status">No servers match these filters.</p>}<div className="server-grid">{visibleServers.map((server) => {
            const room = snapshot.matches.find((match) => match.hostId === server.id && match.status === "open");
            return (
            <article className="server-card" key={server.id}>
              <div className="server-body">
                <div className="server-card-head">
                  <div className="server-title"><h2>{server.name}</h2>{server.simulated && <span className="simulation">SIMULATED</span>}</div>
                  <span className={`state-pill state-${server.status}`}><i className="tiny-dot" />{!live ? "Last reported: " : ""}{server.canJoin ? "Ready to join" : server.status}</span>
                </div>
                <p className="server-meta">{server.map ? server.map.replace(/^de_/, "") : "Map unavailable"}{server.status === "running" && server.players !== null && server.maxPlayers !== null ? ` · ${server.players}/${server.maxPlayers} players` : ""}</p>
                <div className="connection"><code>{connection(server)}</code></div>
                <p className="small muted">{server.preset ? `${presetName(server.preset)} · ${presetStatus(server.presetStatus)}` : server.activePreset ? `${presetName(server.activePreset)} running` : "Host default preset"}</p>
                {(server.matchPhase === "live" || server.matchPhase === "paused") && <p className="small muted" role="status">{server.matchPhase === "paused" ? "MatchZy pause requested or active" : "Match live"}</p>}
                {server.gameBuildStatus === "outdated" && <p className="small error" role="status">{!live && "Last reported: "}CS2 server update available. Connections may fail until the host updates it.</p>}
                {!server.canJoin && server.joinBlocker && <p className="small muted" role="status">{server.joinBlocker}</p>}
                {room && <button className="server-room-link" onClick={() => setView("matches", room.id)}><span><strong>{room.title}</strong><small>{room.players} joined · {room.ready} ready</small></span><span aria-hidden="true">→</span></button>}
                <p className="small muted join-help">Use the join password shared by the host.</p>
                <div className="server-footer"><span className="small muted">{live ? (server.canJoin ? "Ready to join" : "Waiting for the host") : "Reconnecting…"}</span><button className="primary" disabled={!live || !server.canJoin} onClick={() => void copy(`connect ${connection(server)}`)}>Copy join command ↗</button></div>
              </div>
            </article>
          ); })}</div></>)}
          {identity?.steamId && shownView === "matches" && <section className="match-rooms" id="player-matches" aria-labelledby="player-matches-heading">
            <h2 id="player-matches-heading" className="visually-hidden">Matches</h2>
            {!!snapshot?.servers.some((server) => server.canOpenMatch) && <details><summary>Open a match</summary><form className="match-create-form" onSubmit={(event) => void createRoom(event)}>
              <label>Server<select name="hostId" required defaultValue=""><option value="">Choose a server…</option>{snapshot.servers.filter((server) => server.canOpenMatch).map((server) => <option key={server.id} value={server.id}>{server.name}</option>)}</select></label>
              <label>Match name<input name="title" required maxLength={80} /></label>
              <label>Map<select name="map" defaultValue=""><option value="">Current server map</option>{(snapshot.maps || []).map((map) => <option key={map} value={map}>{mapLabel(map)}</option>)}</select></label>
              <label>Room size<select name="playersPerSide" defaultValue="5">{[1, 2, 3, 4, 5].map((size) => <option key={size} value={size}>{size}v{size}</option>)}</select></label>
              <label>Your side<select name="side" defaultValue="ct"><option value="ct">CT</option><option value="t">T</option></select></label>
              <label className="overtime-choice"><input type="checkbox" name="overtime" /> Enable overtime</label>
              <label className="overtime-choice"><input type="checkbox" name="manualReady" /> Require in-game ready</label>
              <p className="small muted">Players use .ready in CS2 before starting. This requires an updated host agent and reporter; uneven teams always use in-game ready.</p>
              <p className="small muted">You join this room automatically. You can load the match when both sides are ready.</p>
              <button className="primary" disabled={!live || creatingRoom}>{creatingRoom ? "Opening…" : "Open match room"}</button>
            </form></details>}
            {!snapshot && <p role="status">Connecting to your matches…</p>}
            {snapshot && snapshot.matches.length >= 50 && <p className="small muted">Showing active and unresolved rooms and up to 50 recent closed rooms.</p>}
            {snapshot && snapshot.matches.length === 0 && !selectedRoomID && <p className="small muted">No matches available.</p>}
            {snapshot && selectedRoomID && !selectedRoom && <div className="empty" role="status"><p>This match room is not available to your account. If it was completed, check Match history.</p><button className="secondary" onClick={() => setView("history")}>View match history</button></div>}
            {snapshot && snapshot.matches.length > 0 && <><MatchFilter value={matchFilter} onChange={(filter) => { setMatchFilter(filter); if (selectedRoomID) setView("matches"); }} openCount={openMatches.length} closedCount={closedMatches.length} label="Filter match rooms" /><label>Search match rooms<input aria-label="Search match rooms" type="search" value={matchSearch} onChange={(event) => setMatchSearch(event.target.value)} /></label>{matchSearch && <button type="button" className="secondary" onClick={() => setMatchSearch("")}>Clear search</button>}</>}
            {snapshot && snapshot.matches.length > 0 && visibleMatches.length === 0 && <p className="small muted" role="status">{matchSearch ? "No match rooms match your search." : `No ${matchFilter} match rooms.`} Use Match history to browse recorded results.</p>}
            <div className="server-grid match-grid">{visibleMatches.map((match) => {
              const matchServer = snapshot?.servers.find((server) => server.id === match.hostId);
              const roomStatus = matchRoomStatus(match);
              return <article className="server-card match-card public-room-card" id={`match-room-${match.id}`} tabIndex={-1} key={match.id}>
              <div className="server-body"><div className="server-card-head"><div><div className="server-title"><h2>{match.title}</h2></div><p className="server-meta">{match.hostName} · {match.map.replace(/^de_/, "")} · {match.overtime ? "Overtime on" : "Overtime off"}{match.manualReady && " · In-game ready required"}</p></div><span className={`state-pill state-${roomStatus.state}`}>{roomStatus.label}</span></div>
                <p className="match-summary">{match.players} joined <span aria-hidden="true">·</span> {match.ready} ready</p>
                <p className="player-match-next-step" role="status"><strong>Next step</strong>{matchNextStep(match, matchServer, live)}</p>
                {match.joined && match.loadBlockers && match.loadBlockers.length > 0 && <div className="match-preflight"><strong>Before loading</strong><ul>{loadBlockerLabels(match.loadBlockers).map((blocker, index) => <li key={index}>{blocker}</li>)}</ul></div>}
                {match.manualReady && match.loadStatus === "" && matchServer && !matchServer.supportsManualReady && <p className="small error" role="status">The host must update the agent and reporter before loading this match with in-game ready.</p>}
                {matchServer?.gameBuildStatus === "outdated" && <p className="small error" role="status">{!live && "Last reported: "}CS2 server update available. Connections may fail until the host updates it.</p>}
                <RoomLiveScore score={match.liveScore} connected={live} active={match.matchPhase === "live" || match.matchPhase === "paused"} />
                {match.actions.load && <button className="primary" disabled={!live || busyMatch === match.id} onClick={() => void manageOwnRoom(match, "load")}>Load match</button>}
                {match.actions.close && (confirmPlayerClose !== match.id ? <button className="secondary" disabled={!live || busyMatch === match.id} onClick={() => setConfirmPlayerClose(match.id)}>Close room</button> : <div className="team-confirmation"><p>Close this unstarted room?</p><button className="secondary" disabled={!live || busyMatch === match.id} onClick={() => void manageOwnRoom(match, "close")}>Confirm close room</button><button className="secondary" disabled={busyMatch === match.id} onClick={() => setConfirmPlayerClose("")}>Keep room</button></div>)}

                <MatchLineups participants={match.participants} playersPerSide={match.playersPerSide} ctPlayers={match.ctPlayers} tPlayers={match.tPlayers}
                  renderPlayer={(participant, index) => <div className="match-player" key={`${participant.side}-${index}`}><PlayerScore participant={participant} matchId={match.id} online={live} showReady={match.status === "open"} compact /><PlayerRoomRemove match={match} participant={participant} connected={live} /></div>}
                  renderSideActions={(side) => {
                    if (!match.joined && match.status === "open") {
                      const canJoin = side === "ct" ? match.actions.joinCT : match.actions.joinT;
                      const sideCount = side === "ct" ? match.ctPlayers : match.tPlayers;
                      return <button type="button" className={`match-side-join match-side-join-${side}`} aria-label={canJoin ? `Join ${side.toUpperCase()} side in ${match.title}` : `${side.toUpperCase()} side ${sideCount >= match.playersPerSide ? "full" : "unavailable"}`} disabled={!live || !canJoin || busyMatch === match.id} onClick={() => void matchAction(match, "join", side)}>
                        {canJoin ? `Join ${side.toUpperCase()}` : sideCount >= match.playersPerSide ? `${side.toUpperCase()} full` : `${side.toUpperCase()} unavailable`}
                      </button>;
                    }
                    if (!match.joined || match.mySide !== side) return null;
                    return <div className="match-side-actions">
                      {match.actions.switchSide && <button type="button" className="secondary" disabled={!live || busyMatch === match.id} onClick={() => void matchAction(match, "side")}>Switch sides</button>}
                      {(match.actions.ready || match.actions.unready) && <button type="button" className="primary" disabled={!live || busyMatch === match.id} onClick={() => void matchAction(match, "ready")}>{busyMatch === match.id ? "Saving…" : match.myReady ? "Unready" : "Ready up"}</button>}
                      {match.actions.leave && <button type="button" className="secondary" disabled={!live || busyMatch === match.id} onClick={() => void matchAction(match, "leave")}>Leave room</button>}
                    </div>;
                  }} />
                <MatchMapVote votes={match.mapVotes || []} maps={snapshot?.maps || []} current={match.map} mine={match.myMapVote} canVote={match.actions.voteMap} canChoose={!!match.actions.edit && !!match.settingsRevision} connected={live} busy={busyMatch === match.id} onVote={(map) => void voteMap(match, map)} onChoose={(map) => void chooseVotedMap(match, map)} />
                {match.loadStatus === "pending" && <p className="small muted" role="status">Loading the MatchZy setup…</p>}
                {match.loadStatus === "complete" && !match.setupUnloaded && <p className="small muted" role="status">MatchZy confirmed the setup was loaded · {match.joined ? "your side is locked" : "the roster is locked"}</p>}
                {match.joined && match.loadStatus === "complete" && match.status === "open" && !match.setupUnloaded && matchServer?.canJoin && <div className="server-footer"><span className="small muted">{live ? "Server ready" : "Last reported: server ready"} · {connection(matchServer)}. Use the join password shared by the host.</span><button className="primary" disabled={!live} onClick={() => void copy(`connect ${connection(matchServer)}`)}>Copy connect command ↗</button></div>}
                {match.joined && match.loadStatus === "complete" && match.status === "open" && !match.setupUnloaded && !matchServer?.canJoin && <p className="small muted" role="status">Waiting for the server to be ready before showing the connect command.</p>}
                {match.setupUnloaded && <p role="status">No match setup is currently loaded on the server. The host can close this room.</p>}
                {match.loadStatus === "complete" && match.status === "open" && !match.setupUnloaded && (!match.manualReady && match.ctPlayers === match.tPlayers
                  ? <p className="small muted">Balanced rosters auto-ready in MatchZy when players join their assigned sides.</p>
                  : <p className="small muted">{match.ctPlayers === match.tPlayers ? "Players use MatchZy’s in-game .ready command before starting." : "Uneven rosters use MatchZy’s in-game .ready command. Wait until everyone on the larger side has joined first."}</p>)}
                {match.endedAt && <p className="small muted" role="status">MatchZy ended the match without recording a winner</p>}
                {match.resultCT != null && match.resultT != null && <p role="status">Manual result · started CT {match.resultCT} – {match.resultT} started T</p>}
                {match.reportedCT != null && match.reportedT != null && <p role="status">{match.seriesMatch ? `Reported final map · CT ${match.reportedCT} – ${match.reportedT} T` : `Reported final score · started CT ${match.reportedCT} – ${match.reportedT} started T`}</p>}
                {match.loadStatus === "failed" && <p className="small error" role="status">Match setup failed: {match.loadError || "ask the host to retry"}</p>}
                {(match.actions.pause || match.actions.resume || match.controlStatus === "pending" || match.controlStatus === "failed" || match.matchPhase === "live" || match.matchPhase === "paused") && <section className="match-admin-controls" aria-label={`Match controls for ${match.title}`}>
                  <div><strong>{match.matchPhase === "paused" ? "Match paused or pause requested" : match.matchPhase === "live" ? "Match live" : "Match controls"}</strong>
                    <p className="small muted" role="status">{match.controlStatus === "pending" ? `${match.controlAction === "pause" ? "Pause" : match.controlAction === "resume" ? "Resume" : "End"} requested. Waiting for MatchZy.` : match.controlStatus === "failed" ? match.controlError || "Match control failed. Wait for fresh server state before retrying." : "Pause and resume requests are applied by MatchZy."}</p>
                  </div>
                  {match.matchPhase === "live" && <PauseVoting threshold={match.pauseVoteThreshold} votes={match.pauseVotes} voted={match.myPauseVote} canVote={match.actions.pause} connected={live} busy={busyMatch === match.id} onVote={() => void controlMatch(match, "pause")} />}
                  <div className="heading-actions">
                    {match.actions.resume && <button className="secondary" disabled={!live || busyMatch === match.id} onClick={() => void controlMatch(match, "resume")}>Request resume</button>}
                  </div>
                </section>}
                <div className="public-room-tools"><ShareRoom id={match.id} title={match.title} /><RoomReopen id={match.id} title={match.title} revision={match.settingsRevision} allowed={!!match.actions.reopen} connected={live} /><PlayerRoomSettings match={match} maps={snapshot?.maps || []} connected={live} /><PlayerRoomShuffle match={match} connected={live} /></div>
                {match.actions.report && <details><summary>Report a problem</summary><form className="match-report-form" onSubmit={(event) => void submitReport(event, match)}><label>Describe the problem<textarea name="description" required maxLength={1000} rows={3} /></label><p className="small muted">Only you and administrators can see this report. One report per match.</p><button className="secondary" type="submit" disabled={!live || busyMatch === match.id}>Submit report</button></form></details>}
                {match.joined && <p className="small muted">Your side: {match.mySide.toUpperCase() || "not selected"} · {match.myReady ? "ready" : "not ready"}</p>}
              </div>
              </article>;
            })}</div>
          </section>}
          {identity?.steamId && shownView === "history" && <MatchHistory player onClose={() => setView("servers")} />}
          {identity?.steamId && shownView === "stats" && <Statistics player />}
          {identity?.steamId && shownView === "leaderboard" && <Leaderboard player />}
          {identity?.steamId && shownView === "reports" && <Reports player />}
          {shownView === "access" && <PlayerAccessRequests key={`${identity?.steamId || "anonymous"}:${selectedEntityID}`} identity={identity} hostId={selectedEntityID} />}
          {identity?.steamId && shownView === "schedule" && <FixtureSchedule tournaments={snapshot?.tournaments || []} connected={live} loading={!snapshot} onOpen={(id) => setView("tournaments", id)} />}
        </main>
      </div>
    </div>
  );
}
