import { useEffect, useState } from "react";
import { connectLive } from "./live";
import type { MatchParticipant } from "./api";
import { request } from "./api";
import { PlayerScore } from "./PlayerScore";
import "./MatchHistory.css";

type HistoryPage = {
  capturedAt: string;
  nextCursor: string;
  matches: { id: string; title: string; hostName: string; map: string; createdAt: string; resultCT: number | null; resultT: number | null; reportedCT: number | null; reportedT: number | null; seriesMatch: boolean; seriesTeam1: number | null; seriesTeam2: number | null; seriesMaps?: { number: number; map: string; team1: number | null; team2: number | null }[]; resultRevision: number; canEditResult: boolean; removedHost: boolean; participants: MatchParticipant[] }[];
};

export function MatchHistory({ token, player, onClose }: { token?: string; player?: boolean; onClose: () => void }) {
  const [cursors, setCursors] = useState([""]);
  const [page, setPage] = useState<(HistoryPage & { cursor: string; query: string; pageNumber: number }) | null>(null);
  const [online, setOnline] = useState(false);
  const [expired, setExpired] = useState(false);
  const [editing, setEditing] = useState<{ id: string; revision: number } | null>(null);
  const [editingSeriesMap, setEditingSeriesMap] = useState<{ id: string; mapNumber: number; revision: number } | null>(null);
  const [saving, setSaving] = useState(false);
  const [savingSeriesMap, setSavingSeriesMap] = useState(false);
  const [error, setError] = useState("");
  const [search, setSearch] = useState("");
  const [query, setQuery] = useState("");
  const cursor = cursors[cursors.length - 1];
  useEffect(() => {
    setOnline(false);
    setExpired(false);
    return connectLive<HistoryPage>(player ? "player-history" : "history", token,
      (next) => setPage((previous) => !previous || previous.cursor !== cursor || previous.query !== query || next.capturedAt >= previous.capturedAt ? { ...next, cursor, query, pageNumber: cursors.length } : previous),
      setOnline, () => setExpired(true), cursor, query);
  }, [cursor, cursors.length, player, token, query]);
  function navigate(next: string[]) {
    setOnline(false);
    setEditing(null);
    setEditingSeriesMap(null);
    setCursors(next);
  }
  async function saveResult(scores: { ct: number; t: number } | null) {
    if (!editing || !token || player || !online || saving) return;
    const match = page?.matches.find((match) => match.id === editing.id);
    if (!match?.canEditResult || match.resultRevision !== editing.revision) return;
    setSaving(true); setError("");
    try {
      await request(token, `/matches/${editing.id}/result`, { method: "PATCH", body: JSON.stringify({ revision: editing.revision, ...(scores || { clear: true }) }) });
      setEditing(null);
    } catch (err) { setError(err instanceof Error ? err.message : "Could not save the result"); }
    finally { setSaving(false); }
  }
  async function saveSeriesMapResult(matchId: string, mapNumber: number, revision: number, scores: { team1: number; team2: number } | null) {
    if (!token || player || !online || savingSeriesMap) return;
    const match = page?.matches.find((match) => match.id === matchId);
    if (!match?.canEditResult || match.resultRevision !== revision) return;
    setSavingSeriesMap(true); setError("");
    try {
      await request(token, `/matches/${matchId}/series-map-result`, { method: "PATCH", body: JSON.stringify({ mapNumber, revision, ...(scores || { clear: true }) }) });
      setEditingSeriesMap(null);
    } catch (err) { setError(err instanceof Error ? err.message : "Could not save the map score"); }
    finally { setSavingSeriesMap(false); }
  }
  return <section className="match-rooms" aria-labelledby="history-heading">
    <div className="page-heading"><h2 id="history-heading" className={player ? "visually-hidden" : undefined}>Match history</h2><button className="secondary" onClick={onClose}>Close history</button></div>
    <p className="small muted">Closed rooms with a confirmed loaded setup. Reported player stats include only players available to the server at capture; manual values are separate.</p>
    <form className="history-search" onSubmit={(event) => { event.preventDefault(); const next = search.trim(); if (next !== query || cursor) { navigate([""]); setQuery(next); } }}><label>Search match history<input type="search" maxLength={40} value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Match, server, map or player" /></label><button className="secondary" type="submit">Search</button></form>
    {expired ? <p className="error" role="alert">Session expired. Sign in again.</p> : !online ? <p role="status">{page ? "Reconnecting to match history…" : "Connecting to match history…"}</p> : null}
    {page && <p className="small muted history-snapshot" role="status">{!online ? "Last received snapshot" : "Updated"} · <time dateTime={page.capturedAt}>{new Date(page.capturedAt).toLocaleString()}</time> · Page {page.pageNumber}{page.query ? ` · Search: ${page.query}` : ""}{!online ? " · Editing and paging unavailable until connected." : ""}</p>}
    {page && (page.matches.length === 0 ? <p className="small muted" role="status">{page.query ? "No matches found." : "No matches on this page."}</p> : <div className="history-cards">{page.matches.map((match) => <article className="history-card" key={match.id}>
      <header className="history-card-heading"><div><h3>{match.title}</h3><p className="small muted">{match.hostName}{match.removedHost ? " · removed server" : ""} · {match.seriesMatch ? "First map: " : ""}{match.map.replace(/^de_/, "")} · <time dateTime={match.createdAt}>{new Date(match.createdAt).toLocaleDateString()}</time></p></div><span className="state-pill state-stopped">Closed room</span></header>
      {match.seriesMatch && match.seriesTeam1 != null && match.seriesTeam2 != null && <div className="history-series-result"><strong>Reported series result</strong><span>Team 1 {match.seriesTeam1} – {match.seriesTeam2} Team 2</span></div>}
      <div className="history-result"><span>{match.resultCT != null && match.resultT != null ? `Manual result · started CT ${match.resultCT} – ${match.resultT} started T` : (match.reportedCT == null || match.reportedT == null) && (match.seriesTeam1 == null || match.seriesTeam2 == null) ? "No score recorded" : ""}</span>
        {match.reportedCT != null && match.reportedT != null && <p>{match.seriesMatch ? `Reported final map · CT ${match.reportedCT} – ${match.reportedT} T` : `Reported final score · started CT ${match.reportedCT} – ${match.reportedT} started T`}</p>}
        {!player && match.canEditResult && editing?.id !== match.id && <button className="secondary" disabled={!online} onClick={() => { setEditing({ id: match.id, revision: match.resultRevision }); setError(""); }}>{match.resultCT == null ? "Record result" : "Edit result"}</button>}
        {!player && match.canEditResult && editing?.id === match.id && <form onSubmit={(event) => { event.preventDefault(); const data = new FormData(event.currentTarget); void saveResult({ ct: Number(data.get("ct")), t: Number(data.get("t")) }); }}>
          <p className="small muted">Manual record only; this does not end or change the game.</p>
          <label>Team that started CT<input type="number" name="ct" min="0" max="99" step="1" required defaultValue={match.resultCT ?? ""} /></label>
          <label>Team that started T<input type="number" name="t" min="0" max="99" step="1" required defaultValue={match.resultT ?? ""} /></label>
          {editing.revision !== match.resultRevision && <p className="error" role="alert">The result changed. Cancel and reopen to edit it.</p>}
          {error && <p className="error" role="alert">{error}</p>}
          <div className="heading-actions"><button className="primary" type="submit" disabled={!online || saving || editing.revision !== match.resultRevision}>Save</button>{match.resultCT != null && <button type="button" className="secondary" disabled={!online || saving || editing.revision !== match.resultRevision} onClick={() => void saveResult(null)}>Clear</button>}<button type="button" className="secondary" disabled={saving} onClick={() => setEditing(null)}>Cancel</button></div>
        </form>}
      </div>
      {match.seriesMatch && (match.seriesMaps?.length || 0) > 1 && <details className="history-series"><summary id={`series-maps-${match.id}`}>Series maps · {match.seriesMaps?.length} configured</summary>
        <div className="history-series-heading"><div><p className="small muted">Scores are manual records for the teams that started CT and T on map 1. A missing score does not indicate whether a map was played.</p></div></div>
        <div className="history-series-maps">
          {match.seriesMaps?.map((seriesMap) => {
            const hasScore = seriesMap.team1 != null && seriesMap.team2 != null;
            const isEditing = !player && match.canEditResult && editingSeriesMap?.id === match.id && editingSeriesMap.mapNumber === seriesMap.number;
            const stale = isEditing && editingSeriesMap.revision !== match.resultRevision;
            return <div className="history-series-map" key={`${match.id}-${seriesMap.number}`}>
              <div className="history-series-map-heading"><strong>Map {seriesMap.number} · {seriesMap.map.replace(/^de_/, "")}</strong><span className="small muted">Configured map</span></div>
              <div className="history-series-score" aria-label={`Map ${seriesMap.number} score`}>
                <span>Team 1: {hasScore ? seriesMap.team1 : "Score not recorded"}</span>
                <span>Team 2: {hasScore ? seriesMap.team2 : "Score not recorded"}</span>
              </div>
              {!player && match.canEditResult && !isEditing && <button className="text-button" disabled={!online || savingSeriesMap} onClick={() => { setEditingSeriesMap({ id: match.id, mapNumber: seriesMap.number, revision: match.resultRevision }); setError(""); }}>{hasScore ? "Edit map score" : "Record map score"}</button>}
              {isEditing && <form className="history-series-map-form" onSubmit={(event) => { event.preventDefault(); const data = new FormData(event.currentTarget); void saveSeriesMapResult(match.id, seriesMap.number, editingSeriesMap.revision, { team1: Number(data.get("team1")), team2: Number(data.get("team2")) }); }}>
                <p className="small muted">Manual record only; this does not change the tournament result.</p>
                <label>Team 1 (started CT on map 1)<input type="number" name="team1" min="0" max="99" step="1" required defaultValue={seriesMap.team1 ?? ""} /></label>
                <label>Team 2 (started T on map 1)<input type="number" name="team2" min="0" max="99" step="1" required defaultValue={seriesMap.team2 ?? ""} /></label>
                {stale && <p className="error" role="alert">The series changed. Cancel and reopen to edit it.</p>}
                {error && <p className="error" role="alert">{error}</p>}
                <div className="heading-actions"><button className="primary" type="submit" disabled={!online || savingSeriesMap || stale}>Save</button>{hasScore && <button type="button" className="secondary" disabled={!online || savingSeriesMap || stale} onClick={() => void saveSeriesMapResult(match.id, seriesMap.number, editingSeriesMap.revision, null)}>Clear</button>}<button type="button" className="secondary" disabled={savingSeriesMap} onClick={() => setEditingSeriesMap(null)}>Cancel</button></div>
              </form>}
            </div>;
          })}
        </div>
      </details>}
      <details className="history-scoreboard-details"><summary>Player scoreboard · {match.participants.length} {match.participants.length === 1 ? "player" : "players"}</summary><div className="history-scoreboard" tabIndex={0} role="region" aria-label={`${match.title} player scoreboard`}><table><caption>Player scoreboard</caption><thead><tr><th scope="col">Player</th><th scope="col">Starting side</th><th scope="col">Reported K / D / A</th><th scope="col">Manual K / D / A</th></tr></thead><tbody>
      {match.participants.map((p,i) => <tr key={p.steamId || `${p.side}-${p.gameName}-${i}`}><th scope="row"><PlayerScore participant={p} matchId={match.id} revision={match.resultRevision} token={token} canEdit={!player && match.canEditResult} online={online} compact hideStats /></th><td>{p.side.toUpperCase()}</td><td>{p.reportedStats ? `${p.reportedStats.kills} / ${p.reportedStats.deaths} / ${p.reportedStats.assists}` : "—"}</td><td>{p.stats ? `${p.stats.kills} / ${p.stats.deaths} / ${p.stats.assists}` : "—"}</td></tr>)}
      </tbody></table>{match.participants.length === 0 && <p className="small muted">No player scoreboard was captured.</p>}</div></details>
    </article>)}</div>)}
    <div className="heading-actions"><button className="secondary" disabled={!online || cursors.length === 1} onClick={() => navigate(cursors.slice(0, -1))}>Previous</button><span className="small muted">Page {cursors.length}</span><button className="secondary" disabled={!online || !page?.nextCursor} onClick={() => page && navigate([...cursors, page.nextCursor])}>Next</button></div>
  </section>;
}
