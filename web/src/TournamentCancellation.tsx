import { useEffect, useState } from "react";
import type { Tournament } from "./api";

export function TournamentCancellation({ tournament, connected, busy, onCancel }: { tournament: Tournament; connected: boolean; busy: boolean; onCancel: (phase: string) => Promise<boolean> }) {
  const [phase, setPhase] = useState("");
  const [submitted, setSubmitted] = useState(false);
  useEffect(() => { if (tournament.status === "cancelled") { setPhase(""); setSubmitted(false); } }, [tournament.status]);
  if (!phase && !tournament.canCancel) return null;
  if (!phase) return <button className="text-button" disabled={!connected || busy} onClick={() => { setPhase(tournament.status); setSubmitted(false); }}>Cancel tournament</button>;
  const stale = phase !== tournament.status || !tournament.canCancel;
  return <section className="team-confirmation" aria-label={`Cancel ${tournament.name}`}>
    <p>Cancel {tournament.name}? Its registrations remain in history, and this tournament releases its roster locks. This cannot be undone.</p>
    {stale && <p className="error" role="alert">The tournament changed. Close this confirmation and review its current stage.</p>}
    {submitted && <p className="small muted" role="status">Cancellation accepted. Waiting for the updated tournament.</p>}
    <div className="heading-actions"><button className="secondary" disabled={!connected || busy || stale || submitted} onClick={async () => { if (await onCancel(phase)) setSubmitted(true); }}>Confirm cancellation</button><button className="secondary" disabled={busy} onClick={() => setPhase("")}>Keep tournament</button></div>
  </section>;
}
