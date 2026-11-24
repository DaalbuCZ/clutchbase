import { useState } from "react";
import { request } from "./api";
import type { PlayerMatch } from "./api";

export function PlayerRoomShuffle({ match, connected }: { match: PlayerMatch; connected: boolean }) {
  const [confirmation, setConfirmation] = useState<{ settingsRevision: number; rosterFingerprint: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const stale = !!confirmation && (!match.actions.shuffle || confirmation.settingsRevision !== match.settingsRevision || confirmation.rosterFingerprint !== match.rosterFingerprint);
  async function submit() {
    if (!confirmation || stale || !connected) return;
    setBusy(true); setError("");
    try {
      await request("", `/player/matches/${match.id}/shuffle`, { method: "POST", body: JSON.stringify(confirmation) });
      setConfirmation(null);
    } catch (err) { setError(err instanceof Error ? err.message : "Could not shuffle sides"); }
    finally { setBusy(false); }
  }
  if (!match.actions.shuffle && !confirmation) return null;
  return <div className="team-confirmation" aria-label={`Shuffle sides in ${match.title}`}>
    {!confirmation ? <button className="secondary" disabled={!connected || busy || !match.settingsRevision || !match.rosterFingerprint} onClick={() => { setConfirmation({ settingsRevision: match.settingsRevision!, rosterFingerprint: match.rosterFingerprint! }); setError(""); }}>Shuffle sides</button> : <>
      <p>Randomly redistribute these players into sides of similar size? Everyone will need to ready up again.</p>
      {stale && <p className="error" role="alert">The room or roster changed. Cancel and review the current players.</p>}
      {error && <p className="error" role="alert">{error}</p>}
      <div className="heading-actions"><button className="secondary" disabled={!connected || busy || stale} onClick={() => void submit()}>{busy ? "Shuffling…" : "Confirm shuffle"}</button><button className="secondary" disabled={busy} onClick={() => { setConfirmation(null); setError(""); }}>Cancel</button></div>
    </>}
  </div>;
}
