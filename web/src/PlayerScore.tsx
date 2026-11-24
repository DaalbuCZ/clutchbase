import { useState } from "react";
import { request } from "./api";
import type { MatchParticipant } from "./api";

export function PlayerScore({ participant, matchId, revision, token, canEdit, online, showReady, compact, hideStats }: { participant: MatchParticipant; matchId: string; revision?: number; token?: string; canEdit?: boolean; online: boolean; showReady?: boolean; compact?: boolean; hideStats?: boolean }) {
  const [editingRevision, setEditingRevision] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const stats = participant.stats;
  const reported = participant.reportedStats;
  async function save(values: { kills: number; deaths: number; assists: number } | null) {
    if (!token || !participant.steamId || editingRevision === null) return;
    setBusy(true); setError("");
    try {
      await request(token, `/matches/${matchId}/stats/${participant.steamId}`, { method: "PATCH", body: JSON.stringify({ revision: editingRevision, ...(values || { clear: true }) }) });
      setEditingRevision(null);
    } catch (err) { setError(err instanceof Error ? err.message : "Could not save stats"); }
    finally { setBusy(false); }
  }
  const stale = editingRevision !== null && editingRevision !== revision;
  const sideLabel = compact ? "" : ` · ${showReady ? "" : "started "}${participant.side.toUpperCase()}`;
  return <div>
    <span className="small muted">{participant.gameName}{sideLabel}{showReady && participant.ready ? " · Ready" : showReady ? " · Not ready" : ""}{!hideStats && stats ? ` · K/D/A ${stats.kills}/${stats.deaths}/${stats.assists} (manual)` : ""}{!hideStats && reported ? ` · K/D/A ${reported.kills}/${reported.deaths}/${reported.assists} (reported)` : ""}</span>
    {canEdit && token && participant.steamId && editingRevision === null && <button className="text-button" disabled={!online} onClick={() => { setEditingRevision(revision ?? 0); setError(""); }}>{stats ? "Edit stats" : "Record stats"}</button>}
    {editingRevision !== null && <form onSubmit={(event) => { event.preventDefault(); const data = new FormData(event.currentTarget); void save({ kills: Number(data.get("kills")), deaths: Number(data.get("deaths")), assists: Number(data.get("assists")) }); }}>
      <p className="small muted">Enter a manual scoreboard for {participant.gameName}.</p>
      <label>Kills<input name="kills" type="number" min="0" max="999" step="1" required defaultValue={stats?.kills ?? ""} /></label>
      <label>Deaths<input name="deaths" type="number" min="0" max="999" step="1" required defaultValue={stats?.deaths ?? ""} /></label>
      <label>Assists<input name="assists" type="number" min="0" max="999" step="1" required defaultValue={stats?.assists ?? ""} /></label>
      {stale && <p className="error" role="alert">The result changed. Cancel and reopen to edit.</p>}
      {error && <p className="error" role="alert">{error}</p>}
      <div className="heading-actions"><button className="primary" type="submit" disabled={!online || busy || stale}>Save</button>{stats && <button className="secondary" type="button" disabled={!online || busy || stale} onClick={() => void save(null)}>Clear</button>}<button className="secondary" type="button" disabled={busy} onClick={() => setEditingRevision(null)}>Cancel</button></div>
    </form>}
  </div>;
}
