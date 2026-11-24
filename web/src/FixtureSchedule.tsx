import type { Tournament } from "./api";
import "./FixtureSchedule.css";

export function FixtureSchedule({ tournaments, connected, loading, onOpen }: { tournaments: Tournament[]; connected: boolean; loading: boolean; onOpen: (id: string) => void }) {
  const fixtures = tournaments.flatMap((tournament) => tournament.status === "active" ? tournament.matches.filter((match) => match.status === "pending" && match.scheduledAt && Number.isFinite(Date.parse(match.scheduledAt))).map((match) => ({ tournament, match })) : []).sort((a, b) => Date.parse(a.match.scheduledAt!) - Date.parse(b.match.scheduledAt!) || a.tournament.name.localeCompare(b.tournament.name) || a.match.round - b.match.round || a.match.number - b.match.number);
  if (loading) return <p role="status">Loading schedule…</p>;
  return <section className="fixture-list" aria-label="Scheduled tournament fixtures">
    {!connected && <p role="status" className="small muted">Connection lost · showing the last received schedule.</p>}
    {fixtures.length === 0 ? <p className="muted">No scheduled fixtures.</p> : fixtures.map(({ tournament, match }) => <button type="button" className="fixture-list-row" key={`${tournament.id}-${match.id}`} onClick={() => onOpen(tournament.id)}>
      <span><strong>{match.teamA?.teamName || "To be decided"} vs {match.teamB?.teamName || "To be decided"}</strong><small>{tournament.name} · Round {match.round}, match {match.number}</small></span>
      <time dateTime={match.scheduledAt!}>{new Date(match.scheduledAt!).toLocaleString(undefined, { year: "numeric", month: "short", day: "numeric", hour: "numeric", minute: "2-digit", timeZoneName: "short" })}</time>
    </button>)}
  </section>;
}
