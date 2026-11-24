import type { Tournament } from "./api";

export function TournamentStandings({ tournament }: { tournament: Tournament }) {
  if (tournament.status === "cancelled" || tournament.format !== "round_robin") return null;
  return <section className="tournament-standings" aria-label={`${tournament.name} standings`}>
    <h3>Standings</h3>
    <p className="small muted">Each team plays every other team once. Fixture wins determine rank; equal wins share a rank. Rooms open one round at a time.</p>
    {tournament.standings?.length ? <div className="standings-scroll"><table>
      <caption className="visually-hidden">{tournament.name} standings</caption>
      <thead><tr><th scope="col">Rank</th><th scope="col">Team</th><th scope="col">Played</th><th scope="col">Won</th><th scope="col">Lost</th></tr></thead>
      <tbody>{tournament.standings.map((team) => <tr key={team.teamId}><td>{team.rank}</td><th scope="row">{team.teamName}{tournament.status === "complete" && team.rank === 1 ? " · First place" : ""}</th><td>{team.played}</td><td>{team.wins}</td><td>{team.losses}</td></tr>)}</tbody>
    </table></div> : <p className="small muted">Standings appear when checked-in teams receive their fixtures.</p>}
  </section>;
}
