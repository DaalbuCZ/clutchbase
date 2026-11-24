import { useState } from "react";

export function PlayerDirectory({ players }: { players: { steamId: string; gameName: string }[] }) {
  const [search, setSearch] = useState("");
  const [notice, setNotice] = useState("");
  const query = search.trim().toLocaleLowerCase();
  const shown = players.filter((player) => `${player.gameName} ${player.steamId}`.toLocaleLowerCase().includes(query));
  return <>
    <label>Search players<input type="search" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Name or SteamID" /></label>
    <p className="small muted" role="status">{shown.length} of {players.length} players</p>
    {notice && <p role="status" className="small muted">{notice}</p>}
    {players.length === 0 ? <p>No players have signed in yet.</p> : shown.length === 0 ? <p>No players match your search.</p> : <div className="machine-list">{shown.map((player) => <div className="machine-row" key={player.steamId}><strong>{player.gameName || "Unnamed player"}</strong><span className="small muted">{player.steamId}</span><button type="button" className="text-button" aria-label={`Copy SteamID for ${player.gameName || player.steamId}`} onClick={() => void Promise.resolve().then(() => navigator.clipboard.writeText(player.steamId)).then(() => setNotice(`Copied SteamID for ${player.gameName || player.steamId}.`)).catch(() => setNotice(`Copy failed. Select the displayed SteamID ${player.steamId}.`))}>Copy SteamID</button></div>)}</div>}
  </>;
}
