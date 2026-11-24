import { useState } from "react";
import type { Team } from "./api";
import "./TeamDirectory.css";

export function TeamDirectory({ teams }: { teams: Team[] }) {
  const [search, setSearch] = useState("");
  const query = search.trim().toLocaleLowerCase();
  const shown = teams.filter((team) => !query || [team.name, ...team.members.flatMap((member) => [member.gameName, member.steamId])].some((value) => value.toLocaleLowerCase().includes(query)));
  const memberCount = teams.reduce((count, team) => count + team.members.length, 0);

  return <div className="team-directory">
    <div className="team-directory-tools">
      <label>Search teams or members<input type="search" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Team, player, or SteamID" /></label>
      <p className="small muted" role="status">{shown.length} of {teams.length} teams · {memberCount} {memberCount === 1 ? "member" : "members"}</p>
    </div>
    {teams.length === 0 ? <div className="empty"><p>No teams yet.</p><p className="small muted">Teams appear here when players create them.</p></div> : shown.length === 0 ? <div className="empty"><p>No teams or members match “{search.trim()}”.</p></div> : <div className="team-directory-list">{shown.map((team) => <details className="team-directory-item" key={team.id}>
      <summary><span className="team-directory-name">{team.name}</span><span className="small muted">{team.members.length}/5 players</span></summary>
      <ul>{team.members.map((member) => <li key={member.steamId}><span>{member.gameName || "Unnamed player"}</span><span className="small muted">{member.steamId === team.captainSteamId ? "Captain" : "Member"}</span></li>)}</ul>
    </details>)}</div>}
  </div>;
}
