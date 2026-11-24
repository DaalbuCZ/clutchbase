import { useEffect, useState } from "react";
import { connectLive } from "./live";
import "./Statistics.css";

type LeaderboardSnapshot = { capturedAt: string; truncated: boolean; players: { rank: number; name: string; wins: number; losses: number; draws: number; rooms: number }[] };

export function Leaderboard({ token, player = false }: { token?: string; player?: boolean }) {
  const [snapshot, setSnapshot] = useState<LeaderboardSnapshot | null>(null);
  const [online, setOnline] = useState(false);
  const [expired, setExpired] = useState(false);
  useEffect(() => connectLive<LeaderboardSnapshot>(player ? "player-leaderboard" : "leaderboard", token,
    (next) => setSnapshot((old) => !old || next.capturedAt >= old.capturedAt ? next : old), setOnline, () => setExpired(true)), [player, token]);
  return <section className="statistics" aria-label="Leaderboard">
    {!player && <h1>Leaderboard</h1>}
    {expired ? <p role="alert" className="error">Session expired. Sign in again.</p> : !snapshot ? <p role="status">{online ? "Loading leaderboard…" : "Connecting to leaderboard…"}</p> : <>
      {!online && <p role="status" className="stats-connection">Reconnecting to leaderboard… Showing the last received snapshot.</p>}
      {snapshot.players.length === 0 ? <p className="muted">No scored matches yet.</p> : <div className="stats-table-wrap"><table><caption className="visually-hidden">Leaderboard by wins</caption><thead><tr><th scope="col">Rank</th><th scope="col">Player</th><th scope="col">W–L–D</th><th scope="col">Rooms</th></tr></thead><tbody>{snapshot.players.map((entry, i) => <tr key={i}><td>{entry.rank}</td><th scope="row">{entry.name}</th><td>{entry.wins}–{entry.losses}–{entry.draws}</td><td>{entry.rooms}</td></tr>)}</tbody></table></div>}
      <p className="small muted">Ranked by wins; equal wins share a rank. Only players with a scored result appear. Rooms include unscored matches. {player ? "Results cover servers you can currently access." : "Results include retained history from removed servers."}{snapshot.truncated ? " Showing the first 200 players." : ""} This is not a skill rating.</p>
    </>}
  </section>;
}
