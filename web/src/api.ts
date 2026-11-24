export type Host = {
  availableMaps?: string[];
  reporterInventory?: { status: "unknown" | "unavailable" | "current" | "different"; installedHash: string; stagedHash: string; checkedAt: string | null };
  restartPhase?: "" | "stopping" | "starting" | "cancelling" | "complete" | "cancelled" | "failed";
  restartSupported?: boolean;
  preset: string | null;
  presetApplied: boolean;
  activePreset: string;
  presetStatus: string;
  modeCommandMode: string;
  modeCommandStatus: "" | "pending" | "complete" | "failed" | "cancelled";
  modeCommandError: string;
  matchPhase: "unknown" | "idle" | "setup" | "live" | "paused";
  canLoadMatch: boolean;
  supportsMatchSeries: boolean;
  supportsManualReady?: boolean;
  matchControlAction: "" | "pause" | "resume" | "end";
  matchControlStatus: "" | "pending" | "complete" | "failed";
  matchControlError: string;
  id: string;
  machineId: string;
  name: string;
  address: string;
  port: number;
  map: string;
  desiredState: "running" | "stopped";
  observedState: string;
  activeMap: string;
  canObserveMap: boolean;
  reporterUpdateAvailable: boolean;
  reporterUpdateStatus: "" | "pending" | "complete" | "failed";
  reporterUpdateError: string;
  gameUpdateStatus: "" | "pending" | "complete" | "failed" | "cancelled";
  gameUpdateError: string;
  gameUpdateSupported?: boolean;
  allowPlayerRooms?: boolean;
  gameBuildStatus?: "unknown" | "current" | "outdated";
  gameBuildInstalled?: string;
  gameBuildLatest?: string;
  gameBuildCheckedAt?: string | null;
  status: string;
  revision: number;
  actions: { openMatch: boolean; start: boolean; stop: boolean; changeMap: boolean; remove: boolean; changeWhitelist: boolean; changePreset: boolean; switchMode: boolean; pauseMatch: boolean; resumeMatch: boolean; updateReporter: boolean; updateGame: boolean; stopAndUpdate?: boolean; cancelGameUpdate?: boolean; changePlayerRooms?: boolean; restart?: boolean; cancelRestart?: boolean };
  removing: boolean;
  whitelist: string[] | null;
  whitelistApplied: boolean;
  driver: string;
  error: string;
  players: number | null;
  maxPlayers: number | null;
  logTail: string;
  accessStatus: "" | "password" | "pending" | "verified" | "blocked" | "unknown";
  lastSeen: string | null;
  online: boolean;
};

