import { useEffect, useState } from "react";
import type { Tournament } from "./api";

type Selection = { teamRevision: number; teamId: string; entryStatus: string; phase: string };
export function TournamentWithdrawal({ tournament, teamName, live, busy, onWithdraw }: { tournament: Tournament; teamName?: string; live: boolean; busy: boolean; onWithdraw: (id: string, selection: Selection) => Promise<boolean> }) {
  const [confirmation, setConfirmation] = useState<Selection | null>(null);
  const [submitted, setSubmitted] = useState(false);
  const stale = !!confirmation && (confirmation.teamRevision !== tournament.myTeamRevision || confirmation.teamId !== tournament.myTeamId || confirmation.entryStatus !== tournament.myEntryStatus || confirmation.phase !== tournament.status);
  useEffect(() => { if (submitted && stale) { setConfirmation(null); setSubmitted(false); } }, [submitted, stale]);
  if (!tournament.canWithdrawTeam && !confirmation) return null;
  return <div className="tournament-withdrawal">
    {!confirmation ? <button className="text-button" disabled={!live || busy} onClick={() => { setConfirmation({ teamRevision: tournament.myTeamRevision!, teamId: tournament.myTeamId!, entryStatus: tournament.myEntryStatus!, phase: tournament.status }); setSubmitted(false); }}>Withdraw team</button> : <section className="team-confirmation" aria-label={`Withdraw from ${tournament.name}`}>
      <p>Withdraw {teamName || "your team"} from {tournament.name}? This removes its registration and check-in.{tournament.status === "checkin" ? " Registration is closed, so the team cannot rejoin." : " The team can register again while registration stays open."}</p>
      {stale && <p className="error" role="alert">The tournament or team changed. Cancel and choose the action again.</p>}
      {submitted && <p className="small muted" role="status">Withdrawal accepted. Waiting for the updated tournament.</p>}
      <div className="heading-actions"><button className="secondary" disabled={!live || busy || stale || submitted || !tournament.canWithdrawTeam} onClick={async () => { if (confirmation && await onWithdraw(tournament.id, confirmation)) setSubmitted(true); }}>Confirm withdrawal</button><button className="secondary" disabled={busy} onClick={() => { setConfirmation(null); setSubmitted(false); }}>Cancel</button></div>
    </section>}
  </div>;
}
