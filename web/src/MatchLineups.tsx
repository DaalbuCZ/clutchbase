import type { ReactNode } from "react";
import type { MatchParticipant } from "./api";
import "./MatchLineups.css";

export function MatchLineups<T extends MatchParticipant>({
  participants,
  playersPerSide,
  ctPlayers,
  tPlayers,
  renderPlayer,
  renderSideActions,
}: {
  participants: T[];
  playersPerSide: number;
  ctPlayers: number;
  tPlayers: number;
  renderPlayer: (participant: T, index: number) => ReactNode;
  renderSideActions?: (side: "ct" | "t") => ReactNode;
}) {
  return <div className="match-lineups" aria-label="Match lineups">
    {([
      { side: "ct", label: "Counter-Terrorists", count: ctPlayers },
      { side: "t", label: "Terrorists", count: tPlayers },
    ] as const).map(({ side, label, count }) => {
      const players = participants.filter((participant) => participant.side === side);
      const ready = players.filter((participant) => participant.ready).length;
      return <section className={`match-side match-side-${side}`} key={side} aria-label={`${label} lineup`}>
        <h3><span>{label}</span><span className="match-side-count">{count}/{playersPerSide}</span></h3>
        <div className={`match-capacity match-capacity-${side}`} role="progressbar" aria-label={`${label} capacity`} aria-valuemin={0} aria-valuemax={playersPerSide} aria-valuenow={count}>
          <span style={{ width: `${Math.min(100, Math.max(0, count / playersPerSide * 100))}%` }} />
        </div>
        <p className={`match-side-readiness${count > 0 && ready === count ? " complete" : ""}`}>{count === 0 ? "Waiting for players" : `${ready} of ${count} ready`}</p>
        {renderSideActions?.(side)}
        {players.length === 0 && <p className="small muted match-empty-side">No players yet</p>}
        {players.map((participant, index) => renderPlayer(participant, index))}
        {players.length < playersPerSide && <p className="match-open-slots">{playersPerSide - players.length} open {playersPerSide - players.length === 1 ? "slot" : "slots"}</p>}
      </section>;
    })}
  </div>;
}