export type Machine = {
  resources?: { cpuPercent: number | null; memoryTotalBytes: number; memoryAvailableBytes: number } | null;
  storage: { totalBytes: number; availableBytes: number } | null;
  id: string;
  name: string;
  address: string;
  driver: string;
  online: boolean;
  lastSeen: string | null;
  createServer: boolean;
};
export type MatchParticipant = { gameName: string; side: "ct" | "t"; ready: boolean; steamId?: string; removalToken?: string; stats: { kills: number; deaths: number; assists: number } | null; reportedStats: { kills: number; deaths: number; assists: number } | null };
export type LiveScore = { setupId: string; map: string; ct: number; t: number; observedAt: number };
export type MapVote = { map: string; votes: number };
export type MatchRoom = { loadBlockers?: string[]; canConfigurePauseVoting: boolean; pauseVoteThreshold: number; pauseVotes: number; controlRevision: number; mapVotes?: MapVote[]; liveScore?: LiveScore | null; id: string; hostId: string; hostName: string; title: string; map: string; overtime: boolean; manualReady?: boolean; status: "open" | "closed"; players: number; playersPerSide: number; ctPlayers: number; tPlayers: number; ready: number; participants: (MatchParticipant & { steamId: string })[]; createdAt: string; loadStatus?: "" | "pending" | "complete" | "failed"; loadError?: string; canLoad?: boolean; loadBlockedByFailure: boolean; canCancelLoad: boolean; cancelStatus: "" | "pending" | "complete" | "failed"; cancelError: string; canClose: boolean; canReopen?: boolean; canEnd: boolean; canEdit: boolean; canRemovePlayer: boolean; canShuffle?: boolean; settingsRevision: number; endedAt: string | null; resultCT: number | null; resultT: number | null; reportedCT: number | null; reportedT: number | null; seriesMatch?: boolean; resultRevision: number; canEditResult: boolean; setupUnloaded: boolean };
export function matchRoomStatus(match: Pick<MatchRoom, "status" | "loadStatus" | "setupUnloaded" | "endedAt">): { label: string; state: string } {
  if (match.status === "closed") return { label: "Closed", state: "stopped" };
  if (match.endedAt || match.setupUnloaded) return { label: "Setup ended", state: "stopped" };
  if (match.loadStatus === "pending") return { label: "Loading setup", state: "starting" };
  if (match.loadStatus === "failed") return { label: "Load failed", state: "error" };
  if (match.loadStatus === "complete") return { label: "Setup loaded", state: "running" };
  return { label: "Open room", state: "stopped" };
}
export type TeamMember = { steamId: string; gameName: string };
export type Team = { id: string; name: string; captainSteamId: string; members: TeamMember[]; revision: number; rosterLocked: boolean; registered: boolean; actions: { invite: boolean; leave: boolean; removeMember: boolean; transferCaptain: boolean; disband: boolean }; inviteCode?: string };
export type TournamentEntry = { teamId: string; teamName: string; status: string; members?: number };
export type TournamentMatch = { winnerHistory?: { previousWinner: string; newWinner: string; revision: number; changedAt: string }[]; winnerCorrected?: boolean; resultRevision?: number; canCorrectWinner?: boolean; dispute?: { reason: string; resolution?: string; revision: number }; canDispute?: boolean; scheduledAt?: string | null; scheduleRevision?: number; canSchedule?: boolean; bracketStage?: string; id: string; round: number; number: number; status: string; teamA: TournamentEntry | null; teamB: TournamentEntry | null; winnerTeamId: string; roomId?: string; roomStatus?: string; map?: string; reportedCT?: number | null; reportedT?: number | null; resultCT?: number | null; resultT?: number | null; teamAScore?: number | null; teamBScore?: number | null; vetoBans: string[]; vetoMaps: string[]; mapSequence: string[]; vetoStep: number; vetoTurnTeamId?: string; vetoTurnTeam?: string; teamBSide?: "ct" | "t"; canVeto: boolean; canReportWinner: boolean; canOpenRoom: boolean };
export type TournamentStanding = { teamId: string; teamName: string; rank: number; played: number; wins: number; losses: number };
export type Tournament = { canCancel?: boolean; format?: "single_elimination" | "double_elimination" | "round_robin"; standings?: TournamentStanding[]; id: string; name: string; playersPerSide: number; bestOf: 1 | 3 | 5; mapPool: string[]; status: string; winnerTeamId: string; entries: TournamentEntry[]; matches: TournamentMatch[]; canCheckIn: boolean; canBuildBracket: boolean; myTeamId?: string; myTeamRevision?: number; myEntryStatus?: string; canRegisterTeam: boolean; canCheckInTeam: boolean; canWithdrawTeam?: boolean };
export function tournamentMatchProgress(match: Pick<TournamentMatch, "status" | "teamA" | "teamB" | "roomStatus" | "vetoStep" | "canReportWinner">): { key: string; label: string } {
  if (match.status === "complete") return { key: "complete", label: "Complete" };
  if (match.status === "bye") return { key: "bye", label: "Bye" };
  if (match.status === "skipped") return { key: "skipped", label: "Not needed" };
  if (match.canReportWinner) return { key: "decision", label: "Result needed" };
  if (match.roomStatus === "open") return { key: "live", label: "In progress" };
  if (match.roomStatus === "closed") return { key: "result", label: "Processing result" };
  if (!match.teamA || !match.teamB) return { key: "waiting", label: "Waiting for teams" };
  if (match.vetoStep >= 7) return { key: "ready", label: "Ready for server" };
  if (match.vetoStep > 0) return { key: "veto", label: "Map veto" };
  return { key: "pending", label: "Ready to start" };
}
export type Workspace = { capturedAt: string; hosts: Host[]; matches: MatchRoom[]; maps: string[]; presets: string[]; machines: Machine[]; players: { steamId: string; gameName: string }[]; teams: Team[]; tournaments: Tournament[] };

