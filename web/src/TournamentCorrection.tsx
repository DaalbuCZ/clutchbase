import { useEffect, useState } from "react";
import type { TournamentMatch } from "./api";

export function TournamentCorrection({ match, connected, busy, roundRobin, onCorrect }: { match: TournamentMatch; connected: boolean; busy: boolean; roundRobin: boolean; onCorrect: (teamId: string, revision: number) => Promise<boolean> }) {
  const [draft, setDraft] = useState<{ teamId: string; revision: number } | null>(null);
  const [submitted, setSubmitted] = useState(false);
  useEffect(() => {
    if (submitted && draft && match.resultRevision !== draft.revision) { setDraft(null); setSubmitted(false); }
  }, [match.resultRevision]);
  if (!match.canCorrectWinner || !match.teamA || !match.teamB || match.resultRevision == null) return null;
  const next = match.winnerTeamId === match.teamA.teamId ? match.teamB : match.teamA;
  const stale = !!draft && (draft.revision !== match.resultRevision || draft.teamId !== next.teamId);
  return <div className="tournament-correction">
    {!draft ? <button type="button" className="text-button" disabled={!connected || busy} onClick={() => { setDraft({ teamId: next.teamId, revision: match.resultRevision! }); setSubmitted(false); }}>Correct winner</button> : <>
      <p>Change the fixture winner to {next.teamName}? {roundRobin ? "Standings will be recalculated." : match.bracketStage === "grand_final" ? match.winnerTeamId === match.teamA.teamId ? "The tournament will reopen and a reset final will be required." : "The reset final will be skipped and the tournament will finish." : "The bracket will be updated."} The reported score remains unchanged.</p>
      {stale && <p className="error" role="alert">The fixture changed. Cancel and review the current result.</p>}
      <button type="button" className="secondary" disabled={!connected || busy || stale || submitted} onClick={() => { setSubmitted(true); void onCorrect(draft.teamId, draft.revision).then((ok) => { if (!ok) setSubmitted(false); }); }}>Confirm correction</button>
      <button type="button" className="text-button" disabled={busy} onClick={() => { setDraft(null); setSubmitted(false); }}>Cancel</button>
    </>}
  </div>;
}
