import type { MapVote } from "./api";

export function MatchMapVote({ votes, maps, current, mine = "", canVote, canChoose, connected, busy, onVote, onChoose }: {
  votes: MapVote[]; maps: string[]; current: string; mine?: string; canVote?: boolean; canChoose?: boolean; connected: boolean; busy: boolean;
  onVote?: (map: string) => void; onChoose?: (map: string) => void;
}) {
  if (!canVote && !canChoose && votes.length === 0) return null;
  return <section className="match-map-vote" aria-label="Map preferences">
    <strong>Map preferences</strong>
    <p className="small muted">Votes are advisory. Management or the room creator chooses the match map; changing it clears ready checks.</p>
    {!connected && <p className="small muted" role="status">Last received votes · reconnecting to live updates.</p>}
    {votes.length === 0 ? <p className="small muted">No votes yet.</p> : (!canVote || canChoose) && <ul>{votes.map((vote) => <li key={vote.map}>{vote.map.replace(/^de_/, "")} · {vote.votes} {vote.votes === 1 ? "vote" : "votes"}{canChoose && vote.map !== current && <button className="text-button" disabled={!connected || busy} onClick={() => onChoose?.(vote.map)}>Use {vote.map.replace(/^de_/, "")}</button>}</li>)}</ul>}
    {canVote && <div className="veto-map-grid">{maps.map((map) => { const count = votes.find((vote) => vote.map === map)?.votes || 0; return <button key={map} type="button" className="secondary" aria-pressed={mine === map} disabled={!connected || busy} onClick={() => onVote?.(map)}>{map.replace(/^de_/, "")} · {count} {count === 1 ? "vote" : "votes"}{mine === map ? " · Your vote" : ""}</button>; })}{mine && <button type="button" className="text-button" disabled={!connected || busy} onClick={() => onVote?.("")}>Clear my vote</button>}</div>}
  </section>;
}