export function tournamentSeriesDecided(bestOf: number, teamA?: number | null, teamB?: number | null): boolean {
  const target = Math.floor(bestOf / 2) + 1;
  return (teamA === target && teamB != null && teamB < target) || (teamB === target && teamA != null && teamA < target);
}

export class APIError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

export async function request<T>(
  token: string,
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const response = await fetch(`/api${path}`, {
    signal: AbortSignal.timeout(10000),
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...options.headers,
    },
  });
  const data = await response
    .json()
    .catch(() => ({ error: "The API returned an invalid response" }));
  if (!response.ok)
    throw new APIError(response.status, data.error || "Request failed");
  return data as T;
}

export function connection(host: Pick<Host, "address" | "port">): string {
  return `${host.address.includes(":") ? `[${host.address}]` : host.address}:${host.port}`;
}


export type PlayerServer = {
  availableMaps?: string[];
  supportsManualReady?: boolean;
  gameBuildStatus?: "unknown" | "current" | "outdated";
  matchPhase: "unknown" | "idle" | "setup" | "live" | "paused";
  preset: string | null;
  activePreset: string;
  presetStatus: string;
  simulated: boolean;
  id: string;
  name: string;
  address: string;
  port: number;
  map: string;
  status: string;
  players: number | null;
  maxPlayers: number | null;
  canJoin: boolean;
  canOpenMatch?: boolean;
  joinBlocker?: string;
};
export type PlayerWorkspace = {
  capturedAt: string;
  steamId: string;
  gameName: string;
  maps?: string[];
  servers: PlayerServer[];
  matches: PlayerMatch[];
  team: Team | null;
  tournaments: Tournament[];
};

export type AccessRequestStatus = "pending" | "approved" | "rejected" | "revoked";
export type PlayerAccessRequest = { id: string; hostId: string; hostName: string; status: AccessRequestStatus; revision: number };
export type AdminAccessRequest = PlayerAccessRequest & { steamId: string; gameName: string; canApprove: boolean; requiresWhitelistEnable: boolean; blocker: string };

export type PlayerMatch = Omit<MatchRoom, "createdAt" | "canLoad" | "loadBlockedByFailure" | "canCancelLoad" | "cancelStatus" | "cancelError" | "canClose" | "canEnd" | "canEdit" | "canRemovePlayer" | "canShuffle" | "settingsRevision" | "resultRevision" | "canEditResult" | "participants"> & { myPauseVote: boolean; matchPhase?: string; controlStatus?: string; controlAction?: string; controlError?: string; participants: MatchParticipant[]; settingsRevision?: number; rosterFingerprint?: string; myRoom?: boolean; myMapVote?: string; joined: boolean; myReady: boolean; mySide: "ct" | "t" | ""; canJoin: boolean; actions: { reopen?: boolean; edit?: boolean; shuffle?: boolean; load?: boolean; close?: boolean; voteMap?: boolean; report?: boolean; pause?: boolean; resume?: boolean; joinCT: boolean; joinT: boolean; switchSide: boolean; ready: boolean; unready: boolean; leave: boolean } };

export function presetName(value: string | null): string {
  return value === "match" ? "Match" : value === "practice" ? "Practice" : "Host default";
}
export function presetStatus(value: string): string {
  return ({ verified: "Runtime verified", pending: "Awaiting runtime verification", unknown: "Runtime mode unavailable", mismatch: "Running mode differs from selected preset" } as Record<string, string>)[value] || "";
}
