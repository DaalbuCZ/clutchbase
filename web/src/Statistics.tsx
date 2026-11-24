import { useEffect, useState } from "react";
import { connectLive } from "./live";
import "./Statistics.css";

export type StatisticsPlayer = { steamId: string; gameName: string; rooms: number; scoredRooms: number; manualScores: number; wins: number; losses: number; draws: number; statsRooms: number; manualStats: number; kills: number | null; deaths: number | null; assists: number | null; maps?: { map: string; rooms: number; scoredRooms: number; wins: number; losses: number; draws: number }[]; mapExcludedRooms?: number };
export type StatisticsSnapshot = { capturedAt: string; players: StatisticsPlayer[]; truncated: boolean };

export function Statistics({ token, player = false }: { token?: string; player?: boolean }) {
  const [snapshot, setSnapshot] = useState<StatisticsSnapshot | null>(null);
  const [online, setOnline] = useState(false);
  const [expired, setExpired] = useState(false);
  useEffect(() => connectLive<StatisticsSnapshot>(player ? "player-stats" : "stats", token,
    (next) => setSnapshot((old) => !old || next.capturedAt >= old.capturedAt ? next : old), setOnline, () => setExpired(true)), [player, token]);
  const heading = player ? <h2 id="stats-heading" className="visually-hidden">My statistics</h2> : <h1 id="stats-heading">Statistics</h1>;
  if (expired) return <section className="statistics" aria-labelledby="stats-heading">{heading}<p className="error" role="alert">Session expired. Sign in again.</p></section>;
  if (!snapshot) return <section className="statistics" aria-labelledby="stats-heading">{heading}<p role="status">{online ? "Loading statistics…" : "Connecting to statistics…"}</p></section>;
  const totals = snapshot.players.reduce((t, p) => ({ rooms: t.rooms + p.rooms, wins: t.wins + p.wins, losses: t.losses + p.losses, draws: t.draws + p.draws }), { rooms: 0, wins: 0, losses: 0, draws: 0 });
  return <section className="statistics" aria-labelledby="stats-heading">
    {heading}
    {!online && <p className="stats-connection" role="status">Reconnecting to statistics… Showing the last received snapshot.</p>}
    {snapshot.players.length === 0 ? <p className="muted">No statistics are available yet.</p> : <>
      <div className="stats-summary"><article><span>{player ? "Rooms played" : "Player appearances"}</span><strong>{totals.rooms}</strong></article><article><span>{player ? "Wins" : "Player wins"}</span><strong>{totals.wins}</strong></article><article><span>{player ? "Losses" : "Player losses"}</span><strong>{totals.losses}</strong></article><article><span>{player ? "Draws" : "Player draws"}</span><strong>{totals.draws}</strong></article></div>
      <div className="stats-table-wrap"><table><caption className="visually-hidden">Player statistics</caption><thead><tr>{!player && <th scope="col">Player</th>}<th scope="col">Rooms</th><th scope="col">W–L–D</th><th scope="col">K / D / A</th><th scope="col">Score coverage</th><th scope="col">Stats coverage</th></tr></thead><tbody>{snapshot.players.map((p) => <tr key={p.steamId}>{!player && <th scope="row">{p.gameName || p.steamId}</th>}<td>{p.rooms}</td><td>{p.wins}–{p.losses}–{p.draws}</td><td>{p.kills === null || p.deaths === null || p.assists === null ? "Unavailable" : `${p.kills} / ${p.deaths} / ${p.assists}`}</td><td>{p.scoredRooms}/{p.rooms} ({p.rooms ? Math.round(p.scoredRooms / p.rooms * 100) : 0}%)</td><td>{p.statsRooms}/{p.rooms} ({p.rooms ? Math.round(p.statsRooms / p.rooms * 100) : 0}%)</td></tr>)}</tbody></table></div>
    </>}
    {snapshot.players.map((p) => <details className="stats-maps" key={p.steamId}>
      <summary>{player ? "Map performance" : `Map performance · ${p.gameName || p.steamId}`}</summary>
      {!p.maps?.length ? <p className="small muted">No map appearances available.</p> : <div className="stats-table-wrap"><table><caption className="visually-hidden">Map performance</caption><thead><tr><th scope="col">Map</th><th scope="col">Appearances</th><th scope="col">W–L–D</th><th scope="col">Score coverage</th></tr></thead><tbody>{p.maps.map((m) => <tr key={m.map}><th scope="row">{m.map.replace(/^de_/, "")}</th><td>{m.rooms}</td><td>{m.wins}–{m.losses}–{m.draws}</td><td>{m.scoredRooms}/{m.rooms}</td></tr>)}</tbody></table></div>}
      <p className="small muted">Includes single-map rooms and manually recorded series map scores. Unrecorded series maps are not counted; a configured map may not have been played. {p.mapExcludedRooms || 0} {(p.mapExcludedRooms || 0) === 1 ? "room has" : "rooms have"} no map appearance here. Single-map rooms without a score are included in coverage but not W–L–D.</p>
    </details>)}
    <p className="small muted">Manual K/D/A and single-map score corrections take precedence. Series W/L use reported series results. Coverage is scored rooms or stats rooms divided by rooms. Reported K/D/A may be partial or captured only from the final map; unavailable values are not zero.{snapshot.truncated ? " Showing the first 200 players by rooms played; summary metrics cover these players." : ""}</p>
  </section>;
}
