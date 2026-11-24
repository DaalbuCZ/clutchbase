import type { ReactNode } from "react";
import "./TournamentBracket.css";

type TournamentRound = { id: string; round: number; number: number; bracketStage?: string; status?: string; roomStatus?: string; vetoStep?: number; canReportWinner?: boolean };

export function TournamentBracket<T extends TournamentRound>({
  matches,
  format,
  label,
  renderMatch,
}: {
  matches: T[];
  format?: "single_elimination" | "double_elimination" | "round_robin";
  label: string;
  renderMatch: (match: T) => ReactNode;
}) {
  const rounds = [...new Set(matches.map((match) => `${match.bracketStage || "single"}:${match.round}`))].sort((a, b) => {
    return Number(a.split(":")[1]) - Number(b.split(":")[1]);
  });
  const winnersRounds = matches.filter((match) => match.bracketStage === "winners").map((match) => match.round);
  const upperRounds = winnersRounds.length ? Math.max(...winnersRounds) : 0;
  const roundMatches = rounds.map((key) => {
    const [stage, roundValue] = key.split(":");
    const round = Number(roundValue);
    return {
    round,
    stage,
    matches: matches.filter((match) => (match.bracketStage || "single") === stage && match.round === round).sort((matchA, matchB) => matchA.number - matchB.number),
  };
  });
  const currentRound = roundMatches.find(({ matches: round }) => round.some((match) => {
    return match.status !== "complete" && match.status !== "bye" && match.status !== "skipped" && (match.roomStatus === "open" || match.canReportWinner || (match.vetoStep !== undefined && match.vetoStep > 0));
  }))?.round ?? roundMatches.find(({ matches: round }) => round.some((match) => match.status !== "complete" && match.status !== "bye" && match.status !== "skipped"))?.round;
  return <div className={`tournament-bracket ${format === "round_robin" ? "round-robin-fixtures" : ""}`} role="region" aria-label={label}>
    {rounds.length > 3 && <nav className="bracket-round-nav" aria-label={`${label} rounds`}>
      <span className="small muted">Jump to round</span>
      {rounds.map((key) => {
        const [stage, value] = key.split(":");
        const round = Number(value);
        const short = stage === "winners" ? `W${round}` : stage === "losers" ? `L${round - upperRounds}` : stage === "grand_final" ? "GF" : stage === "reset_final" ? "Reset" : `R${round}`;
        return <button type="button" key={key} onClick={() => document.getElementById(`${label.replace(/[^a-z0-9]+/gi, "-").toLowerCase()}-round-${round}`)?.scrollIntoView({ block: "nearest", inline: "start" })} aria-current={round === currentRound ? "location" : undefined}>{short}</button>;
      })}
    </nav>}
    {roundMatches.map(({ round, stage, matches: orderedMatches }) => {
      const done = orderedMatches.filter((match) => match.status === "complete" || match.status === "bye" || match.status === "skipped").length;
      const roundId = `${label.replace(/[^a-z0-9]+/gi, "-").toLowerCase()}-round-${round}`;
      const title = stage === "winners" ? `Winners · Round ${round}` : stage === "losers" ? `Losers · Round ${round - upperRounds}` : stage === "grand_final" ? "Grand final" : stage === "reset_final" ? "Grand final reset" : `Round ${round}`;
      return <section className={`bracket-round ${round === currentRound ? "is-current-round" : ""}`} key={`${stage}-${round}`} id={roundId} aria-label={title}>
      <h3><span>{title}</span><span className="small muted">{done}/{orderedMatches.length} complete</span></h3>
      <div className="bracket-round-matches">
        {orderedMatches.map(renderMatch)}
      </div>
    </section>;
    })}
  </div>;
}
