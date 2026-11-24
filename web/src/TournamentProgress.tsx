import type { Tournament } from "./api";


export function TournamentProgress({ tournament }: { tournament: Tournament }) {
  if (tournament.status === "cancelled") return <section className="tournament-progress" aria-label="Tournament progress"><p role="status">Tournament cancelled. No fixtures were started; this tournament no longer locks team rosters.</p></section>;
  const fixtureLabel = tournament.format === "round_robin" ? "fixtures" : "bracket";
  const stages = ["Registration", "Team check-in", tournament.format === "round_robin" ? "Fixtures" : "Bracket"];
  const checkedIn = tournament.entries.filter((entry) => entry.status === "checked_in").length;
  const fixturesDone = tournament.matches.filter((match) => match.status === "complete" || match.status === "bye" || match.status === "skipped").length;
  const liveFixtures = tournament.matches.filter((match) => match.roomStatus === "open").length;
  const vetoes = tournament.matches.filter((match) => match.teamA && match.teamB && match.status === "pending" && match.vetoStep < 7).length;
  const stageIndex = tournament.status === "open" ? 0 : tournament.status === "checkin" ? 1 : 2;
  const summary = tournament.status === "open"
    ? `${tournament.entries.length} team${tournament.entries.length === 1 ? "" : "s"} registered · registration is open`
    : tournament.status === "checkin"
      ? checkedIn >= 2
        ? `${checkedIn} teams checked in · the ${fixtureLabel} ${fixtureLabel === "fixtures" ? "are" : "is"} ready to generate`
        : `${checkedIn} of ${tournament.entries.length} teams checked in · at least two are needed to generate the ${fixtureLabel}`
      : tournament.status === "complete"
        ? `Tournament complete · ${tournament.entries.find((entry) => entry.teamId === tournament.winnerTeamId)?.teamName || (tournament.format === "round_robin" ? "Shared first place" : "Champion decided")}${tournament.winnerTeamId ? " won" : ""}`
        : `${fixturesDone} of ${tournament.matches.length} fixtures complete · ${liveFixtures} live${vetoes ? ` · ${vetoes} map veto${vetoes === 1 ? "" : "es"} in progress` : ""}`;

  return <section className="tournament-progress" aria-label="Tournament progress">
    <ol>{stages.map((stage, index) => {
      const state = tournament.status === "complete" || index < stageIndex ? "complete" : index === stageIndex ? "current" : "upcoming";
      const stepLabel = tournament.status === "complete" ? "Complete" : state === "current" ? "Current" : state === "complete" ? "Complete" : "Next";
      return <li className={`tournament-progress-step is-${state}`} key={stage} aria-current={state === "current" ? "step" : undefined}>
        <span className="tournament-step-marker" aria-hidden="true">{state === "complete" ? "✓" : index + 1}</span>
        <span><strong>{stage}</strong><small>{stepLabel}</small></span>
      </li>;
    })}</ol>
    <p role="status">{summary}</p>
  </section>;
}
