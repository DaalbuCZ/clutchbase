import { useState } from "react";
import { request } from "./api";
import type { MatchParticipant, PlayerMatch } from "./api";

export function PlayerRoomRemove({ match, participant, connected }: { match: PlayerMatch; participant: MatchParticipant; connected: boolean }) {
  const [confirmation, setConfirmation] = useState<{ settingsRevision: number; rosterFingerprint: string; removalToken: string; name: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const stale = !!confirmation && (!match.actions.shuffle || confirmation.settingsRevision !== match.settingsRevision || confirmation.rosterFingerprint !== match.rosterFingerprint || confirmation.removalToken !== participant.removalToken || confirmation.name !== participant.gameName);
  async function submit() {
    if (!confirmation || stale || !connected) return;
    setBusy(true); setError("");
    try {
      const { settingsRevision, rosterFingerprint, removalToken } = confirmation;
      await request("", `/player/matches/${match.id}/remove-player`, { method: "POST", body: JSON.stringify({ settingsRevision, rosterFingerprint, removalToken }) });
      setConfirmation(null);
    } catch (err) { setError(err instanceof Error ? err.message : "Could not remove player"); }
    finally { setBusy(false); }
  }
  if (!participant.removalToken && !confirmation) return null;
  return <div className="team-confirmation">
    {!confirmation ? <button className="text-button" disabled={!connected || busy || !match.settingsRevision || !match.rosterFingerprint} aria-label={`Remove ${participant.gameName} from the room`} onClick={() => setConfirmation({ settingsRevision: match.settingsRevision!, rosterFingerprint: match.rosterFingerprint!, removalToken: participant.removalToken!, name: participant.gameName })}>Remove</button> : <>
      <p>Remove {confirmation.name} from this room? Remaining players must ready up again. They can rejoin.</p>
      {stale && <p className="error" role="alert">The room or roster changed. Cancel and review the current players.</p>}
      {error && <p className="error" role="alert">{error}</p>}
      <button className="secondary" disabled={!connected || busy || stale} onClick={() => void submit()}>{busy ? "Removing…" : "Confirm removal"}</button><button className="secondary" disabled={busy} onClick={() => { setConfirmation(null); setError(""); }}>Cancel</button>
    </>}
  </div>;
}
