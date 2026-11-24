import { useState } from "react";
import { request } from "./api";
import type { MatchRoom } from "./api";

type RosterPlayer = { steamId: string; side: "ct" | "t"; ready: boolean };
function roster(match: MatchRoom): RosterPlayer[] {
  return match.participants.map(({ steamId, side, ready }) => ({ steamId, side, ready })).sort((a, b) => a.steamId.localeCompare(b.steamId));
}
export function MatchShuffle({ match, token, connected }: { match: MatchRoom; token: string; connected: boolean }) {
  const [confirmation, setConfirmation] = useState<{ settingsRevision: number; players: RosterPlayer[] } | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const stale = !!confirmation && (!match.canShuffle || confirmation.settingsRevision !== match.settingsRevision || JSON.stringify(confirmation.players) !== JSON.stringify(roster(match)));
  async function submit() {
    if (!confirmation || stale || !connected) return;
    setBusy(true); setError("");
    try {
      await request(token, `/matches/${match.id}/shuffle`, { method: "POST", body: JSON.stringify(confirmation) });
      setConfirmation(null);
    } catch (err) { setError(err instanceof Error ? err.message : "Could not shuffle sides"); }
    finally { setBusy(false); }
  }
  if (!match.canShuffle && !confirmation) return null;
  return <div className="team-confirmation" aria-label={`Shuffle sides in ${match.title}`}>
    {!confirmation ? <button className="secondary" disabled={!connected || busy} onClick={() => { setConfirmation({ settingsRevision: match.settingsRevision, players: roster(match) }); setError(""); }}>Shuffle sides</button> : <>
      <p>Randomly redistribute these players into sides of similar size? Everyone will need to ready up again.</p>
      {stale && <p className="error" role="alert">The room or roster changed. Cancel and review the current players.</p>}
      {error && <p className="error" role="alert">{error}</p>}
      <div className="heading-actions"><button className="secondary" disabled={!connected || busy || stale} onClick={() => void submit()}>{busy ? "Shuffling…" : "Confirm shuffle"}</button><button className="secondary" disabled={busy} onClick={() => { setConfirmation(null); setError(""); }}>Cancel</button></div>
    </>}
  </div>;
}
